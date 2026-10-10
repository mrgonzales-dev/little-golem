package app

import (
	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
)

// HistoryChars sizes the chat history in chars, including tool-call
// arguments, so the app can react to context pressure immediately instead
// of waiting one round trip for the server's usage totals.
func HistoryChars(h []llama.ChatMessage) int {
	n := 0
	for _, msg := range h {
		n += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			n += len(tc.Function.Arguments)
		}
	}
	return n
}

// EstimatedTokens is the live context estimate: the max of the server's
// authoritative TokenUsed and the char-based estimate of what is actually
// queued (prompt + history). It catches huge tool results the moment they
// land in history, before the next Stream reports usage.
func EstimatedTokens(m *model.App) int {
	est := (len(config.Prompt()) + HistoryChars(m.History)) / 4
	if m.TokenUsed > est {
		return m.TokenUsed
	}
	return est
}

// ShouldCompactForSize reports char-level context pressure: true when the
// queued history alone already reaches the compaction threshold, even if
// TokenUsed (updated only from server usage events) is still stale.
func ShouldCompactForSize(m *model.App) bool {
	if m.CompactFailed || len(m.History) == 0 {
		return false
	}
	return EstimatedTokens(m) >= config.CompactAt
}

// syncTokenEstimate lifts TokenUsed to the live estimate right after a tool
// result lands, so the ctx meter and the auto-compact threshold react in
// the same turn instead of one round late. It never lowers the server's
// authoritative count.
func syncTokenEstimate(m *model.App) {
	if est := EstimatedTokens(m); est > m.TokenUsed {
		m.TokenUsed = est
	}
}
