package ui

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// NewSpinner returns the bubbles dot spinner used while the model loads
// and while the agent works.
func NewSpinner() spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(UIWarning).Background(UIBg)),
	)
}

// NewInput returns the textarea used by the input panel. Default height
// is 1 line; ctrl+j / shift+enter inserts a newline and the box grows up
// to 2 lines, after which the buffer scrolls internally.
func NewInput() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "  ← Ask anything…"
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.EndOfBufferCharacter = ' '
	ta.SetHeight(1)
	ta.MaxHeight = 2
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j", "shift+enter")
	ta.SetStyles(InputStyles())
	ta.Focus()
	return ta
}

// NewReason returns the one-line input used for "Deny with reason".
func NewReason() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "why? (sent back to the model)"
	st := textinput.DefaultDarkStyles()
	for _, s := range []*textinput.StyleState{&st.Focused, &st.Blurred} {
		s.Text = s.Text.Foreground(UIText).Background(UIBgEl)
		s.Placeholder = s.Placeholder.Foreground(UIMuted).Background(UIBgEl)
		s.Prompt = s.Prompt.Background(UIBgEl)
	}
	ti.SetStyles(st)
	return ti
}
