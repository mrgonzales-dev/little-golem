package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"little-golem/src/model"
)

// PlainLines strips ANSI escapes from painted viewport content and splits
// it into lines, 1:1 with the painted rows.
func PlainLines(painted string) []string {
	return strings.Split(ansi.Strip(painted), "\n")
}

// ApplySelection highlights the selected content lines. Selected rows are
// re-rendered as plain highlighted text (no markdown colors), so the copy
// target is obvious — the same tradeoff as opencode's copy mode, which
// collapses formatting for copying.
func ApplySelection(painted string, m *model.App) string {
	lo, hi, ok := m.SelRange()
	if !ok {
		return painted
	}
	lines := strings.Split(painted, "\n")
	if lo >= len(lines) {
		return painted
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	w := m.Viewport.Width()
	for i := lo; i <= hi; i++ {
		plain := strings.TrimRight(ansi.Strip(lines[i]), " \t")
		lines[i] = UISelectStyle.Width(w).Render(plain)
	}
	return strings.Join(lines, "\n")
}

// SelectedLines returns the trimmed plain text of the selected content
// lines: trailing padding per line is dropped and surrounding blank lines
// (e.g. the transcript top padding) are trimmed.
func SelectedLines(m *model.App) ([]string, bool) {
	lo, hi, ok := m.SelRange()
	if !ok {
		return nil, false
	}
	lines := PlainLines(m.Viewport.GetContent())
	if lo >= len(lines) {
		return nil, false
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	out := make([]string, 0, hi-lo+1)
	for _, l := range lines[lo : hi+1] {
		out = append(out, strings.TrimRight(l, " \t"))
	}
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// SelectedText joins SelectedLines for clipboard copy.
func SelectedText(m *model.App) (string, bool) {
	lines, ok := SelectedLines(m)
	if !ok {
		return "", false
	}
	return strings.Join(lines, "\n"), true
}
