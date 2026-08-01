package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVerdict_FailClosedContract(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		want       VerificationState
		wantReason string
		wantErr    string
	}{
		{
			name:       "pass",
			text:       `evidence\n<packetcode-workflow-verdict>{"version":1,"verdict":"pass","reason":"tests green"}</packetcode-workflow-verdict>`,
			want:       VerificationPassed,
			wantReason: "tests green",
		},
		{
			name:       "explicit fail",
			text:       `<packetcode-workflow-verdict>{"version":1,"verdict":"fail","reason":"lint failed"}</packetcode-workflow-verdict>`,
			want:       VerificationFailed,
			wantReason: "lint failed",
		},
		{name: "missing", text: "looks good", want: VerificationFailed, wantErr: "missing"},
		{name: "malformed", text: `<packetcode-workflow-verdict>nope</packetcode-workflow-verdict>`, want: VerificationFailed, wantErr: "malformed"},
		{name: "future", text: `<packetcode-workflow-verdict>{"version":2,"verdict":"pass"}</packetcode-workflow-verdict>`, want: VerificationFailed, wantErr: "unsupported verdict version"},
		{name: "unknown verdict", text: `<packetcode-workflow-verdict>{"version":1,"verdict":"maybe"}</packetcode-workflow-verdict>`, want: VerificationFailed, wantErr: "unknown verdict"},
		{name: "unknown field", text: `<packetcode-workflow-verdict>{"version":1,"verdict":"pass","extra":true}</packetcode-workflow-verdict>`, want: VerificationFailed, wantErr: "unknown field"},
		{name: "trailing text", text: `<packetcode-workflow-verdict>{"version":1,"verdict":"pass"}</packetcode-workflow-verdict> but maybe`, want: VerificationFailed, wantErr: "final response block"},
		{name: "multiple blocks", text: `<packetcode-workflow-verdict>{"version":1,"verdict":"fail"}</packetcode-workflow-verdict><packetcode-workflow-verdict>{"version":1,"verdict":"pass"}</packetcode-workflow-verdict>`, want: VerificationFailed, wantErr: "exactly one"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason, err := ParseVerdict(PassContractV1, tc.text)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantReason, reason)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestParseVerdict_RejectsUnsupportedContract(t *testing.T) {
	state, _, err := ParseVerdict("future-contract", `<packetcode-workflow-verdict>{"version":1,"verdict":"pass"}</packetcode-workflow-verdict>`)
	require.Equal(t, VerificationFailed, state)
	require.ErrorContains(t, err, "unsupported pass contract")
}
