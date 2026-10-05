package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/app"
	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
	"little-golem/src/session"
)

func inTempProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := config.WorkDir
	config.WorkDir = dir
	t.Cleanup(func() { config.WorkDir = old })
	return dir
}

func TestSessionSavedAndRestored(t *testing.T) {
	dir := inTempProject(t)
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	a.ModelIdx, a.TokenUsed = 1, 1234
	a.Input.SetValue("hello")
	app.Send(a)

	if b, err := os.ReadFile(filepath.Join(dir, ".little-golem", ".gitignore")); err != nil || string(b) != "*\n" {
		t.Fatalf(".gitignore: %q %v", b, err)
	}
	s, err := session.Load()
	if err != nil || s == nil {
		t.Fatalf("load: %v %v", s, err)
	}
	if s.ModelIdx() != 1 || s.TokenUsed != 1234 || len(s.History) != 5 || s.History[4].Content != "hello" {
		t.Fatalf("restored %+v", s)
	}

	b := compactApp(fakeLlama(t, "s", false, &bodies))
	b.History, b.Entries = nil, nil
	s.Apply(b)
	if b.ModelIdx != 1 || len(b.History) != 5 || len(b.Entries) != 1 || b.Entries[0].Content != "hello" {
		t.Fatalf("applied: idx=%d hist=%d entries=%+v", b.ModelIdx, len(b.History), b.Entries)
	}
}

func TestSessionDropsUnfinishedToolRound(t *testing.T) {
	inTempProject(t)
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	call := llama.ToolCall{ID: "call_0", Type: "function"}
	a.History = []llama.ChatMessage{
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []llama.ToolCall{call, call}},
		{Role: "tool", Content: "one"},
	}
	a.Entries = []model.Entry{{Kind: model.EntryUser, Content: "go"}, {Kind: model.EntryAssistant, Streaming: true}}
	if err := session.Save(a); err != nil {
		t.Fatal(err)
	}
	s, _ := session.Load()
	if len(s.History) != 1 || len(s.Entries) != 1 {
		t.Fatalf("history %+v entries %+v", s.History, s.Entries)
	}
}

func TestSessionCorruptFileIsMovedAside(t *testing.T) {
	dir := inTempProject(t)
	os.MkdirAll(filepath.Join(dir, ".little-golem"), 0o755)
	os.WriteFile(filepath.Join(dir, ".little-golem", "session.json"), []byte("{nope"), 0o644)
	if s, err := session.Load(); s != nil || err == nil || !strings.Contains(err.Error(), "session.json.bad") {
		t.Fatalf("got %v %v", s, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".little-golem", "session.json.bad")); err != nil {
		t.Fatal(err)
	}
}

func TestSlashNewClearsSession(t *testing.T) {
	dir := inTempProject(t)
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	a.Entries = []model.Entry{{Kind: model.EntryUser, Content: "x"}}
	if err := session.Save(a); err != nil {
		t.Fatal(err)
	}
	a.Input.SetValue("/new")
	app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(a.History) != 0 || len(a.Entries) != 0 || a.TokenUsed != 0 || a.Notice != "new session" {
		t.Fatalf("state not cleared: %+v", a.Notice)
	}
	if _, err := os.Stat(filepath.Join(dir, ".little-golem", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("session file still there: %v", err)
	}
}
