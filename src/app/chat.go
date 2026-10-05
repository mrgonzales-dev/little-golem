package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
	"little-golem/src/ui"
)

// Send submits the input text and starts a background completion stream.
func Send(m *model.App) tea.Cmd {
	text := strings.TrimSpace(m.Input.Value())
	if text == "" {
		return nil
	}
	m.Input.Reset()
	m.Notice = ""
	m.Err = nil
	m.Follow = true
	m.TurnStart = time.Now()
	m.VerbSeed = config.NewVerbSeed()
	m.TurnDone, m.ReqTokens, m.Rounds = 0, 0, 0
	m.Cancelled = false
	m.Request = text
	m.Seen, m.Repeats = nil, 0

	m.History = append(m.History, llama.ChatMessage{Role: "user", Content: text})
	m.Entries = append(m.Entries,
		model.Entry{Kind: model.EntryUser, Content: text},
		model.Entry{Kind: model.EntryAssistant, Model: m.ModelName(), Streaming: true, Started: time.Now()},
	)

	ctx, cancel := context.WithCancel(context.Background())
	m.Busy = true
	m.Cancel = cancel
	m.EventsChan = make(chan llama.StreamEvent, 64)

	apiMsgs := make([]llama.ChatMessage, 0, len(m.History)+1)
	apiMsgs = append(apiMsgs, llama.ChatMessage{Role: "system", Content: config.Prompt()})
	apiMsgs = append(apiMsgs, m.History...)
	go m.Server.Stream(ctx, apiMsgs, m.EventsChan)

	saveSession(m)
	ui.RenderEntries(m)
	return AwaitToken(m)
}

// AppendDelta folds a streamed chunk into the in-progress assistant entry.
// Tool deltas accumulate separately and do not touch the transcript.
func AppendDelta(m *model.App, reasoning, content string, tool *llama.ToolDelta) {
	if n := len(m.Entries); n > 0 && m.Entries[n-1].Kind == model.EntryAssistant {
		e := &m.Entries[n-1]
		e.Reasoning += reasoning
		if content != "" {
			e.Content += content
			if e.ThinkEnd.IsZero() {
				e.ThinkEnd = time.Now()
			}
		}
	}
	if tool != nil {
		acc := m.BeginToolAcc(tool.Index)
		acc.Accumulate(tool.Name, tool.Args)
	}
	if reasoning != "" || content != "" || tool != nil {
		ui.RenderEntries(m)
	}
}

// ExecutePendingTools turns accumulated tool_call fragments into queued
// pending calls and starts the queue. Approval-requiring calls (bash,
// edit, write) head the queue; read and other silent tools run immediately.
func ExecutePendingTools(m *model.App) tea.Cmd {
	if len(m.ToolAcc) == 0 {
		return nil
	}
	// Cluster by streamed index in call order.
	idxs := make([]int, 0, len(m.ToolAcc))
	for i := range m.ToolAcc {
		if m.ToolAcc[i].Name == "" {
			continue // never completed; stray argument fragments
		}
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	if len(idxs) == 0 {
		m.ToolAcc = nil
		return nil
	}

	// The assistant entry on screen is the one that made the calls; carry
	// its (possibly empty) content into the history message.
	var entry *model.Entry
	if n := len(m.Entries); n > 0 && m.Entries[n-1].Kind == model.EntryAssistant {
		entry = &m.Entries[n-1]
		entry.Streaming = false
		entry.Ended = time.Now()
	}

	var calls []llama.ToolCall
	for _, i := range idxs {
		acc := m.ToolAcc[i]
		tc := llama.ToolCall{Type: "function"}
		tc.ID = fmt.Sprintf("call_%d", i)
		tc.Function.Name = acc.Name
		tc.Function.Arguments = acc.Args()
		calls = append(calls, tc)
	}

	// Record the assistant's tool_call request in history.
	msg := llama.ChatMessage{Role: "assistant", ToolCalls: calls}
	if entry != nil {
		msg.Content = entry.Content
	}
	m.History = append(m.History, msg)
	m.ToolAcc = nil

	var q []model.PendingCall
	var silent []model.PendingCall
	for _, tc := range calls {
		pc := model.PendingCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments}
		pc.CallIndex = strings.TrimPrefix(tc.ID, "call_")
		if m.Tools.NeedsApproval(pc.Name) {
			q = append(q, pc)
			continue
		}
		silent = append(silent, pc)
	}
	m.Pending = append(q, silent...)
	return RunNext(m)
}

// StartToolEntry shows a call in the transcript while it runs.
func StartToolEntry(m *model.App, pc model.PendingCall) {
	m.Entries = append(m.Entries, model.Entry{
		Kind: model.EntryTool, Tool: pc.Name, Cmd: pc.Summary(),
		CallID: pc.CallIndex, Streaming: true, Started: time.Now(),
		Diff: pc.DiffFor(), // captured now: the file is still unchanged
	})
}

// RecordToolResult completes the call's transcript entry (or appends one
// for calls that ran inline) and adds the tool message to the history.
func RecordToolResult(m *model.App, pc model.PendingCall, content string) {
	done := false
	for i := len(m.Entries) - 1; i >= 0 && !done; i-- {
		if e := &m.Entries[i]; e.Kind == model.EntryTool && e.Streaming && e.CallID == pc.CallIndex {
			e.Content, e.Streaming, e.Ended = content, false, time.Now()
			done = true
		}
	}
	if !done {
		m.Entries = append(m.Entries, model.Entry{
			Kind: model.EntryTool, Tool: pc.Name, Cmd: pc.Summary(),
			CallID: pc.CallIndex, Content: content,
		})
	}
	m.History = append(m.History, llama.ChatMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: fmt.Sprintf("call_%s", pc.CallIndex),
		Name:       pc.Name,
	})
}

// Continue sends a follow-up completion carrying the accumulated history,
// including tool results. A fresh streaming assistant entry receives the
// next turn.
func Continue(m *model.App) tea.Cmd {
	m.Rounds++
	ctx, cancel := context.WithCancel(context.Background())
	m.Busy = true
	m.Cancel = cancel
	m.EventsChan = make(chan llama.StreamEvent, 64)
	m.Entries = append(m.Entries, model.Entry{
		Kind:      model.EntryAssistant,
		Model:     m.ModelName(),
		Streaming: true,
		Started:   time.Now(),
	})

	apiMsgs := make([]llama.ChatMessage, 0, len(m.History)+1)
	apiMsgs = append(apiMsgs, llama.ChatMessage{Role: "system", Content: config.Prompt()})
	apiMsgs = append(apiMsgs, m.History...)
	go m.Server.Stream(ctx, apiMsgs, m.EventsChan)

	ui.RenderEntries(m)
	return AwaitToken(m)
}

// FinishStream closes out the in-progress assistant entry and records it
// in the API history. Entries that made tool calls were already recorded
// by ExecutePendingTools; skip those to avoid a duplicate empty message.
func FinishStream(m *model.App) {
	m.Busy = false
	m.Cancel = nil
	m.TurnDone += m.ReqTokens // no usage chunk arrived (e.g. interrupted)
	m.ReqTokens = 0
	if n := len(m.Entries); n > 0 && m.Entries[n-1].Kind == model.EntryAssistant {
		e := &m.Entries[n-1]
		e.Streaming = false
		e.Ended = time.Now()
		if e.Content != "" {
			m.History = append(m.History, llama.ChatMessage{
				Role:    "assistant",
				Content: e.Content,
			})
		}
	}
	ui.RenderEntries(m)
}

// Interrupt cancels an in-flight stream or compaction, if any.
func Interrupt(m *model.App) {
	if (m.Busy || m.Compacting) && m.Cancel != nil {
		m.Cancel()
		m.Cancel = nil
		m.Cancelled = m.Busy
		m.Notice = "interrupted"
	}
}

// LastReply returns the content of the most recent completed assistant entry.
func LastReply(m *model.App) (string, bool) {
	if m.Busy {
		return "", false
	}
	for i := len(m.Entries) - 1; i >= 0; i-- {
		if m.Entries[i].Kind == model.EntryAssistant && !m.Entries[i].Streaming {
			return m.Entries[i].Content, true
		}
	}
	return "", false
}

// AwaitToken yields one command per stream event; channel close ends the stream.
func AwaitToken(m *model.App) tea.Cmd {
	ch := m.EventsChan
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return StreamDoneMsg{}
		}
		if ev.Err != nil {
			return ErrMsg{Err: ev.Err}
		}
		return TokenMsg(ev)
	}
}
