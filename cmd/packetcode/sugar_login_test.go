package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/provider/sugar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSugarLoginDiscoversLiveModelsAndPersistsProvider(t *testing.T) {
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	originalSleep, originalOpen := sugarLoginSleep, sugarLoginOpenBrowser
	t.Cleanup(func() { sugarLoginSleep, sugarLoginOpenBrowser = originalSleep, originalOpen })
	sugarLoginSleep = func(context.Context, time.Duration) error { return nil }
	var openedURL string
	sugarLoginOpenBrowser = func(rawURL string) error { openedURL = rawURL; return nil }
	polls := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/device/code":
			assert.Equal(t, http.MethodPost, r.Method)
			fmt.Fprintf(w, `{"device_code":"sgr_device_test","user_code":"ABCD-EFGH","verification_uri":"%s/portal/connect","verification_uri_complete":"%s/portal/connect?user_code=ABCD-EFGH","expires_in":600,"interval":5}`, serverURL, serverURL)
		case "/api/v1/auth/device/token":
			polls++
			if polls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"authorization_pending","error_description":"Waiting for approval."}`)
				return
			}
			if polls == 2 {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"slow_down","error_description":"Poll less frequently."}`)
				return
			}
			fmt.Fprint(w, `{"token":"sgr_live","name":"Ian"}`)
		case "/api/v1/models":
			assert.Equal(t, "Bearer sgr_live", r.Header.Get("Authorization"))
			fmt.Fprint(w, `{"object":"list","data":[{"id":"sugar/glm","object":"model"},{"id":"sugar/conduit","object":"model"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	var stdout, stderr bytes.Buffer
	code := runSugarLogin(
		[]string{"--server", server.URL, "--name", "Ian"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		server.Client(),
	)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "2 live models available")
	assert.Contains(t, stdout.String(), "ABCD-EFGH")
	assert.Equal(t, server.URL+"/portal/connect?user_code=ABCD-EFGH", openedURL)
	assert.Equal(t, 3, polls)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "sugar", cfg.Default.Provider)
	assert.Equal(t, "sugar/conduit", cfg.Default.Model)
	assert.Equal(t, "sgr_live", cfg.Providers["sugar"].APIKey)
	assert.Equal(t, server.URL+"/api/v1", cfg.Providers["sugar"].BaseURL)
}

func TestSugarLoginRejectsCrossOriginVerificationURL(t *testing.T) {
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"device_code":"sgr_device_test","user_code":"ABCD-EFGH","verification_uri":"https://attacker.example/connect","expires_in":600,"interval":5}`)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--server", server.URL, "--no-browser"}, strings.NewReader(""), &stdout, &stderr, server.Client())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "origin did not match")
}

func TestSugarLoginRejectsInsecureRemoteHTTP(t *testing.T) {
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := runSugarLogin(
		[]string{"--server", "http://sugar.example"},
		strings.NewReader("code\n"),
		&stdout,
		&stderr,
		http.DefaultClient,
	)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "must use HTTPS")
}

// fakeSugarService is a Sugar that approves immediately and records the client
// identity it was handed, so sign-in ergonomics can be asserted on the wire.
type fakeSugarService struct {
	server     *httptest.Server
	clientID   string
	clientName string
	requests   int
	deviceCode func(w http.ResponseWriter)
	token      func(w http.ResponseWriter)
}

func newFakeSugarService(t *testing.T) *fakeSugarService {
	t.Helper()
	fake := &fakeSugarService{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.requests++
		switch r.URL.Path {
		case "/api/v1/auth/device/code":
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			require.NoError(t, err)
			var payload map[string]string
			require.NoError(t, json.Unmarshal(body, &payload))
			fake.clientID, fake.clientName = payload["client_id"], payload["client_name"]
			if fake.deviceCode != nil {
				fake.deviceCode(w)
				return
			}
			fmt.Fprintf(w, `{"device_code":"sgr_device_test","user_code":"ABCD-EFGH","verification_uri_complete":"%s/portal/connect?user_code=ABCD-EFGH","expires_in":600,"interval":5}`, fake.server.URL)
		case "/api/v1/auth/device/token":
			if fake.token != nil {
				fake.token(w)
				return
			}
			fmt.Fprint(w, `{"token":"sgr_live","name":"Ian"}`)
		case "/api/v1/models":
			fmt.Fprint(w, `{"object":"list","data":[{"id":"sugar/conduit","object":"model"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// stubSugarLogin isolates config, the poll delay, and the browser launch so a
// sign-in test only exercises the sign-in decisions.
func stubSugarLogin(t *testing.T) {
	t.Helper()
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	t.Setenv("PACKETCODE_SUGAR_BASE_URL", "")
	sleep, openBrowser, hostname := sugarLoginSleep, sugarLoginOpenBrowser, sugarLoginHostname
	t.Cleanup(func() {
		sugarLoginSleep, sugarLoginOpenBrowser, sugarLoginHostname = sleep, openBrowser, hostname
	})
	sugarLoginSleep = func(context.Context, time.Duration) error { return nil }
	sugarLoginOpenBrowser = func(string) error { return nil }
}

func TestSugarLoginNamesTheKeyAfterTheMachine(t *testing.T) {
	stubSugarLogin(t)
	sugarLoginHostname = func() (string, error) { return "ian-laptop", nil }
	fake := newFakeSugarService(t)

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--server", fake.server.URL, "--no-browser"}, strings.NewReader(""), &stdout, &stderr, fake.server.Client())
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "ian-laptop", fake.clientName)
	assert.Equal(t, "packetcode", fake.clientID)
}

func TestSugarLoginFallsBackToFriendWithoutAUsableHostname(t *testing.T) {
	stubSugarLogin(t)
	sugarLoginHostname = func() (string, error) { return "", fmt.Errorf("no hostname") }
	fake := newFakeSugarService(t)

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--server", fake.server.URL, "--no-browser"}, strings.NewReader(""), &stdout, &stderr, fake.server.Client())
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "friend", fake.clientName)
}

func TestDefaultSugarClientNameKeepsOnlyNamesSugarAccepts(t *testing.T) {
	original := sugarLoginHostname
	t.Cleanup(func() { sugarLoginHostname = original })
	for name, tc := range map[string]struct {
		hostname string
		err      error
		want     string
	}{
		"hostname wins":        {hostname: "ian-laptop", want: "ian-laptop"},
		"accents survive":      {hostname: "Café-Étoile", want: "Café-Étoile"},
		"whitespace trimmed":   {hostname: "  ian-laptop \n", want: "ian-laptop"},
		"too short falls back": {hostname: "x", want: "friend"},
		"empty falls back":     {hostname: "", want: "friend"},
		"lookup error":         {err: fmt.Errorf("boom"), want: "friend"},
		"long name is cut to the limit in characters, not bytes": {hostname: strings.Repeat("é", 200), want: strings.Repeat("é", 80)},
	} {
		t.Run(name, func(t *testing.T) {
			sugarLoginHostname = func() (string, error) { return tc.hostname, tc.err }
			assert.Equal(t, tc.want, defaultSugarClientName())
		})
	}
}

func TestSugarLoginRejectsNamesSugarWouldRefuse(t *testing.T) {
	for name, tc := range map[string]struct {
		arg  string
		want string
	}{
		"too short": {arg: "x", want: "at least 2 characters"},
		"too long":  {arg: strings.Repeat("a", 81), want: "at most 80 characters"},
	} {
		t.Run(name, func(t *testing.T) {
			stubSugarLogin(t)
			fake := newFakeSugarService(t)

			var stdout, stderr bytes.Buffer
			code := runSugarLogin([]string{"--server", fake.server.URL, "--name", tc.arg, "--no-browser"}, strings.NewReader(""), &stdout, &stderr, fake.server.Client())
			assert.Equal(t, 2, code)
			assert.Contains(t, stderr.String(), tc.want)
			assert.Zero(t, fake.requests, "a name Sugar would reject should never reach the service")
		})
	}
}

func TestSugarLoginPromptsForTheServiceOnAnUnconfiguredMachine(t *testing.T) {
	stubSugarLogin(t)
	sugarLoginHostname = func() (string, error) { return "ian-laptop", nil }
	fake := newFakeSugarService(t)

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--no-browser"}, strings.NewReader(fake.server.URL+"\n"), &stdout, &stderr, fake.server.Client())
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "Sugar service URL ["+sugar.HostedService+"]")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, fake.server.URL+"/api/v1", cfg.Providers["sugar"].BaseURL)
}

func TestSugarLoginTakesTheHostedServiceWhenThePromptGoesUnanswered(t *testing.T) {
	stubSugarLogin(t)
	sugarLoginHostname = func() (string, error) { return "ian-laptop", nil }
	fake := newFakeSugarService(t)
	hosted := sugar.HostedService
	t.Cleanup(func() { sugar.HostedService = hosted })
	sugar.HostedService = fake.server.URL

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--no-browser"}, strings.NewReader(""), &stdout, &stderr, fake.server.Client())
	require.Equal(t, 0, code, stderr.String())

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, fake.server.URL+"/api/v1", cfg.Providers["sugar"].BaseURL)
}

func TestSugarLoginSkipsThePromptOnceAServiceIsSaved(t *testing.T) {
	stubSugarLogin(t)
	sugarLoginHostname = func() (string, error) { return "ian-laptop", nil }
	fake := newFakeSugarService(t)
	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.Providers = map[string]config.ProviderConfig{"sugar": {BaseURL: fake.server.URL}}
	require.NoError(t, cfg.Save())

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--no-browser"}, strings.NewReader(""), &stdout, &stderr, fake.server.Client())
	require.Equal(t, 0, code, stderr.String())
	assert.NotContains(t, stdout.String(), "Sugar service URL")
}

func TestSugarLoginExplainsInvalidClientFromTheDeviceRequest(t *testing.T) {
	stubSugarLogin(t)
	fake := newFakeSugarService(t)
	fake.deviceCode = func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_client","error_description":"Client name is not allowed."}`)
	}

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--server", fake.server.URL, "--name", "Ian", "--no-browser"}, strings.NewReader(""), &stdout, &stderr, fake.server.Client())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Sugar rejected this client: Client name is not allowed.")
	assert.Contains(t, stderr.String(), `The name "Ian" must be 2-80 characters`)
	assert.Contains(t, stderr.String(), "client registry")
}

func TestSugarLoginExplainsInvalidClientWhilePolling(t *testing.T) {
	stubSugarLogin(t)
	fake := newFakeSugarService(t)
	fake.token = func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_client","error_description":"Unknown client."}`)
	}

	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--server", fake.server.URL, "--name", "Ian", "--no-browser"}, strings.NewReader(""), &stdout, &stderr, fake.server.Client())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Sugar rejected this client: Unknown client.")
	assert.Contains(t, stderr.String(), "client registry")
}
