package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"little-golem/src/app"
	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
	"little-golem/src/tools"
)

func stripANSI(s string) string { return ansi.Strip(s) }

// fakeLlama answers non-streaming requests with the summary (or an HTTP
// error when fail is set) and ends streaming ones immediately. Request
// bodies of the non-streaming calls are collected in *bodies.
func fakeLlama(t *testing.T, summary string, fail bool, bodies *[]string) *llama.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct{ Stream bool }
		json.Unmarshal(raw, &req)
		if req.Stream {
			io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		*bodies = append(*bodies, string(raw))
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": summary}}}})
	}))
	t.Cleanup(ts.Close)
	srv := &llama.Server{Base: ts.URL}
	prev := app.LoadServer
	t.Cleanup(func() { app.LoadServer = prev })
	app.LoadServer = func(path string) (*llama.Server, error) {
		if path != config.CompactModel {
			t.Errorf("compaction loaded %s", path)
		}
		return srv, nil
	}
	return srv
}

func compactApp(srv *llama.Server) *model.App {
	a := app.New(srv).App
	a.Ready, a.Width, a.Height = true, 80, 24
	a.Tools = tools.NewRegistry(tools.NewBash(nil))
	a.History = []llama.ChatMessage{
		{Role: "user", Content: "fix the parser"},
		{Role: "assistant", Content: "looking"},
		{Role: "assistant", ToolCalls: []llama.ToolCall{{Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{"bash", `{"command":"cat parser.go"}`}}}},
		{Role: "tool", Name: "bash", Content: strings.Repeat("x", 5000)},
	}
	a.Request = "fix the parser"
	return a
}

func TestSlashCompactReplacesHistory(t *testing.T) {
	var bodies []string
	a := compactApp(fakeLlama(t, "- goal: fix the parser", false, &bodies))
	a.Input.SetValue("/compact keep file names")
	cmd, handled := app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd == nil || !a.Compacting || !a.Working() || a.Input.Value() != "" {
		t.Fatalf("compaction did not start: handled=%v compacting=%v", handled, a.Compacting)
	}
	msg := cmd().(app.CompactDoneMsg)
	if cmd := app.FinishCompact(a, msg); cmd != nil {
		t.Fatal("manual compaction must not start a turn")
	}
	if a.Compacting || a.Err != nil || a.Busy {
		t.Fatalf("state after compact: %+v", a.Err)
	}
	if len(a.History) != 2 || a.History[0].Role != "user" || !strings.Contains(a.History[0].Content, "- goal: fix the parser") || a.History[1].Role != "assistant" {
		t.Fatalf("history %+v", a.History)
	}
	if n := len(a.Entries); n != 1 || a.Entries[0].Kind != model.EntryNote || !strings.Contains(a.Entries[0].Content, "context compacted") {
		t.Fatalf("entries %+v", a.Entries)
	}
	body := bodies[0]
	if !strings.Contains(body, "keep file names") || !strings.Contains(body, "cat parser.go") {
		t.Fatalf("summarizer request missing focus or transcript: %s", body)
	}
	if strings.Contains(body, strings.Repeat("x", 3000)) {
		t.Fatal("tool result was not clipped")
	}
}

func TestCompactIgnoredWhileWorkingOrEmpty(t *testing.T) {
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	a.Busy = true
	a.Input.SetValue("/compact")
	if cmd, _ := app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || a.Compacting || len(a.Entries) != 0 {
		t.Fatal("compacted while busy")
	}
	a.Busy, a.History = false, nil
	a.Input.SetValue("/compact")
	if cmd, _ := app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || a.Notice != "nothing to compact" {
		t.Fatalf("empty history: notice %q", a.Notice)
	}
}

func TestAutoCompactBetweenToolRounds(t *testing.T) {
	var bodies []string
	a := compactApp(fakeLlama(t, "- did stuff", false, &bodies))

	a.TokenUsed = config.CompactAt - 1
	app.RunNext(a)
	if a.Compacting || !a.Busy {
		t.Fatal("compacted below the threshold")
	}

	a = compactApp(fakeLlama(t, "- did stuff", false, &bodies))
	a.TokenUsed = config.CompactAt
	cmd := app.RunNext(a)
	if !a.Compacting || cmd == nil {
		t.Fatal("no auto compaction at 50% of the window")
	}
	next := app.FinishCompact(a, cmd().(app.CompactDoneMsg))
	if next == nil || !a.Busy {
		t.Fatal("turn did not resume after compaction")
	}
	if len(a.History) != 1 || !strings.Contains(a.History[0].Content, "- did stuff") || !strings.HasSuffix(a.History[0].Content, "fix the parser") {
		t.Fatalf("history %+v", a.History)
	}
	if a.TokenUsed >= config.CompactAt {
		t.Fatalf("TokenUsed %d not reset", a.TokenUsed)
	}
}

func TestAutoCompactAfterFinalAnswer(t *testing.T) {
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	m := &app.M{App: a}
	a.TokenUsed = config.CompactAt
	_, cmd := m.Update(app.StreamDoneMsg{})
	if cmd == nil || !a.Compacting {
		t.Fatal("no auto compaction when the turn ended over the threshold")
	}
}

func TestAutoCompactFailureResumesAndDisables(t *testing.T) {
	var bodies []string
	a := compactApp(fakeLlama(t, "", true, &bodies))
	a.TokenUsed = config.CompactAt
	cmd := app.RunNext(a)
	next := app.FinishCompact(a, cmd().(app.CompactDoneMsg))
	if next == nil || !a.Busy || a.Err == nil || !a.CompactFailed || len(a.History) != 4 {
		t.Fatalf("failure handling: err=%v failed=%v hist=%d", a.Err, a.CompactFailed, len(a.History))
	}
	if app.ShouldAutoCompact(a) {
		t.Fatal("auto compaction should stay off after a failure")
	}
}
