package app

import (
	"context"
	"strings"
	"sync"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/permissions"
)

// uiApprover is the bridge between the agent's blocking Approver call
// and the TUI's event-loop-driven approval prompt.
//
// Mechanism: Approve() enqueues the request and blocks on a per-request
// response channel. The App's tea.Update() calls Next(), raises the approval
// modal for the envelope it returns, waits for the user to hit y/n, and
// resolves that envelope by id. Background agents can ask for approvals
// concurrently, so decisions must never share one global response channel and
// must never be applied to "whatever is showing".
//
// The queue is a slice rather than a channel because three things have to be
// possible that a channel cannot do: pull the highest-priority request rather
// than the oldest, remove a request whose context died before anyone answered
// it, and count what is still live.
type uiApprover struct {
	mu        sync.Mutex
	autoTrust bool // when true, every Approve returns Approved without prompting
	policy    *permissions.Policy
	notify    func()
	nextID    uint64
	nextSeq   uint64
	active    *approvalEnvelope
	queue     []*approvalEnvelope
}

// approvalOrigin records who is blocked on a request. It is a field rather
// than something recovered from the "[job:<id>] " display prefix: that prefix
// is a label for humans, and deriving authority-relevant behaviour from a
// display string means any change to the label silently reclassifies a
// background job's approval as the user's own turn.
type approvalOrigin int

const (
	// originForeground is the zero value on purpose. An approval whose origin
	// is unknown is treated as the user's own turn, which is the direction
	// that errs towards rejecting rather than towards leaving it standing.
	originForeground approvalOrigin = iota
	originBackground
)

type approvalEnvelope struct {
	id  uint64
	seq uint64 // arrival order; ties are broken by it so ordering is deterministic
	ctx context.Context
	req agent.ApprovalRequest
	// result is buffered so a delivery never blocks on a caller that has
	// already given up on its context.
	result      chan agent.ApprovalDecision
	policyBound bool
	origin      approvalOrigin
	jobID       string
}

func (e *approvalEnvelope) live() bool { return e != nil && e.ctx.Err() == nil }

// pendingApproval is what the App needs in order to display one request and
// later resolve it. The id travels with the request so the decision the user
// makes can be matched back to the envelope they were actually looking at.
type pendingApproval struct {
	id     uint64
	req    agent.ApprovalRequest
	origin approvalOrigin
	jobID  string
}

func newUIApprover() *uiApprover {
	return &uiApprover{policy: permissions.DefaultPolicy()}
}

func (u *uiApprover) Approve(ctx context.Context, req agent.ApprovalRequest) agent.ApprovalDecision {
	return u.decideOrPrompt(ctx, req, true)
}

// PromptApproval queues a request that a caller has already classified as
// "ask". This is used by background agents whose permission policy is a
// launch-time snapshot: a later foreground policy change must not silently
// broaden the running job's authority.
func (u *uiApprover) PromptApproval(ctx context.Context, req agent.ApprovalRequest) agent.ApprovalDecision {
	return u.prompt(ctx, req, false, originBackground, "")
}

// PromptJobApproval is PromptApproval with the originating job recorded.
// jobs.NewJobApprover prefers this method so origin reaches the queue as data
// instead of being parsed back out of the prompt's title.
func (u *uiApprover) PromptJobApproval(ctx context.Context, jobID string, req agent.ApprovalRequest) agent.ApprovalDecision {
	return u.prompt(ctx, req, false, originBackground, jobID)
}

func (u *uiApprover) policyDecision(req agent.ApprovalRequest, requiresApproval bool) permissions.Result {
	u.mu.Lock()
	policy := u.policy
	u.mu.Unlock()
	if policy == nil {
		policy = permissions.DefaultPolicy()
	}
	name := ""
	if req.Tool != nil {
		name = req.Tool.Name()
	}
	if name == "" {
		name = stripJobApprovalPrefix(req.ToolCall.Name)
	}
	return policy.Decide(permissions.Request{
		ToolName:         name,
		RequiresApproval: requiresApproval,
		Params:           req.Params,
	})
}

func (u *uiApprover) decideOrPrompt(ctx context.Context, req agent.ApprovalRequest, requiresApproval bool) agent.ApprovalDecision {
	decision := u.policyDecision(req, requiresApproval)
	switch decision.Decision {
	case permissions.DecisionDeny:
		return agent.ApprovalDecision{
			Approved: false,
			Reason:   "permission policy denied " + req.ToolCall.Name + " (" + decision.Reason + ")",
		}
	case permissions.DecisionAllow:
		return agent.ApprovalDecision{Approved: true, EditedParams: req.Params}
	}
	u.mu.Lock()
	trusted := u.autoTrust
	u.mu.Unlock()
	if trusted {
		return agent.ApprovalDecision{Approved: true, EditedParams: req.Params}
	}
	return u.prompt(ctx, req, true, originForeground, "")
}

func (u *uiApprover) prompt(ctx context.Context, req agent.ApprovalRequest, policyBound bool, origin approvalOrigin, jobID string) agent.ApprovalDecision {
	if ctx.Err() != nil {
		return agent.ApprovalDecision{Approved: false, Reason: "cancelled"}
	}
	u.mu.Lock()
	u.nextID++
	u.nextSeq++
	env := &approvalEnvelope{
		id:          u.nextID,
		seq:         u.nextSeq,
		ctx:         ctx,
		req:         req,
		result:      make(chan agent.ApprovalDecision, 1),
		policyBound: policyBound,
		origin:      origin,
		jobID:       jobID,
	}
	u.queue = append(u.queue, env)
	u.mu.Unlock()
	u.notifyPending()

	select {
	case dec := <-env.result:
		return dec
	case <-ctx.Done():
		// The App may be showing this envelope right now. Drop it and say so:
		// without the notify, a cancelled background job leaves its modal on
		// screen forever, blocking the foreground input and every other job's
		// request behind a prompt nobody is waiting for.
		u.drop(env.id)
		u.notifyPending()
		return agent.ApprovalDecision{Approved: false, Reason: "cancelled"}
	}
}

// drop removes an envelope from wherever it is. Used when its context dies.
func (u *uiApprover) drop(id uint64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.active != nil && u.active.id == id {
		u.active = nil
	}
	for i, env := range u.queue {
		if env.id == id {
			u.queue = append(u.queue[:i], u.queue[i+1:]...)
			return
		}
	}
}

// Next returns the request the App should display, or (zero, false) when the
// one already on screen is still live and nothing should replace it.
//
// Nothing ever displaces a live displayed envelope. That is the guarantee
// behind "a decision cannot land on a different request than the one shown":
// the displayed envelope can only leave the slot by being resolved or by
// dying, and either way its id stops matching.
func (u *uiApprover) Next() (pendingApproval, bool) {
	var (
		out      pendingApproval
		found    bool
		resolved []resolvedEnvelope
	)
	u.mu.Lock()
	if u.active.live() {
		u.mu.Unlock()
		return pendingApproval{}, false
	}
	u.active = nil
	u.pruneLocked()
	for {
		env := u.popLocked()
		if env == nil {
			break
		}
		// A queued request the live policy can settle is settled without ever
		// being shown: an allow that arrived while it waited, or a deny that
		// revokes it.
		if decision, done := envelopePolicyDecision(u.policy, env); done {
			resolved = append(resolved, resolvedEnvelope{env: env, decision: decision})
			continue
		}
		u.active = env
		out = pendingApproval{id: env.id, req: env.req, origin: env.origin, jobID: env.jobID}
		found = true
		break
	}
	u.mu.Unlock()
	for _, r := range resolved {
		deliverApprovalDecision(r.env, r.decision)
	}
	return out, found
}

type resolvedEnvelope struct {
	env      *approvalEnvelope
	decision agent.ApprovalDecision
}

// pruneLocked forgets queued envelopes whose caller has already given up.
// Caller must hold u.mu.
func (u *uiApprover) pruneLocked() {
	kept := u.queue[:0]
	for _, env := range u.queue {
		if env.live() {
			kept = append(kept, env)
		}
	}
	for i := len(kept); i < len(u.queue); i++ {
		u.queue[i] = nil
	}
	u.queue = kept
}

// popLocked removes and returns the highest-priority live envelope.
//
// Ordering is (origin, arrival). A foreground approval is the thing standing
// between the user and their own turn finishing, so it outranks a background
// job's — the job keeps running asynchronously either way, and showing it
// first would make the user's own work wait on someone else's. Within one
// origin the order is arrival order, so the queue is fair and deterministic
// rather than dependent on map or scheduler ordering.
func (u *uiApprover) popLocked() *approvalEnvelope {
	best := -1
	for i, env := range u.queue {
		if !env.live() {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		cur := u.queue[best]
		if env.origin < cur.origin || (env.origin == cur.origin && env.seq < cur.seq) {
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	env := u.queue[best]
	u.queue = append(u.queue[:best], u.queue[best+1:]...)
	return env
}

// Pending is Next without the identity. It exists for callers that only want
// to know what would be displayed; anything that will later resolve the
// request must use Next and keep the id.
func (u *uiApprover) Pending() (agent.ApprovalRequest, bool) {
	next, ok := u.Next()
	return next.req, ok
}

// IsLive reports whether id is still the displayed envelope and its caller is
// still waiting. The App polls this to notice an approval that died while it
// was on screen.
func (u *uiApprover) IsLive(id uint64) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return id != 0 && u.active != nil && u.active.id == id && u.active.live()
}

func (u *uiApprover) SetNotify(fn func()) {
	u.mu.Lock()
	u.notify = fn
	u.mu.Unlock()
}

func (u *uiApprover) notifyPending() {
	u.mu.Lock()
	fn := u.notify
	u.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// ResolveActiveByPolicy re-evaluates the displayed approval after a live
// permission-mode change. It returns true when the new policy made a terminal
// allow/deny decision and that approval was resolved. Ask decisions remain
// visible for explicit user input.
//
// It takes the id the caller believes is displayed so a mode change cannot
// settle an envelope that replaced the one the user was looking at.
func (u *uiApprover) ResolveActiveByPolicy(id uint64) bool {
	u.mu.Lock()
	if id == 0 || u.active == nil || u.active.id != id || !u.active.live() {
		u.mu.Unlock()
		return false
	}
	env := u.active
	decision, resolved := envelopePolicyDecision(u.policy, env)
	if !resolved {
		u.mu.Unlock()
		return false
	}
	u.active = nil
	u.mu.Unlock()
	deliverApprovalDecision(env, decision)
	return true
}

func envelopePolicyDecision(policy *permissions.Policy, env *approvalEnvelope) (agent.ApprovalDecision, bool) {
	decision, resolved := policyApprovalDecision(policy, env.req)
	if env.policyBound || !resolved || !decision.Approved {
		return decision, resolved
	}
	// A snapshot-bound background "ask" remains an explicit prompt even if
	// the foreground policy later broadens. A later deny still revokes it.
	return agent.ApprovalDecision{}, false
}

func policyApprovalDecision(policy *permissions.Policy, req agent.ApprovalRequest) (agent.ApprovalDecision, bool) {
	if policy == nil {
		policy = permissions.DefaultPolicy()
	}
	toolName := ""
	requiresApproval := true
	if req.Tool != nil {
		toolName = req.Tool.Name()
		requiresApproval = req.Tool.RequiresApproval()
	}
	if toolName == "" {
		toolName = stripJobApprovalPrefix(req.ToolCall.Name)
	}
	result := policy.Decide(permissions.Request{
		ToolName:         toolName,
		RequiresApproval: requiresApproval,
		Params:           req.Params,
	})
	switch result.Decision {
	case permissions.DecisionAllow:
		return agent.ApprovalDecision{Approved: true, EditedParams: req.Params}, true
	case permissions.DecisionDeny:
		return agent.ApprovalDecision{
			Approved: false,
			Reason:   "permission policy denied " + req.ToolCall.Name + " (" + result.Reason + ")",
		}, true
	default:
		return agent.ApprovalDecision{}, false
	}
}

func deliverApprovalDecision(env *approvalEnvelope, decision agent.ApprovalDecision) {
	select {
	case env.result <- decision:
	default:
	}
}

// QueueDepth counts the requests a user still has to answer, including the
// one on screen. Dead envelopes are excluded: a count that included them
// would tell the user there is more waiting than there is.
func (u *uiApprover) QueueDepth() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	depth := 0
	if u.active.live() {
		depth++
	}
	for _, env := range u.queue {
		if env.live() {
			depth++
		}
	}
	if depth < 1 {
		return 1
	}
	return depth
}

// ResolveID posts a decision to the envelope it was made about. It reports
// false when id is not the displayed envelope — the request was cancelled, or
// resolved by a policy change, and something else may now be on screen.
//
// Callers must honour that false: it is what stops a "yes, and don't ask
// again" for a read_file from installing a standing rule for whatever
// replaced it.
func (u *uiApprover) ResolveID(id uint64, decision agent.ApprovalDecision) bool {
	u.mu.Lock()
	if id == 0 || u.active == nil || u.active.id != id {
		u.mu.Unlock()
		return false
	}
	env := u.active
	u.active = nil
	u.mu.Unlock()
	if !env.live() {
		// The caller stopped waiting. Deliver nothing and, crucially, report
		// false so no session-wide rule is remembered on its behalf.
		return false
	}
	deliverApprovalDecision(env, decision)
	return true
}

// SetTrust toggles trust mode. When enabled, future Approve() calls
// return immediately without raising the modal.
func (u *uiApprover) SetTrust(trust bool) {
	u.mu.Lock()
	u.autoTrust = trust
	u.mu.Unlock()
}

// IsTrusted reports trust mode.
func (u *uiApprover) IsTrusted() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.autoTrust || (u.policy != nil && u.policy.Profile() == permissions.ProfileFull)
}

func (u *uiApprover) SetPermissionPolicy(policy *permissions.Policy) {
	u.mu.Lock()
	u.policy = policy
	u.mu.Unlock()
}

func (u *uiApprover) PermissionPolicy() *permissions.Policy {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.policy == nil {
		return permissions.DefaultPolicy()
	}
	return u.policy
}

func stripJobApprovalPrefix(name string) string {
	if strings.HasPrefix(name, "[job:") {
		if _, rest, ok := strings.Cut(name, "] "); ok {
			return rest
		}
	}
	return name
}
