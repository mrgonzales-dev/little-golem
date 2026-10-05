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
	"little-golem/src/ui"
)

func diffText(d *tools.Diff) string {
	var out []string
	for _, l := range d.Lines {
		sign := map[tools.DiffKind]string{tools.DiffCtx: " ", tools.DiffAdd: "+", tools.DiffDel: "-", tools.DiffSkip: "~"}[l.Kind]
		out = append(out, fmt.Sprintf("%s%d:%s", sign, l.Num, l.Text))
	}
	return strings.Join(out, "|")
}

func TestBuildDiffEditKeepsContextAndNumbers(t *testing.T) {
	_, dir := fileRegistry(t)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("1\n2\n3\n4\nold\nkeep\n7\n8\n9\n10\n"), 0o644)
	d, err := tools.BuildDiff("edit", `{"path":"f.txt","old_string":"4\nold\nkeep","new_string":"4\nnew\nextra\nkeep"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := " 4:4|-5:old|+5:new|+6:extra| 7:keep"
	if got := diffText(d); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if d.Adds != 2 || d.Dels != 1 || d.Matches != 1 || d.Created {
		t.Fatalf("%+v", d)
	}
}

func TestBuildDiffWriteCollapsesUnchangedRuns(t *testing.T) {
	_, dir := fileRegistry(t)
	var old, nw []string
	for i := 1; i <= 20; i++ {
		old = append(old, fmt.Sprintf("line %d", i))
		nw = append(nw, fmt.Sprintf("line %d", i))
	}
	nw[9] = "CHANGED"
	os.WriteFile(filepath.Join(dir, "w.txt"), []byte(strings.Join(old, "\n")+"\n"), 0o644)
	d, err := tools.BuildDiff("write", fmt.Sprintf(`{"path":"w.txt","content":%q}`, strings.Join(nw, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "~7:|" + // 7 unchanged lines skipped
		" 8:line 8| 9:line 9|-10:line 10|+10:CHANGED| 11:line 11| 12:line 12|~8:"
	if got := diffText(d); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if d.Created || d.Adds != 1 || d.Dels != 1 {
		t.Fatalf("%+v", d)
	}

	d, _ = tools.BuildDiff("write", `{"path":"brand/new.txt","content":"a\nb\n"}`)
	if !d.Created || d.Adds != 2 || d.Dels != 0 || diffText(d) != "+1:a|+2:b" {
		t.Fatalf("new file: %+v %s", d, diffText(d))
	}
	if _, err := tools.BuildDiff("write", `{"path":"../out.txt","content":"x"}`); err == nil {
		t.Fatal("diff for a path outside the project")
	}
}

func TestTranscriptShowsDiffAfterEdit(t *testing.T) {
	r, dir := fileRegistry(t)
	os.WriteFile(filepath.Join(dir, "g.py"), []byte("def greet():\n    print('hi')\n\ngreet()\n"), 0o644)
	a := app.New(nil).App
	a.Ready, a.Width, a.Height = true, 100, 40
	a.Tools = r
	a.Bypass = true
	pc := model.PendingCall{Name: "edit", Arguments: `{"path":"g.py","old_string":"print('hi')","new_string":"print('hello')"}`, CallIndex: "0"}
	a.Pending = []model.PendingCall{pc}

	msg := app.RunNext(a)().(app.ExecDoneMsg)
	app.RecordToolResult(a, msg.Call, msg.Content)

	e := a.Entries[len(a.Entries)-1]
	if e.Diff == nil || diffText(e.Diff) != "-2:print('hi')|+2:print('hello')" {
		t.Fatalf("diff must be captured before the file changes: %+v", e.Diff)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "g.py")); !strings.Contains(string(b), "print('hello')") {
		t.Fatalf("edit did not run: %q", b)
	}
	var b strings.Builder
	ui.RenderTool(&b, e, 80, false)
	out := stripANSI(b.String())
	for _, want := range []string{"edit: g.py", "+1 -1", "2 - ", "print('hi')", "2 + ", "print('hello')"} {
		if !strings.Contains(out, want) {
			t.Errorf("transcript missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "edited g.py") {
		t.Errorf("result text should give way to the diff:\n%s", out)
	}

	// A failed edit keeps the plain error panel.
	bad := model.PendingCall{Name: "edit", Arguments: `{"path":"g.py","old_string":"zzz","new_string":"y"}`, CallIndex: "1"}
	a.Pending = []model.PendingCall{bad}
	msg = app.RunNext(a)().(app.ExecDoneMsg)
	app.RecordToolResult(a, msg.Call, msg.Content)
	b.Reset()
	ui.RenderTool(&b, a.Entries[len(a.Entries)-1], 80, false)
	if out := stripANSI(b.String()); !strings.Contains(out, "not found") || strings.Contains(out, "+0 -0") {
		t.Errorf("failed edit render:\n%s", out)
	}
}

func TestLongDiffIsCutUntilExpanded(t *testing.T) {
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("row %d", i))
	}
	d, _ := tools.BuildDiff("write", fmt.Sprintf(`{"path":"long.txt","content":%q}`, strings.Join(lines, "\n")))
	e := model.Entry{Kind: model.EntryTool, Tool: "write", Cmd: "long.txt", Diff: d, Content: "created long.txt"}
	var b strings.Builder
	ui.RenderTool(&b, e, 80, false)
	short := stripANSI(b.String())
	if !strings.Contains(short, "more lines (ctrl+x to expand)") || strings.Contains(short, "row 39") || !strings.Contains(short, "new file") {
		t.Errorf("collapsed:\n%s", short)
	}
	b.Reset()
	ui.RenderTool(&b, e, 80, true)
	if full := stripANSI(b.String()); !strings.Contains(full, "row 39") || strings.Contains(full, "more lines") {
		t.Errorf("expanded:\n%s", full)
	}
}
