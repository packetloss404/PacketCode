package tools

import (
	"path"
	"strings"
)

// IsSecretFilePath reports whether name is a dotenv-style secret file: `.env`
// or `.env.<anything>`, at any depth, with the documented non-secret spellings
// (`.env.example`, `.env.sample`, `.env.template`, `.env.dist`) excepted.
//
// packetcode reads provider API keys from `<project>/.env` and
// `~/.packetcode/.env` itself (see internal/config/dotenv.go), and goes to
// some trouble never to hand those values to a subprocess. The model-facing
// read tools had no such rule: read_file is a read-only tool that no profile
// prompts for, so the model could inline the same file into a transcript that
// is then sent to a provider, persisted under ~/.packetcode/sessions, and
// available to every later tool call. The refusal lives here, in one
// function, so read_file, search_codebase, and @-mentions cannot disagree.
//
// This is a name check, deliberately: it is cheap, it works identically on
// the local and SSH backends, and it needs no secret detection heuristics. It
// is not a sandbox. execute_command can still `cat .env` -- that call is
// approval-gated and the command is shown to the user, which is the boundary
// that applies to everything a shell can reach.
func IsSecretFilePath(name string) bool {
	base := strings.ToLower(path.Base(strings.ReplaceAll(name, `\`, "/")))
	if base == ".env" {
		return true
	}
	if !strings.HasPrefix(base, ".env.") {
		return false
	}
	switch strings.TrimPrefix(base, ".env.") {
	case "example", "sample", "template", "dist":
		return false
	}
	return true
}

// secretFileRefusal is the tool result body for a refused read. It says why,
// because a model that only sees "permission denied" will try the next
// spelling; one that is told the file is a credential store will not.
func secretFileRefusal(tool, name string) string {
	return tool + ": refusing to read " + name + ": dotenv secret files hold provider credentials and are never inlined into model context (packetcode reads them itself for API keys)"
}
