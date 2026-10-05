package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"little-golem/src/model"
)

const (
	maxCmdLines  = 4
	maxDiffLines = 14
	maxPartLines = 5 // old/new lines shown per side of an edit
)

// approvalPreview returns the approval card heading and the body lines for
// a pending call. Each body line starts with a two-character marker:
// "$ " command, "+ " added, "- " removed, "  " plain.
func approvalPreview(pc model.PendingCall) (string, []string) {
	a := pc.Args()
	switch pc.Name {
	case "edit":
		body := []string{"  " + a.Path}
		body = append(body, prefixed("- ", a.OldString, maxPartLines)...)
		return "Edit file?", append(body, prefixed("+ ", a.NewString, maxPartLines)...)
	case "write":
		n := strings.Count(a.Content, "\n") + min(1, len(a.Content))
		body := []string{"  " + a.Path + " (" + strconv.Itoa(n) + " lines)"}
		return "Write file?", append(body, prefixed("+ ", a.Content, maxPartLines+3)...)
	}
	return "Run " + pc.Name + " command?", []string{"$ " + pc.Command()}
}

// prefixed splits s into lines carrying prefix, capped at limit lines.
func prefixed(prefix, s string, limit int) []string {
	s = strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\t", "    ")
	lines := strings.Split(s, "\n")
	if len(lines) > limit {
		lines = append(lines[:limit-1], "… +"+strconv.Itoa(len(lines)-limit+1)+" more lines")
	}
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return lines
}

// bgEl returns a style on the approval card background.
func bgEl() lipgloss.Style { return lipgloss.NewStyle().Background(UIBgEl) }

// ApprovalCard renders the bash approval prompt that replaces the input
// box: the command, the choices, and a key hint. In reason mode the
// choices give way to a one-line input.
func ApprovalCard(m *model.App) string {
	inner := max(1, m.Width-4) // thick left border (1) + padding (2) + 1 spare
	line := func(st lipgloss.Style, s string) string { return st.Width(inner).Render(s) }

	heading, body := approvalPreview(*m.Current)
	rows := []string{line(bgEl().Foreground(UIWarning).Bold(true), heading)}

	limit := maxDiffLines
	if m.Current.Name == "bash" {
		limit = maxCmdLines
	}
	type row struct {
		st   lipgloss.Style
		text string
	}
	var wrapped []row
	for _, l := range body {
		st := bgEl().Foreground(UIText)
		switch l[:2] {
		case "+ ":
			st = bgEl().Foreground(UISuccess)
		case "- ":
			st = bgEl().Foreground(UIError)
		}
		for _, seg := range strings.Split(lipgloss.Wrap(l, inner, ""), "\n") {
			wrapped = append(wrapped, row{st, seg})
		}
	}
	if len(wrapped) > limit {
		wrapped = append(wrapped[:limit-1], row{bgEl().Foreground(UIMuted), "…"})
	}
	for _, r := range wrapped {
		rows = append(rows, line(r.st, r.text))
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
