package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/config"
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
	require.NotNil(t, cfg.Sugar.Enabled)
	assert.True(t, *cfg.Sugar.Enabled)
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

func TestSugarLoginRespectsExplicitDisableWithoutNetwork(t *testing.T) {
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	cfg := config.Default()
	disabled := false
	cfg.Sugar.Enabled = &disabled
	require.NoError(t, cfg.Save())

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := runSugarLogin([]string{"--server", server.URL}, strings.NewReader(""), &stdout, &stderr, server.Client())

	assert.Equal(t, 1, code)
	assert.Zero(t, requests)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "Sugar integration is disabled")
}
