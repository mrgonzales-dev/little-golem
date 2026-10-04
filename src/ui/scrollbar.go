package ui

import "strings"

// Scrollbar renders a one-column vertical scrollbar for a viewport of the
// given height. Blank track cells are returned when everything fits, so the
// column always keeps its width.
func Scrollbar(height, total, visible, offset int) string {
	if height <= 0 {
		return ""
	}
	maxOff := total - visible
	thumb, pos := 0, 0
	if maxOff > 0 {
		thumb = max(1, height*visible/total)
		if space := height - thumb; space > 0 {
			pos = min(space, offset*space/maxOff)
		}
	}
	var sb strings.Builder
	for i := range height {
		if i > 0 {
			sb.WriteByte('\n')
		}
		switch {
		case maxOff <= 0:
			sb.WriteString(UIScrollTrack.Render(" "))
		case i >= pos && i < pos+thumb:
			sb.WriteString(UIScrollThumb.Render("┃"))
		default:
			sb.WriteString(UIScrollTrack.Render("│"))
		}
	}
	return sb.String()
}
