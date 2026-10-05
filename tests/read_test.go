package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/tools"
)

func readRegistry(t *testing.T) (*tools.Registry, string) {
	r, dir := fileRegistry(t)
	r.Add(tools.NewRead())
	return r, dir
}

func TestReadReturnsWholeFileWithLineNumbers(t *testing.T) {
	r, dir := readRegistry(t)
	os.WriteFile(filepath.Join(dir, "PLAN.md"), []byte("# Plan\nsee PLAN.md\nlast\n"), 0o644)
	res := run(r, "read", map[string]any{"path": "PLAN.md"})
	want := "1| # Plan\n2| see PLAN.md\n3| last\n(end of file, 3 lines)"
	if res.IsError || res.Content != want {
		t.Fatalf("got %+v", res)
	}
	// The bare-path form small models emit.
	if res := r.Execute(t.Context(), tools.Call{Name: "read", Arguments: []byte("PLAN.md")}); res.IsError || !strings.Contains(res.Content, "3| last") {
		t.Fatalf("bare path: %+v", res)
	}
}

func TestReadPaginates(t *testing.T) {
	r, dir := readRegistry(t)
	var b strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(b.String()), 0o644)

	res := run(r, "read", map[string]any{"path": "big.txt"})
	if !strings.Contains(res.Content, "  1| line 1\n") || !strings.Contains(res.Content, "250| line 250\n") || strings.Contains(res.Content, "line 251") ||
		!strings.HasSuffix(res.Content, "(showing lines 1-250 of 300; call read again with offset=251 to continue)") {
		t.Fatalf("first page wrong:\n%s", res.Content[len(res.Content)-200:])
	}
	res = run(r, "read", map[string]any{"path": "big.txt", "offset": 251})
	if !strings.HasPrefix(res.Content, "251| line 251\n") || !strings.HasSuffix(res.Content, "(end of file, 300 lines)") {
		t.Fatalf("second page wrong: %q", res.Content)
	}
	res = run(r, "read", map[string]any{"path": "big.txt", "offset": 10, "limit": 2})
	if !strings.HasPrefix(res.Content, " 10| line 10\n 11| line 11\n(showing lines 10-11 of 300") {
		t.Fatalf("limit wrong: %q", res.Content)
	}
	if res := run(r, "read", map[string]any{"path": "big.txt", "offset": 999}); !res.IsError || !strings.Contains(res.Content, "past the end") {
		t.Fatalf("got %+v", res)
	}
}

func TestReadEdgeCases(t *testing.T) {
	r, dir := readRegistry(t)
	os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("ab\x00cd"), 0o644)
	os.WriteFile(filepath.Join(dir, "long.txt"), []byte(strings.Repeat("x", 2000)+"\n"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "a.py"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub", "deep"), 0o755)

	if res := run(r, "read", map[string]any{"path": "empty.txt"}); res.Content != "(empty file)" {
		t.Errorf("empty: %+v", res)
	}
	if res := run(r, "read", map[string]any{"path": "bin.dat"}); !res.IsError || !strings.Contains(res.Content, "binary") {
		t.Errorf("binary: %+v", res)
	}
	if res := run(r, "read", map[string]any{"path": "long.txt"}); len(res.Content) > 600 || !strings.Contains(res.Content, "…") {
		t.Errorf("long line not cut: %d bytes", len(res.Content))
	}
	if res := run(r, "read", map[string]any{"path": "sub"}); res.Content != "a.py\ndeep/" {
		t.Errorf("dir: %+v", res)
	}
	if res := run(r, "read", map[string]any{"path": "nope.txt"}); !res.IsError || !strings.Contains(res.Content, "glob") {
		t.Errorf("missing: %+v", res)
	}
	for _, p := range []string{"../x", "/etc/passwd"} {
		if res := run(r, "read", map[string]any{"path": p}); !res.IsError || !strings.Contains(res.Content, "outside the project folder") {
			t.Errorf("%s: %+v", p, res)
		}
	}
}

func TestGrepAndGlobUseTheIndex(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "calc.py"), []byte("def add(a, b):\n    return a + b\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hello\n"), 0o644)
	if err := tools.InitFFF(dir, 10000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tools.ShutdownFFF)

	r := tools.NewRegistry(tools.NewGrep(), tools.NewGlob())
	res := run(r, "glob", map[string]any{"pattern": "*.py"})
	if res.IsError || res.Content != "src/calc.py" {
		t.Fatalf("glob: %+v", res)
	}
	if res := run(r, "glob", map[string]any{"pattern": "*.zzz"}); res.Content != "(no matches)" {
		t.Fatalf("glob none: %+v", res)
	}
	res = run(r, "grep", map[string]any{"query": "return"})
	if res.IsError || !strings.Contains(res.Content, "src/calc.py:2:") {
		t.Fatalf("grep: %+v", res)
	}
	if res := run(r, "grep", map[string]any{"query": "nonexistentword"}); res.Content != "(no matches)" {
		t.Fatalf("grep none: %+v", res)
	}
}

func TestRepeatedReadOnlyCallsAreBlockedUntilSomethingChanges(t *testing.T) {
	r, dir := readRegistry(t)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644)
	var bodies []string
	a := app.New(fakeLlama(t, "", false, &bodies)).App
	a.Ready, a.Width, a.Height = true, 80, 24
	a.Tools = r
	call := model.PendingCall{Name: "read", Arguments: `{"path":"a.txt"}`}

	a.Pending = []model.PendingCall{call, call}
	app.RunNext(a)
	if n := len(a.History); n != 2 || !strings.Contains(a.History[0].Content, "1| hi") || !strings.HasPrefix(a.History[1].Content, "You already ran this exact call") {
		t.Fatalf("history %+v", a.History)
	}

	// Three blocked repeats in a row stop the turn instead of looping forever.
	a.Busy = false // the first RunNext started a (fake) follow-up stream
	a.Pending = []model.PendingCall{call, call, call, call}
	if cmd := app.RunNext(a); cmd != nil || a.Busy || !strings.HasPrefix(a.Notice, "stopped:") || len(a.Pending) != 0 {
		t.Fatalf("loop not stopped: notice=%q busy=%v", a.Notice, a.Busy)
	}

	// A write resets the memory: the same read is allowed again.
	a.Bypass = true
	a.Notice = ""
	a.Pending = []model.PendingCall{{Name: "write", Arguments: `{"path":"a.txt","content":"changed\n"}`}}
	cmd := app.RunNext(a)
	cmd()
	a.Pending = []model.PendingCall{call}
	app.RunNext(a)
	if last := a.History[len(a.History)-1].Content; !strings.Contains(last, "1| changed") {
		t.Fatalf("read after write blocked or stale: %q", last)
	}
}
