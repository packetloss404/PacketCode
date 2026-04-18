package app

import (
	"errors"
	"strings"
)

// ParseSlashCommand inspects a raw input line and, if it starts with "/",
// returns the command name (without the leading slash) and its
// whitespace-split arguments. ok=false means the input is not a slash
// command and should be treated as a normal user prompt.
//
// Recognised commands: "/spawn", "/jobs", "/cancel". Anything else returns
// ok=false so the caller can decide whether to surface an "unknown
// command" system message or just ignore.
func ParseSlashCommand(text string) (cmd string, args []string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return "", nil, false
	}
	body := strings.TrimPrefix(trimmed, "/")
	if body == "" {
		return "", nil, false
	}
	fields := strings.Fields(body)
	name := fields[0]
	switch name {
	case "spawn", "jobs", "cancel":
		return name, fields[1:], true
	}
	return "", nil, false
}

// ParseSpawnFlags handles the argument tail of a /spawn command. Accepted
// flags (in any order, preceding the prompt):
//   --provider <slug>
//   --model    <id>
//   --write
//
// After the last flag value, every remaining token (joined by single
// spaces) forms the prompt. An empty prompt is a user error.
func ParseSpawnFlags(args []string) (provSlug, modelID string, allowWrite bool, prompt string, err error) {
	var rest []string
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--provider":
			if i+1 >= len(args) {
				return "", "", false, "", errors.New("--provider requires a value")
			}
			provSlug = args[i+1]
			i += 2
		case "--model":
			if i+1 >= len(args) {
				return "", "", false, "", errors.New("--model requires a value")
			}
			modelID = args[i+1]
			i += 2
		case "--write":
			allowWrite = true
			i++
		default:
			// First non-flag token starts the prompt. Everything after
			// it is also part of the prompt, even if it looks like a
			// flag — matches the spec's "flag-then-prompt" simplicity.
			rest = args[i:]
			i = len(args)
		}
	}
	prompt = strings.TrimSpace(strings.Join(rest, " "))
	if prompt == "" {
		return "", "", false, "", errors.New("prompt is required")
	}
	return provSlug, modelID, allowWrite, prompt, nil
}
