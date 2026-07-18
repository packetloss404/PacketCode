package workflow

import (
	"bytes"
	"strings"
	"text/template"
)

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
	t, err := template.New("prompt").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", err
	}
	data := map[string]any{
		"inputs": nonNil(inputs),
		"steps":  nonNil(steps),
		"item":   item,
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

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
