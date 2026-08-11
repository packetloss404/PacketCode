package sugar

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/openaicompat"
)

// RuntimeEventType matches Sugar v8's bounded, decision-only shadow telemetry.
// No prompts, tool arguments, code, command output, or arbitrary metadata are
// accepted by this client type.
type RuntimeEventType string

const (
	RuntimeToolResult RuntimeEventType = "tool_result"
	RuntimeValidation RuntimeEventType = "validation"
	RuntimeProgress   RuntimeEventType = "progress"
	RuntimeBlocked    RuntimeEventType = "blocked"
	RuntimeProvider   RuntimeEventType = "provider"
)

type RuntimeToolCategory string

const (
	RuntimeToolShell     RuntimeToolCategory = "shell"
	RuntimeToolTest      RuntimeToolCategory = "test"
	RuntimeToolLint      RuntimeToolCategory = "lint"
	RuntimeToolTypecheck RuntimeToolCategory = "typecheck"
	RuntimeToolBuild     RuntimeToolCategory = "build"
	RuntimeToolFile      RuntimeToolCategory = "file"
	RuntimeToolNetwork   RuntimeToolCategory = "network"
	RuntimeToolDatabase  RuntimeToolCategory = "database"
	RuntimeToolOther     RuntimeToolCategory = "other"
)

type RuntimeFailureKind string

const (
	RuntimeCallerInvalid       RuntimeFailureKind = "caller_invalid"
	RuntimeProviderRateLimited RuntimeFailureKind = "provider_rate_limited"
	RuntimeProviderUnavailable RuntimeFailureKind = "provider_unavailable"
	RuntimeProviderAmbiguous   RuntimeFailureKind = "provider_ambiguous"
	RuntimePolicyRefusal       RuntimeFailureKind = "policy_refusal"
	RuntimeToolBlocked         RuntimeFailureKind = "tool_blocked"
	RuntimeToolTransient       RuntimeFailureKind = "tool_transient"
	RuntimeValidationFailed    RuntimeFailureKind = "validation_failed"
	RuntimeSemanticIncomplete  RuntimeFailureKind = "semantic_incomplete"
	RuntimeContextExhausted    RuntimeFailureKind = "context_exhausted"
	RuntimeOutputTruncated     RuntimeFailureKind = "output_truncated"
	RuntimeBudgetExhausted     RuntimeFailureKind = "budget_exhausted"
	RuntimeDeadline            RuntimeFailureKind = "deadline"
	RuntimeUnknown             RuntimeFailureKind = "unknown"
)

// RuntimeRunStart is an explicit, opt-in mirror of one existing chat request.
// Packetcode does not call it automatically. Sugar classifies it but never
// executes an upstream model request.
type RuntimeRunStart struct {
	IdempotencyKey string
	Request        provider.ChatRequest
}

type RuntimeRun struct {
	ID             string `json:"id"`
	Object         string `json:"object,omitempty"`
	Mode           string `json:"mode,omitempty"`
	RequestedModel string `json:"requestedModel,omitempty"`
	Status         string `json:"status,omitempty"`
}

type RuntimeRunResponse struct {
	Run              RuntimeRun `json:"run"`
	Replayed         bool       `json:"replayed"`
	ExecutesUpstream bool       `json:"executesUpstream"`
}

// RuntimeEvent is the exact Sugar v8 event allowlist. Optional numeric and
// boolean values are pointers so a reported zero/false is not dropped.
// FailureFingerprint must be an already-opaque sha256 digest.
type RuntimeEvent struct {
	RunID              string              `json:"-"`
	Seq                int                 `json:"seq"`
	IdempotencyKey     string              `json:"idempotencyKey"`
	Type               RuntimeEventType    `json:"type"`
	ToolCategory       RuntimeToolCategory `json:"toolCategory,omitempty"`
	Success            *bool               `json:"success,omitempty"`
	FailureKind        RuntimeFailureKind  `json:"failureKind,omitempty"`
	FailureFingerprint string              `json:"failureFingerprint,omitempty"`
	ExitCode           *int                `json:"exitCode,omitempty"`
	NewFailures        *int                `json:"newFailures,omitempty"`
	FilesTouched       *int                `json:"filesTouched,omitempty"`
	DurationMS         *int                `json:"durationMs,omitempty"`
}

type RuntimeEventResult struct {
	Seq        int              `json:"seq"`
	Type       RuntimeEventType `json:"type"`
	ScoreDelta int              `json:"scoreDelta"`
	Duplicate  bool             `json:"duplicate"`
}

type RuntimeEventResponse struct {
	Object           string             `json:"object"`
	Event            RuntimeEventResult `json:"event"`
	Run              RuntimeRun         `json:"run"`
	ExecutesUpstream bool               `json:"executesUpstream"`
}

type RuntimeDecision struct {
	Action         string   `json:"action"`
	WouldEscalate  bool     `json:"wouldEscalate"`
	Tier           string   `json:"tier"`
	PredictedModel string   `json:"predictedModel"`
	ReasonCodes    []string `json:"reasonCodes"`
	StopReason     *string  `json:"stopReason"`
}

type RuntimeContinueResponse struct {
	Object           string          `json:"object"`
	Decision         RuntimeDecision `json:"decision"`
	Run              RuntimeRun      `json:"run"`
	Replayed         bool            `json:"replayed"`
	ExecutesUpstream bool            `json:"executesUpstream"`
}

// RuntimeHooks is the disabled-by-default seam the app can adopt after an
// explicit user/admin opt-in. It is intentionally not invoked by normal chat.
type RuntimeHooks interface {
	StartRun(context.Context, RuntimeRunStart) (*RuntimeRunResponse, error)
	EmitEvent(context.Context, RuntimeEvent) (*RuntimeEventResponse, error)
	Continue(context.Context, string, string) (*RuntimeContinueResponse, error)
}

type DisabledRuntimeHooks struct{}

func (DisabledRuntimeHooks) StartRun(context.Context, RuntimeRunStart) (*RuntimeRunResponse, error) {
	return nil, nil
}
func (DisabledRuntimeHooks) EmitEvent(context.Context, RuntimeEvent) (*RuntimeEventResponse, error) {
	return nil, nil
}
func (DisabledRuntimeHooks) Continue(context.Context, string, string) (*RuntimeContinueResponse, error) {
	return nil, nil
}

// RuntimeClient speaks Sugar v8's shadow-only endpoints but is inert unless
// explicitly created with enabled=true. There is no automatic production
// shadowing and no model decision is applied to the active chat request.
type RuntimeClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	enabled    atomic.Bool
}

func NewRuntimeClient(baseURL, apiKey string, httpClient *http.Client, enabled bool) *RuntimeClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	client := &RuntimeClient{baseURL: NormalizeBaseURL(baseURL), apiKey: apiKey, httpClient: httpClient}
	client.enabled.Store(enabled)
	return client
}

func (c *RuntimeClient) Enabled() bool { return c != nil && c.enabled.Load() }

func (c *RuntimeClient) StartRun(ctx context.Context, start RuntimeRunStart) (*RuntimeRunResponse, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if !validOpaqueID(start.IdempotencyKey, 128) {
		return nil, fmt.Errorf("Conduit run idempotency key is invalid")
	}
	if start.Request.Model != DefaultModel {
		return nil, fmt.Errorf("Conduit shadow runs require model %q", DefaultModel)
	}
	body, err := openaicompat.MarshalChatRequest(start.Request)
	if err != nil {
		return nil, fmt.Errorf("marshal Conduit shadow run: %w", err)
	}
	status, responseBody, err := c.postJSON(ctx, "/conduit/runs", start.IdempotencyKey, body)
	if err != nil {
		return nil, err
	}
	if c.startFallback(status) {
		return nil, nil
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return nil, runtimeStatusError("start Conduit shadow run", status, responseBody)
	}
	var response RuntimeRunResponse
	if err := json.Unmarshal(responseBody, &response); err != nil || !validOpaqueID(response.Run.ID, 128) || response.ExecutesUpstream {
		return nil, fmt.Errorf("start Conduit shadow run: invalid response")
	}
	return &response, nil
}

func (c *RuntimeClient) EmitEvent(ctx context.Context, event RuntimeEvent) (*RuntimeEventResponse, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if err := validateRuntimeEvent(event); err != nil {
		return nil, err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	path := "/conduit/runs/" + url.PathEscape(event.RunID) + "/events"
	status, responseBody, err := c.postJSON(ctx, path, "", body)
	if err != nil {
		return nil, err
	}
	if runtimeEndpointUnavailable(status, responseBody) {
		c.enabled.Store(false)
		return nil, nil
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return nil, runtimeStatusError("emit Conduit shadow event", status, responseBody)
	}
	var response RuntimeEventResponse
	if err := json.Unmarshal(responseBody, &response); err != nil || response.ExecutesUpstream {
		return nil, fmt.Errorf("emit Conduit shadow event: invalid response")
	}
	return &response, nil
}

func (c *RuntimeClient) Continue(ctx context.Context, runID, idempotencyKey string) (*RuntimeContinueResponse, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if !validOpaqueID(runID, 128) || !validOpaqueID(idempotencyKey, 128) {
		return nil, fmt.Errorf("Conduit continue identifiers are invalid")
	}
	path := "/conduit/runs/" + url.PathEscape(runID) + "/continue"
	status, responseBody, err := c.postJSON(ctx, path, idempotencyKey, nil)
	if err != nil {
		return nil, err
	}
	if runtimeEndpointUnavailable(status, responseBody) {
		c.enabled.Store(false)
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, runtimeStatusError("continue Conduit shadow run", status, responseBody)
	}
	var response RuntimeContinueResponse
	if err := json.Unmarshal(responseBody, &response); err != nil || response.ExecutesUpstream {
		return nil, fmt.Errorf("continue Conduit shadow run: invalid response")
	}
	return &response, nil
}

func (c *RuntimeClient) postJSON(ctx context.Context, path, idempotencyKey string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, responseBody, nil
}

func (c *RuntimeClient) startFallback(status int) bool {
	if status != http.StatusNotFound && status != http.StatusMethodNotAllowed && status != http.StatusNotImplemented {
		return false
	}
	c.enabled.Store(false)
	return true
}

func runtimeEndpointUnavailable(status int, body []byte) bool {
	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		return true
	}
	if status != http.StatusNotFound {
		return false
	}
	// A Sugar JSON 404 identifies a missing/expired run and should remain an
	// actionable error. A generic router 404 means this optional nested route
	// is not deployed yet and should make the hook inert.
	var payload struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &payload) != nil || payload.Error.Type == ""
}

func runtimeStatusError(operation string, status int, body []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error.Message != "" {
		return fmt.Errorf("%s: status %d: %s", operation, status, payload.Error.Message)
	}
	return fmt.Errorf("%s: status %d", operation, status)
}

func validateRuntimeEvent(event RuntimeEvent) error {
	if !validOpaqueID(event.RunID, 128) || !validOpaqueID(event.IdempotencyKey, 128) {
		return fmt.Errorf("Conduit event identifiers are invalid")
	}
	if event.Seq < 1 || event.Seq > 1_000_000 {
		return fmt.Errorf("Conduit event seq is invalid")
	}
	switch event.Type {
	case RuntimeToolResult, RuntimeValidation, RuntimeProgress, RuntimeBlocked, RuntimeProvider:
	default:
		return fmt.Errorf("Conduit event type is invalid")
	}
	if event.ToolCategory != "" && !validToolCategory(event.ToolCategory) {
		return fmt.Errorf("Conduit event tool category is invalid")
	}
	if event.FailureKind != "" && !validFailureKind(event.FailureKind) {
		return fmt.Errorf("Conduit event failure kind is invalid")
	}
	providerFailure := event.FailureKind == RuntimeProviderRateLimited || event.FailureKind == RuntimeProviderUnavailable || event.FailureKind == RuntimeProviderAmbiguous
	if (event.Type == RuntimeProvider) != providerFailure && event.FailureKind != "" {
		return fmt.Errorf("Conduit provider failures require event type provider, and provider events accept provider failures only")
	}
	if event.FailureFingerprint != "" && !validSHA256Fingerprint(event.FailureFingerprint) {
		return fmt.Errorf("Conduit failure fingerprint must be sha256:<64 lowercase hex characters>")
	}
	if !validOptionalInt(event.ExitCode, -32_768, 32_767) || !validOptionalInt(event.NewFailures, 0, 100_000) || !validOptionalInt(event.FilesTouched, 0, 100_000) || !validOptionalInt(event.DurationMS, 0, 86_400_000) {
		return fmt.Errorf("Conduit event counter is out of range")
	}
	if event.Type == RuntimeValidation && event.Success != nil && *event.Success {
		if event.FailureKind != "" || (event.ExitCode != nil && *event.ExitCode != 0) || (event.NewFailures != nil && *event.NewFailures != 0) {
			return fmt.Errorf("successful Conduit validation cannot report a failure, nonzero exit code, or new failures")
		}
	}
	return nil
}

func validToolCategory(category RuntimeToolCategory) bool {
	switch category {
	case RuntimeToolShell, RuntimeToolTest, RuntimeToolLint, RuntimeToolTypecheck, RuntimeToolBuild, RuntimeToolFile, RuntimeToolNetwork, RuntimeToolDatabase, RuntimeToolOther:
		return true
	default:
		return false
	}
}

func validFailureKind(kind RuntimeFailureKind) bool {
	switch kind {
	case RuntimeCallerInvalid, RuntimeProviderRateLimited, RuntimeProviderUnavailable, RuntimeProviderAmbiguous, RuntimePolicyRefusal, RuntimeToolBlocked, RuntimeToolTransient, RuntimeValidationFailed, RuntimeSemanticIncomplete, RuntimeContextExhausted, RuntimeOutputTruncated, RuntimeBudgetExhausted, RuntimeDeadline, RuntimeUnknown:
		return true
	default:
		return false
	}
}

func validSHA256Fingerprint(fingerprint string) bool {
	if !strings.HasPrefix(fingerprint, "sha256:") || len(fingerprint) != 71 {
		return false
	}
	digest := fingerprint[len("sha256:"):]
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32 && strings.ToLower(digest) == digest
}

func validOptionalInt(value *int, min, max int) bool {
	return value == nil || (*value >= min && *value <= max)
}
