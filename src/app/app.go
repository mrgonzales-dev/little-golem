// Package app is the Bubble Tea controller: Init/Update/View delegation,
// key bindings, and stream folding. UI rendering lives in package ui;
// shared state lives in package model.
package app

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"little-golem/src/llama"
	"little-golem/src/model"
	"little-golem/src/ui"
)

// Bubble Tea messages.

type ServerReadyMsg struct{}

type ErrMsg struct{ Err error }

type TokenMsg llama.StreamEvent

type StreamDoneMsg struct{}

// M wraps *model.App so that Bubble Tea methods (Init/Update/View) live on
// a type owned by this package, while the shared state stays in model.
type M struct{ *model.App }

// New constructs a controller bound to a started llama-server.
func New(srv *llama.Server) *M {
	return &M{&model.App{
		Server:   srv,
		Input:    ui.NewInput(),
		Spinner:  ui.NewSpinner(),
		Reason:   ui.NewReason(),
		Approved: map[string]bool{},
		Denied:   map[string]bool{},
		Follow:   true,
	}}
}

func (m *M) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			if err := m.Server.WaitReady(); err != nil {
				return ErrMsg{Err: err}
			}
			return ServerReadyMsg{}
		},
		m.Spinner.Tick,
	)
}

func (m *M) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		ui.Layout(m.App)
		return m, nil

	case ServerReadyMsg:
		m.Ready = true
		ui.RenderEntries(m.App)
		return m, nil

	case ErrMsg:
		m.Err = msg.Err
		if m.Busy {
			return m, AwaitToken(m.App)
		}
		return m, nil

	case TokenMsg:
		if u := msg.Usage; u != nil {
			// Authoritative totals replace the per-chunk estimate.
			m.TokenUsed = u.TotalTokens
			m.TurnDone += u.CompletionTokens
			m.ReqTokens = 0
		} else {
			m.ReqTokens++
		}
		AppendDelta(m.App, msg.Reasoning, msg.Content, msg.ToolDelta)
		return m, AwaitToken(m.App)

	case StreamDoneMsg:
		FinishStream(m.App)
		if m.Cancelled {
			m.ToolAcc, m.Cancelled = nil, false
			return m, nil
		}
		if len(m.ToolAcc) > 0 {
			return m, ExecutePendingTools(m.App)
		}
		if m.Err == nil && ShouldAutoCompact(m.App) {
			return m, Compact(m.App, "", true, false)
		}
		return m, nil

	case CompactDoneMsg:
		return m, FinishCompact(m.App, msg)

	case ModelSwitchedMsg:
		FinishSwitch(m.App, msg)
		return m, nil

	case ExecDoneMsg:
		m.Running = ""
		RecordToolResult(m.App, msg.Call, msg.Content)
		ui.RenderEntries(m.App)
		return m, RunNext(m.App)

	case spinner.TickMsg:
		// The tick chain never stops; View reads the frame, so no
		// transcript re-render is needed per tick.
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		m.Frame++
		return m, cmd

	case tea.KeyPressMsg:
		if cmd, handled := HandleKey(m.App, msg); handled {
			return m, cmd
		}
	}

	if cmd, ok := Scroll(m.App, msg); ok {
		return m, cmd
	}

	var cmd tea.Cmd
	if m.Reasoning {
		m.Reason, cmd = m.Reason.Update(msg)
		return m, cmd
	}
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

// View delegates to the UI package.
func (m *M) View() tea.View {
	return ui.View(m.App)
}

var _ tea.Model = (*M)(nil)
