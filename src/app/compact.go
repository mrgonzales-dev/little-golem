package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
	"little-golem/src/ui"
)

const (
	summaryMaxTokens = 4096
	clipResult       = 1500 // chars of one tool result fed to the summarizer
	clipArgs         = 600  // chars of one tool call's arguments

	compactSystem = `You compress a coding-agent session so the work can continue from an empty context.
Write a dense, factual summary with these parts, omitting any that are empty:
GOAL: what the user wants, including every standing instruction or preference.
DONE: what has been accomplished, including files created or edited and what changed in each, and commands that were run with their key results.
FACTS: file paths, function names, line numbers, error messages and other details that will be needed again. Copy them exactly.
STATE: where the work stands right now.
NEXT: the next step, and anything unresolved or failing.
Use short bullet points. Never invent anything that is not in the conversation. No preamble.`

	summaryHeader = "Summary of our conversation so far (the earlier messages were compacted to save context):\n\n"
	summaryAck    = "Understood. I have the summary and will continue from there."
)

// CompactDoneMsg reports a finished summarization.
type CompactDoneMsg struct {
	Summary string
	Err     error
	Auto    bool // started by the context threshold, not /compact
	Resume  bool // a turn is in progress and continues afterwards
	Before  int  // tokens in use when compaction started
}

// ShouldAutoCompact reports whether the context has reached the
// auto-compaction threshold.
func ShouldAutoCompact(m *model.App) bool {
	return !m.CompactFailed && len(m.History) > 0 && m.TokenUsed >= config.CompactAt
}

// Compact summarizes the history in the background and replaces it with the
// summary (see FinishCompact). focus optionally steers what the summary
// keeps. resume continues the running turn once done.
func Compact(m *model.App, focus string, auto, resume bool) tea.Cmd {
	if len(m.History) == 0 {
		m.Notice = "nothing to compact"
		return nil
	}
	if !auto {
		m.Err, m.Notice, m.Follow = nil, "", true
		m.TurnStart, m.VerbSeed = time.Now(), config.NewVerbSeed()
		m.TurnDone, m.ReqTokens = 0, 0
	}
	ask := "Conversation to summarize:\n\n" + transcript(m.History) + "\n\nWrite the summary now."
	if focus != "" {
		ask += " Pay particular attention to: " + focus
	}
	msgs := []llama.ChatMessage{
		{Role: "system", Content: compactSystem},
		{Role: "user", Content: ask},
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.Compacting, m.Cancel = true, cancel
	srv, before := m.Server, m.TokenUsed
	ui.RenderEntries(m)
	return func() tea.Msg {
		if fast, err := bootServer(config.CompactModel); err == nil {
			defer fast.Stop()
			srv = fast
		}
		s, err := srv.Complete(ctx, msgs, summaryMaxTokens)
		if err == nil && s == "" {
			err = errors.New("the model returned an empty summary")
		}
		return CompactDoneMsg{Summary: s, Err: err, Auto: auto, Resume: resume, Before: before}
	}
}

// FinishCompact swaps the history for the summary, or reports why that
// failed. When a turn was in progress it carries on with the new context.
func FinishCompact(m *model.App, msg CompactDoneMsg) tea.Cmd {
	m.Compacting, m.Cancel = false, nil
	if errors.Is(msg.Err, context.Canceled) {
		m.Notice = "compaction cancelled"
		return nil
	}
	if msg.Err != nil {
		m.Err = fmt.Errorf("compact failed: %w", msg.Err)
		m.CompactFailed = m.CompactFailed || msg.Auto
		ui.RenderEntries(m)
		if msg.Resume {
			return Continue(m)
		}
		return nil
	}

	summary := summaryHeader + msg.Summary
	if msg.Resume {
		summary += "\n\nThe user's current request (keep working on it):\n" + m.Request
		m.History = []llama.ChatMessage{{Role: "user", Content: summary}}
	} else {
		m.History = []llama.ChatMessage{{Role: "user", Content: summary}, {Role: "assistant", Content: summaryAck}}
	}
	m.CompactFailed = false
	m.TokenUsed = (len(config.Prompt()) + len(summary)) / 4
	m.Entries = append(m.Entries, model.Entry{
		Kind: model.EntryNote,
		Content: fmt.Sprintf("context compacted · %s → ~%s tokens",
			model.FmtTokens(msg.Before), model.FmtTokens(m.TokenUsed)),
	})
	ui.RenderEntries(m)
	if msg.Resume {
		return Continue(m)
	}
	return nil
}

// transcript flattens the history to plain text, with long tool output and
// call arguments clipped, for the summarizer.
func transcript(h []llama.ChatMessage) string {
	var b strings.Builder
	for _, msg := range h {
		switch msg.Role {
		case "user":
			b.WriteString("USER: " + msg.Content + "\n\n")
		case "assistant":
			if msg.Content != "" {
				b.WriteString("ASSISTANT: " + msg.Content + "\n\n")
			}
			for _, tc := range msg.ToolCalls {
				b.WriteString("ASSISTANT CALLED " + tc.Function.Name + ": " + clip(tc.Function.Arguments, clipArgs) + "\n\n")
			}
		case "tool":
			b.WriteString("TOOL RESULT (" + msg.Name + "): " + clip(msg.Content, clipResult) + "\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// clip cuts s to about n bytes, marking how much was dropped.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + fmt.Sprintf("… [%d more chars]", len(s)-n)
}
