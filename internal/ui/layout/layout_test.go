package layout

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFrame_OrdersBodyOverlayInputStatus(t *testing.T) {
	out := Frame("BODY", "OVL", "INPUT", "STATUS")
	lines := strings.Split(out, "\n")
	assert.Equal(t, []string{"BODY", "OVL", "INPUT", "STATUS"}, lines)
}

func TestFrame_OmitsEmptyRegions(t *testing.T) {
	out := Frame("BODY", "", "INPUT", "STATUS")
	assert.Equal(t, "BODY\nINPUT\nSTATUS", out)
}

func TestFrame_DoesNotTrim(t *testing.T) {
	body := strings.Repeat("X\n", 50) + "TAIL"
	out := Frame(body, "", "I", "S")
	assert.Contains(t, out, "X\nX\nX", "body content must pass through unmodified")
	assert.Contains(t, out, "TAIL")
}
