package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/permissions"
)

// Workspace is the immutable execution target captured by a job at spawn
// time. An empty ComputerID denotes the local PacketCode workspace.
type Workspace struct {
	ComputerID   string
	ComputerName string
	WorkingDir   string
	Identity     string
	Kind         computers.Kind
	Policy       computers.Policy
}

// WorkspaceResolver maps a user-facing computer name (or a stable id during
// resubmission) to a validated workspace without opening a connection.
type WorkspaceResolver func(selector string) (Workspace, error)

// BackendOpener creates a new independently-owned backend for one remote job.
// The worker closes it on every exit path.
type BackendOpener func(context.Context, Workspace) (computers.RuntimeBackend, error)

func localWorkspace(root string) Workspace {
	return Workspace{WorkingDir: root, Kind: computers.KindLocal}
}

func normalizeWorkspace(ws Workspace, localRoot string) (Workspace, error) {
	ws.ComputerID = strings.TrimSpace(ws.ComputerID)
	ws.ComputerName = strings.TrimSpace(ws.ComputerName)
	ws.WorkingDir = strings.TrimSpace(ws.WorkingDir)
	ws.Identity = strings.TrimSpace(ws.Identity)
	if ws.ComputerID == "" {
		if ws.WorkingDir == "" {
			ws.WorkingDir = localRoot
		}
		ws.ComputerName = ""
		ws.Kind = computers.KindLocal
		return ws, nil
	}
	if ws.ComputerName == "" {
		return Workspace{}, fmt.Errorf("remote workspace %s has no computer name", ws.ComputerID)
	}
	if ws.WorkingDir == "" {
		return Workspace{}, fmt.Errorf("remote workspace %s has no working directory", ws.ComputerID)
	}
	if ws.Identity == "" {
		return Workspace{}, fmt.Errorf("remote workspace %s has no endpoint identity", ws.ComputerID)
	}
	if ws.Kind == "" {
		ws.Kind = computers.KindSSH
	}
	if ws.Kind != computers.KindSSH {
		return Workspace{}, fmt.Errorf("computer %s uses unsupported job backend %s", ws.ComputerName, ws.Kind)
	}
	ws.Policy = normalizeComputerPolicy(ws.Policy)
	return ws, nil
}

func workspaceOfJob(j *Job, localRoot string) Workspace {
	if j == nil {
		return localWorkspace(localRoot)
	}
	ws := Workspace{
		ComputerID:   j.ComputerID,
		ComputerName: j.ComputerName,
		WorkingDir:   j.WorkingDir,
		Identity:     j.WorkspaceIdentity,
		Policy:       j.ComputerPolicy,
	}
	if ws.ComputerID == "" {
		ws.Kind = computers.KindLocal
	} else {
		ws.Kind = computers.KindSSH
	}
	if ws.WorkingDir == "" {
		ws.WorkingDir = localRoot
	}
	return ws
}

func sameWorkspace(a, b Workspace) bool {
	return a.ComputerID == b.ComputerID && a.WorkingDir == b.WorkingDir && a.Identity == b.Identity
}

func (m *Manager) resolveWorkspaceSelector(selector string) (Workspace, *SpawnError) {
	m.mu.RLock()
	resolver := m.cfg.ResolveWorkspace
	localRoot := m.cfg.Root
	m.mu.RUnlock()
	if resolver == nil {
		return Workspace{}, &SpawnError{
			Code:   "workspace_unavailable",
			Reason: fmt.Sprintf("computer %q cannot be resolved", selector),
		}
	}
	ws, err := resolver(selector)
	if err != nil {
		return Workspace{}, &SpawnError{Code: "workspace_unavailable", Reason: err.Error()}
	}
	ws, err = normalizeWorkspace(ws, localRoot)
	if err != nil {
		return Workspace{}, &SpawnError{Code: "workspace_unavailable", Reason: err.Error()}
	}
	return ws, nil
}

// resolveSpawnWorkspace applies explicit top-level selection, nested
// inheritance, then the manager default. A nested explicit selector is only
// accepted when it resolves to the exact workspace already owned by its
// parent; background jobs cannot jump hosts or roots.
func (m *Manager) resolveSpawnWorkspace(req SpawnRequest) (Workspace, *SpawnError) {
	selector := strings.TrimSpace(req.Computer)
	if req.ParentJobID != "" {
		m.mu.RLock()
		parent := m.jobs[req.ParentJobID]
		localRoot := m.cfg.Root
		parentWS := workspaceOfJob(parent, localRoot)
		m.mu.RUnlock()
		if parent == nil {
			return Workspace{}, &SpawnError{
				Code:   "unknown_parent",
				Reason: fmt.Sprintf("parent job %s does not exist", req.ParentJobID),
			}
		}
		if selector == "" {
			return parentWS, nil
		}
		requested, perr := m.resolveWorkspaceSelector(selector)
		if perr != nil {
			return Workspace{}, perr
		}
		if !sameWorkspace(requested, parentWS) {
			return Workspace{}, &SpawnError{
				Code: "cross_target_denied",
				Reason: fmt.Sprintf(
					"background job %s is bound to %s; it cannot spawn on %s",
					req.ParentJobID, workspaceLabel(parentWS), workspaceLabel(requested),
				),
			}
		}
		return parentWS, nil
	}
	if selector != "" {
		return m.resolveWorkspaceSelector(selector)
	}

	m.mu.RLock()
	ws := m.cfg.DefaultWorkspace
	localRoot := m.cfg.Root
	m.mu.RUnlock()
	ws, err := normalizeWorkspace(ws, localRoot)
	if err != nil {
		return Workspace{}, &SpawnError{Code: "workspace_unavailable", Reason: err.Error()}
	}
	return ws, nil
}

func workspaceLabel(ws Workspace) string {
	if ws.ComputerID == "" {
		return "local workspace " + ws.WorkingDir
	}
	return fmt.Sprintf("computer %s (%s) root %s", ws.ComputerName, ws.ComputerID, ws.WorkingDir)
}

func normalizeComputerPolicy(policy computers.Policy) computers.Policy {
	defaults := computers.DefaultPolicy()
	if !computers.ValidPolicyMode(policy.Network) {
		policy.Network = defaults.Network
	}
	if !computers.ValidPolicyMode(policy.Write) {
		policy.Write = defaults.Write
	}
	if !computers.ValidPolicyMode(policy.Shell) {
		policy.Shell = defaults.Shell
	}
	if !computers.ValidPolicyMode(policy.Secrets) {
		policy.Secrets = defaults.Secrets
	}
	if !computers.ValidApprovalMode(policy.Approval) {
		policy.Approval = defaults.Approval
	}
	return policy
}

// policyForWorkspace composes the immutable foreground policy snapshot with
// the remote computer policy. Remote rules only preserve or reduce authority:
// "allow" adds nothing, "ask" forces a prompt, and "deny" is a deny floor.
// ApprovalExplicit forces prompts for both mutation axes even when the stored
// axis says allow. Existing global deny floors continue to win.
func policyForWorkspace(base *permissions.Policy, ws Workspace) *permissions.Policy {
	if ws.ComputerID == "" {
		return base
	}
	policy := normalizeComputerPolicy(ws.Policy)
	out := base
	writeDecision := computerPolicyDecision(policy.Write)
	shellDecision := computerPolicyDecision(policy.Shell)
	if policy.Approval == computers.ApprovalExplicit {
		if writeDecision != permissions.DecisionDeny {
			writeDecision = permissions.DecisionAsk
		}
		if shellDecision != permissions.DecisionDeny {
			shellDecision = permissions.DecisionAsk
		}
	}
	if writeDecision != permissions.DecisionAllow {
		out = out.WithRule("write_file", writeDecision)
		out = out.WithRule("patch_file", writeDecision)
	}
	if shellDecision != permissions.DecisionAllow {
		out = out.WithRule("execute_command", shellDecision)
	}
	return out
}

func computerPolicyDecision(mode computers.PolicyMode) permissions.Decision {
	switch mode {
	case computers.PolicyDeny:
		return permissions.DecisionDeny
	case computers.PolicyAllow:
		return permissions.DecisionAllow
	default:
		return permissions.DecisionAsk
	}
}
