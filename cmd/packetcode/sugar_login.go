package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/provider/sugar"
)

type sugarTokenResponse struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

type sugarDeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type sugarDeviceTokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type sugarAPIError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var (
	sugarLoginSleep       = waitForSugarPoll
	sugarLoginOpenBrowser = openSugarBrowser
)

func runSugarLoginCommand(args []string, stdout, stderr io.Writer) int {
	return runSugarLogin(args, os.Stdin, stdout, stderr, http.DefaultClient)
}

func runSugarLogin(args []string, stdin io.Reader, stdout, stderr io.Writer, client *http.Client) int {
	_ = stdin // Retained in the internal signature for command test compatibility.
	fs := flag.NewFlagSet("sugar login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", "", "Sugar service URL (for example https://sugar.example)")
	name := fs.String("name", "friend", "name attached to the Sugar API key")
	noBrowser := fs.Bool("no-browser", false, "do not open the Sugar approval page automatically")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "packetcode: load config: %v\n", err)
		return 1
	}
	baseURL := strings.TrimSpace(*server)
	if baseURL == "" {
		baseURL = sugarBaseURL(cfg)
	}
	baseURL = sugar.NormalizeBaseURL(baseURL)
	if err := validateSugarLoginURL(baseURL); err != nil {
		fmt.Fprintf(stderr, "packetcode: %v\n", err)
		return 2
	}

	payload, _ := json.Marshal(map[string]string{
		"client_id":   "packetcode",
		"client_name": strings.TrimSpace(*name),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/auth/device/code", bytes.NewReader(payload))
	if err != nil {
		cancel()
		fmt.Fprintf(stderr, "packetcode: create Sugar device request: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		fmt.Fprintf(stderr, "packetcode: start Sugar sign-in: %v\n", err)
		return 1
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	cancel()
	if readErr != nil {
		fmt.Fprintf(stderr, "packetcode: read Sugar sign-in: %v\n", readErr)
		return 1
	}
	if resp.StatusCode/100 != 2 {
		fmt.Fprintf(stderr, "packetcode: Sugar sign-in failed: %s\n", sugarErrorMessage(body, resp.StatusCode))
		return 1
	}
	var device sugarDeviceCodeResponse
	if err := json.Unmarshal(body, &device); err != nil {
		fmt.Fprintf(stderr, "packetcode: decode Sugar device sign-in: %v\n", err)
		return 1
	}
	if strings.TrimSpace(device.DeviceCode) == "" || strings.TrimSpace(device.UserCode) == "" || device.ExpiresIn <= 0 || device.ExpiresIn > 3600 {
		fmt.Fprintln(stderr, "packetcode: Sugar returned an invalid device sign-in response")
		return 1
	}
	if device.Interval < 1 {
		device.Interval = 5
	}
	if device.Interval > 60 {
		device.Interval = 60
	}
	verificationURL := strings.TrimSpace(device.VerificationURIComplete)
	if verificationURL == "" {
		verificationURL = strings.TrimSpace(device.VerificationURI)
	}
	if err := validateSugarVerificationURL(baseURL, verificationURL); err != nil {
		fmt.Fprintf(stderr, "packetcode: Sugar returned an unsafe verification URL: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Open %s\nEnter code: %s\n", verificationURL, device.UserCode)
	if !*noBrowser {
		if err := sugarLoginOpenBrowser(verificationURL); err != nil {
			fmt.Fprintln(stdout, "Could not open a browser automatically; use the URL above.")
		}
	}
	fmt.Fprintln(stdout, "Waiting for approval…")

	pollCtx, stopPolling := context.WithTimeout(context.Background(), time.Duration(device.ExpiresIn)*time.Second)
	defer stopPolling()
	var tokenResponse sugarTokenResponse
	interval := time.Duration(device.Interval) * time.Second
	for {
		if err := sugarLoginSleep(pollCtx, interval); err != nil {
			fmt.Fprintln(stderr, "packetcode: Sugar device code expired before it was approved")
			return 1
		}
		pollPayload, _ := json.Marshal(map[string]string{
			"client_id":   "packetcode",
			"device_code": device.DeviceCode,
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
		})
		requestCtx, requestCancel := context.WithTimeout(pollCtx, 20*time.Second)
		pollRequest, requestErr := http.NewRequestWithContext(requestCtx, http.MethodPost, baseURL+"/auth/device/token", bytes.NewReader(pollPayload))
		if requestErr != nil {
			requestCancel()
			fmt.Fprintf(stderr, "packetcode: create Sugar token request: %v\n", requestErr)
			return 1
		}
		pollRequest.Header.Set("Content-Type", "application/json")
		pollResponse, requestErr := client.Do(pollRequest)
		if requestErr != nil {
			requestCancel()
			fmt.Fprintf(stderr, "packetcode: poll Sugar sign-in: %v\n", requestErr)
			return 1
		}
		pollBody, pollReadErr := io.ReadAll(io.LimitReader(pollResponse.Body, 1<<20))
		pollResponse.Body.Close()
		requestCancel()
		if pollReadErr != nil {
			fmt.Fprintf(stderr, "packetcode: read Sugar token response: %v\n", pollReadErr)
			return 1
		}
		if pollResponse.StatusCode/100 == 2 {
			if err := json.Unmarshal(pollBody, &tokenResponse); err != nil || strings.TrimSpace(tokenResponse.Token) == "" {
				fmt.Fprintln(stderr, "packetcode: Sugar returned an invalid token response")
				return 1
			}
			break
		}
		var tokenError sugarDeviceTokenError
		_ = json.Unmarshal(pollBody, &tokenError)
		switch tokenError.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied", "expired_token":
			message := strings.TrimSpace(tokenError.ErrorDescription)
			if message == "" {
				message = strings.ReplaceAll(tokenError.Error, "_", " ")
			}
			fmt.Fprintf(stderr, "packetcode: Sugar sign-in failed: %s\n", message)
			return 1
		default:
			fmt.Fprintf(stderr, "packetcode: Sugar sign-in failed: %s\n", sugarErrorMessage(pollBody, pollResponse.StatusCode))
			return 1
		}
	}

	if cfg.Providers == nil {
		cfg.Providers = map[string]config.ProviderConfig{}
	}
	pc := cfg.Providers[sugar.Slug]
	pc.APIKey = tokenResponse.Token
	pc.BaseURL = baseURL
	cfg.Providers[sugar.Slug] = pc
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(stderr, "packetcode: Sugar key was issued but could not be saved: %v\n", err)
		return 1
	}

	discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer discoveryCancel()
	provider := sugar.NewWithBaseURL(baseURL, tokenResponse.Token)
	models, err := provider.ListModels(discoveryCtx)
	if err != nil {
		fmt.Fprintf(stderr, "packetcode: Sugar key was issued but model discovery failed: %v\n", err)
		return 1
	}
	if len(models) == 0 {
		fmt.Fprintln(stderr, "packetcode: Sugar returned no models")
		return 1
	}
	defaultModel := models[0].ID
	for _, model := range models {
		if model.ID == sugar.DefaultModel {
			defaultModel = model.ID
			break
		}
	}
	pc = cfg.Providers[sugar.Slug]
	pc.DefaultModel = defaultModel
	cfg.Providers[sugar.Slug] = pc
	cfg.Default.Provider = sugar.Slug
	cfg.Default.Model = defaultModel
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(stderr, "packetcode: save Sugar provider: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Connected to Sugar as %s. %d live models available; default is %s.\n", tokenResponse.Name, len(models), defaultModel)
	return 0
}

func sugarErrorMessage(body []byte, status int) string {
	var apiError sugarAPIError
	if json.Unmarshal(body, &apiError) == nil && strings.TrimSpace(apiError.Error.Message) != "" {
		return strings.TrimSpace(apiError.Error.Message)
	}
	var tokenError sugarDeviceTokenError
	if json.Unmarshal(body, &tokenError) == nil && strings.TrimSpace(tokenError.ErrorDescription) != "" {
		return strings.TrimSpace(tokenError.ErrorDescription)
	}
	return fmt.Sprintf("status %d", status)
}

func waitForSugarPoll(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func openSugarBrowser(rawURL string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		command = exec.Command("open", rawURL)
	default:
		command = exec.Command("xdg-open", rawURL)
	}
	return command.Start()
}

func validateSugarLoginURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("invalid Sugar server URL")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return nil
	}
	return fmt.Errorf("Sugar server must use HTTPS unless it is localhost")
}

func validateSugarVerificationURL(baseURL, verificationURL string) error {
	base, baseErr := url.Parse(baseURL)
	verification, verificationErr := url.Parse(verificationURL)
	if baseErr != nil || verificationErr != nil || verification.Host == "" || verification.User != nil {
		return fmt.Errorf("invalid verification URL")
	}
	if verification.Scheme != base.Scheme || !strings.EqualFold(verification.Host, base.Host) {
		return fmt.Errorf("verification URL origin did not match the Sugar service")
	}
	return validateSugarLoginURL(verificationURL)
}
