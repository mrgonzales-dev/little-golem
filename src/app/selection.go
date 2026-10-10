package app

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/model"
	"little-golem/src/ui"
)

// Selection implements opencode-style in-app text selection: the terminal
// keeps mouse reporting on (so wheel scroll works), left-drag selects
// transcript lines, and copy is explicit (right-click, ctrl+c or ctrl+y)
// instead of copy-on-release.
//
// Coordinates are content lines: the transcript viewport sits at screen
// row 0, so contentLine = YOffset + mouseY. While dragging near the top
// or bottom edge the view auto-scrolls and tail-follow is off, mirroring
// opencode's selection auto-scroll.

// SelectClick starts a selection on left press inside the transcript, or
// copies the selection on right press.
func SelectClick(m *model.App, msg tea.MouseClickMsg) (tea.Cmd, bool) {
	if !m.MouseEnabled {
		return nil, false
	}
	switch msg.Button {
	case tea.MouseRight:
		return CopySelection(m), true
	case tea.MouseLeft:
		y := msg.Y
		if y < 0 || y >= m.Viewport.Height() || m.Viewport.TotalLineCount() == 0 {
			if m.SelActive {
				m.ClearSelection()
				ui.RenderEntries(m)
			}
			return nil, true
		}
		line := m.Viewport.YOffset() + y
		if max := m.Viewport.TotalLineCount() - 1; line > max {
			line = max
		}
		m.SelActive, m.SelDragging = true, true
		m.SelAnchor, m.SelFocus = line, line
		m.Follow = false // opencode: tail mode off during selection
		ui.RenderEntries(m)
		return nil, true
	default:
		return nil, true
	}
}

// SelectDrag extends the selection while the left button is held. Dragging
// past the visible edge auto-scrolls the transcript.
func SelectDrag(m *model.App, msg tea.MouseMotionMsg) (tea.Cmd, bool) {
	if !m.MouseEnabled || !m.SelDragging {
		return nil, true // hover or disabled: swallow, no state change
	}
	h := m.Viewport.Height()
	y := msg.Y
	switch {
	case y < 0:
		m.Viewport.ScrollUp(1)
		m.SelFocus = m.Viewport.YOffset()
	case y >= h:
		m.Viewport.ScrollDown(1)
		m.SelFocus = m.Viewport.YOffset() + h - 1
	default:
		m.SelFocus = m.Viewport.YOffset() + y
	}
	if max := m.Viewport.TotalLineCount() - 1; m.SelFocus > max {
		m.SelFocus = max
	}
	if m.SelFocus < 0 {
		m.SelFocus = 0
	}
	m.Follow = false
	ui.RenderEntries(m)
	return nil, true
}

// SelectRelease ends the drag. A bare click (no drag) clears the selection;
// otherwise the selection is copied to the clipboard immediately
// (copy-on-release, like terminal select-to-copy) and the highlight is
// dropped once copied.
func SelectRelease(m *model.App, _ tea.MouseReleaseMsg) (tea.Cmd, bool) {
	if !m.MouseEnabled || !m.SelDragging {
		return nil, true
	}
	m.SelDragging = false
	if m.SelAnchor == m.SelFocus {
		m.ClearSelection()
		ui.RenderEntries(m)
		return nil, true
	}
	m.Follow = m.Viewport.AtBottom()
	cmd := CopySelection(m)
	ui.RenderEntries(m)
	return cmd, true
}

// ShowToast displays a transient top-right confirmation for
// model.ToastDuration and returns the command that dismisses it.
func ShowToast(m *model.App, text string) tea.Cmd {
	m.Toast, m.ToastAt = text, time.Now()
	return tea.Tick(model.ToastDuration, func(time.Time) tea.Msg {
		return ClearToastMsg{}
	})
}

// CopySelection copies the selected lines to the system clipboard (OSC52),
// drops the highlight, and shows a top-right toast confirmation.
func CopySelection(m *model.App) tea.Cmd {
	text, ok := ui.SelectedText(m)
	if !ok {
		m.Notice = "nothing selected"
		return nil
	}
	m.ClearSelection()
	return tea.Batch(
		tea.SetClipboard(text),
		ShowToast(m, "Copied to clipboard"),
	)
}
