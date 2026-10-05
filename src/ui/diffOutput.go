// diffOutput.go is the whole visual side of edit/write diffs.
// tools.BuildDiff computes the change; everything that draws it (the
// "+3 -1" summary, the tinted numbered rows, the transcript panel and the
// approval-card body) lives here.

package ui

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"little-golem/src/tools"
)

// toolMaxDiffLines caps an edit/write diff panel in the transcript; diffs
// read better with a little more room than command output.
const toolMaxDiffLines = 16

// DiffStats is the "+3 -1" summary of a diff, with +/- colored.
func DiffStats(d *tools.Diff, bg color.Color) string {
	st := lipgloss.NewStyle().Background(bg)
	return st.Foreground(UISuccess).Render("+"+strconv.Itoa(d.Adds)) + st.Render(" ") +
		st.Foreground(UIError).Render("-"+strconv.Itoa(d.Dels))
}

// diffRows renders a diff as full-width rows: a line-number gutter, a +/-
// sign and the text, with added and removed lines tinted. ctxBg is the
// background of unchanged lines. With limit > 0 only that many rows are
// returned and hidden reports how many were cut.
func diffRows(d *tools.Diff, w, limit int, ctxBg color.Color) (rows []string, hidden int) {
	rows = diffRowsGW(d.Lines, w, gutterWidth(d.Lines), ctxBg)
	if limit > 0 && len(rows) > limit {
		hidden = len(rows) - limit
		rows = rows[:limit]
	}
	return rows, hidden
}

// gutterWidth is the number of digits needed for the largest line number.
func gutterWidth(lines []tools.DiffLine) int {
	gw := 1
	for _, l := range lines {
		if l.Kind != tools.DiffSkip {
			gw = max(gw, len(strconv.Itoa(l.Num)))
		}
	}
	return gw
}

// diffRowsGW renders lines with a gutter gw digits wide.
func diffRowsGW(lines []tools.DiffLine, w, gw int, ctxBg color.Color) (rows []string) {
	textW := max(4, w-gw-4) // gutter + space + sign + space + text

	for _, l := range lines {
		bg, signFg, textFg, sign := ctxBg, UIMuted, UIMuted, " "
		switch l.Kind {
		case tools.DiffAdd:
			bg, signFg, textFg, sign = UIDiffAddBg, UISuccess, UIText, "+"
		case tools.DiffDel:
			bg, signFg, textFg, sign = UIDiffDelBg, UIError, UIText, "-"
		case tools.DiffSkip:
			row := lipgloss.NewStyle().Background(ctxBg).Foreground(UIMuted).Italic(true).Width(w).
				Render(strings.Repeat(" ", gw+1) + "⋯ " + strconv.Itoa(l.Num) + " unchanged lines")
			rows = append(rows, row)
			continue
		}
		base := lipgloss.NewStyle().Background(bg)
		num := ""
		if l.Num > 0 {
			num = strconv.Itoa(l.Num)
		}
		gutter := base.Foreground(UIMuted).Width(gw).Align(lipgloss.Right)
		text := strings.ReplaceAll(l.Text, "\t", "    ")
		for i, seg := range strings.Split(lipgloss.Wrap(text, textW, ""), "\n") {
			n, s := "", " "
			if i == 0 {
				n, s = num, sign
			}
			rows = append(rows, base.Width(w).Render(
				gutter.Render(n)+base.Render(" ")+base.Foreground(signFg).Bold(true).Render(s)+
					base.Render(" ")+base.Foreground(textFg).Render(seg)))
		}
	}
	return rows
}

// renderDiff draws an edit/write result: a "+3 -1" summary on the header
// line, then the changed lines in a tinted, numbered panel.
func renderDiff(b *strings.Builder, d *tools.Diff, w int, expand bool) {
	note := ""
	switch {
	case d.Created:
		note = " · new file"
	case d.Matches > 1:
		note = " · " + strconv.Itoa(d.Matches) + " matches"
	}
	b.WriteString("  " + DiffStats(d, UIBg) + UIHintStyle.Render(note))

	limit := toolMaxDiffLines
	if expand {
		limit = 0
	}
	rows, hidden := diffRows(d, max(10, w-2), limit, UIBgPanel)
	indent := lipgloss.NewStyle().MarginLeft(2).MarginBackground(UIBg)
	for _, r := range rows {
		b.WriteString("\n" + indent.Render(r))
	}
	if hidden > 0 {
		more := lipgloss.NewStyle().Foreground(UIPrimary).Background(UIBgPanel).Padding(0, 1).Width(max(10, w-2)).
			Render("… +" + strconv.Itoa(hidden) + " more lines (ctrl+x to expand)")
		b.WriteString("\n" + indent.Render(more))
	}
}

// renderDiffDraft draws an edit/write diff while the model is still writing
// the call: a live "+3 -0" summary under the header, then the most recent
// lines. Earlier lines are folded into a "… N earlier lines" row on top, so
// the panel stays a fixed height as the content grows (expand shows all).
func renderDiffDraft(b *strings.Builder, d *tools.Diff, w int, expand bool) {
	note := ""
	switch {
	case d.Created:
		note = " · new file"
	case d.Overwriting:
		note = " · overwriting"
	case d.Matches > 1:
		note = " · " + strconv.Itoa(d.Matches) + " matches"
	}
	b.WriteString("  " + DiffStats(d, UIBg) + UIHintStyle.Render(note+" · writing"))

	lines, skipped := d.Lines, 0
	if !expand && len(lines) > toolMaxDiffLines {
		skipped = len(lines) - toolMaxDiffLines
		lines = lines[skipped:]
	}
	panelW := max(10, w-2)
	rows := diffRowsGW(lines, panelW, gutterWidth(d.Lines), UIBgPanel)
	if !expand && len(rows) > toolMaxDiffLines {
		skipped += len(rows) - toolMaxDiffLines
		rows = rows[len(rows)-toolMaxDiffLines:]
	}
	indent := lipgloss.NewStyle().MarginLeft(2).MarginBackground(UIBg)
	if skipped > 0 {
		earlier := lipgloss.NewStyle().Foreground(UIMuted).Background(UIBgPanel).Padding(0, 1).Width(panelW).
			Render("… " + strconv.Itoa(skipped) + " earlier lines")
		b.WriteString("\n" + indent.Render(earlier))
	}
	for _, r := range rows {
		b.WriteString("\n" + indent.Render(r))
	}
}

// diffCardRows is the card body for an edit or write: heading, the path
// with a +/- summary, and the numbered diff.
func diffCardRows(name string, d *tools.Diff, inner int, line func(lipgloss.Style, string) string) []string {
	heading, note := "Edit file?", ""
	switch {
	case name == "write" && d.Created:
		heading = "Create file?"
	case name == "write":
		heading = "Overwrite file?"
	case d.Matches > 1:
		note = " · " + strconv.Itoa(d.Matches) + " matches"
	}
	rows := []string{
		line(bgEl().Foreground(UIWarning).Bold(true), heading),
		line(bgEl(), bgEl().Foreground(UIPrimary).Render(d.Path)+bgEl().Render("  ")+DiffStats(d, UIBgEl)+bgEl().Foreground(UIMuted).Render(note)),
	}
	if name == "edit" && d.Matches == 0 {
		rows = append(rows, line(bgEl().Foreground(UIError), "old_string was not found in this file; the edit will fail"))
	}
	body, hidden := diffRows(d, inner, maxDiffLines, UIBgEl)
	rows = append(rows, body...)
	if hidden > 0 {
		rows = append(rows, line(bgEl().Foreground(UIMuted), "… +"+strconv.Itoa(hidden)+" more lines"))
	}
	return rows
}
