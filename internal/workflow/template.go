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

// verifierWorkspace is what a verifier will be able to look at for itself:
// the work job whose worktree becomes its root (empty when there is none),
// and the prompt text that tells it which of the two it got. The pairing is
// deliberate — a verifier told it can read the change when it cannot would
// report "no problems found" from an empty search.
type verifierWorkspace struct {
	WorktreeJobID string
	Framing       string
}

// workspaceForVerifier picks the worktree a verifier should be rooted in.
//
// One worktree is the case worth wiring: the verifier reads the candidate
// change directly. A read-only work step has none, and it needs none — its
// agent read the same project tree the verifier will. A parallel step
// produces one worktree per agent, and packetcode never merges them, so
// there is no single tree that represents the step's work; rooting at one of
// several would present part of the change as all of it.
func workspaceForVerifier(results []jobs.Result) verifierWorkspace {
	worktrees := make([]jobs.Result, 0, len(results))
	for _, result := range results {
		if strings.TrimSpace(result.WorktreePath) != "" {
			worktrees = append(worktrees, result)
		}
	}
	switch len(worktrees) {
	case 0:
		return verifierWorkspace{}
	case 1:
		return verifierWorkspace{
			WorktreeJobID: worktrees[0].JobID,
			Framing: fmt.Sprintf(
				"Your workspace root is the isolated git worktree background job %s wrote in, not the user's project tree. "+
					"The files you read there are the candidate change itself, so check the work against them (read the changed files, search for what the summary claims) rather than against the summary above. "+
					"Your tools are read-only and you cannot run commands: report anything you could not check as unchecked instead of assuming it passed.\n\n",
				worktrees[0].JobID,
			),
		}
	default:
		return verifierWorkspace{
			Framing: fmt.Sprintf(
				"This step ran %d write agents, each in its own isolated worktree, and packetcode does not merge them. "+
					"Your workspace root is therefore the user's project tree, which does not contain any of their changes: you cannot read this work, only the evidence quoted above. "+
					"Judge on that evidence alone and fail if it is not sufficient.\n\n",
				len(worktrees),
			),
		}
	}
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
