package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"little-golem/src/model"
)

const maxCmdLines = 4

// bgEl returns a style on the approval card background.
func bgEl() lipgloss.Style { return lipgloss.NewStyle().Background(UIBgEl) }

// ApprovalCard renders the bash approval prompt that replaces the input
// box: the command, the choices, and a key hint. In reason mode the
// choices give way to a one-line input.
func ApprovalCard(m *model.App) string {
	inner := max(1, m.Width-4) // thick left border (1) + padding (2) + 1 spare
	line := func(st lipgloss.Style, s string) string { return st.Width(inner).Render(s) }

	title := line(bgEl().Foreground(UIWarning).Bold(true), "Run "+m.Current.Name+" command?")

	cmd := strings.Split(lipgloss.Wrap("$ "+m.Current.Command(), inner, ""), "\n")
	if len(cmd) > maxCmdLines {
		cmd = append(cmd[:maxCmdLines-1], "…")
	}
	rows := []string{title}
	for _, c := range cmd {
		rows = append(rows, line(bgEl().Foreground(UIText), c))
	}

	if m.Reasoning {
		m.Reason.SetWidth(max(1, inner-8))
		rows = append(rows,
			line(bgEl().Foreground(UIPrimary), "Deny — tell the model why:"),
			line(bgEl(), bgEl().Foreground(UIPrimary).Render("› ")+m.Reason.View()),
			line(bgEl().Foreground(UIMuted), "enter send · esc back"))
	} else {
		for i, a := range model.Approvals {
			marker, label := "  ", bgEl().Foreground(UIMuted)
			if i == m.ApprovalSel {
				marker, label = "› ", bgEl().Foreground(UIPrimary).Bold(true)
			}
			rows = append(rows, line(bgEl(),
				bgEl().Foreground(UIPrimary).Render(marker)+
					label.Width(22).Render(a.Label)+
					bgEl().Foreground(UIMuted).Render(a.Key)))
		}
		rows = append(rows, line(bgEl().Foreground(UIMuted), "↑/↓ select · enter confirm · esc deny"))
	}
	return UIApprovalBox.Width(m.Width).Render(strings.Join(rows, "\n"))
}

// InputBlock is whatever sits under the transcript: the approval card while
// a command awaits an answer, else the chat input.
func InputBlock(m *model.App) string {
	if m.Current != nil {
		return ApprovalCard(m)
	}
	return UIInputStyle.Width(m.Width).Render(m.Input.View())
}
