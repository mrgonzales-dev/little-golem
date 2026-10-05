package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"little-golem/src/model"
)

const (
	toolMaxCmdLines = 3
	toolMaxOutLines = 10
)

// toolOutput cleans a tool result for display: no escape codes, tabs
// expanded, and the "$ cmd" echo the bash tool adds on failures dropped
// (the header already shows the command).
func toolOutput(e model.Entry) string {
	out := strings.ReplaceAll(ansi.Strip(e.Content), "\r", "")
	out = strings.ReplaceAll(out, "\t", "    ")
	out = strings.TrimRight(out, "\n ")
	if echo := "$ " + e.Cmd + "\n"; strings.HasPrefix(out, echo) {
		out = out[len(echo):]
	}
	return out
}

// toolFailed reports whether a finished call errored or was denied.
func toolFailed(out string) bool {
	return strings.HasPrefix(out, "the user denied") ||
		strings.Contains(out, "\n(exit status ") || strings.HasPrefix(out, "(exit status ") ||
		strings.Contains(out, "(timed out") || strings.HasPrefix(out, "tool ") && strings.Contains(out, " failed:")
}

// RenderTool draws one tool call: a "bash: <command>" header with a status
// glyph, then the output in a panel. Output is cut to toolMaxOutLines unless
// expand is set.
func RenderTool(b *strings.Builder, e model.Entry, w int, expand bool) {
	out := toolOutput(e)
	failed := !e.Streaming && toolFailed(out)

	glyph := UIToolStyle.Foreground(UISuccess).Render(" ✓")
	switch {
	case e.Streaming:
		glyph = UIToolStyle.Foreground(UIWarning).Render(" …")
	case failed:
		glyph = UIToolStyle.Foreground(UIError).Render(" ✗")
	}

	cmdW := max(10, w-len(e.Tool)-6)
	cmd := strings.Split(lipgloss.Wrap(e.Cmd, cmdW, ""), "\n")
	if len(cmd) > toolMaxCmdLines && !expand {
		if e.Draft {
			cmd = append([]string{"…"}, cmd[len(cmd)-toolMaxCmdLines+1:]...)
		} else {
			cmd = append(cmd[:toolMaxCmdLines-1], "…")
		}
	}
	name := UIGlyphStyle.Bold(true).Render(e.Tool)
	sep := UIToolStyle.Render(": ")
	for i, c := range cmd {
		if i == 0 && c == "" {
			b.WriteString(glyph + " " + name)
			continue
		}
		if i == 0 {
			b.WriteString(glyph + " " + name + sep + UIAssistStyle.Background(UIBg).Render(c))
		} else {
			b.WriteString("\n" + UIToolStyle.Render(strings.Repeat(" ", len(e.Tool)+5)) + UIAssistStyle.Background(UIBg).Render(c))
		}
	}
	if e.Streaming {
		if e.Draft && e.Diff != nil {
			renderDiffDraft(b, e.Diff, w, expand)
			return
		}
		hint := "  running"
		if e.Draft {
			hint = "  writing"
		}
		b.WriteString(UIHintStyle.Render(hint))
		return
	}
	if e.Diff != nil && !failed {
		renderDiff(b, e.Diff, w, expand)
		return
	}
	if out == "" {
		return
	}

	panelW := max(10, w-2)
	lines := strings.Split(lipgloss.Wrap(out, panelW-2, ""), "\n")
	more := 0
	if !expand && len(lines) > toolMaxOutLines {
		more = len(lines) - toolMaxOutLines
		lines = lines[:toolMaxOutLines]
	}
	base := lipgloss.NewStyle().Foreground(UIMuted).Background(UIBgPanel).
		Padding(0, 1).Width(panelW).MarginLeft(2).MarginBackground(UIBg)
	for _, l := range lines {
		st := base
		switch {
		case strings.HasPrefix(l, "(exit status"), strings.HasPrefix(l, "the user denied"):
			st = st.Foreground(UIError)
		case l == "(no output)":
			st = st.Italic(true)
		}
		b.WriteString("\n" + st.Render(l))
	}
	if more > 0 {
		b.WriteString("\n" + base.Foreground(UIPrimary).Render("… +"+strconv.Itoa(more)+" more lines (ctrl+x to expand)"))
	}
}
