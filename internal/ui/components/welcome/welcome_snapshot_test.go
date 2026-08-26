package welcome

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var escapeSequence = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestRenderFixedCellSnapshots(t *testing.T) {
	for _, tc := range []struct {
		width, height int
		wantArt       bool
	}{{100, 30, true}, {120, 40, true}, {60, 20, false}} {
		// strconv.Itoa, not string(rune(...)): the latter turns 100 into "d"
		// and 30 into a raw control byte, producing unreadable subtest names.
		t.Run(strings.Join([]string{"cells", strconv.Itoa(tc.width), strconv.Itoa(tc.height)}, "-"), func(t *testing.T) {
			got := escapeSequence.ReplaceAllString(Render(tc.width, tc.height, "v-test"), "")
			lines := strings.Split(got, "\n")
			if len(lines) != tc.height {
				t.Fatalf("height = %d, want %d", len(lines), tc.height)
			}
			for i, line := range lines {
				if ansi.StringWidth(line) != tc.width {
					t.Fatalf("line %d width = %d, want %d", i, ansi.StringWidth(line), tc.width)
				}
			}
			if tc.wantArt != strings.Contains(got, "██████╗") {
				t.Fatalf("art presence = %v, want %v", strings.Contains(got, "██████╗"), tc.wantArt)
			}
			if !strings.Contains(got, "type a message below to begin · / for commands") {
				t.Fatal("missing stable welcome help")
			}
		})
	}
}
