package workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	verdictOpen  = "<packetcode-workflow-verdict>"
	verdictClose = "</packetcode-workflow-verdict>"
)

type verdictPayload struct {
	Version int    `json:"version"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
}

// ParseVerdict extracts and validates the last structured verifier verdict in
// text. It never treats absent, malformed, future-versioned, or unknown
// verdicts as a pass.
func ParseVerdict(contract, text string) (VerificationState, string, error) {
	if contract != PassContractV1 {
		return VerificationFailed, "", fmt.Errorf("unsupported pass contract %q", contract)
	}
	openCount := strings.Count(text, verdictOpen)
	closeCount := strings.Count(text, verdictClose)
	if openCount == 0 {
		return VerificationFailed, "", fmt.Errorf("missing %s verdict", PassContractV1)
	}
	if openCount != 1 || closeCount > 1 {
		return VerificationFailed, "", fmt.Errorf("expected exactly one %s verdict block", PassContractV1)
	}
	open := strings.LastIndex(text, verdictOpen)
	payloadStart := open + len(verdictOpen)
	closeRel := strings.Index(text[payloadStart:], verdictClose)
	if closeRel < 0 {
		return VerificationFailed, "", fmt.Errorf("unterminated %s verdict", PassContractV1)
	}
	closeEnd := payloadStart + closeRel + len(verdictClose)
	if strings.TrimSpace(text[closeEnd:]) != "" {
		return VerificationFailed, "", fmt.Errorf("%s verdict must be the final response block", PassContractV1)
	}
	payloadText := strings.TrimSpace(text[payloadStart : payloadStart+closeRel])
	dec := json.NewDecoder(strings.NewReader(payloadText))
	dec.DisallowUnknownFields()
	var payload verdictPayload
	if err := dec.Decode(&payload); err != nil {
		return VerificationFailed, "", fmt.Errorf("malformed %s verdict: %w", PassContractV1, err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return VerificationFailed, "", fmt.Errorf("malformed %s verdict: %w", PassContractV1, err)
	}
	if payload.Version != CurrentVerdictVersion {
		return VerificationFailed, strings.TrimSpace(payload.Reason), fmt.Errorf("unsupported verdict version %d", payload.Version)
	}
	switch strings.ToLower(strings.TrimSpace(payload.Verdict)) {
	case "pass":
		return VerificationPassed, strings.TrimSpace(payload.Reason), nil
	case "fail":
		return VerificationFailed, strings.TrimSpace(payload.Reason), nil
	default:
		return VerificationFailed, strings.TrimSpace(payload.Reason), fmt.Errorf("unknown verdict %q", payload.Verdict)
	}
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func verifierContractInstruction(contract string) string {
	return fmt.Sprintf(`Evaluate the completed work against the verifier request above.
Return exactly one verdict block at the end of your response using pass contract %q:
%s{"version":1,"verdict":"pass|fail","reason":"brief evidence"}%s
Use "pass" only when the evidence satisfies the request. If evidence is missing or uncertain, use "fail".`,
		contract, verdictOpen, verdictClose)
}
