package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/config"
)

func TestDoctorJSONOutputNoConfig(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q, stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", report.SchemaVersion)
	}
	if report.Status != doctorWarn {
		t.Fatalf("status = %q, want warn", report.Status)
	}
	if report.EffectiveHome != filepath.Join(os.Getenv("USERPROFILE"), ".packetcode") {
		t.Fatalf("effective_home = %q", report.EffectiveHome)
	}
	if report.HomeSource != "default" {
		t.Fatalf("home_source = %q, want default", report.HomeSource)
	}
	if report.ProviderSummary != (doctorProviderSummary{}) {
		t.Fatalf("provider_summary = %+v, want empty", report.ProviderSummary)
	}
	assertDoctorCheck(t, report, "config.file", doctorWarn)
	assertDoctorCheck(t, report, "providers.none", doctorWarn)
	assertDoctorCheck(t, report, "mcp.none", doctorOK)
}

func TestDoctorReportsEffectiveHomeAndProviderSummary(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	override := filepath.Join(t.TempDir(), "packetcode-data")
	t.Setenv(config.HomeEnv, override)
	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "qwen3"
	cfg.Providers["ollama"] = config.ProviderConfig{
		Host:         "http://localhost:11434",
		DefaultModel: "qwen3",
	}
	cfg.Providers["openai"] = config.ProviderConfig{
		DefaultModel: "gpt-4.1",
	}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q, stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	if report.EffectiveHome != override || report.HomeSource != "environment" {
		t.Fatalf("home = %q (%s), want %q (environment)", report.EffectiveHome, report.HomeSource, override)
	}
	want := doctorProviderSummary{Configured: 2, Ready: 1, Warning: 1}
	if report.ProviderSummary != want {
		t.Fatalf("provider_summary = %+v, want %+v", report.ProviderSummary, want)
	}
}

func TestDoctorReportsDisabledOptionalIntegrationsWithoutProbing(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	disabled := false
	cfg := config.Default()
	cfg.PacketComputers.Enabled = &disabled
	cfg.Sugar.Enabled = &disabled
	cfg.ACP.Enabled = &disabled
	// The parent Sugar gate must win even if stale configuration asks for the
	// subordinate shadow runtime.
	cfg.Conduit.ShadowEnabled = true
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json", "--check", "integrations"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q, stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	packetComputers := assertDoctorCheck(t, report, "integrations.packet_computers", doctorSkip)
	if !strings.Contains(packetComputers.Detail, "registry") || !strings.Contains(packetComputers.Detail, "SSH") {
		t.Fatalf("Packet Computers disabled detail = %q", packetComputers.Detail)
	}
	sugar := assertDoctorCheck(t, report, "integrations.sugar", doctorSkip)
	if !strings.Contains(sugar.Detail, "login") || !strings.Contains(sugar.Detail, "Conduit") {
		t.Fatalf("Sugar disabled detail = %q", sugar.Detail)
	}
	acp := assertDoctorCheck(t, report, "integrations.acp", doctorSkip)
	if !strings.Contains(acp.Detail, "before protocol") || !strings.Contains(acp.Detail, "MCP") {
		t.Fatalf("ACP disabled detail = %q", acp.Detail)
	}
	shadow := assertDoctorCheck(t, report, "integrations.conduit_shadow", doctorSkip)
	if !strings.Contains(shadow.Detail, "no Conduit shadow runtime calls") {
		t.Fatalf("Conduit disabled detail = %q", shadow.Detail)
	}
}

func TestDoctorReportsCompatibilityDefaultsAndConduitOptIn(t *testing.T) {
	r := doctorReport{}
	cfg := config.Default()
	addIntegrationChecks(&r, cfg)
	assertDoctorCheck(t, r, "integrations.packet_computers", doctorOK)
	assertDoctorCheck(t, r, "integrations.sugar", doctorSkip)
	assertDoctorCheck(t, r, "integrations.acp", doctorOK)
	assertDoctorCheck(t, r, "integrations.conduit_shadow", doctorSkip)

	r = doctorReport{}
	enabled := true
	cfg.Sugar.Enabled = &enabled
	cfg.Conduit.ShadowEnabled = true
	addIntegrationChecks(&r, cfg)
	assertDoctorCheck(t, r, "integrations.conduit_shadow", doctorOK)
}

func TestDoctorRejectsDisabledBuiltInSugarDefault(t *testing.T) {
	cfg := config.Default()
	disabled := false
	cfg.Sugar.Enabled = &disabled
	cfg.Default.Provider = "sugar"
	cfg.Default.Model = "sugar/conduit"
	cfg.Providers["sugar"] = config.ProviderConfig{APIKey: "stale", DefaultModel: "sugar/conduit"}

	report := doctorReport{}
	addDefaultProviderChecks(&report, cfg)
	check := assertDoctorCheck(t, report, "config.default_provider", doctorFail)
	if !strings.Contains(check.Message, "disabled") {
		t.Fatalf("disabled default diagnostic = %#v", check)
	}
}

func TestDoctorAcceptsBuiltInSugarLoginBaseURLAndSkipsPreservedDisabledRecord(t *testing.T) {
	base := config.Default()
	base.Providers["sugar"] = config.ProviderConfig{
		APIKey: "token", DefaultModel: "sugar/conduit", BaseURL: "https://sugar.example/api/v1",
	}
	base.Default.Provider = "sugar"
	base.Default.Model = "sugar/conduit"

	report := doctorReport{}
	addProviderChecks(&report, base)
	for _, check := range report.Checks {
		if check.ID == "providers.sugar.custom_fields" {
			t.Fatalf("normal built-in Sugar base_url was rejected: %#v", check)
		}
	}

	disabled := false
	base.Sugar.Enabled = &disabled
	report = doctorReport{}
	addProviderChecks(&report, base)
	for _, check := range report.Checks {
		if strings.HasPrefix(check.ID, "providers.sugar") {
			t.Fatalf("preserved disabled built-in Sugar record should be inactive, got %#v", check)
		}
	}
}

func TestDoctorReportsExistingSugarAutoStateAsActive(t *testing.T) {
	r := doctorReport{}
	cfg := config.Default()
	cfg.Providers["sugar"] = config.ProviderConfig{APIKey: "existing", DefaultModel: "sugar/conduit"}
	addIntegrationChecks(&r, cfg)
	check := assertDoctorCheck(t, r, "integrations.sugar", doctorOK)
	if check.Message != "Sugar active" || !strings.Contains(check.Detail, "existing Sugar configuration") {
		t.Fatalf("Sugar automatic state = %+v", check)
	}
}

func TestDoctorPlainOutputDoesNotLeakSecrets(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "gpt-4.1"
	cfg.Providers["openai"] = config.ProviderConfig{APIKey: "sk-secret-value", DefaultModel: "gpt-4.1"}
	cfg.Providers["ollama"] = config.ProviderConfig{Host: "http://user:host-secret@localhost:11434?token=query-secret", DefaultModel: "gpt-4.1"}
	cfg.MCP["secret"] = config.MCPServerConfig{
		Command: doctorTempExecutable(t),
		Args:    []string{"--token", "arg-secret-token", "--api-key=arg-secret-key"},
		Env:     map[string]string{"API_TOKEN": "top-secret-token"},
	}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q, stdout=%s", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Packetcode Doctor") {
		t.Fatalf("plain output missing header:\n%s", out)
	}
	for _, secret := range []string{"sk-secret-value", "top-secret-token", "arg-secret-token", "arg-secret-key", "host-secret", "query-secret"} {
		if strings.Contains(out, secret) {
			t.Fatalf("doctor leaked secret %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{"--token [REDACTED]", "--api-key=[REDACTED]", "user:%5BREDACTED%5D@localhost", "token=%5BREDACTED%5D"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing redaction marker %q:\n%s", want, out)
		}
	}
}

func TestDoctorDispatchSubcommand(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	var stdout, stderr bytes.Buffer
	code, ok := dispatchSubcommand([]string{"doctor", "--json", "--check", "version"}, &stdout, &stderr)
	if !ok {
		t.Fatal("doctor subcommand was not dispatched")
	}
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"id": "version.binary"`) {
		t.Fatalf("doctor dispatch did not run doctor command:\n%s", stdout.String())
	}

	if _, ok := dispatchSubcommand([]string{"unknown"}, &stdout, &stderr); ok {
		t.Fatal("unknown subcommand should not dispatch")
	}
}

func TestDoctorConfigParseErrorIsActionable(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	dir, err := config.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[default\nprovider = "), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1; stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	check := assertDoctorCheck(t, report, "config.file", doctorFail)
	if !strings.Contains(check.Detail, "config.toml") || check.Fix == "" {
		t.Fatalf("parse error is not actionable: %+v", check)
	}
}

func TestDoctorMissingDefaultProviderKeyFails(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cfg := config.Default()
	cfg.Default.Provider = "openai"
	cfg.Default.Model = "gpt-4.1"
	cfg.Providers["openai"] = config.ProviderConfig{DefaultModel: "gpt-4.1"}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1; stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	check := assertDoctorCheck(t, report, "config.default_provider", doctorFail)
	if !strings.Contains(check.Fix, "PACKETCODE_OPENAI_API_KEY") {
		t.Fatalf("missing key fix not useful: %+v", check)
	}
}

func TestDoctorOllamaNeedsNoAPIKey(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "qwen2.5-coder:14b"
	cfg.Providers["ollama"] = config.ProviderConfig{DefaultModel: "qwen2.5-coder:14b"}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	assertDoctorCheck(t, report, "config.default_provider", doctorOK)
	assertDoctorCheck(t, report, "providers.ollama", doctorOK)
}

func TestDoctorCustomProviderAccepted(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cfg := config.Default()
	cfg.Default.Provider = "localai"
	cfg.Default.Model = "local-model"
	keyless := false
	cfg.Providers["localai"] = config.ProviderConfig{
		Type:           "openai_compatible",
		BaseURL:        "http://localhost:8080/v1",
		DefaultModel:   "local-model",
		APIKeyRequired: &keyless,
	}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	assertDoctorCheck(t, report, "config.default_provider", doctorOK)
	check := assertDoctorCheck(t, report, "providers.localai", doctorOK)
	if !strings.Contains(check.Detail, "keyless") || !strings.Contains(check.Detail, "http://localhost:8080/v1") {
		t.Fatalf("custom provider detail not useful: %+v", check)
	}
}

func TestDoctorAcceptsCustomSugarOnlyWhenBuiltinIsExplicitlyDisabled(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	disabled := false
	keyless := false
	cfg := config.Default()
	cfg.Sugar.Enabled = &disabled
	cfg.Default.Provider = "sugar"
	cfg.Default.Model = "custom-model"
	cfg.Providers["sugar"] = config.ProviderConfig{
		Type: "openai_compatible", BaseURL: "http://localhost:8080/v1",
		DefaultModel: "custom-model", APIKeyRequired: &keyless,
	}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	assertDoctorCheck(t, report, "config.default_provider", doctorOK)
	check := assertDoctorCheck(t, report, "providers.sugar", doctorOK)
	if !strings.Contains(check.Message, "custom provider") || !strings.Contains(check.Detail, "keyless") {
		t.Fatalf("custom Sugar check = %+v", check)
	}
	for _, check := range report.Checks {
		if check.ID == "providers.sugar.custom_fields" {
			t.Fatalf("custom Sugar incorrectly treated as built-in: %+v", check)
		}
	}
}

func TestDoctorCustomProviderInvalidBaseURLFails(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cfg := config.Default()
	cfg.Default.Provider = "localai"
	cfg.Default.Model = "local-model"
	cfg.Providers["localai"] = config.ProviderConfig{
		Type:         "openai_compatible",
		BaseURL:      "localhost:8080/v1",
		DefaultModel: "local-model",
	}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1; stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	assertDoctorCheck(t, report, "config.default_provider", doctorFail)
}

func TestDoctorBuiltInProviderRejectsCustomFields(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	keyless := false
	cfg := config.Default()
	cfg.Default.Provider = "openai"
	cfg.Default.Model = "gpt-4.1"
	cfg.Providers["openai"] = config.ProviderConfig{
		APIKey:         "sk-test",
		DefaultModel:   "gpt-4.1",
		Type:           "openai_compatible",
		BaseURL:        "http://localhost:8080/v1",
		APIKeyRequired: &keyless,
	}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1; stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	check := assertDoctorCheck(t, report, "providers.openai.custom_fields", doctorFail)
	if !strings.Contains(check.Detail, "type") || !strings.Contains(check.Detail, "base_url") || !strings.Contains(check.Detail, "api_key_required") {
		t.Fatalf("custom field detail not useful: %+v", check)
	}
}

func TestDoctorMCPStaticChecks(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cmdPath := doctorTempExecutable(t)
	disabled := false
	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "model"
	cfg.MCP["ok"] = config.MCPServerConfig{Command: cmdPath, Env: map[string]string{"TOKEN": "secret"}, EnvFrom: []string{"GITHUB_TOKEN"}}
	cfg.MCP["disabled"] = config.MCPServerConfig{Command: "missing-disabled-command", Enabled: &disabled}
	cfg.MCP["missing"] = config.MCPServerConfig{Command: "missing-packetcode-doctor-command"}
	cfg.MCP["bad.name"] = config.MCPServerConfig{Command: cmdPath}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1; stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	ok := assertDoctorCheck(t, report, "mcp.ok", doctorOK)
	if strings.Contains(ok.Detail, "secret") || !strings.Contains(ok.Detail, "auth:env:TOKEN") || !strings.Contains(ok.Detail, "from:GITHUB_TOKEN") {
		t.Fatalf("MCP auth summary leaked or omitted env key: %+v", ok)
	}
	assertDoctorCheck(t, report, "mcp.disabled", doctorSkip)
	assertDoctorCheck(t, report, "mcp.missing.command", doctorFail)
	assertDoctorCheck(t, report, "mcp.bad.name.name", doctorFail)
}

func TestDoctorMCPEnvFromMissingWarnsWithoutLeaking(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cmdPath := doctorTempExecutable(t)
	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "model"
	cfg.MCP["github"] = config.MCPServerConfig{Command: cmdPath, EnvFrom: []string{"MISSING_PACKETCODE_DOCTOR_ENV_FROM"}}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json", "--check", "mcp"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	warn := assertDoctorCheck(t, report, "mcp.github.env_from", doctorWarn)
	if !strings.Contains(warn.Detail, "MISSING_PACKETCODE_DOCTOR_ENV_FROM") {
		t.Fatalf("missing env_from detail not useful: %+v", warn)
	}
	ok := assertDoctorCheck(t, report, "mcp.github", doctorOK)
	if !strings.Contains(ok.Detail, "from:MISSING_PACKETCODE_DOCTOR_ENV_FROM:missing") {
		t.Fatalf("MCP detail should mark missing env_from: %+v", ok)
	}
}

func TestDoctorPermissionPolicyChecks(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "model"
	cfg.Permissions.Profile = "edit"
	cfg.Permissions.Rules = []config.PermissionRule{{Tool: "execute_command", Action: "deny"}}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json", "--check", "permissions"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	check := assertDoctorCheck(t, report, "permissions.profile", doctorOK)
	if !strings.Contains(check.Detail, "profile: accept_edits") || !strings.Contains(check.Detail, "rule: execute_command -> deny") {
		t.Fatalf("permission detail missing policy summary: %+v", check)
	}
}

func TestDoctorPermissionPolicyInvalidConfigFails(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "model"
	cfg.Permissions.Profile = "nope"
	cfg.Permissions.Rules = []config.PermissionRule{{Tool: "write_file", Action: "maybe"}}
	requireSaveConfig(t, cfg)

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--json", "--check", "permissions"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1; stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	check := assertDoctorCheck(t, report, "permissions.config", doctorFail)
	if !strings.Contains(check.Detail, "unknown permission") {
		t.Fatalf("permission failure not actionable: %+v", check)
	}
}

func TestResolveCommandRejectsNonExecutablePathOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executability is extension/PATHEXT based")
	}
	path := filepath.Join(t.TempDir(), "mcp-server")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCommand(path, t.TempDir()); err == nil {
		t.Fatal("expected non-executable path command to fail")
	}
}

func TestDoctorCheckFilterLimitsSections(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--check", "config", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stderr=%q stdout=%s", code, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, stdout.String())
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected filtered checks")
	}
	for _, check := range report.Checks {
		if check.Section != "config" {
			t.Fatalf("filtered report included non-config check: %+v", check)
		}
	}
}

func TestDoctorCheckFilterRejectsUnknown(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	var stdout, stderr bytes.Buffer
	code := runDoctorCommand([]string{"--check", "network"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("doctor exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unknown doctor check "network"`) {
		t.Fatalf("stderr missing unknown check error: %s", stderr.String())
	}
}

func isolateDoctorEnv(t *testing.T) func() {
	t.Helper()
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(config.HomeEnv, "")
	t.Setenv("PACKETCODE_OPENAI_API_KEY", "")
	t.Setenv("PACKETCODE_ANTHROPIC_API_KEY", "")
	t.Setenv("PACKETCODE_GEMINI_API_KEY", "")
	t.Setenv("PACKETCODE_MINIMAX_API_KEY", "")
	t.Setenv("PACKETCODE_OPENROUTER_API_KEY", "")
	t.Setenv("PACKETCODE_OLLAMA_HOST", "")
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	}
}

func requireSaveConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
}

func assertDoctorCheck(t *testing.T, report doctorReport, id, status string) doctorCheck {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			if check.Status != status {
				t.Fatalf("%s status = %q, want %q; check=%+v", id, check.Status, status, check)
			}
			return check
		}
	}
	t.Fatalf("missing doctor check %q in %#v", id, report.Checks)
	return doctorCheck{}
}

func doctorTempExecutable(t *testing.T) string {
	t.Helper()
	name := "doctor-exec"
	content := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		name = "doctor-exec.bat"
		content = "@echo off\r\nexit /b 0\r\n"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
