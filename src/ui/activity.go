package ui

import (
	"strconv"
	"strings"
	"time"

	"little-golem/src/config"
	"little-golem/src/model"
)

// Verb names what the agent is doing right now. Approval and tool runs are
// specific; otherwise it is a rotating "Golem <verb>" flavor word.
func Verb(m *model.App) string {
	switch {
	case m.Current != nil:
		return "Awaiting approval"
	case m.Compacting:
		return "Compacting context"
	case m.Running != "":
		return "Running " + m.Running
	}
	return "Golem " + config.VerbAt(m.VerbSeed, time.Since(m.TurnStart))
}

// thinkingNow reports whether the latest reply is still in its reasoning
// phase with some reasoning text already streamed.
func thinkingNow(m *model.App) bool {
	n := len(m.Entries)
	if n == 0 {
		return false
	}
	e := m.Entries[n-1]
	return e.Kind == model.EntryAssistant && e.Streaming && e.ThinkEnd.IsZero() && e.Content == "" && e.Reasoning != ""
}

// thoughtFor reports how long the model reasoned in the latest assistant
// entry that has a reasoning trace this turn.
func thoughtFor(m *model.App) time.Duration {
	for i := len(m.Entries) - 1; i >= 0; i-- {
		e := m.Entries[i]
		if e.Kind == model.EntryUser {
			break
		}
		if e.Kind == model.EntryAssistant && e.Reasoning != "" {
			return e.ThinkDuration()
		}
	}
	return 0
}

// ActivityLine is the row above the input; it also carries errors, quit
// confirmation and transient notices. While the agent works it shows
// the spinner, a verb, and elapsed time/token stats; when idle it only
// nudges the user back to the bottom if they scrolled away.
func ActivityLine(m *model.App) string {
	switch {
	case m.Err != nil:
		return UIErrorStyle.Render(" error: " + m.Err.Error())
	case m.ConfirmQuit:
		return UIErrorStyle.Render(" " + m.ConfirmQuitNotice)
	case m.Notice != "":
		return UIHintStyle.Render(" " + m.Notice)
	}
	if !m.Working() {
		if !m.Follow {
			return UIHintStyle.Render(" ↓ scrolled up · end jumps to latest")
		}
		return ""
	}
	var stats []string
	if m.Current == nil {
		stats = append(stats, "esc to interrupt")
	}
	if !m.TurnStart.IsZero() {
		stats = append(stats, model.FmtDur(time.Since(m.TurnStart)))
	}
	if n := m.TurnTokens(); n > 0 {
		stats = append(stats, "↓ "+model.FmtTokens(n)+" tokens")
	}
	if !m.ShowThinking && thinkingNow(m) {
		stats = append(stats, "ctrl+o to see thinking")
	}
	if d := thoughtFor(m); d > 0 {
		stats = append(stats, "thought for "+model.FmtDur(d))
	}
	return " " + m.Spinner.View() +
		Shimmer(Verb(m)+"…", m.Frame) +
		UIHintStyle.Render(" ("+strings.Join(stats, " · ")+")")
}

// CtxLabel renders "ctx 12.4K/128.0K (10%)" from the last reported usage.
func CtxLabel(m *model.App) string {
	if m.TokenUsed <= 0 {
		return ""
	}
	pct := m.TokenUsed * 100 / config.CtxSize
	return "ctx " + model.FmtTokens(m.TokenUsed) + "/" + model.FmtTokens(config.CtxSize) +
		" (" + strconv.Itoa(pct) + "%)"
}
