package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// OverlayToast draws text as a small box pinned to the top right, over the
// screen content. width is the terminal width in cells. The overlay is
// surgical: only the covered cells of the affected rows are replaced, so
// the full-width background padding of every row is preserved.
func OverlayToast(width int, screen, text string) string {
	if width <= 0 {
		return screen
	}
	box := strings.Split(UIToastStyle.Render("✓ "+text), "\n")
	bw := lipgloss.Width(box[0])
	x := width - bw - 2
	if x < 0 {
		x = 0
	}
	lines := strings.Split(screen, "\n")
	for i, bl := range box {
		if r := 1 + i; r >= 0 && r < len(lines) {
			lines[r] = ansi.Truncate(lines[r], x, "") + bl
			// Restore full-width background padding on the right.
			if pad := width - ansi.StringWidth(lines[r]); pad > 0 {
				lines[r] += lipgloss.NewStyle().Background(UIBg).Render(strings.Repeat(" ", pad))
			}
		}
	}
	return strings.Join(lines, "\n")
}
