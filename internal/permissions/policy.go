package permissions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/packetcode/packetcode/internal/config"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"

	ActionAllow = DecisionAllow
	ActionAsk   = DecisionAsk
	ActionDeny  = DecisionDeny
)

type Profile string

const (
	ProfileSafe Profile = "safe"
	ProfileAsk  Profile = "ask"
	ProfileEdit Profile = "edit"
	ProfileAuto Profile = "auto"
	ProfileFull Profile = "full"
)

type Rule struct {
	Tool          string
	Decision      Decision
	Reason        string
	Command       string
	CommandPrefix []string
	DenyFloor     bool
}

type Request struct {
	ToolName         string
	RequiresApproval bool
	Params           json.RawMessage
}

type Result struct {
	Decision Decision
	Profile  Profile
	Reason   string
	Rule     *Rule
}

type Policy struct {
	profile Profile
	rules   []Rule
}

func New(cfg config.PermissionConfig) (*Policy, error) {
	profileName := strings.TrimSpace(cfg.Profile)
	if profileName == "" {
		profileName = "balanced"
	}

	profile, rules, err := configProfile(profileName, cfg)
	if err != nil {
		return nil, err
	}
	for _, rule := range cfg.Rules {
		converted := Rule{
			Tool:          strings.TrimSpace(rule.Tool),
			Decision:      NormalizeDecision(Decision(rule.Action)),
			Reason:        strings.TrimSpace(rule.Reason),
			Command:       strings.TrimSpace(rule.Command),
			CommandPrefix: append([]string(nil), rule.CommandPrefix...),
		}
		if converted.Tool == "" {
			return nil, fmt.Errorf("permission rule missing tool")
		}
		if !validDecision(converted.Decision) {
			return nil, fmt.Errorf("permission rule for %s has invalid action %q", converted.Tool, rule.Action)
		}
		converted.DenyFloor = converted.Decision == DecisionDeny
		rules = append(rules, converted)
	}

	// Backward-compatible inline overrides from the early permission
	// config shape: [permissions] default/tools.
	if strings.TrimSpace(cfg.Default) != "" {
		decision := NormalizeDecision(Decision(cfg.Default))
		if !validDecision(decision) {
			return nil, fmt.Errorf("permissions.default has invalid action %q", cfg.Default)
		}
		rules = append(rules, Rule{Tool: "*", Decision: decision, Reason: "inline default"})
	}
	toolKeys := make([]string, 0, len(cfg.Tools))
	for tool := range cfg.Tools {
		toolKeys = append(toolKeys, tool)
	}
	sortRuleKeys(toolKeys)
	for _, tool := range toolKeys {
		action := cfg.Tools[tool]
		decision := NormalizeDecision(Decision(action))
		if !validDecision(decision) {
			return nil, fmt.Errorf("permission rule for %s has invalid action %q", tool, action)
		}
		rules = append(rules, Rule{Tool: tool, Decision: decision, Reason: "inline tool rule", DenyFloor: decision == DecisionDeny})
	}

	return &Policy{profile: profile, rules: rules}, nil
}

func DefaultPolicy() *Policy {
	p, _ := New(config.PermissionConfig{Profile: "balanced"})
	return p
}

func Must(cfg config.PermissionConfig) *Policy {
	p, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return p
}

func (p *Policy) Profile() Profile {
	if p == nil || p.profile == "" {
		return ProfileAsk
	}
	return p.profile
}

func (p *Policy) Rules() []Rule {
	if p == nil || len(p.rules) == 0 {
		return nil
	}
	out := make([]Rule, len(p.rules))
	copy(out, p.rules)
	return out
}

func (p *Policy) Decide(req Request) Result {
	if p == nil {
		p = DefaultPolicy()
	}
	profile := p.Profile()
	if profile == ProfileSafe && !readOnlyTool(req.ToolName) {
		return Result{Decision: DecisionDeny, Profile: profile, Reason: "safe profile denies non-read-only tools"}
	}
	if rule, ok := p.matchingRule(req); ok {
		return Result{
			Decision: rule.Decision,
			Profile:  profile,
			Reason:   firstNonEmpty(rule.Reason, "permission rule matched "+rule.Tool),
			Rule:     &rule,
		}
	}
	decision, reason := profileDecision(profile, req)
	return Result{Decision: decision, Profile: profile, Reason: reason}
}

func (p *Policy) WithProfile(profile Profile) *Policy {
	normalized := NormalizeProfile(profile)
	if err := validateProfile(normalized); err != nil {
		return p
	}
	out := &Policy{profile: normalized}
	if p != nil {
		out.rules = p.Rules()
	}
	return out
}

func (p *Policy) WithRule(tool string, decision Decision) *Policy {
	out := &Policy{profile: ProfileAsk}
	if p != nil {
		out.profile = p.Profile()
		out.rules = p.Rules()
	}
	out.rules = append(out.rules, Rule{
		Tool:      strings.TrimSpace(tool),
		Decision:  NormalizeDecision(decision),
		Reason:    "session rule",
		DenyFloor: NormalizeDecision(decision) == DecisionDeny,
	})
	return out
}

// WithCommandPrefixRule returns a copy with an execute_command prefix rule.
// Prefix rules are intended for explicit configuration, not remembered
// approvals: shell syntax makes inferred command families unsafe.
func (p *Policy) WithCommandPrefixRule(prefix []string, decision Decision) *Policy {
	out := &Policy{profile: ProfileAsk}
	if p != nil {
		out.profile = p.Profile()
		out.rules = p.Rules()
	}
	trimmed := make([]string, 0, len(prefix))
	for _, field := range prefix {
		if field = strings.TrimSpace(field); field != "" {
			trimmed = append(trimmed, field)
		}
	}
	if len(trimmed) == 0 {
		return out
	}
	out.rules = append(out.rules, Rule{
		Tool:          "execute_command",
		CommandPrefix: trimmed,
		Decision:      NormalizeDecision(decision),
		Reason:        "session rule",
		DenyFloor:     NormalizeDecision(decision) == DecisionDeny,
	})
	return out
}

// WithCommandRule returns a copy with an execute_command rule that matches the
// complete shell program byte-for-byte.
func (p *Policy) WithCommandRule(command string, decision Decision) *Policy {
	out := &Policy{profile: ProfileAsk}
	if p != nil {
		out.profile = p.Profile()
		out.rules = p.Rules()
	}
	if command == "" {
		return out
	}
	out.rules = append(out.rules, Rule{
		Tool:      "execute_command",
		Command:   command,
		Decision:  NormalizeDecision(decision),
		Reason:    "session rule",
		DenyFloor: NormalizeDecision(decision) == DecisionDeny,
	})
	return out
}

func (p *Policy) SummaryLines() []string {
	profile := p.Profile()
	lines := []string{
		"profile: " + ProfileConfigName(profile),
		"summary: " + ProfileSummary(profile),
	}
	for _, rule := range p.Rules() {
		detail := fmt.Sprintf("rule: %s -> %s", rule.Tool, rule.Decision)
		if len(rule.CommandPrefix) > 0 {
			detail += " when command starts " + strings.Join(rule.CommandPrefix, " ")
		}
		if rule.Command != "" {
			detail += " when command equals " + rule.Command
		}
		if rule.DenyFloor {
			detail += " (deny floor)"
		}
		lines = append(lines, detail)
	}
	return lines
}

func (p *Policy) matchingRule(req Request) (Rule, bool) {
	for i := len(p.rules) - 1; i >= 0; i-- {
		rule := p.rules[i]
		if rule.DenyFloor && ruleMatchesRequest(rule, req) {
			return rule, true
		}
	}
	for i := len(p.rules) - 1; i >= 0; i-- {
		rule := p.rules[i]
		if ruleMatchesRequest(rule, req) {
			return rule, true
		}
	}
	return Rule{}, false
}

func ruleMatchesRequest(rule Rule, req Request) bool {
	if !toolPatternMatches(rule.Tool, req.ToolName) {
		return false
	}
	if rule.Command != "" && !commandMatches(req.Params, rule.Command) {
		return false
	}
	if len(rule.CommandPrefix) > 0 && !commandPrefixMatches(req.Params, rule.CommandPrefix) {
		return false
	}
	return true
}

func configProfile(name string, cfg config.PermissionConfig) (Profile, []Rule, error) {
	normalized := NormalizeProfile(Profile(name))
	if err := validateProfile(normalized); err == nil {
		return normalized, nil, nil
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		return "", nil, fmt.Errorf("unknown permission profile %q", name)
	}
	base := ProfileAsk
	var rules []Rule
	if raw := strings.TrimSpace(prof["default"]); raw != "" {
		decision := NormalizeDecision(Decision(raw))
		if !validDecision(decision) {
			return "", nil, fmt.Errorf("permissions.profiles.%s.default has invalid action %q", name, raw)
		}
		rules = append(rules, Rule{Tool: "*", Decision: decision, Reason: "profile " + name + " default"})
	}
	toolKeys := make([]string, 0, len(prof))
	for tool := range prof {
		if tool == "default" {
			continue
		}
		toolKeys = append(toolKeys, tool)
	}
	sortRuleKeys(toolKeys)
	for _, tool := range toolKeys {
		raw := prof[tool]
		decision := NormalizeDecision(Decision(raw))
		if !validDecision(decision) {
			return "", nil, fmt.Errorf("permissions.profiles.%s.%s has invalid action %q", name, tool, raw)
		}
		rules = append(rules, Rule{Tool: profileToolPattern(tool), Decision: decision, Reason: "profile " + name, DenyFloor: decision == DecisionDeny})
	}
	return base, rules, nil
}

func profileToolPattern(tool string) string {
	if tool == "mcp" {
		return "mcp:*"
	}
	return tool
}

func sortRuleKeys(keys []string) {
	sort.SliceStable(keys, func(i, j int) bool {
		left := profileToolPattern(keys[i])
		right := profileToolPattern(keys[j])
		if ls, rs := ruleSpecificity(left), ruleSpecificity(right); ls != rs {
			return ls < rs
		}
		return left < right
	})
}

func ruleSpecificity(pattern string) int {
	pattern = strings.TrimSpace(pattern)
	switch {
	case pattern == "" || pattern == "*":
		return 0
	case pattern == "mcp:*":
		return 1
	case strings.HasSuffix(pattern, "*"):
		return 2
	default:
		return 3
	}
}

func profileDecision(profile Profile, req Request) (Decision, string) {
	switch profile {
	case ProfileSafe:
		if readOnlyTool(req.ToolName) {
			return DecisionAllow, "read-only tool"
		}
		return DecisionDeny, "safe profile denies non-read tools"
	case ProfileEdit:
		switch req.ToolName {
		case "write_file", "patch_file":
			return DecisionAllow, "edit profile allows file edits"
		case "execute_command":
			return DecisionAsk, "edit profile prompts for shell commands"
		}
		if readOnlyTool(req.ToolName) {
			return DecisionAllow, "read-only tool"
		}
		if req.RequiresApproval || isMCPTool(req.ToolName) {
			return DecisionAsk, "edit profile prompts for approval-gated tools"
		}
		return DecisionAllow, "tool does not require approval"
	case ProfileAuto:
		switch req.ToolName {
		case "write_file", "patch_file", "execute_command":
			return DecisionAllow, "auto profile allows file edits and shell commands"
		}
		if readOnlyTool(req.ToolName) {
			return DecisionAllow, "read-only tool"
		}
		// Auto still prompts for the unusual, higher-risk surface — MCP
		// tools and anything a tool explicitly flags as approval-gated —
		// so it is a rung below full "bypass".
		if isMCPTool(req.ToolName) {
			return DecisionAsk, "auto profile prompts for MCP tools"
		}
		if req.RequiresApproval {
			return DecisionAsk, "auto profile prompts for approval-gated tools"
		}
		return DecisionAllow, "tool does not require approval"
	case ProfileFull:
		return DecisionAllow, "full profile allows tools unless a deny rule matches"
	case ProfileAsk:
		fallthrough
	default:
		if readOnlyTool(req.ToolName) {
			return DecisionAllow, "read-only tool"
		}
		if req.RequiresApproval || isMCPTool(req.ToolName) {
			return DecisionAsk, "ask profile prompts for approval-gated tools"
		}
		return DecisionAllow, "tool does not require approval"
	}
}

func readOnlyTool(name string) bool {
	switch name {
	case "read_file", "search_codebase", "list_directory", "list_symbols", "find_definition", "find_references", "get_diagnostics", "collect_agent_results":
		return true
	default:
		return false
	}
}

func isMCPTool(name string) bool {
	return strings.Contains(name, "__")
}

func toolPatternMatches(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	switch {
	case pattern == "" || pattern == "*":
		return true
	case pattern == "mcp:*":
		return isMCPTool(name)
	case strings.HasPrefix(pattern, "mcp:"):
		return strings.TrimPrefix(pattern, "mcp:") == name
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	default:
		return pattern == name
	}
}

func commandMatches(params json.RawMessage, want string) bool {
	command, ok := commandParam(params)
	return ok && command == want
}

func commandPrefixMatches(params json.RawMessage, prefix []string) bool {
	if len(prefix) == 0 {
		return true
	}
	command, ok := commandParam(params)
	if !ok {
		return false
	}
	// A prefix rule describes one simple command invocation. Never let it
	// authorize a larger shell program assembled with control operators,
	// redirections, substitutions, or newlines. Exact command rules remain
	// available when a user deliberately approves such a program verbatim.
	if strings.ContainsAny(command, ";&|<>`$()\n\r") {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) < len(prefix) {
		return false
	}
	for i, want := range prefix {
		if fields[i] != want {
			return false
		}
	}
	return true
}

func commandParam(params json.RawMessage) (string, bool) {
	var obj struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(params, &obj); err != nil {
		return "", false
	}
	return obj.Command, strings.TrimSpace(obj.Command) != ""
}

func ParseProfile(raw string) (Profile, error) {
	profile := NormalizeProfile(Profile(raw))
	if err := validateProfile(profile); err != nil {
		return "", err
	}
	return profile, nil
}

func ParseAction(raw string) (Decision, error) {
	decision := NormalizeDecision(Decision(raw))
	if !validDecision(decision) {
		return "", fmt.Errorf("unknown permission action %q", raw)
	}
	return decision, nil
}

func NormalizeProfile(profile Profile) Profile {
	switch strings.ToLower(strings.TrimSpace(string(profile))) {
	case "", "balanced", "ask":
		return ProfileAsk
	case "read_only", "read-only", "readonly", "safe":
		return ProfileSafe
	case "edit", "accept_edits", "accept-edits", "workspace-write", "workspace_write":
		return ProfileEdit
	case "auto", "auto_accept", "auto-accept", "automatic":
		return ProfileAuto
	case "trusted", "trust", "bypass", "full", "bypass_permissions":
		return ProfileFull
	default:
		return Profile(strings.ToLower(strings.TrimSpace(string(profile))))
	}
}

func NormalizeDecision(decision Decision) Decision {
	return Decision(strings.ToLower(strings.TrimSpace(string(decision))))
}

func validateProfile(profile Profile) error {
	switch profile {
	case ProfileSafe, ProfileAsk, ProfileEdit, ProfileAuto, ProfileFull:
		return nil
	default:
		return fmt.Errorf("unknown permission profile %q", profile)
	}
}

func validDecision(decision Decision) bool {
	switch decision {
	case DecisionAllow, DecisionAsk, DecisionDeny:
		return true
	default:
		return false
	}
}

func Profiles() []Profile {
	return []Profile{ProfileSafe, ProfileAsk, ProfileEdit, ProfileAuto, ProfileFull}
}

func ProfileConfigName(profile Profile) string {
	switch NormalizeProfile(profile) {
	case ProfileSafe:
		return "read_only"
	case ProfileAsk:
		return "ask"
	case ProfileEdit:
		return "accept_edits"
	case ProfileAuto:
		return "auto"
	case ProfileFull:
		return "bypass"
	default:
		return string(profile)
	}
}

func ProfileSummary(profile Profile) string {
	switch NormalizeProfile(profile) {
	case ProfileSafe:
		return "read/search/list auto; edits, shell, MCP, and agents denied"
	case ProfileAsk:
		return "read/search/list auto; edits, shell, MCP, and agents prompt"
	case ProfileEdit:
		return "file edits auto; shell, MCP, and agents prompt"
	case ProfileAuto:
		return "file edits and shell auto; MCP and approval-gated tools prompt"
	case ProfileFull:
		return "all tools auto-approve unless an explicit deny rule matches"
	default:
		return "unknown profile"
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
