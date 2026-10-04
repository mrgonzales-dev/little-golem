package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"little-golem/src/model"
)

// CenterLogo centers the golem glyph block plus a status line in the given
// box. label is the already-rendered status line under the logo.
func CenterLogo(width, height int, label string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	px := 2
	if width < 62 {
		px = 1
	}
	content := lipgloss.JoinVertical(lipgloss.Center,
		UILogoStyle.Render(GolemText(px)), "", UISubtitleStyle.Render(label))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(UIBg)))
}

// Layout computes the chat viewport dimensions and triggers a redraw.
// Every block is given its total width (padding included): the body is
// padded 2 each side with a 1-column scrollbar, so the viewport is W-5.
func Layout(m *model.App) {
	const (
		headerH   = 1
		activityH = 1
	)
	m.Input.SetWidth(m.Width - 2)
	vH := m.Height - headerH - activityH - lipgloss.Height(InputBlock(m))
	if vH < 1 {
		vH = 1
	}
	m.Viewport.SetWidth(max(1, m.Width-5))
	m.Viewport.SetHeight(vH)
	// Match glamour's wrap width to the viewport's inner content width.
	SetWrap(m.Viewport.Width() - 2)
	RenderEntries(m)
}

// RenderEntries writes the transcript into the viewport.
func RenderEntries(m *model.App) {
	if m.Width == 0 {
		return
	}
	w := m.Viewport.Width()
	var b strings.Builder

	// Top padding: two blank rows above the transcript.
	for i := 0; i < 2; i++ {
		b.WriteString("\n")
	}

	if len(m.Entries) == 0 && m.Ready {
		b.WriteString(CenterLogo(m.Viewport.Width(), m.Viewport.Height(), "ready — ask anything"))
	}

	for _, e := range m.Entries {
		switch e.Kind {
		case model.EntryUser:
			// User chat bubble, composed with lipgloss: fit-content
			// width, wrapped to the viewport so long lines still break.
			maxW := w - 5 // border(1) + padding(2)
			text := e.Content
			if lipgloss.Width(text) > maxW && maxW > 0 {
				text = lipgloss.Wrap(text, maxW, " ")
			}
			b.WriteString(UIUserStyle.Render(text))
			b.WriteString("\n\n")
		case model.EntryAssistant:
			RenderAssistant(&b, m, e, w)
			b.WriteString("\n\n")
		case model.EntryTool:
			RenderTool(&b, e, w, m.ShowTools)
			b.WriteString("\n\n")
		}
	}

	// Paint the entire viewport body in one pass: Width pads every line
	// to the full viewport width and Height pads the block to full
	// height, so separator and filler lines carry the bg too.
	painted := lipgloss.NewStyle().
		Background(UIBg).
		Width(m.Viewport.Width()).
		Height(m.Viewport.Height()).
		Render(b.String())

	m.Viewport.SetContent(painted)
	// Stick to the latest output only while following. Scrolling up clears
	// Follow (see app.Scroll), so history stays put while the agent works.
	if m.Follow {
		m.Viewport.GotoBottom()
	}
}

// RenderAssistant renders one assistant entry: thinking header, optional
// thinking body, content, and the opencode-style footer once finished.
func RenderAssistant(b *strings.Builder, owner *model.App, e model.Entry, w int) {
	thinking := e.Streaming && e.ThinkEnd.IsZero() && e.Content == ""
	switch {
	case thinking:
		// The live verb sits on the activity line above the input.
	case e.Reasoning != "":
		mark := "+"
		if owner.ShowThinking {
			mark = "-"
		}
		head := mark + " Thought"
		if d := e.ThinkDuration(); d > 0 {
			head += " · " + model.FmtDur(d)
		}
		b.WriteString(UIThinkHeadStyle.Render(head))
		if owner.ShowThinking {
			b.WriteString("\n")
			b.WriteString(UIThinkBodyStyle.Width(w - 2).PaddingLeft(2).Render(e.Reasoning))
		}
		b.WriteString("\n\n")
	}

	if e.Content != "" {
		body := RenderMarkdown(e.Content)
		b.WriteString(UIAssistStyle.Width(w).Render(strings.TrimRight(body, " \n")))
		b.WriteString("\n")
	}
	if !e.Streaming {
		footer := UIGlyphStyle.Render(" ▣") + UIHintStyle.Render(" minicpm")
		if d := e.Duration(); d > 0 {
			footer += UIHintStyle.Render(" · " + model.FmtDur(d))
		}
		b.WriteString(footer)
	}
}

// View renders the full screen.
func View(m *model.App) tea.View {
	if m.Width == 0 {
		return tea.NewView("")
	}

	var body string
	if !m.Ready && m.Err == nil {
		body = CenterLogo(m.Viewport.Width(), m.Viewport.Height(), m.Spinner.View()+" waking the golem…")
	} else {
		body = m.Viewport.View()
	}
	bar := Scrollbar(m.Viewport.Height(), m.Viewport.TotalLineCount(),
		m.Viewport.VisibleLineCount(), m.Viewport.YOffset())
	body = UIBodyBlock.Width(m.Width).Render(lipgloss.JoinHorizontal(lipgloss.Top, body, bar))

	ctx := ""
	if c := CtxLabel(m); c != "" {
		ctx = "  " + UIHintStyle.Render(c)
	}
	if m.Bypass {
		ctx += "  " + UIBypassStyle.Render("▶▶ bypass on") + UIHintStyle.Render(" (shift+tab)")
	}
	header := UIHeaderBlock.Width(m.Width).Render(
		UIGlyphStyle.Render("▣") + " " + UITitleStyle.Render("little-golem") +
			"  " + UISubtitleStyle.Render("minicpm-2b · llama.cpp") + ctx)

	activity := UIStatusBlock.Width(m.Width).Render(ActivityLine(m))

	screen := lipgloss.JoinVertical(lipgloss.Left,
		body,
		activity,
		InputBlock(m),
		header,
	)

	v := tea.NewView(screen)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "little-golem"
	v.BackgroundColor = UIBg
	return v
}
