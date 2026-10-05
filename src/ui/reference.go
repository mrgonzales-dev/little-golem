package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"little-golem/src/model"
)

// RefBlock is the @file/@folder picker shown above the input while a word
// starting with @ is typed.
func RefBlock(m *model.App) string {
	if len(m.Refs) == 0 {
		return ""
	}
	panel := lipgloss.NewStyle().Background(UIBgPanel).Padding(0, 1).Width(m.Width)
	inner := max(1, m.Width-4)
	rows := make([]string, 0, len(m.Refs)+1)
	for i, p := range m.Refs {
		if r := []rune(p); len(r) > inner {
			p = "…" + string(r[len(r)-inner+1:])
		}
		marker, st := "  ", lipgloss.NewStyle().Foreground(UIMuted).Background(UIBgPanel)
		if i == m.RefSel {
			marker, st = "› ", lipgloss.NewStyle().Foreground(UIPrimary).Bold(true).Background(UIBgPanel)
		}
		rows = append(rows, panel.Render(st.Render(marker+p)))
	}
	rows = append(rows, panel.Render(lipgloss.NewStyle().Foreground(UIMuted).Background(UIBgPanel).Render("↑/↓ select · tab/enter insert · esc close")))
	return strings.Join(rows, "\n")
}
