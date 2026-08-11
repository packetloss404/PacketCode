// Package handoff defines local-only, bounded specialist handoff artifacts.
// Capsules are session metadata: they must never be embedded in Conduit
// runtime telemetry or a provider request without an explicit local handoff.
package handoff

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	SchemaVersion   = 1
	DefaultMaxBytes = 8 * 1024
)

// SpecialistCapsule is intentionally a closed local-storage schema. It has no
// transcript, tool arguments, environment, or absolute-path field.
type SpecialistCapsule struct {
	SchemaVersion       int          `json:"schema_version"`
	Generation          int          `json:"generation"`
	Intent              string       `json:"intent,omitempty"`
	Constraints         []string     `json:"constraints,omitempty"`
	ChangeBuckets       []string     `json:"change_buckets,omitempty"`
	Changes             []Change     `json:"changes,omitempty"`
	FailedGates         []FailedGate `json:"failed_gates,omitempty"`
	ChangedAPIsSchemas  []string     `json:"changed_apis_schemas,omitempty"`
	UnresolvedDecisions []string     `json:"unresolved_decisions,omitempty"`
	Evidence            []Evidence   `json:"evidence,omitempty"`
}

type Change struct {
	Path         string `json:"path,omitempty"`
	Summary      string `json:"summary"`
	PatchExcerpt string `json:"patch_excerpt,omitempty"`
}

type FailedGate struct {
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Fingerprint string `json:"fingerprint"`
	Excerpt     string `json:"excerpt,omitempty"`
}

type Evidence struct {
	Kind        string `json:"kind"`
	Command     string `json:"command,omitempty"`
	Result      string `json:"result"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Normalize returns a deterministic, secret-redacted capsule whose JSON form
// is no larger than maxBytes. Lower-priority evidence is removed before task
// intent, constraints, or failed-gate identity.
func Normalize(in SpecialistCapsule, maxBytes int) SpecialistCapsule {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	in.SchemaVersion = SchemaVersion
	if in.Generation < 0 {
		in.Generation = 0
	}
	in.Intent = safeText(in.Intent, 1024)
	in.Constraints = normalizedStrings(in.Constraints, 12, 320)
	in.ChangeBuckets = normalizedStrings(in.ChangeBuckets, 12, 64)
	in.ChangedAPIsSchemas = normalizedStrings(in.ChangedAPIsSchemas, 12, 320)
	in.UnresolvedDecisions = normalizedStrings(in.UnresolvedDecisions, 12, 320)
	for i := range in.Changes {
		in.Changes[i].Path = relativePath(in.Changes[i].Path)
		in.Changes[i].Summary = safeText(in.Changes[i].Summary, 320)
		in.Changes[i].PatchExcerpt = safeText(in.Changes[i].PatchExcerpt, 768)
	}
	in.Changes = compactChanges(in.Changes, 16)
	for i := range in.FailedGates {
		in.FailedGates[i].Name = safeText(in.FailedGates[i].Name, 96)
		in.FailedGates[i].Summary = safeText(in.FailedGates[i].Summary, 320)
		in.FailedGates[i].Fingerprint = fingerprintOnly(in.FailedGates[i].Fingerprint)
		in.FailedGates[i].Excerpt = safeText(in.FailedGates[i].Excerpt, 768)
	}
	if len(in.FailedGates) > 12 {
		in.FailedGates = in.FailedGates[:12]
	}
	for i := range in.Evidence {
		in.Evidence[i].Kind = safeText(in.Evidence[i].Kind, 64)
		in.Evidence[i].Command = safeText(in.Evidence[i].Command, 320)
		in.Evidence[i].Result = safeText(in.Evidence[i].Result, 320)
		in.Evidence[i].Fingerprint = fingerprintOnly(in.Evidence[i].Fingerprint)
	}
	if len(in.Evidence) > 20 {
		in.Evidence = in.Evidence[:20]
	}

	// Deterministic priority: evidence, change excerpts, changes, API notes,
	// unresolved decisions, and constraints are trimmed in that order.
	for jsonSize(in) > maxBytes && len(in.Evidence) > 0 {
		in.Evidence = in.Evidence[:len(in.Evidence)-1]
	}
	for jsonSize(in) > maxBytes && hasChangeExcerpt(in.Changes) {
		for i := len(in.Changes) - 1; i >= 0; i-- {
			if in.Changes[i].PatchExcerpt != "" {
				in.Changes[i].PatchExcerpt = ""
				break
			}
		}
	}
	for jsonSize(in) > maxBytes && len(in.Changes) > 0 {
		in.Changes = in.Changes[:len(in.Changes)-1]
	}
	for jsonSize(in) > maxBytes && len(in.ChangedAPIsSchemas) > 0 {
		in.ChangedAPIsSchemas = in.ChangedAPIsSchemas[:len(in.ChangedAPIsSchemas)-1]
	}
	for jsonSize(in) > maxBytes && len(in.UnresolvedDecisions) > 0 {
		in.UnresolvedDecisions = in.UnresolvedDecisions[:len(in.UnresolvedDecisions)-1]
	}
	for jsonSize(in) > maxBytes && len(in.Constraints) > 0 {
		in.Constraints = in.Constraints[:len(in.Constraints)-1]
	}
	for jsonSize(in) > maxBytes && len(in.FailedGates) > 0 {
		last := len(in.FailedGates) - 1
		if in.FailedGates[last].Excerpt != "" {
			in.FailedGates[last].Excerpt = ""
		} else {
			in.FailedGates = in.FailedGates[:last]
		}
	}
	for jsonSize(in) > maxBytes && len(in.Intent) > 64 {
		in.Intent = truncateUTF8(in.Intent, len(in.Intent)*3/4)
	}
	return in
}

func ExtractConstraints(intent string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(intent, func(r rune) bool { return r == '\n' || r == '.' || r == ';' }) {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "must") || strings.Contains(lower, "never") || strings.Contains(lower, "do not") || strings.Contains(lower, "only") || strings.Contains(lower, "without") {
			out = append(out, line)
		}
	}
	return normalizedStrings(out, 12, 320)
}

func jsonSize(value SpecialistCapsule) int { b, _ := json.Marshal(value); return len(b) }

func normalizedStrings(values []string, maxItems, maxLen int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = safeText(value, maxLen)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) == maxItems {
			break
		}
	}
	return out
}

func compactChanges(values []Change, maxItems int) []Change {
	out := make([]Change, 0, len(values))
	for _, value := range values {
		if value.Path == "" && value.Summary == "" {
			continue
		}
		out = append(out, value)
		if len(out) == maxItems {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

var (
	secretLine       = regexp.MustCompile(`(?i)(authorization|api[_ -]?key|password|passwd|secret|token|sgr_live_|sk-[a-z0-9])`)
	highEntropyToken = regexp.MustCompile(`[A-Za-z0-9_+/=-]{32,}`)
	windowsAbsPath   = regexp.MustCompile(`(?i)[a-z]:[\\/][^\s"']+`)
	unixAbsPath      = regexp.MustCompile(`(^|\s)/[^\s"']+`)
)

func safeText(value string, max int) string {
	value = strings.ReplaceAll(value, "\x00", "")
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i, line := range lines {
		if secretLine.MatchString(line) {
			lines[i] = "[redacted secret-bearing line]"
			continue
		}
		line = highEntropyToken.ReplaceAllString(line, "[redacted high-entropy token]")
		line = windowsAbsPath.ReplaceAllString(line, "[redacted absolute path]")
		line = unixAbsPath.ReplaceAllString(line, "$1[redacted absolute path]")
		lines[i] = line
	}
	value = strings.Join(lines, "\n")
	return truncateUTF8(value, max)
}

func relativePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || filepath.IsAbs(value) || strings.HasPrefix(value, "/") {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return safeText(clean, 240)
}

func fingerprintOnly(value string) string {
	if len(value) == 71 && strings.HasPrefix(value, "sha256:") {
		for _, r := range value[7:] {
			if !strings.ContainsRune("0123456789abcdef", r) {
				return ""
			}
		}
		return value
	}
	return ""
}

func truncateUTF8(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	value = value[:max]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value) + "…"
}

func hasChangeExcerpt(changes []Change) bool {
	for _, change := range changes {
		if change.PatchExcerpt != "" {
			return true
		}
	}
	return false
}
