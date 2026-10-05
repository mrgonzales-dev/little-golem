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
	summaryMaxTokens = 1024
	clipResult       = 800  // chars of one tool result fed to the summarizer
	clipArgs         = 500  // chars of one tool call's arguments
	keepTailMax      = 6000 // chars of the latest tool exchange kept verbatim
	headKeep         = 2000 // chars of the transcript start kept when it must be cut

	// transcriptMax keeps the summarizer's prompt plus its answer inside its
	// context window (about 3 chars per token, with room for the prompt).
	transcriptMax = (config.CtxSize - summaryMaxTokens - 600) * 3

	compactSystem = `You compress a coding-agent session so the work can continue from an empty context.
Write a short, dense, factual summary. Put the most important things first and leave out anything minor.
Use these sections in this order, omitting empty ones:
GOAL: what the user wants and every standing instruction or preference, in the user's own words where possible. Most important section.
NEXT: the exact next step, and anything unresolved, failing or blocked.
STATE: where the work stands right now, including the file or task in progress.
DONE: only finished work, one line each: files created or edited and what changed, commands run and their key result. State clearly that these are finished so they are not redone.
FACTS: exact file paths, function names, line numbers and error messages that will be needed again. Copy them verbatim.
If space is short, trim DONE first. Never drop GOAL, NEXT or exact names.
Use short bullet points. Never invent anything that is not in the conversation. No preamble.`

	summaryHeader = "Summary of our conversation so far (the earlier messages were compacted to save context):\n\n"
	summaryAck    = "Understood. I have the summary and will continue from there."
)

// CompactDoneMsg reports a finished summarization.
type CompactDoneMsg struct {
	Summary string
	Err     error
	Auto    bool                // started by the context threshold, not /compact
	Resume  bool                // a turn is in progress and continues afterwards
	Before  int                 // tokens in use when compaction started
	Tail    []llama.ChatMessage // latest tool exchange, kept verbatim after the summary
}

// latestExchange returns the trailing assistant tool calls and their results
// when they are small enough to keep verbatim, so a resumed turn still sees
// what it just did.
func latestExchange(h []llama.ChatMessage) []llama.ChatMessage {
	i := len(h)
	for i > 0 && h[i-1].Role == "tool" {
		i--
	}
	if i == len(h) || i < 2 || h[i-1].Role != "assistant" || len(h[i-1].ToolCalls) == 0 {
		return nil
	}
	tail, size := h[i-1:], 0
	for _, msg := range tail {
		size += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			size += len(tc.Function.Arguments)
		}
	}
	if size > keepTailMax {
		return nil
	}
	return tail
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
	var tail []llama.ChatMessage
	if resume {
		tail = latestExchange(m.History)
	}
	ask := "Conversation to summarize:\n\n" + transcript(m.History[:len(m.History)-len(tail)]) +
		"\n\nWrite the summary now. Lead with the most important things."
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
		return CompactDoneMsg{Summary: s, Err: err, Auto: auto, Resume: resume, Before: before, Tail: tail}
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
	used := len(config.Prompt()) + len(summary)
	if msg.Resume {
		summary = summaryHeader + "The user's original request (for reference; it is already in progress):\n" + m.Request +
			"\n\n" + msg.Summary +
			"\n\nIMPORTANT: you are in the MIDDLE of this task, not at the start. Everything listed as DONE is finished: do not redo it and do not start over. Continue from NEXT."
		m.History = append([]llama.ChatMessage{{Role: "user", Content: summary}}, msg.Tail...)
		used = len(config.Prompt()) + len(summary)
		for _, t := range msg.Tail {
			used += len(t.Content)
			for _, tc := range t.ToolCalls {
				used += len(tc.Function.Arguments)
			}
		}
	} else {
		m.History = []llama.ChatMessage{{Role: "user", Content: summary}, {Role: "assistant", Content: summaryAck}}
	}
	m.CompactFailed = false
	m.TokenUsed = used / 4
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
	s := strings.TrimSpace(b.String())
	if len(s) > transcriptMax {
		s = strings.ToValidUTF8(s[:headKeep], "") + "\n\n[... middle of the conversation omitted ...]\n\n" +
			strings.ToValidUTF8(s[len(s)-(transcriptMax-headKeep):], "")
	}
	return s
}

// clip cuts s to about n bytes, marking how much was dropped.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + fmt.Sprintf("… [%d more chars]", len(s)-n)
}
