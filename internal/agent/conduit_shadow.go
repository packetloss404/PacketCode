package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/packetcode/packetcode/internal/handoff"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/sugar"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

type SugarCacheConfig struct {
	Mode      provider.SugarCacheMode
	Retention provider.SugarCacheRetention
	Privacy   provider.SugarPrivacyMode
}

type ConduitShadowConfig struct {
	Enabled         bool
	Timeout         time.Duration
	CapsuleMaxBytes int
}

type conduitRuntimeProvider interface {
	RuntimeHooks() sugar.RuntimeHooks
}

// conduitShadowState belongs to one user turn. It is deliberately separate
// from the live provider path: decisions are recorded as local evidence only.
type conduitShadowState struct {
	cfg      ConduitShadowConfig
	sessions *session.Manager
	hooks    sugar.RuntimeHooks
	runID    string
	seq      int
	started  bool
	active   bool
	salt     [32]byte
	capsule  handoff.SpecialistCapsule
}

func newConduitShadowState(cfg ConduitShadowConfig, sessions *session.Manager, intent string) *conduitShadowState {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 1500 * time.Millisecond
	}
	if cfg.CapsuleMaxBytes <= 0 {
		cfg.CapsuleMaxBytes = handoff.DefaultMaxBytes
	}
	state := &conduitShadowState{cfg: cfg, sessions: sessions}
	state.capsule = handoff.SpecialistCapsule{
		SchemaVersion: handoff.SchemaVersion,
		Intent:        intent,
		Constraints:   handoff.ExtractConstraints(intent),
	}
	if current := sessions.Current(); current != nil {
		state.capsule.Generation = current.Cache.CompactionGeneration
	}
	if _, err := rand.Read(state.salt[:]); err != nil {
		// Without per-run entropy, opaque fingerprints could become a useful
		// dictionary oracle. Fail the optional shadow feature closed.
		state.cfg.Enabled = false
	}
	return state
}

func (s *conduitShadowState) start(ctx context.Context, prov provider.Provider, req provider.ChatRequest) {
	if s == nil || s.started || !s.cfg.Enabled {
		return
	}
	s.started = true
	if prov == nil || prov.Slug() != sugar.Slug || req.Model != sugar.DefaultModel {
		return
	}
	runtimeProvider, ok := prov.(conduitRuntimeProvider)
	if !ok {
		return
	}
	s.hooks = runtimeProvider.RuntimeHooks()
	if s.hooks == nil {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	response, err := s.hooks.StartRun(callCtx, sugar.RuntimeRunStart{
		IdempotencyKey: "pc-run-" + uuid.NewString(),
		Request:        req,
	})
	cancel()
	if err != nil || response == nil || response.Run.ID == "" || response.ExecutesUpstream {
		return
	}
	s.runID = response.Run.ID
	s.active = true
	s.persistCapsule()
}

func (s *conduitShadowState) providerFailure(ctx context.Context, err error) {
	if s == nil || !s.active || err == nil {
		return
	}
	lower := strings.ToLower(err.Error())
	kind := sugar.RuntimeProviderAmbiguous
	switch {
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "429"):
		kind = sugar.RuntimeProviderRateLimited
	case strings.Contains(lower, "unavailable") || strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") || strings.Contains(lower, "503"):
		kind = sugar.RuntimeProviderUnavailable
	}
	s.emit(ctx, sugar.RuntimeEvent{
		Type:               sugar.RuntimeProvider,
		FailureKind:        kind,
		FailureFingerprint: s.fingerprint("provider", string(kind), err.Error()),
	}, handoff.Evidence{Kind: "provider", Result: string(kind)})
}

func (s *conduitShadowState) blocked(ctx context.Context, call provider.ToolCall, reason string) {
	// Guard before fingerprint(): it reads s.salt, so an inactive-shadow call
	// would hash for nothing and a nil receiver would panic. providerFailure
	// and toolResult already lead with the same check.
	if s == nil || !s.active {
		return
	}
	category := classifyTool(call)
	s.emit(ctx, sugar.RuntimeEvent{
		Type:               sugar.RuntimeBlocked,
		ToolCategory:       category,
		Success:            boolPointer(false),
		FailureKind:        sugar.RuntimeToolBlocked,
		FailureFingerprint: s.fingerprint("blocked", call.Name, reason),
	}, handoff.Evidence{Kind: "blocked", Result: string(category) + " blocked"})
}

func (s *conduitShadowState) toolResult(ctx context.Context, call provider.ToolCall, result tools.ToolResult) {
	if s == nil || !s.active {
		return
	}
	category := classifyTool(call)
	success := !result.IsError
	eventType := sugar.RuntimeToolResult
	if isGateCategory(category) || call.Name == "get_diagnostics" {
		eventType = sugar.RuntimeValidation
	}
	event := sugar.RuntimeEvent{Type: eventType, ToolCategory: category, Success: boolPointer(success)}
	exitCode, hasExit := metadataInt(result.Metadata, "exit_code")
	if hasExit {
		event.ExitCode = &exitCode
		if exitCode != 0 {
			success = false
			*event.Success = false
		}
	}
	if call.Name == "get_diagnostics" {
		count, ok := metadataInt(result.Metadata, "diagnostic_count")
		if ok {
			event.NewFailures = &count
			if count > 0 {
				success = false
				*event.Success = false
			}
		}
	}
	if call.Name == "write_file" || call.Name == "patch_file" {
		one := 1
		event.FilesTouched = &one
	}
	if duration, ok := metadataInt(result.Metadata, "duration_ms"); ok && duration >= 0 {
		event.DurationMS = &duration
	}
	if !success {
		if eventType == sugar.RuntimeValidation {
			event.FailureKind = sugar.RuntimeValidationFailed
		} else {
			event.FailureKind = sugar.RuntimeToolTransient
		}
		event.FailureFingerprint = s.fingerprint(string(category), call.Name, result.Content)
	}

	summary := outcomeSummary(category, success, event.ExitCode, event.NewFailures)
	evidence := handoff.Evidence{Kind: string(category), Command: gateCommand(category), Result: summary, Fingerprint: event.FailureFingerprint}
	if !success && eventType == sugar.RuntimeValidation {
		s.capsule.FailedGates = append(s.capsule.FailedGates, handoff.FailedGate{
			Name: string(category), Summary: summary, Fingerprint: event.FailureFingerprint, Excerpt: result.Content,
		})
	}
	s.captureLocalChange(call)
	s.emit(ctx, event, evidence)
}

func (s *conduitShadowState) emit(ctx context.Context, event sugar.RuntimeEvent, evidence handoff.Evidence) {
	if s == nil || !s.active {
		return
	}
	next := s.seq + 1
	event.RunID = s.runID
	event.Seq = next
	event.IdempotencyKey = fmt.Sprintf("pc-event-%06d-%s", next, uuid.NewString())
	callCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	response, err := s.hooks.EmitEvent(callCtx, event)
	cancel()
	if err != nil || response == nil || response.ExecutesUpstream || response.Event.Seq != next {
		s.active = false
		return
	}
	s.seq = next
	s.capsule.Evidence = append(s.capsule.Evidence, evidence)
	if event.ToolCategory != "" {
		s.capsule.ChangeBuckets = append(s.capsule.ChangeBuckets, string(event.ToolCategory))
	}

	callCtx, cancel = context.WithTimeout(ctx, s.cfg.Timeout)
	decision, err := s.hooks.Continue(callCtx, s.runID, fmt.Sprintf("pc-continue-%06d", next))
	cancel()
	if err != nil || decision == nil || decision.ExecutesUpstream {
		s.active = false
		s.persistCapsule()
		return
	}
	// Decision metadata is local telemetry only. In particular, PredictedModel
	// is never passed to Registry.SetActive or copied into a ChatRequest.
	if decision.Decision.Action == "repair" || decision.Decision.Action == "escalate" {
		reasons := strings.Join(decision.Decision.ReasonCodes, ",")
		s.capsule.UnresolvedDecisions = append(s.capsule.UnresolvedDecisions,
			fmt.Sprintf("shadow recommendation: %s (%s)", decision.Decision.Action, reasons))
	}
	s.persistCapsule()
}

func (s *conduitShadowState) persistCapsule() {
	if s == nil || s.sessions == nil {
		return
	}
	// Keep even the in-memory capsule normalized; raw patch/gate excerpts must
	// not linger in a reusable handoff object between persistence points.
	s.capsule = handoff.Normalize(s.capsule, s.cfg.CapsuleMaxBytes)
	_ = s.sessions.SetSpecialistCapsule(s.capsule, s.cfg.CapsuleMaxBytes)
}

func (s *conduitShadowState) captureLocalChange(call provider.ToolCall) {
	if call.Name != "write_file" && call.Name != "patch_file" {
		return
	}
	var args map[string]any
	if json.Unmarshal([]byte(call.Arguments), &args) != nil {
		return
	}
	path, _ := args["path"].(string)
	change := handoff.Change{Path: path, Summary: call.Name + " completed"}
	if call.Name == "patch_file" {
		change.PatchExcerpt, _ = args["patch"].(string)
	}
	s.capsule.Changes = append(s.capsule.Changes, change)
	if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx") || strings.HasSuffix(path, ".json") {
		s.capsule.ChangedAPIsSchemas = append(s.capsule.ChangedAPIsSchemas, path)
	}
}

func (s *conduitShadowState) fingerprint(parts ...string) string {
	hash := sha256.New()
	_, _ = hash.Write(s.salt[:])
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(part))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func classifyTool(call provider.ToolCall) sugar.RuntimeToolCategory {
	switch call.Name {
	// These are the registered tool names (see internal/tools). Getting one
	// wrong is silent: the call falls through to RuntimeToolOther and the
	// shadow record claims the turn touched no files.
	case "write_file", "patch_file", "read_file", "search_codebase", "list_directory", "get_diagnostics":
		if call.Name == "get_diagnostics" {
			return sugar.RuntimeToolTypecheck
		}
		return sugar.RuntimeToolFile
	case "execute_command":
		var args struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &args)
		command := strings.ToLower(args.Command)
		switch {
		case strings.Contains(command, "go test"), strings.Contains(command, "pytest"), strings.Contains(command, "cargo test"), strings.Contains(command, "npm test"), strings.Contains(command, "pnpm test"), strings.Contains(command, "vitest"):
			return sugar.RuntimeToolTest
		case strings.Contains(command, "golangci"), strings.Contains(command, "eslint"), strings.Contains(command, "ruff"), strings.Contains(command, " clippy"), strings.Contains(command, " lint"):
			return sugar.RuntimeToolLint
		case strings.Contains(command, "tsc"), strings.Contains(command, "mypy"), strings.Contains(command, "go vet"), strings.Contains(command, "typecheck"):
			return sugar.RuntimeToolTypecheck
		case strings.Contains(command, "go build"), strings.Contains(command, "cargo build"), strings.Contains(command, "npm run build"), strings.Contains(command, "pnpm build"):
			return sugar.RuntimeToolBuild
		default:
			return sugar.RuntimeToolShell
		}
	default:
		return sugar.RuntimeToolOther
	}
}

func metadataInt(metadata map[string]any, key string) (int, bool) {
	if metadata == nil {
		return 0, false
	}
	switch value := metadata[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), value == float64(int(value))
	default:
		return 0, false
	}
}

func isGateCategory(category sugar.RuntimeToolCategory) bool {
	return category == sugar.RuntimeToolTest || category == sugar.RuntimeToolLint || category == sugar.RuntimeToolTypecheck || category == sugar.RuntimeToolBuild
}

func gateCommand(category sugar.RuntimeToolCategory) string {
	if isGateCategory(category) {
		return string(category)
	}
	return ""
}

func outcomeSummary(category sugar.RuntimeToolCategory, success bool, exitCode, failures *int) string {
	state := "passed"
	if !success {
		state = "failed"
	}
	summary := string(category) + " " + state
	if exitCode != nil {
		summary += fmt.Sprintf(" (exit_code=%d)", *exitCode)
	}
	if failures != nil {
		summary += fmt.Sprintf(" (new_failures=%d)", *failures)
	}
	return summary
}

func boolPointer(value bool) *bool { return &value }
