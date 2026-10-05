// Package model is the Bubble Tea state: chat history, transcript entries,
// spinner, viewport, textarea. Both the app (controller) and ui (renderer)
// packages depend on it to avoid an import cycle.
package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"

	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/tools"
)

// EntryKind tags a transcript line with who wrote it.
type EntryKind int

const (
	EntryUser EntryKind = iota
	EntryAssistant
	EntryTool
	EntryNote    // a divider line such as "context compacted"
	EntrySummary // the text a compaction produced, shown under its divider
)

// Entry is one rendered line of chat history. Assistant entries may carry
// a separate reasoning (thinking) trace and timing metadata.
type Entry struct {
	Kind      EntryKind
	Content   string // assistant text, or a tool's raw result
	Tool      string // tool entries: tool name
	Cmd       string // tool entries: command or query the model passed
	CallID    string // tool entries: matches PendingCall.CallIndex while running
	Reasoning string
	Model     string // assistant entries: name of the model that wrote it
	Streaming bool
	Draft     bool        // tool entries: the model is still writing the call
	Diff      *tools.Diff // edit/write entries: the change, captured before it ran

	Started  time.Time
	ThinkEnd time.Time // when the first content token arrived
	Ended    time.Time
}

// ThinkDuration reports how long the reasoning phase lasted.
func (e Entry) ThinkDuration() time.Duration {
	end := e.ThinkEnd
	if end.IsZero() {
		end = e.Ended
	}
	if end.IsZero() || e.Started.IsZero() {
		return 0
	}
	return end.Sub(e.Started)
}

// Duration reports total time from send to stream completion.
func (e Entry) Duration() time.Duration {
	if e.Ended.IsZero() || e.Started.IsZero() {
		return 0
	}
	return e.Ended.Sub(e.Started)
}

// FmtDur renders durations like opencode: 4s, 1m12s.
func FmtDur(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	switch {
	case s < 1:
		return "<1s"
	case s < 60:
		return fmt.Sprintf("%ds", s)
	default:
		return fmt.Sprintf("%dm%ds", s/60, s%60)
	}
}

// FmtTokens renders token counts like opencode: 840, 12.4K, 1.2M.
func FmtTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return strconv.Itoa(n)
	}
}

// App is the root Bubble Tea model state shared across packages.
type App struct {
	Server *llama.Server
	// ModelIdx indexes config.Models: the model Server is running.
	// Switching names the model being loaded while a switch is in flight.
	ModelIdx  int
	Switching string

	Width, Height int
	Ready         bool
	Busy          bool

	Viewport viewport.Model
	Input    textarea.Model
	Spinner  spinner.Model

	Entries []Entry
	History []llama.ChatMessage
	// Request is the user's message that started the current turn.
	Request string

	EventsChan chan llama.StreamEvent
	Cancel     context.CancelFunc

	Tools   *tools.Registry
	ToolAcc map[int]*toolAcc // streamed tool_call index -> accumulator

	// Pending holds tool calls waiting to run. Calls to tools that need
	// approval (bash) pause here until the user answers y/n in keys.go.
	Pending []PendingCall
	// Current is the call awaiting user confirmation, if any.
	Current *PendingCall
	// CurrentDiff caches the preview of Current when it is an edit or
	// write, keyed by the call (see CurrentDiffFor).
	CurrentDiff    *tools.Diff
	currentDiffKey string
	// ApprovalSel is the highlighted Approvals row; Reasoning is true while
	// the user types a denial reason into Reason.
	ApprovalSel int
	Reasoning   bool
	Reason      textinput.Model
	// Bypass runs approval-gated tools (bash, edit, write) without asking.
	Bypass bool
	// Approved commands auto-run for the rest of the session after the
	// user answers y once; Denied commands auto-skip after n.
	Approved map[string]bool
	Denied   map[string]bool

	ShowThinking bool // opencode "thinking" mode: hidden by default
	ShowTools    bool // show full tool output instead of a short preview

	// ActivityH is the row count the activity line occupied at the last
	// layout; View re-lays out when it changes.
	ActivityH int

	// Follow keeps the viewport glued to the latest output. It is set
	// on new turns and cleared when the user scrolls up, mirroring
	// opencode's AtBottom-gated auto-scroll.
	Follow bool
	// TokenUsed is the last reported prompt+completion token count, i.e.
	// the context currently in use.
	TokenUsed int

	// Per-turn activity shown above the input. A turn spans the user's
	// message through the final answer, including tool rounds.
	TurnStart time.Time
	VerbSeed  int    // starting index into config.Verbs for this turn
	Frame     int    // animation frame, advanced by the spinner tick
	TurnDone  int    // completion tokens of finished requests this turn
	ReqTokens int    // streamed chunks of the in-flight request (estimate)
	Rounds    int    // model->tools round trips this turn
	Running   string // name of the tool currently executing, if any
	Cancelled bool   // the user interrupted the current turn

	// Seen holds the read-only calls (read, grep, glob) already run since
	// the last bash/edit/write; running one again cannot return anything
	// new. Repeats counts consecutive blocked repeats.
	Seen    map[string]bool
	Repeats int

	// Compacting is true while the history is being summarized.
	// CompactFailed turns auto-compaction off after a failure, until a
	// manual /compact succeeds.
	Compacting    bool
	CompactFailed bool

	// Refs are the files and folders offered while an @word is typed; RefTok
	// is the word they match and RefOff the word dismissed with esc.
	Refs   []string
	RefSel int
	RefTok string
	RefOff string

	Err    error
	Notice string

	// ConfirmQuit is set when the second ctrl+c has armed a pending quit;
	// the next ctrl+c actually quits. The transient banner comes from
	// ConfirmQuitNotice, which is cleared by any other keypress.
	ConfirmQuit       bool
	ConfirmQuitNotice string
}

// Decision is the user's answer to a bash approval prompt.
type Decision int

const (
	AllowOnce Decision = iota
	AllowSession
	Deny
	DenyReason
)

// Approval is one selectable row of the approval card.
type Approval struct {
	Label string
	Key   string // hotkey
	Do    Decision
}

// Approvals lists the approval choices in display order.
var Approvals = []Approval{
	{"Allow once", "y", AllowOnce},
	{"Allow for session", "a", AllowSession},
	{"Deny", "n", Deny},
	{"Deny with reason…", "r", DenyReason},
}

// DiffFor previews an edit or write call, or returns nil for other tools
// and for calls whose arguments do not parse.
func (p PendingCall) DiffFor() *tools.Diff {
	if p.Name != "edit" && p.Name != "write" {
		return nil
	}
	d, _ := tools.BuildDiff(p.Name, p.Arguments)
	return d
}

// ResetDiff drops the cached preview once the call has been answered.
func (m *App) ResetDiff() { m.CurrentDiff, m.currentDiffKey = nil, "" }

// CurrentDiffFor returns the cached preview of the call awaiting approval,
// building it on first use so the file is not re-read on every frame.
func (m *App) CurrentDiffFor() *tools.Diff {
	if m.Current == nil {
		return nil
	}
	if key := m.Current.Key(); key != m.currentDiffKey {
		m.CurrentDiff, m.currentDiffKey = m.Current.DiffFor(), key
	}
	return m.CurrentDiff
}

// Working reports whether the agent is mid-turn: streaming, waiting for
// tool approval, executing a tool, compacting, or loading another model.
func (m *App) Working() bool {
	return m.Busy || m.Current != nil || m.Running != "" || m.Compacting || m.Switching != ""
}

// ModelName is the short name of the running model.
func (m *App) ModelName() string { return config.Models[m.ModelIdx].Name }

// ModelLabel is the header label of the running model.
func (m *App) ModelLabel() string { return config.Models[m.ModelIdx].Label }

// TurnTokens is the running completion-token count for the current turn.
func (m *App) TurnTokens() int { return m.TurnDone + m.ReqTokens }

// BeginToolAcc returns the accumulator for streamed tool call index i,
// creating it on first use.
func (m *App) BeginToolAcc(i int) *toolAcc {
	if m.ToolAcc == nil {
		m.ToolAcc = map[int]*toolAcc{}
	}
	acc, ok := m.ToolAcc[i]
	if !ok {
		acc = &toolAcc{}
		m.ToolAcc[i] = acc
	}
	return acc
}

// toolAcc assembles one streamed tool call from fragments.
type toolAcc struct {
	ID       string
	Name     string
	argsBuf  strings.Builder
	complete bool
	// Snap caches the file an edit/write targets while its diff streams.
	Snap tools.FileSnap
}

// DraftDiff previews the edit/write call still being written, or nil.
func (a *toolAcc) DraftDiff() *tools.Diff {
	if a.Name != "edit" && a.Name != "write" {
		return nil
	}
	return tools.PartialDiff(a.Name, a.Args(), &a.Snap)
}

// PendingCall is one assembled tool call waiting to run.
type PendingCall struct {
	Name      string
	Arguments string
	CallIndex string // streamed index string; used for the call_ ID
}

// CallArgs is the union of the arguments the tools take.
type CallArgs struct {
	Command   string `json:"command"`
	Query     string `json:"query"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// Args decodes the call's arguments; fields that do not parse stay empty.
func (p PendingCall) Args() CallArgs {
	var a CallArgs
	_ = json.Unmarshal([]byte(p.Arguments), &a)
	return a
}

// Summary is what the call is about, for display: the bash command, the
// search query, the file path, or the raw arguments when none parse.
func (p PendingCall) Summary() string {
	a := p.Args()
	for _, s := range []string{a.Command, a.Query, a.Path} {
		if s != "" {
			return s
		}
	}
	return strings.TrimSpace(p.Arguments)
}

// Key identifies the call for session allow/deny memory: the command for
// bash, the whole call for tools that act on files.
func (p PendingCall) Key() string {
	if p.Name == "bash" {
		return p.Command()
	}
	return p.Name + " " + p.Arguments
}

// DraftSummary is PartialSummary for a call still being written, per
// tool. For write and edit the streamed diff panel below the header
// already shows the content, so the header only needs the file path —
// echoing the raw JSON (or arriving before "path") is noise.
func DraftSummary(name, args string) string {
	switch name {
	case "write", "edit":
		vals, _ := tools.PartialFields(args)
		return vals["path"]
	}
	return PartialSummary(args)
}

// PartialSummary is Summary for a call whose arguments are still
// streaming: it pulls the "command", "query" or "path" string out of
// incomplete JSON, so the text can be shown as the model writes it.
func PartialSummary(args string) string {
	for _, key := range []string{"command", "query", "path"} {
		if s, ok := partialJSONString(args, key); ok {
			return s
		}
	}
	return strings.TrimSpace(args)
}

// partialJSONString decodes the string value of the top-level key from
// possibly truncated JSON, as far as it has been written.
func partialJSONString(s, key string) (string, bool) {
	vals, _ := tools.PartialFields(s)
	v, ok := vals[key]
	return v, ok
}

// Index returns the numeric streamed index, or 0 when unknown.
func (p PendingCall) Index() int {
	n, _ := strconv.Atoi(p.CallIndex)
	return n
}

// Command returns the bash command carried in the arguments JSON, falling
// back to the raw arguments when parsing fails.
func (p PendingCall) Command() string {
	if c := p.Args().Command; c != "" {
		return c
	}
	return p.Arguments
}

// Args returns the accumulated JSON arguments string.
func (a *toolAcc) Args() string { return a.argsBuf.String() }

// ID returns the accumulated tool call id.
func (a *toolAcc) IDValue() string { return a.ID }

// Accumulate apps one delta fragment.
func (a *toolAcc) Accumulate(name, args string) {
	if name != "" && a.Name == "" {
		a.Name = name
	}
	a.argsBuf.WriteString(args)
}

// CancelType indicates how a tool accumulation finished.
type toolFinish int

const (
	toolFinishNone toolFinish = iota
	toolFinishArgsReady
)
