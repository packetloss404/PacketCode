// Package welcome renders the initial splash screen shown before the
// user has typed their first message: a centred ASCII-art wordmark plus
// a small version label.
//
// The wordmark is wide (~84 columns). On narrower terminals we fall back
// to a plain centred "packetcode <version>" line so the screen stays
// readable.
package welcome

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/ui/theme"
)

// art is the block-letter wordmark. The trailing column-width is 84.
var art = []string{
	`██████╗  █████╗  ██████╗██╗  ██╗███████╗████████╗ ██████╗ ██████╗ ██████╗ ███████╗`,
	`██╔══██╗██╔══██╗██╔════╝██║ ██╔╝██╔════╝╚══██╔══╝██╔════╝██╔═══██╗██╔══██╗██╔════╝`,
	`██████╔╝███████║██║     █████╔╝ █████╗     ██║   ██║     ██║   ██║██║  ██║█████╗  `,
	`██╔═══╝ ██╔══██║██║     ██╔═██╗ ██╔══╝     ██║   ██║     ██║   ██║██║  ██║██╔══╝  `,
	`██║     ██║  ██║╚██████╗██║  ██╗███████╗   ██║   ╚██████╗╚██████╔╝██████╔╝███████╗`,
	`╚═╝     ╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝`,
}

const artWidth = 84

// Render returns the centred welcome screen for a viewport of the given
// width and height. The version string is shown beneath the wordmark.
func Render(width, height int, version string) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	var contentLines []string
	if width >= artWidth {
		artStyle := lipgloss.NewStyle().Foreground(theme.AccentPrimary)
		for _, line := range art {
			contentLines = append(contentLines, artStyle.Render(line))
		}
	} else {
		// Narrow fallback: plain wordmark.
		contentLines = []string{
			lipgloss.NewStyle().Foreground(theme.AccentPrimary).Bold(true).Render("packetcode"),
		}
	}
	contentLines = append(contentLines, "")
	contentLines = append(contentLines,
		theme.StyleDim.Render(version),
	)
	contentLines = append(contentLines, "")
	contentLines = append(contentLines,
		theme.StyleSecondary.Render("type a message below to begin · / for commands"),
	)

	body := strings.Join(contentLines, "\n")
	box := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(body)
	return box
}
