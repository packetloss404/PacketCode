package workflow

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/packetcode/packetcode/internal/jobs"
)

const maxVerifierEvidenceRunes = 32_000

// renderPrompt renders an AgentSpec prompt through text/template. Templates
// may reference:
//
//	{{.inputs.<name>}}  — a workflow input value
//	{{.steps.<name>}}   — the concatenated summaries of a prior bound step
//	{{.item}}           — the current fan-out item (empty for single steps)
//
// Missing map keys render as the empty string (missingkey=zero) so a prompt
// that references an absent input or step degrades gracefully rather than
// erroring at render time.
func renderPrompt(tmpl string, inputs, steps map[string]string, item string) (string, error) {
	return renderTemplate(tmpl, map[string]any{
		"inputs": nonNil(inputs),
		"steps":  nonNil(steps),
		"item":   item,
	})
}

// renderVerifyPrompt adds the completed work summary and one-based attempt
// number to the ordinary workflow template context.
func renderVerifyPrompt(tmpl string, inputs, steps map[string]string, result string, attempt int) (string, error) {
	return renderTemplate(tmpl, map[string]any{
		"inputs":  nonNil(inputs),
		"steps":   nonNil(steps),
		"result":  result,
		"attempt": attempt,
	})
}

func renderTemplate(tmpl string, data map[string]any) (string, error) {
	t, err := template.New("prompt").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// summariesOf joins the agent summaries of a completed step into a single
// string suitable for exposing to later steps via {{.steps.<name>}}. Empty
// summaries are skipped; a failed agent contributes its error text instead so
// downstream synthesizers still see something.
func summariesOf(sr StepResult) string {
	parts := make([]string, 0, len(sr.Agents))
	for _, a := range sr.Agents {
		if s := strings.TrimSpace(a.Summary); s != "" {
			parts = append(parts, s)
			continue
		}
		if e := strings.TrimSpace(a.Error); e != "" {
			parts = append(parts, "(error) "+e)
		}
	}
	return strings.Join(parts, "\n\n")
}

// verifierEvidenceOf builds bounded, model-facing evidence for a verifier from
// terminal work results. It includes summaries and artifact previews (diffs,
// test commands, and other captured evidence) so verification is not limited
// to the work agent's self-report.
func verifierEvidenceOf(results []jobs.Result) string {
	var b strings.Builder
	for i, result := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Work job %s (%s/%s, state=%s):\n%s", result.JobID, result.Provider, result.Model, result.State, strings.TrimSpace(result.Summary))
		if result.WorktreePath != "" {
			b.WriteString("\nWorktree: " + result.WorktreePath)
		}
		for _, artifact := range result.Artifacts {
			fmt.Fprintf(&b, "\n\nArtifact %s [%s]: %s", artifact.ID, artifact.Kind, strings.TrimSpace(artifact.Summary))
			if preview := strings.TrimSpace(artifact.Preview); preview != "" {
				b.WriteString("\n" + preview)
			}
		}
	}
	runes := []rune(strings.TrimSpace(b.String()))
	if len(runes) <= maxVerifierEvidenceRunes {
		return string(runes)
	}
	return string(runes[:maxVerifierEvidenceRunes]) + "\n\n[verifier evidence truncated by Packetcode]"
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
