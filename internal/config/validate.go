package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

// ValidationProblems reports settings that will fail, or silently do nothing,
// once the program is running -- and names the setting, and where one is
// missing, the environment variable to set.
//
// This exists for the operator who is not watching the terminal: a missing
// env_from variable, an api_key_env that names an unset variable, a custom
// provider with an unusable base_url, or an enabled MCP server with no
// command are each reported today, but each one somewhere else and some of
// them only when the feature is first used. One list, produced at boot from
// the loaded config, is what makes "why does this option do nothing" a
// question answered by the first lines of stderr rather than by a debugging
// session.
//
// Reported, never fatal, for the same reason CompatProblems is: this is a
// file a person typed, and refusing to start over a warning is the worse
// failure. Anything that IS fatal (an invalid permission policy, an
// out-of-range Sugar/Conduit value) still fails where it always did.
func (c *Config) ValidationProblems() []string {
	if c == nil {
		return nil
	}
	var problems []string

	names := make([]string, 0, len(c.MCP))
	for name := range c.MCP {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := c.MCP[name]
		if _, err := MCPLogFileName(name); err != nil {
			problems = append(problems, fmt.Sprintf("[mcp.%s]: %v", name, err))
			continue
		}
		if !entry.IsEnabled() {
			continue
		}
		if strings.TrimSpace(entry.Command) == "" {
			problems = append(problems, fmt.Sprintf("[mcp.%s] is enabled but has no command; set command, or enabled = false", name))
		}
		if entry.TimeoutSec < 0 {
			problems = append(problems, fmt.Sprintf("[mcp.%s] timeout_sec %d is negative; the default of 10 is used", name, entry.TimeoutSec))
		}
		for _, variable := range entry.EnvFrom {
			variable = strings.TrimSpace(variable)
			if variable == "" {
				continue
			}
			if _, ok := os.LookupEnv(variable); !ok {
				problems = append(problems, fmt.Sprintf("[mcp.%s] env_from names %s, which is not set in this environment; the server starts without it", name, variable))
			}
		}
	}

	slugs := make([]string, 0, len(c.Providers))
	for slug := range c.Providers {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		pc := c.Providers[slug]
		if pc.IsOpenAICompatible() {
			if err := validateProviderBaseURL(pc.BaseURL); err != nil {
				problems = append(problems, fmt.Sprintf("[providers.%s] %v", slug, err))
			}
		}
		if env := strings.TrimSpace(pc.APIKeyEnv); env != "" && c.ProviderRequiresAPIKey(slug) && c.GetProviderKey(slug) == "" {
			problems = append(problems, fmt.Sprintf("[providers.%s] api_key_env names %s, which is not set, and no api_key is configured; set %s or api_key", slug, env, env))
		}
	}

	if active := strings.TrimSpace(c.Default.Provider); active != "" {
		_, configured := c.Providers[active]
		known := configured || isReservedHostedProvider(active) || IsKeylessProvider(active)
		switch {
		case !known:
			problems = append(problems, fmt.Sprintf("[default] provider %q is not a built-in provider and has no [providers.%s] table", active, active))
		case c.ProviderRequiresAPIKey(active) && c.GetProviderKey(active) == "":
			problems = append(problems, fmt.Sprintf("[default] provider %s has no API key: set %s (environment or .env) or api_key under [providers.%s]", active, c.ProviderAPIKeyEnvName(active), active))
		}
	}

	b := c.Behavior
	if b.AutoCompactThreshold < 0 || b.AutoCompactThreshold > 100 {
		problems = append(problems, fmt.Sprintf("[behavior] auto_compact_threshold %d is outside 0..100; the default of 80 is used", b.AutoCompactThreshold))
	}
	if b.ProviderMaxRetries < 0 {
		problems = append(problems, fmt.Sprintf("[behavior] provider_max_retries %d is negative; the default of 3 is used", b.ProviderMaxRetries))
	}
	if b.ProviderStallTimeout < 0 {
		problems = append(problems, fmt.Sprintf("[behavior] provider_stall_timeout %d is negative; the default of 60 is used", b.ProviderStallTimeout))
	}
	if b.BackupRetentionDays < 0 {
		problems = append(problems, fmt.Sprintf("[behavior] backup_retention_days %d is negative; the default of 14 is used. Set backup_prune_disabled to keep backups forever", b.BackupRetentionDays))
	}
	for _, cap := range []struct {
		name  string
		value int
	}{
		{"background_max_concurrent", b.BackgroundMaxConcurrent},
		{"background_max_depth", b.BackgroundMaxDepth},
		{"background_max_total", b.BackgroundMaxTotal},
		{"background_token_budget", b.BackgroundTokenBudget},
		{"workflow_token_budget", b.WorkflowTokenBudget},
	} {
		if cap.value < 0 {
			problems = append(problems, fmt.Sprintf("[behavior] %s %d is negative; the default is used", cap.name, cap.value))
		}
	}

	if c.StatusLine.IsEnabled() && c.StatusLine.TimeoutSec < 0 {
		problems = append(problems, fmt.Sprintf("[statusline] timeout_sec %d is negative; the default of 2 is used", c.StatusLine.TimeoutSec))
	}
	for _, group := range []struct {
		name  string
		hooks []HookConfig
	}{
		{"user_prompt_submit", c.Hooks.UserPromptSubmit},
		{"pre_tool_use", c.Hooks.PreToolUse},
		{"post_tool_use", c.Hooks.PostToolUse},
	} {
		for i, hook := range group.hooks {
			if strings.TrimSpace(hook.Command) == "" && (hook.Enabled == nil || *hook.Enabled) {
				problems = append(problems, fmt.Sprintf("[[hooks.%s]] entry %d has no command and does nothing", group.name, i+1))
			}
			if hook.TimeoutSec < 0 {
				problems = append(problems, fmt.Sprintf("[[hooks.%s]] entry %d timeout_sec %d is negative; the default of 10 is used", group.name, i+1, hook.TimeoutSec))
			}
		}
	}
	return problems
}

// validateProviderBaseURL mirrors the check custom providers apply when they
// are constructed, so a bad base_url is named at boot rather than on the
// first request.
func validateProviderBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("base_url is required for an openai_compatible provider")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("base_url is invalid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base_url must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("base_url must include a host")
	}
	return nil
}
