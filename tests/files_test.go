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
	"little-golem/src/model"
	"little-golem/src/tools"
	"little-golem/src/ui"
)

func fileRegistry(t *testing.T) (*tools.Registry, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := config.WorkDir
	config.WorkDir = dir
	t.Cleanup(func() { config.WorkDir = old })
	return tools.NewRegistry(tools.NewEdit(), tools.NewWrite()), dir
}

func run(r *tools.Registry, name string, args map[string]any) tools.Result {
	raw, _ := json.Marshal(args)
	return r.Execute(context.Background(), tools.Call{Name: name, Arguments: raw})
}

func TestWriteCreatesAndOverwrites(t *testing.T) {
	r, dir := fileRegistry(t)
	res := run(r, "write", map[string]any{"path": "a/b/new.txt", "content": "one\ntwo\n"})
	if res.IsError || !strings.Contains(res.Content, "created a/b/new.txt (2 lines") {
		t.Fatalf("got %+v", res)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a/b/new.txt")); string(b) != "one\ntwo\n" {
		t.Fatalf("file %q", b)
	}
	res = run(r, "write", map[string]any{"path": "a/b/new.txt", "content": "x"})
	if !strings.HasPrefix(res.Content, "overwrote") {
		t.Fatalf("got %+v", res)
	}
}

func TestEditReplacesExactText(t *testing.T) {
	r, dir := fileRegistry(t)
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("a := 1\nb := 1\nc := 2\n"), 0o600)

	if res := run(r, "edit", map[string]any{"path": "f.go", "old_string": "c := 2", "new_string": "c := 3"}); res.IsError {
		t.Fatalf("got %+v", res)
	}
	if b, _ := os.ReadFile(p); string(b) != "a := 1\nb := 1\nc := 3\n" {
		t.Fatalf("file %q", b)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v lost", st.Mode())
	}

	cases := map[string]map[string]any{
		"not found":     {"path": "f.go", "old_string": "zzz", "new_string": "y"},
		"ambiguous":     {"path": "f.go", "old_string": ":= 1", "new_string": ":= 9"},
		"empty old":     {"path": "f.go", "old_string": "", "new_string": "y"},
		"same":          {"path": "f.go", "old_string": "a", "new_string": "a"},
		"missing file":  {"path": "nope.go", "old_string": "a", "new_string": "b"},
		"missing args":  {},
		"outside":       {"path": "../x.go", "old_string": "a", "new_string": "b"},
		"absolute path": {"path": "/etc/passwd", "old_string": "root", "new_string": "r"},
	}
	for name, args := range cases {
		if res := run(r, "edit", args); !res.IsError {
			t.Errorf("%s: expected error, got %+v", name, res)
		}
	}
	if b, _ := os.ReadFile(p); string(b) != "a := 1\nb := 1\nc := 3\n" {
		t.Fatalf("failed edits changed the file: %q", b)
	}

	if res := run(r, "edit", map[string]any{"path": "f.go", "old_string": ":= 1", "new_string": ":= 9", "replace_all": true}); res.IsError || !strings.Contains(res.Content, "2 replaced") {
		t.Fatalf("got %+v", res)
	}
}

func TestWriteStaysInsideWorkDir(t *testing.T) {
	r, dir := fileRegistry(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../escape.txt", filepath.Join(outside, "abs.txt"), "link/escape.txt", "a/../../escape.txt"} {
		if res := run(r, "write", map[string]any{"path": p, "content": "x"}); !res.IsError || !strings.Contains(res.Content, "outside the project folder") {
			t.Errorf("%s: got %+v", p, res)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside the project: %v", entries)
	}
}

func TestFileToolsNeedApprovalAndKeyByCall(t *testing.T) {
	r, _ := fileRegistry(t)
	if !r.NeedsApproval("edit") || !r.NeedsApproval("write") {
		t.Fatal("edit and write must be approval-gated")
	}
	a := model.PendingCall{Name: "edit", Arguments: `{"path":"a","old_string":"x","new_string":"y"}`}
	b := model.PendingCall{Name: "edit", Arguments: `{"path":"a","old_string":"x","new_string":"z"}`}
	if a.Key() == b.Key() {
		t.Fatal("different edits share a session approval")
	}
	if a.Summary() != "a" {
		t.Fatalf("summary %q", a.Summary())
	}
}

func TestEditApprovalCardShowsDiff(t *testing.T) {
	_, dir := fileRegistry(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "x.go"), []byte("package x\n\nfunc a() {\n\told line\n\tkeep\n}\n"), 0o644)

	a := app.New(nil).App
	a.Ready, a.Width, a.Height = true, 80, 30
	a.Current = &model.PendingCall{Name: "edit", Arguments: `{"path":"src/x.go","old_string":"\told line\n\tkeep","new_string":"\tnew line\n\tsecond\n\tkeep"}`}
	ui.Layout(a)
	out := stripANSI(strings.Join(screen(a), "\n"))
	for _, want := range []string{"Edit file?", "src/x.go", "+2 -1", "4 - ", "old line", "4 + ", "new line", "+ ", "second"} {
		if !strings.Contains(out, want) {
			t.Errorf("card missing %q:\n%s", want, out)
		}
	}

	a.Current = &model.PendingCall{Name: "write", Arguments: `{"path":"n.txt","content":"a\nb\nc"}`}
	ui.Layout(a)
	if out := stripANSI(strings.Join(screen(a), "\n")); !strings.Contains(out, "Create file?") || !strings.Contains(out, "n.txt") || !strings.Contains(out, "+3 -0") {
		t.Errorf("write card:\n%s", out)
	}

	os.WriteFile(filepath.Join(dir, "n.txt"), []byte("a\nB\nc\n"), 0o644)
	a.ResetDiff() // the app does this whenever a call is answered
	a.Current = &model.PendingCall{Name: "write", Arguments: `{"path":"n.txt","content":"a\nb\nc"}`}
	ui.Layout(a)
	if out := stripANSI(strings.Join(screen(a), "\n")); !strings.Contains(out, "Overwrite file?") || !strings.Contains(out, "+1 -1") {
		t.Errorf("overwrite card should diff against the old file:\n%s", out)
	}

	a.Current = &model.PendingCall{Name: "edit", Arguments: `{"path":"src/x.go","old_string":"nope","new_string":"y"}`}
	ui.Layout(a)
	if out := stripANSI(strings.Join(screen(a), "\n")); !strings.Contains(out, "old_string was not found") {
		t.Errorf("missing warning:\n%s", out)
	}
}
