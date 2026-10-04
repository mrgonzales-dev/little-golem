package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// shimmerPalette is a looping orange->cream->orange gradient, blended in
// CIELAB by lipgloss.
var shimmerPalette = lipgloss.Blend1D(20,
	lipgloss.Color("#e8862a"), lipgloss.Color("#ffe3b8"), lipgloss.Color("#e8862a"))

// Shimmer colors each letter from a gradient that slides one step per
// frame, so a bright band sweeps across the text.
func Shimmer(text string, frame int) string {
	var b strings.Builder
	n := len(shimmerPalette)
	i := 0
	for _, r := range text {
		var c color.Color = shimmerPalette[((i*2-frame)%n+n)%n]
		b.WriteString(lipgloss.NewStyle().Foreground(c).Background(UIBg).Bold(true).Render(string(r)))
		i++
	}
	return b.String()
}
