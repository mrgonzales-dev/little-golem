// Package ui contains the lipgloss palette, the Bubble Tea view/layout
// code, and the ASCII golem logo. App-level methods on *app.App live in
// package app to keep state mutation close to the model.
package ui

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

// Palette borrowed from opencode's default dark theme.
var (
	UIBg      = lipgloss.Color("#0a0a0a")
	UIBgPanel = lipgloss.Color("#141414")
	UIBgEl    = lipgloss.Color("#1e1e1e")
	UIText    = lipgloss.Color("#eeeeee")
	UIMuted   = lipgloss.Color("#808080")
	UIPrimary = lipgloss.Color("#fab283")
	UIWarning = lipgloss.Color("#f5a742")
	UISuccess = lipgloss.Color("#7fd88f")
	UIError   = lipgloss.Color("#e06c75")

	UITitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(UIText).Background(UIBg)
	UISubtitleStyle = lipgloss.NewStyle().Foreground(UIMuted).Background(UIBg)
	UIHintStyle     = lipgloss.NewStyle().Foreground(UIMuted).Background(UIBg)
	UIErrorStyle    = lipgloss.NewStyle().Foreground(UIError).Background(UIBg)
	UIGlyphStyle    = lipgloss.NewStyle().Foreground(UIPrimary).Background(UIBg)
	UILogoStyle     = lipgloss.NewStyle().Foreground(UIPrimary).Background(UIBg)

	UIBypassStyle = lipgloss.NewStyle().Bold(true).Foreground(UIError).Background(UIBg)
	UIVerbStyle   = lipgloss.NewStyle().Foreground(UIPrimary).Background(UIBg)
	UIScrollThumb = lipgloss.NewStyle().Foreground(UIPrimary).Background(UIBg)
	UIScrollTrack = lipgloss.NewStyle().Foreground(UIBgEl).Background(UIBg)

	UIThinkHeadStyle = lipgloss.NewStyle().Foreground(UIWarning).Background(UIBg)
	UIThinkBodyStyle = lipgloss.NewStyle().Foreground(UIMuted).Background(UIBg)
	UIAssistStyle    = lipgloss.NewStyle().Foreground(UIText)
	UIToolStyle      = lipgloss.NewStyle().Foreground(UIMuted).Background(UIBg)
	UIConfirmStyle   = lipgloss.NewStyle().Foreground(UIWarning).Background(UIBgPanel)

	UIUserStyle = lipgloss.NewStyle().
			Background(UIBgPanel).
			Padding(0, 2)

	UIInputStyle = lipgloss.NewStyle().
			Background(UIBgEl).
			Padding(0, 1)

	UIApprovalBox = lipgloss.NewStyle().
			Background(UIBgEl).
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(UIWarning).
			BorderBackground(UIBgEl).
			Padding(0, 1)

	UIHeaderBlock = lipgloss.NewStyle().
			Background(UIBg).
			Padding(0, 1)

	UIStatusBlock = lipgloss.NewStyle().
			Background(UIBg).
			Padding(0, 1)

	UIBodyBlock = lipgloss.NewStyle().
			Background(UIBg).
			Padding(0, 2)
)

// InputStyles paints the textarea onto the opencode-style dark panel.
func InputStyles() textarea.Styles {
	st := textarea.DefaultDarkStyles()
	for _, s := range []*textarea.StyleState{&st.Focused, &st.Blurred} {
		s.Base = s.Base.Background(UIBgEl)
		s.Text = s.Text.Foreground(UIText).Background(UIBgEl)
		s.CursorLine = s.CursorLine.Background(UIBgEl)
		s.CursorLineNumber = s.CursorLineNumber.Background(UIBgEl)
		s.EndOfBuffer = s.EndOfBuffer.Foreground(UIBgEl).Background(UIBgEl)
		s.Placeholder = s.Placeholder.Foreground(UIMuted).Background(UIBgEl)
		s.Prompt = s.Prompt.Background(UIBgEl)
	}
	return st
}
