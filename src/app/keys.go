package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/model"
	"little-golem/src/ui"
)

// HandleKey processes app-level keybindings. It reports whether the key
// was consumed; unconsumed keys fall through to delegate/input.
func HandleKey(m *model.App, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	m.Notice = ""

	// Any non-ctrl+c key cancels a pending quit confirm. ctrl+c itself
	// drives the multi-press quit flow.
	if msg.String() != "ctrl+c" {
		m.ConfirmQuit = false
		m.ConfirmQuitNotice = ""
	}

	if m.Current != nil {
		return handleApproval(m, msg), true
	}

	switch msg.String() {
	case "shift+tab":
		toggleBypass(m)
		return nil, true
	case "ctrl+c":
		return handleCtrlC(m), true
	case "esc":
		Interrupt(m)
		return nil, true
	case "enter":
		if strings.TrimSpace(m.Input.Value()) == "/super" {
			m.Input.Reset()
			toggleBypass(m)
			return nil, true
		}
		if m.Ready && !m.Working() {
			return Send(m), true
		}
		return nil, true
	case "ctrl+y":
		if reply, ok := LastReply(m); ok {
			m.Notice = "copied reply"
			return tea.SetClipboard(reply), true
		}
		return nil, true
	case "ctrl+x":
		m.ShowTools = !m.ShowTools
		m.Notice = map[bool]string{true: "full tool output", false: "short tool output"}[m.ShowTools]
		ui.RenderEntries(m)
		return nil, true
	case "ctrl+o":
		m.ShowThinking = !m.ShowThinking
		if m.ShowThinking {
			m.Notice = "thinking shown"
		} else {
			m.Notice = "thinking hidden"
		}
		ui.RenderEntries(m)
		return nil, true
	}
	return nil, false
}

// handleCtrlC implements the three-press quit flow:
//  1. with non-empty input, the first ctrl+c clears the input; otherwise
//     it arms the confirm-quit prompt;
//  2. the second ctrl+c arms the prompt (or confirms immediately when
//     the prompt is already armed);
//  3. the third ctrl+c quits.
func handleCtrlC(m *model.App) tea.Cmd {
	if m.ConfirmQuit {
		return tea.Quit
	}
	if input := strings.TrimSpace(m.Input.Value()); input != "" {
		m.Input.Reset()
		m.Notice = "input cleared — press ctrl+c again to quit"
		return nil
	}
	m.ConfirmQuit = true
	m.ConfirmQuitNotice = "press ctrl+c again to quit"
	return nil
}

// Scroll moves the transcript for wheel and paging keys, like opencode's
// chat list: scrolling up always drops Follow, and reaching the bottom
// again re-arms it so new output keeps the view pinned. Mouse clicks and
// drags are ignored.
func Scroll(m *model.App, msg tea.Msg) (tea.Cmd, bool) {
	vp := &m.Viewport
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		*vp, _ = vp.Update(msg)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "pgup":
			vp.PageUp()
		case "pgdown":
			vp.PageDown()
		case "ctrl+up", "alt+up":
			vp.ScrollUp(1)
		case "ctrl+down", "alt+down":
			vp.ScrollDown(1)
		case "ctrl+home":
			vp.GotoTop()
		case "ctrl+end":
			vp.GotoBottom()
		case "home", "end":
			// Only steal these when the input has nothing to navigate.
			if m.Input.Value() != "" {
				return nil, false
			}
			if msg.String() == "home" {
				vp.GotoTop()
			} else {
				vp.GotoBottom()
			}
		default:
			return nil, false
		}
	case tea.MouseMsg:
		return nil, true // clicks, drags, motion: swallow
	default:
		return nil, false
	}
	m.Follow = vp.AtBottom()
	return nil, true
}

// handleApproval drives the bash approval card: arrows/tab move the
// selection, enter confirms it, and y/a/n/r/esc are shortcuts. Scroll keys
// still reach the transcript.
func handleApproval(m *model.App, msg tea.KeyPressMsg) tea.Cmd {
	if m.Reasoning {
		switch msg.String() {
		case "enter":
			return BashConfirm(m, model.DenyReason, strings.TrimSpace(m.Reason.Value()))
		case "esc":
			m.Reasoning = false
			m.Reason.Blur()
			ui.Layout(m)
			return nil
		}
		var cmd tea.Cmd
		m.Reason, cmd = m.Reason.Update(msg)
		return cmd
	}
	n := len(model.Approvals)
	switch k := msg.String(); k {
	case "up", "k":
		m.ApprovalSel = (m.ApprovalSel + n - 1) % n
	case "down", "j", "tab":
		m.ApprovalSel = (m.ApprovalSel + 1) % n
	case "shift+tab":
		toggleBypass(m)
		return BashConfirm(m, model.AllowOnce, "")
	case "enter":
		return decide(m, model.Approvals[m.ApprovalSel].Do)
	case "esc":
		return decide(m, model.Deny)
	default:
		for _, a := range model.Approvals {
			if strings.EqualFold(k, a.Key) {
				return decide(m, a.Do)
			}
		}
		cmd, _ := Scroll(m, msg)
		return cmd
	}
	return nil
}

// decide applies a choice; "Deny with reason" opens the reason input first.
func decide(m *model.App, d model.Decision) tea.Cmd {
	if d != model.DenyReason {
		return BashConfirm(m, d, "")
	}
	m.Reasoning = true
	m.Reason.Reset()
	ui.Layout(m)
	return m.Reason.Focus()
}

// toggleBypass flips bypass mode, where bash runs without asking.
func toggleBypass(m *model.App) {
	m.Bypass = !m.Bypass
	if m.Bypass {
		m.Notice = "bypass mode on: bash runs without asking (shift+tab or /super to turn off)"
	} else {
		m.Notice = "bypass mode off: bash asks first"
	}
}
