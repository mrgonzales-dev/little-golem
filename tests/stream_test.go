package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/tools"
	"little-golem/src/ui"
)

func TestPartialFieldsStreamsTopLevelStrings(t *testing.T) {
	vals, open := tools.PartialFields(`{"path":"a.py","content":"x\ny \"q\" \u00e9`)
	if vals["path"] != "a.py" || vals["content"] != "x\ny \"q\" é" || open != "content" {
		t.Fatalf("%q open=%q", vals, open)
	}
	// Text that merely looks like a key inside a value is not a key.
	vals, open = tools.PartialFields(`{"old_string":"say \"new_string\": 1","new_string":"b"}`)
	if vals["old_string"] != `say "new_string": 1` || vals["new_string"] != "b" || open != "" {
		t.Fatalf("%q open=%q", vals, open)
	}
	// Booleans and numbers are skipped, later keys still found.
	vals, _ = tools.PartialFields(`{"replace_all":true,"n":3,"path":"p"}`)
	if vals["path"] != "p" || len(vals) != 1 {
		t.Fatalf("%q", vals)
	}
	// Escapes cut mid-way wait for the rest instead of emitting junk.
	for _, cut := range []string{`{"c":"a\`, `{"c":"a\u00`, `{"c":"a\ud83d`, `{"c":"a\ud83d\u`, `{"c":"a\ud83d\ude`} {
		if v, _ := tools.PartialFields(cut); v["c"] != "a" {
			t.Errorf("%q -> %q, want %q", cut, v["c"], "a")
		}
	}
	if v, _ := tools.PartialFields(`{"c":"a\ud83d\ude00b"}`); v["c"] != "a😀b" {
		t.Errorf("surrogate pair: %q", v["c"])
	}
	for _, s := range []string{"", "{", `{"`, `{"a`, `{"a"`, `{"a":`, `not json`, `[1,2]`} {
		tools.PartialFields(s) // must not panic
	}
	if got := model.PartialSummary(`{"command":"ls -l /tm`); got != "ls -l /tm" {
		t.Errorf("bash draft summary: %q", got)
	}
}

// samples are real-shaped argument strings; every prefix of them is what
// the app sees at some moment while the model writes the call.
func streamSamples() map[string]string {
	enc := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	return map[string]string{
		"write":      enc(map[string]any{"path": "src/dice.py", "content": "import random\n\ndef roll():\n    \"\"\"Roll — two dice 🎲\"\"\"\n\treturn 1\n日本語\n"}),
		"edit":       enc(map[string]any{"path": "g.py", "old_string": "    print('hi')\n    x = 1", "new_string": "    print(\"hello\")\n    print('é')\n    x = 1", "replace_all": false}),
		"surrogates": `{"path":"s.txt","content":"a\ud83d\ude00b\u00e9\n\u65e5\u672c"}`,
	}
}

func TestEveryPrefixOfAStreamingCallIsSafeAndMonotonic(t *testing.T) {
	_, dir := fileRegistry(t)
	os.WriteFile(filepath.Join(dir, "g.py"), []byte("def f():\n    print('hi')\n    x = 1\n"), 0o644)
	for name, full := range streamSamples() {
		tool := "write"
		if name == "edit" {
			tool = "edit"
		}
		finalVals, _ := tools.PartialFields(full)
		var snap tools.FileSnap
		prevAdds := 0
		for i := 0; i <= len(full); i++ {
			prefix := full[:i]
			vals, _ := tools.PartialFields(prefix)
			for k, v := range vals {
				if !strings.HasPrefix(finalVals[k], v) {
					t.Fatalf("%s@%d: %s=%q is not a prefix of %q", name, i, k, v, finalVals[k])
				}
			}
			d := tools.PartialDiff(tool, prefix, &snap)
			if d == nil {
				continue
			}
			if !d.Streaming {
				t.Fatalf("%s@%d: not marked streaming", name, i)
			}
			if tool == "write" {
				if d.Adds < prevAdds {
					t.Fatalf("%s@%d: added lines went backwards %d -> %d", name, i, prevAdds, d.Adds)
				}
				prevAdds = d.Adds
			}
		}
	}
}

func TestPartialDiffWriteMatchesFinalDiffForNewFiles(t *testing.T) {
	_, dir := fileRegistry(t)
	full := streamSamples()["write"]
	var snap tools.FileSnap
	got := tools.PartialDiff("write", full, &snap)
	want, err := tools.BuildDiff("write", full)
	if err != nil {
		t.Fatal(err)
	}
	if diffText(got) != diffText(want) || !got.Created || got.Overwriting {
		t.Fatalf("stream and final diff disagree:\n%s\n%s", diffText(got), diffText(want))
	}

	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "dice.py"), []byte("old\n"), 0o644)
	snap = tools.FileSnap{}
	if d := tools.PartialDiff("write", full, &snap); d.Created || !d.Overwriting {
		t.Fatalf("existing file should be flagged as overwriting: %+v", d)
	}
}

func TestPartialDiffEditStreamsOldThenNewWithLineNumbers(t *testing.T) {
	_, dir := fileRegistry(t)
	os.WriteFile(filepath.Join(dir, "g.py"), []byte("def f():\n    print('hi')\n    x = 1\n"), 0o644)
	var snap tools.FileSnap

	// path done, old_string still streaming: removed lines appear, numbered.
	d := tools.PartialDiff("edit", `{"path":"g.py","old_string":"    print('hi')\n    x`, &snap)
	if diffText(d) != "-2:    print('hi')|-3:    x" || d.Matches != 0 {
		t.Fatalf("%s", diffText(d))
	}
	// old_string done, new_string streaming: added lines follow.
	d = tools.PartialDiff("edit", `{"path":"g.py","old_string":"    print('hi')","new_string":"    print('hello')\n    pri`, &snap)
	if diffText(d) != "-2:    print('hi')|+2:    print('hello')|+3:    pri" || d.Matches != 1 || d.Adds != 2 || d.Dels != 1 {
		t.Fatalf("%s %+v", diffText(d), d)
	}
	// Nothing yet to show.
	if d := tools.PartialDiff("edit", `{"pa`, &snap); d != nil {
		t.Fatalf("%+v", d)
	}
	if d := tools.PartialDiff("bash", `{"command":"ls"}`, &snap); d != nil {
		t.Fatal("bash has no diff")
	}
}

func TestDraftDiffRendersInTheTranscriptWhileStreaming(t *testing.T) {
	a := app.New(nil).App
	a.Ready, a.Width, a.Height = true, 100, 40
	ui.Layout(a)

	acc := a.BeginToolAcc(0)
	acc.Accumulate("write", `{"path":"hello.py","content":"print(`)
	acc.Accumulate("", `\"hi\")\nprint(`)
	ui.RenderEntries(a)
	out := stripANSI(a.Viewport.View())
	for _, want := range []string{"write: hello.py", "+2 -0", "new file", "writing", "1 + ", `print("hi")`, "2 + ", "print("} {
		if !strings.Contains(out, want) {
			t.Errorf("draft missing %q:\n%s", want, out)
		}
	}

	// More tokens: the same panel grows, no second copy of the header.
	acc.Accumulate("", `\"yo\")\n`)
	ui.RenderEntries(a)
	out = stripANSI(a.Viewport.View())
	if strings.Count(out, "write: hello.py") != 1 || !strings.Contains(out, `print("yo")`) {
		t.Errorf("after more tokens:\n%s", out)
	}
}

func TestLongStreamingDiffShowsOnlyTheTail(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "line %d\\n", i)
	}
	args := `{"path":"big.txt","content":"` + sb.String() + `lin`
	var snap tools.FileSnap
	e := model.Entry{Kind: model.EntryTool, Tool: "write", Cmd: "big.txt", Streaming: true, Draft: true, Diff: tools.PartialDiff("write", args, &snap)}

	var b strings.Builder
	ui.RenderTool(&b, e, 80, false)
	out := stripANSI(b.String())
	if !strings.Contains(out, "earlier lines") || strings.Contains(out, "line 1\n") || !strings.Contains(out, "60 + ") || !strings.Contains(out, "+61 -0") {
		t.Errorf("tail view:\n%s", out)
	}
	if rows := strings.Count(out, "\n"); rows > 20 {
		t.Errorf("panel grew to %d rows", rows)
	}

	b.Reset()
	ui.RenderTool(&b, e, 80, true)
	if full := stripANSI(b.String()); !strings.Contains(full, " 1 + line 1") || strings.Contains(full, "earlier lines") {
		t.Errorf("expanded:\n%s", full)
	}
}

func TestStreamedDraftHandsOverToTheApprovalDiff(t *testing.T) {
	r, dir := fileRegistry(t)
	os.WriteFile(filepath.Join(dir, "g.py"), []byte("def f():\n    print('hi')\n"), 0o644)
	a := app.New(nil).App
	a.Ready, a.Width, a.Height = true, 100, 40
	a.Tools = r
	ui.Layout(a)

	args := `{"path":"g.py","old_string":"    print('hi')","new_string":"    print('hello')"}`
	acc := a.BeginToolAcc(0)
	for i := 0; i < len(args); i += 7 { // token-sized chunks
		acc.Accumulate(map[bool]string{true: "edit"}[i == 0], args[i:min(len(args), i+7)])
		ui.RenderEntries(a) // must not panic or leave stale rows at any point
	}
	if out := stripANSI(a.Viewport.View()); !strings.Contains(out, "writing") || !strings.Contains(out, "2 + ") {
		t.Fatalf("draft at end of stream:\n%s", out)
	}

	// The call completes; the approval card shows the final diff.
	a.Current = &model.PendingCall{Name: "edit", Arguments: args}
	a.ToolAcc = nil
	ui.Layout(a)
	out := stripANSI(strings.Join(screen(a), "\n"))
	if !strings.Contains(out, "Edit file?") || !strings.Contains(out, "+1 -1") || strings.Contains(out, "writing") {
		t.Fatalf("approval card:\n%s", out)
	}
}

func TestWriteDraftHeaderShowsPathNotRawArgs(t *testing.T) {
	// The model may emit "content" before "path"; the header must never
	// echo raw JSON — the diff panel below already shows the content.
	if got := model.DraftSummary("write", `{"content":"\"\"\"doc\"\"\"\nimport csv`); got != "" {
		t.Fatalf("no path yet: %q", got)
	}
	if got := model.DraftSummary("write", `{"content":"x","path":"tracker/storage.py`); got != "tracker/storage.py" {
		t.Fatalf("path mid-stream: %q", got)
	}
	if got := model.DraftSummary("edit", `{"old_string":"x","new_string":"y"}`); got != "" {
		t.Fatalf("edit without path: %q", got)
	}
	if got := model.DraftSummary("bash", `{"command":"ls -l`); got != "ls -l" {
		t.Fatalf("bash keeps its command: %q", got)
	}

	// Before the path arrives the header is a bare "write" (no ": ").
	var b strings.Builder
	ui.RenderTool(&b, model.Entry{Kind: model.EntryTool, Tool: "write", Streaming: true, Draft: true,
		Diff: tools.PartialDiff("write", `{"content":"x`, nil)}, 80, false)
	out := stripANSI(b.String())
	if strings.Contains(out, `"content"`) || strings.Contains(out, "write:") || !strings.Contains(out, "write") {
		t.Fatalf("header:\n%s", out)
	}
}
