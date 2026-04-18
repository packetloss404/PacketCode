package layout

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFrame_OrdersBodyOverlayInputStatus(t *testing.T) {
	out := Frame("BODY", "OVL", "", "INPUT", "STATUS")
	lines := strings.Split(out, "\n")
	assert.Equal(t, []string{"BODY", "OVL", "INPUT", "STATUS"}, lines)
}

func TestFrame_OmitsEmptyRegions(t *testing.T) {
	out := Frame("BODY", "", "", "INPUT", "STATUS")
	assert.Equal(t, "BODY\nINPUT\nSTATUS", out)
}

func TestFrame_DoesNotTrim(t *testing.T) {
	body := strings.Repeat("X\n", 50) + "TAIL"
	out := Frame(body, "", "", "I", "S")
	assert.Contains(t, out, "X\nX\nX", "body content must pass through unmodified")
	assert.Contains(t, out, "TAIL")
}

// TestFrame_AboveInputBetweenOverlayAndInput confirms the new slot
// sits between overlay and input so a visible overlay always covers
// the aboveInput chunk visually (the overlay line comes first).
func TestFrame_AboveInputBetweenOverlayAndInput(t *testing.T) {
	out := Frame("BODY", "OVL", "ABOVE", "INPUT", "STATUS")
	lines := strings.Split(out, "\n")
	assert.Equal(t, []string{"BODY", "OVL", "ABOVE", "INPUT", "STATUS"}, lines)
}

// TestFrame_OmitsAboveInputWhenEmpty confirms an empty aboveInput slot
// doesn't leave a blank line behind — the stacker should skip it.
func TestFrame_OmitsAboveInputWhenEmpty(t *testing.T) {
	out := Frame("BODY", "OVL", "", "INPUT", "STATUS")
	assert.Equal(t, "BODY\nOVL\nINPUT\nSTATUS", out)
}

// TestFrame_AboveInputWithoutOverlay — common case. No modal is up, the
// popup renders on its own above the input.
func TestFrame_AboveInputWithoutOverlay(t *testing.T) {
	out := Frame("BODY", "", "ABOVE", "INPUT", "STATUS")
	assert.Equal(t, "BODY\nABOVE\nINPUT\nSTATUS", out)
}
