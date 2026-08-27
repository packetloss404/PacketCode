package jobs

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

// summaryMaxLen caps the auto-extracted job summary surfaced to the
// main conversation. Per spec ~280 chars (one tweet's worth).
const summaryMaxLen = 280

// runJob is the per-job worker goroutine. It blocks on the manager's
// semaphore (honouring MaxConcurrent), then builds the per-job session,
// backups, tool registry, and agent. It consumes the agent's event
// channel until terminal and then publishes the final snapshot via
// markTerminal. Panics inside the agent loop are recovered and
// translated to StateFailed.
//
// jobCtx is allocated by Manager.Spawn (with its CancelFunc already
// registered into m.cancel) so /cancel works while the job is still
// queued for a sem slot.
func (m *Manager) runJob(j *Job, req SpawnRequest, jobCtx context.Context) {
	var runtimeBackend computers.RuntimeBackend
	defer m.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			if runtimeBackend != nil {
				_ = runtimeBackend.Close()
			}
			stack := string(debug.Stack())
			m.markTerminal(j, StateFailed,
				"", fmt.Sprintf("worker panic: %v\n%s", r, stack), "panic",
				j.InputTokens, j.OutputTokens, j.CostUSD, j.Transcript, nil)
		}
	}()

	// Acquire the semaphore (Queued → Running barrier). Watch jobCtx
	// so /cancel on a queued job aborts before consuming a slot.
	select {
	case m.sem <- struct{}{}:
	case <-jobCtx.Done():
		// Cancelled while queued.
		m.markTerminal(j, StateCancelled, "", "", "cancelled while queued",
			j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
		return
	case <-m.baseCtx.Done():
		// Manager shut down before we got a slot. Still Cancelled, not
		// Abandoned: the job provably never ran, so its outcome is known.
		m.markTerminal(j, StateCancelled, "", "", "manager shutdown before start",
			j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
		return
	}
	defer func() { <-m.sem }()

	// jobCtx already wired into m.cancel by Spawn(). No additional
	// registration needed; just check it hasn't been cancelled while
	// we were waiting.
	if jobCtx.Err() != nil {
		m.markTerminal(j, StateCancelled, "", "", "cancelled before start",
			j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
		return
	}

	m.markRunning(j)
	jobRoot := m.cfg.Root
	if j.ComputerID != "" {
		var backendErr error
		runtimeBackend, backendErr = m.prepareRemoteBackend(jobCtx, j)
		if backendErr != nil {
			if jobCtx.Err() != nil {
				// Past markRunning, so work may already have begun on the
				// remote host. Preserve the real setup error instead of
				// discarding it behind "context canceled". backendErr is
				// concrete evidence the transport failed, so it earns the
				// transport-lost cause when nothing requested the stop.
				state, cause := m.classifyCancelledWithCause(j, AbandonCauseTransportLost)
				m.markTerminalCause(j, state, cause, "", "prepare remote workspace: "+backendErr.Error(), jobCtx.Err().Error(),
					j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
				return
			}
			m.markTerminal(j, StateFailed, "", "prepare remote workspace: "+backendErr.Error(), "",
				j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
			return
		}
		defer runtimeBackend.Close()
		stopBackendWatcher := closeBackendOnCancel(jobCtx, runtimeBackend)
		defer stopBackendWatcher()
		jobRoot = runtimeBackend.Root()
	} else {
		worktree, worktreeErr := m.prepareWorktree(jobCtx, j)
		if worktreeErr != nil {
			if jobCtx.Err() != nil {
				state, cause := m.classifyCancelled(j)
				m.markTerminalCause(j, state, cause, "", "prepare worktree: "+worktreeErr.Error(), jobCtx.Err().Error(),
					j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
				return
			}
			m.markTerminal(j, StateFailed, "", "prepare worktree: "+worktreeErr.Error(), "",
				j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
			return
		}
		jobRoot = worktree.Root
		if jobRoot == "" {
			jobRoot = m.cfg.Root
		}
	}

	// Build the per-job dependencies.
	subSession, sessErr := m.openSubSession(j)
	if sessErr != nil {
		m.markTerminal(j, StateFailed, "", "open sub-session: "+sessErr.Error(), "",
			j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
		return
	}
	m.attachLiveSession(j.ID, subSession)
	defer m.detachLiveSession(j.ID)
	backups := session.NewBackupManager(m.cfg.BackupsDir, j.SessionID)

	subRegistry, regErr := m.buildJobProviderRegistry(j)
	if regErr != nil {
		m.markTerminal(j, StateFailed, "", "build provider registry: "+regErr.Error(), "",
			j.InputTokens, j.OutputTokens, j.CostUSD, nil, nil)
		return
	}

	// Snapshot the late-bindable config fields under the read lock so
	// SetSpawnToolFactory/SetApprover (which take the write lock) don't
	// race with our reads here.
	m.mu.RLock()
	spawnToolFactory := m.cfg.SpawnTool
	collectToolFactory := m.cfg.CollectTool
	systemPromptFor := m.cfg.SystemPromptFor
	parentApprover := m.cfg.Approver
	permissionPolicy := m.cfg.PermissionPolicy
	hookRunner := m.cfg.Hooks
	maxDepth := m.cfg.MaxDepth
	m.mu.RUnlock()
	if runtimeBackend != nil {
		// Hook commands are local processes and must never be pointed at a
		// remote POSIX path or implied to execute on the Packet Computer.
		hookRunner = nil
	} else {
		if hookRunner != nil {
			hookRunner = hookRunner.WithCWD(jobRoot)
		}
	}

	// Conditionally include spawn_agent only when the new job's depth
	// is below MaxDepth-1 (so its children would still be inside the
	// cap). The SpawnAgentTool factory is threaded through extraTools.
	var extraTools []tools.Tool
	if spawnToolFactory != nil && j.Depth < maxDepth-1 {
		if t := spawnToolFactory(j.ID, j.Depth, j.AllowWrite); t != nil {
			extraTools = append(extraTools, t)
		}
	}
	if collectToolFactory != nil {
		if t := collectToolFactory(j.ID, j.Depth); t != nil {
			extraTools = append(extraTools, t)
		}
	}
	// Wired here rather than cloned in the registry so the tool and the Job
	// share one store: that is what lets Agent View show a background agent's
	// plan while it works, instead of only after it finishes.
	extraTools = append(extraTools, tools.NewTodoWriteTool(j.todos))
	toolReg := m.buildJobToolRegistryForBackend(j.Depth, j.AllowWrite, j.ID, backups, extraTools, runtimeBackend, jobRoot)

	systemPrompt := req.SystemPrompt
	if systemPrompt == "" && systemPromptFor != nil {
		systemPrompt = systemPromptFor(j.Depth)
	}
	if runtimeBackend != nil {
		systemPrompt += fmt.Sprintf(
			"\n\n# Remote Packet Computer Job\nAll workspace file and shell tools target Packet Computer %q. The registered source root is %s; this job's active root is %s. Paths are relative to the active root. Local hooks, code-intelligence tools, MCP tools, and /undo are unavailable. This SSH execution is process-lifetime only: it does not survive PacketCode exit, SSH loss, or restart.",
			j.ComputerName, j.WorkingDir, jobRoot,
		)
	}

	approver := NewJobApprover(parentApprover, j.ID, j.AllowWrite)
	policy := policyForWorkspace(permissionPolicy, workspaceOfJob(j, m.cfg.Root))

	a := agent.New(agent.Config{
		Registry:      subRegistry,
		Tools:         toolReg,
		Session:       subSession,
		CostTracker:   m.cfg.CostTracker,
		Approver:      approver,
		Policy:        policy,
		SystemPrompt:  systemPrompt,
		Hooks:         hookRunner,
		TokenBudget:   m.cfg.TokenBudget,
		SugarCache:    m.cfg.SugarCache,
		ConduitShadow: m.cfg.ConduitShadow,
	})

	events := a.Run(jobCtx, j.Prompt)
	m.consumeEvents(j, jobCtx, events, subSession, runtimeBackend)
}

// consumeEvents drains the agent event channel, updating job
// counters as usage events arrive and recording the final assistant
// text for the summary. On EventDone we mark Completed; on EventError
// we mark Failed; on ctx cancellation we mark whatever classifyCancelled
// decides — Cancelled only when a stop was actually requested, otherwise
// Abandoned.
func (m *Manager) consumeEvents(j *Job, ctx context.Context, events <-chan agent.AgentEvent, sess *session.Manager, runtimeBackend computers.RuntimeBackend) {
	var lastAssistantText strings.Builder
	var inflightAssistant strings.Builder
	var inflightReasoning strings.Builder
	var artifacts []Artifact
	var lastErr error
	var sawDone bool

	for ev := range events {
		switch ev.Type {
		case agent.EventReasoningDelta:
			inflightReasoning.WriteString(ev.Text)
			m.updateActivity(j, "thinking", summarise(inflightReasoning.String()), false, false)
		case agent.EventTextDelta:
			inflightReasoning.Reset()
			inflightAssistant.WriteString(ev.Text)
			m.updateActivity(j, "responding", inflightAssistant.String(), false, false)
		case agent.EventToolCallProposed:
			inflightReasoning.Reset()
			needsApproval := j.AllowWrite
			activity := "tool proposed"
			if needsApproval {
				activity = "needs approval"
			}
			m.updateActivity(j, activity, ev.ToolCall.Name, needsApproval, needsApproval)
		case agent.EventToolCallApproved:
			m.updateActivity(j, "tool approved", ev.ToolCall.Name, false, false)
		case agent.EventToolCallRejected:
			m.updateActivity(j, "tool rejected", ev.Text, false, false)
			artifacts = appendTextArtifact(artifacts, "tool_rejection", ev.ToolCall.Name, ev.Text, ev.ToolCall.Name, true, time.Now().UTC())
		case agent.EventToolCallExecuted:
			// A tool call ends the current "assistant turn"; reset the
			// inflight buffer so we capture only the FINAL assistant
			// text (the one preceding EventDone).
			msg := ev.ToolResult.Content
			if msg == "" {
				msg = ev.ToolCall.Name
			}
			m.updateActivity(j, "tool executed", msg, false, false)
			artifacts = appendToolArtifact(artifacts, ev.ToolCall, ev.ToolResult, time.Now().UTC())
			inflightAssistant.Reset()
		case agent.EventUsageUpdate:
			m.applyUsage(j, ev.Usage)
		case agent.EventDone:
			sawDone = true
			// Flush the assistant text accumulated since the last tool
			// call into the "last assistant text" capture.
			if inflightAssistant.Len() > 0 {
				lastAssistantText.Reset()
				lastAssistantText.WriteString(inflightAssistant.String())
				inflightAssistant.Reset()
			}
		case agent.EventError:
			lastErr = ev.Error
		}
	}

	transcript := snapshotTranscript(sess)
	artifacts = appendWorktreeArtifactsForBackend(ctx, artifacts, j, runtimeBackend)
	// A terminal snapshot is an ownership boundary: publish it only after the
	// job-owned remote transport has been closed. This prevents callers from
	// observing "completed" while its SSH/SFTP connection is still live.
	if runtimeBackend != nil {
		_ = runtimeBackend.Close()
	}

	// Order of precedence for terminal state:
	//   1. ctx cancelled → Cancelled if a stop was actually requested,
	//      otherwise Abandoned (the context died without anyone asking)
	//   2. EventError received → Failed
	//   3. EventDone received → Completed
	//   4. Channel closed without Done → Failed (treat as silent error)
	//
	// Rule 1 previously wrote Cancelled unconditionally and dropped lastErr
	// on the floor, so a dead transport was reported as a confirmed
	// cancellation with no error text at all. Both halves of that are fixed
	// here: the request record decides the state, and any error observed on
	// the way down is preserved as evidence.
	if ctx.Err() != nil {
		state, cause := m.classifyCancelled(j)
		errMsg := ""
		if lastErr != nil {
			errMsg = lastErr.Error()
			artifacts = appendTextArtifact(artifacts, "error", "agent error", errMsg, "", true, time.Now().UTC())
		}
		m.markTerminalCause(j, state, cause, summarise(lastAssistantText.String()), errMsg, "",
			j.InputTokens, j.OutputTokens, j.CostUSD, transcript, artifacts)
		return
	}
	if lastErr != nil {
		artifacts = appendTextArtifact(artifacts, "error", "agent error", lastErr.Error(), "", true, time.Now().UTC())
		m.markTerminal(j, StateFailed, summarise(lastAssistantText.String()), lastErr.Error(), "",
			j.InputTokens, j.OutputTokens, j.CostUSD, transcript, artifacts)
		return
	}
	if sawDone {
		m.markTerminal(j, StateCompleted, summarise(lastAssistantText.String()), "", "",
			j.InputTokens, j.OutputTokens, j.CostUSD, transcript, artifacts)
		return
	}
	artifacts = appendTextArtifact(artifacts, "error", "agent stream closed", "agent stream closed without Done event", "", true, time.Now().UTC())
	m.markTerminal(j, StateFailed, summarise(lastAssistantText.String()),
		"agent stream closed without Done event", "",
		j.InputTokens, j.OutputTokens, j.CostUSD, transcript, artifacts)
}

// classifyCancelled decides what a context cancellation after the job began
// actually means. It reads the request stamped by Cancel/CancelAll/Shutdown
// before the context was cancelled — the only durable evidence that a human
// or an app exit asked for this, since context.Canceled itself is identical
// in every case.
//
// No request on record means nothing asked the job to stop and it stopped
// anyway: the transport died, and packetcode cannot say what happened to the
// work. That is Abandoned. Guessing Cancelled there is the specific
// dishonesty this exists to remove, and the SSH case makes it concrete —
// a detached remote descendant may still be running right now.
func (m *Manager) classifyCancelled(j *Job) (State, AbandonCause) {
	return m.classifyCancelledWithCause(j, AbandonCauseUnknown)
}

// classifyCancelledWithCause is classifyCancelled for callers holding
// independent evidence of *why* the job stopped. The fallback applies only
// when nothing requested the stop; a recorded request always wins, because
// the request is the stronger fact.
//
// The fallback exists so transport-lost is claimed only where a transport
// error was actually observed. Every canceller stamps a request, so without
// that evidence the honest default is Unknown rather than a guess dressed up
// as a diagnosis.
func (m *Manager) classifyCancelledWithCause(j *Job, fallback AbandonCause) (State, AbandonCause) {
	m.mu.RLock()
	req := normalizeCancelRequest(j.CancelRequest)
	m.mu.RUnlock()
	switch req {
	case CancelRequestUser:
		return StateCancelled, ""
	case CancelRequestShutdown:
		// The app exited underneath a running job. It was not resumed and
		// will not be, so the outcome is genuinely unknown.
		return StateAbandoned, AbandonCauseAppExit
	default:
		return StateAbandoned, normalizeAbandonCause(fallback)
	}
}

// applyUsage records a usage delta from a stream completion against the
// job's running totals and the shared cost tracker. The job's per-token
// counts are running highs (we accumulate deltas), since per-stream
// usage in this codebase is typically a per-stream total — see
// agent.run's lastUsage behaviour. We mirror what session.Manager does
// internally.
func (m *Manager) applyUsage(j *Job, usage provider.Usage) {
	m.mu.Lock()
	if j.State.IsTerminal() {
		m.mu.Unlock()
		return
	}
	j.InputTokens += usage.InputTokens
	j.OutputTokens += usage.OutputTokens
	if m.cfg.PricingFor != nil {
		in, out := m.cfg.PricingFor(j.Provider, j.Model)
		j.CostUSD = float64(j.InputTokens)*in/1_000_000 + float64(j.OutputTokens)*out/1_000_000
	}
	m.stampSnapshotLocked(j, time.Now().UTC(), "", "", j.NeedsInput, j.NeedsApproval)
	subs := snapshotCallbacks(m.subscribers)
	snap := snapshotOf(j)
	persisted := toPersisted(j)
	m.mu.Unlock()

	_ = m.savePersistedSnapshot(persisted)
	m.fanOut(snap, subs)
}

// openSubSession creates the per-job session.Manager rooted at
// SessionsDir, deriving its session id from the parent's main id (or
// "main" if there is none) plus the job's short id. The resulting
// Manager has Current() set; callers can pass it straight into
// agent.New.
//
// session.Manager.New() generates a fresh UUID with no public
// override, so we hand-write the initial session file under our
// deterministic id and then Load() it. This keeps the Backup directory
// and cost-tracker key in sync with j.SessionID.
func (m *Manager) openSubSession(j *Job) (*session.Manager, error) {
	if m.cfg.SessionsDir == "" {
		// Tests that don't care about persistence may leave SessionsDir
		// empty — fall back to a brand-new in-memory manager.
		sm := session.NewManager("")
		_, err := sm.New(j.Provider, j.Model)
		return sm, err
	}
	if err := writeInitialSubSession(m.cfg.SessionsDir, j); err != nil {
		return nil, err
	}
	sm := session.NewManager(m.cfg.SessionsDir)
	if _, err := sm.Load(j.SessionID); err != nil {
		return nil, err
	}
	return sm, nil
}

// snapshotTranscript copies the current session's messages into a
// fresh slice so the Job's transcript field shares no mutable state
// with the underlying session.Manager.
func snapshotTranscript(sm *session.Manager) []provider.Message {
	if sm == nil {
		return nil
	}
	cur := sm.Current()
	if cur == nil {
		return nil
	}
	return cloneTranscriptMessages(cur.Messages)
}

// summarise extracts the final user-facing summary from the last
// assistant text. We simply trim and cap at summaryMaxLen, appending an
// ellipsis when truncated.
func summarise(text string) string {
	t := strings.TrimSpace(text)
	if len(t) <= summaryMaxLen {
		return t
	}
	// Trim on a rune boundary by walking back to the previous word
	// break — avoids slicing inside a multibyte sequence and keeps the
	// result readable.
	cut := summaryMaxLen
	for cut > 0 && (t[cut]&0xC0) == 0x80 {
		cut--
	}
	out := strings.TrimRight(t[:cut], " \t\n")
	return out + "…"
}
