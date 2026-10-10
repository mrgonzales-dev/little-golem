package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"little-golem/src/app"
	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
	"little-golem/src/tools"
)

// stubTool returns a fixed payload, bypassing any real backend.
type stubTool struct{ content string }

func (s stubTool) Name() string        { return "stub" }
func (s stubTool) Description() string { return "stub" }
func (s stubTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (s stubTool) Run(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: s.content}, nil
}

func TestTruncateShortUntouched(t *testing.T) {
	out, truncated, omittedChars, omittedLines := tools.TruncateResult("hello", "")
	if truncated || omittedChars != 0 || omittedLines != 0 || out != "hello" {
		t.Fatalf("got %q %v %d %d", out, truncated, omittedChars, omittedLines)
	}
}

func TestTruncateLongAddsNotice(t *testing.T) {
	big := strings.Repeat("x", config.ToolMaxChars+3000)
	out, truncated, omitted, _ := tools.TruncateResult(big, "do y")
	if !truncated || omitted == 0 {
		t.Fatal("not truncated")
	}
	if len(out) > config.ToolMaxChars+512 {
		t.Fatalf("over budget: %d", len(out))
	}
	for _, want := range []string{"[truncated:", "omitted", "do y"} {
		if !strings.Contains(out, want) {
			t.Fatalf("notice missing %q: %q", want, out[len(out)-200:])
		}
	}
}

func TestTruncateManyShortLines(t *testing.T) {
	var b strings.Builder
	for i := 0; i < config.ToolMaxLines+100; i++ {
		b.WriteString("a\n")
	}
	out, truncated, _, _ := tools.TruncateResult(b.String(), "")
	if !truncated || !strings.Contains(out, "[truncated:") {
		t.Fatal("line budget not enforced")
	}
}

func TestExecuteGateCapsAnyTool(t *testing.T) {
	r := tools.NewRegistry(stubTool{content: strings.Repeat("z", config.ToolMaxChars+5000)})
	res := r.Execute(context.Background(), tools.Call{Name: "stub", Arguments: json.RawMessage(`{}`)})
	if res.IsError || !res.Truncated {
		t.Fatalf("got %+v", res)
	}
	if len(res.Content) > config.ToolMaxChars+512 || !strings.Contains(res.Content, "[truncated:") {
		t.Fatalf("gate failed: %d chars", len(res.Content))
	}
}

func TestBashHugeOutputCapped(t *testing.T) {
	r, _ := fileRegistry(t)
	r.Add(tools.NewBash(nil)) // Ask=nil: runs without the UI gate
	res := run(r, "bash", map[string]any{"command": "seq 1 5000"})
	if res.IsError {
		t.Fatalf("bash failed: %+v", res)
	}
	if !res.Truncated || len(res.Content) > config.ToolMaxChars+512 {
		t.Fatalf("bash not capped: truncated=%v len=%d", res.Truncated, len(res.Content))
	}
	if !strings.Contains(res.Content, "[truncated:") || !strings.Contains(res.Content, "head -n") {
		t.Fatalf("bash notice missing rerun hint: %q", res.Content[len(res.Content)-200:])
	}
}

func TestReadStaysWithinBudget(t *testing.T) {
	r, dir := readRegistry(t)
	var b strings.Builder
	for i := 1; i <= 2000; i++ {
		b.WriteString(strings.Repeat("y", 40) + "\n")
	}
	os.WriteFile(filepath.Join(dir, "wide.txt"), []byte(b.String()), 0o644)
	res := run(r, "read", map[string]any{"path": "wide.txt", "limit": 1000})
	if res.IsError {
		t.Fatalf("read failed: %+v", res)
	}
	if len(res.Content) > config.ToolMaxChars+512 {
		t.Fatalf("read over budget: %d", len(res.Content))
	}
}

func TestSizeBasedAutoCompactFiresWithStaleTokenCount(t *testing.T) {
	var bodies []string
	a := compactApp(fakeLlama(t, "- s", false, &bodies))
	// Simulate several capped tool results landing before usage arrives:
	// TokenUsed stays stale at 0, but the queued chars exceed the window.
	a.History = append(a.History, llama.ChatMessage{
		Role: "tool", Name: "bash", Content: strings.Repeat("q", config.CompactAt*4),
	})
	a.TokenUsed = 0
	if !app.ShouldCompactForSize(a) {
		t.Fatal("size pressure not detected")
	}
	if !app.ShouldAutoCompact(a) {
		t.Fatal("ShouldAutoCompact ignores char-size estimate")
	}
	cmd := app.Continue(a)
	if cmd == nil || !a.Compacting {
		t.Fatal("Continue streamed an oversized request instead of compacting")
	}
}

func TestRecordToolResultSyncsTokenEstimate(t *testing.T) {
	var bodies []string
	a := compactApp(fakeLlama(t, "- s", false, &bodies))
	a.History = nil
	a.TokenUsed = 0
	before := len(a.History)
	app.RecordToolResult(a, model.PendingCall{Name: "bash", Arguments: `{"command":"seq"}`, CallIndex: "0"}, strings.Repeat("w", 8000))
	if len(a.History) != before+1 {
		t.Fatalf("history %+v", a.History)
	}
	if a.TokenUsed < 8000/4 {
		t.Fatalf("TokenUsed %d not synced to tool output", a.TokenUsed)
	}
}
