package app

import (
	"context"
	"encoding/json"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/model"
	"little-golem/src/tools"
	"little-golem/src/ui"
)

// RunNext pops the next pending call: silent tools and commands the user
// already approved run at once; gated commands the user denied skip at
// once; the rest surface the confirmation prompt. The queue drains before
// the model continues.
func RunNext(m *model.App) tea.Cmd {
	if len(m.Pending) == 0 {
		if m.Rounds >= model.MaxRounds {
			m.Notice = "stopped: too many tool rounds, send a message to continue"
			return nil
		}
		// All calls resolved: hand the transcript back to the model.
		return Continue(m)
	}
	pc := m.Pending[0]
	m.Pending = m.Pending[1:]

	// Ungated tools (read) run inline; they are quick.
	if !m.Tools.NeedsApproval(pc.Name) {
		res := m.Tools.Execute(context.Background(), tools.Call{Name: pc.Name, Arguments: json.RawMessage(pc.Arguments)})
		RecordToolResult(m, pc, res.Content)
		ui.RenderEntries(m)
		return RunNext(m)
	}
	// Bypass mode or a session approval: run without asking, off the UI
	// goroutine since shell commands can take a while.
	if m.Bypass || m.Approved[pc.Command()] {
		return execAsync(m, pc, true, "")
	}
	if m.Denied[pc.Command()] {
		RecordToolResult(m, pc, "the user denied this command")
		ui.RenderEntries(m)
		return RunNext(m)
	}

	// Show the command and wait for y/n.
	pcCopy := pc
	m.Current = &pcCopy
	m.ApprovalSel, m.Reasoning = 0, false
	ui.Layout(m)
	return nil
}

// BashConfirm resolves the pending confirmation. Allow-for-session and a
// plain deny are remembered so identical commands do not re-prompt; allow
// once and deny-with-reason are not, so the model gets to adapt.
func BashConfirm(m *model.App, d model.Decision, reason string) tea.Cmd {
	if m.Current == nil {
		return nil
	}
	pc := *m.Current
	m.Current = nil
	m.Reasoning = false
	m.Reason.Reset()
	m.Reason.Blur()
	yes := d == model.AllowOnce || d == model.AllowSession
	switch d {
	case model.AllowSession:
		m.Approved[pc.Command()] = true
	case model.Deny:
		m.Denied[pc.Command()] = true
	}
	ui.Layout(m)
	return execAsync(m, pc, yes, reason)
}

// execAsync runs (or, when !yes, skips) a gated call in a command and
// reports it with ExecDoneMsg. A non-empty reason rides along on denial.
func execAsync(m *model.App, pc model.PendingCall, yes bool, reason string) tea.Cmd {
	denied := "the user denied this command"
	if reason != "" {
		denied += ": " + reason
	}
	if yes {
		m.Running = pc.Name
		StartToolEntry(m, pc)
		ui.RenderEntries(m)
	}
	return func() tea.Msg {
		res := tools.Result{Content: denied}
		if yes {
			res = m.Tools.Execute(context.Background(), tools.Call{Name: pc.Name, Arguments: json.RawMessage(pc.Arguments)})
		}
		return ExecDoneMsg{Call: pc, Content: res.Content}
	}
}

// ExecDoneMsg reports one confirmed bash run as finished.
type ExecDoneMsg struct {
	Call    model.PendingCall
	Content string
}
