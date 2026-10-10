package app

import (
	"fmt"

	"little-golem/src/model"
	"little-golem/src/session"
	"little-golem/src/ui"
)

// saveSession persists the conversation to .little-golem/.
func saveSession(m *model.App) {
	if err := session.Save(m); err != nil {
		m.Err = fmt.Errorf("saving session: %w", err)
	}
}

// NewSession (/new) forgets the conversation and deletes the saved session.
func NewSession(m *model.App) {
	if m.Working() {
		m.Notice = "can't start a new session while the agent is working"
		return
	}
	if err := session.Clear(); err != nil {
		m.Err = fmt.Errorf("clearing session: %w", err)
		return
	}
	m.History, m.Entries, m.Request = nil, nil, ""
	m.TokenUsed, m.CompactFailed, m.Err = 0, false, nil
	m.Approved, m.Denied, m.Seen = map[string]bool{}, map[string]bool{}, nil
	m.ClearSelection() // content lines are gone
	m.Notice = "new session"
	ui.RenderEntries(m)
}
