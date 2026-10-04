package tests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/ui"
)

func TestToolCards(t *testing.T) {
	a := app.New(nil).App
	a.Ready, a.Width, a.Height = true, 90, 40
	long := make([]string, 25)
	for i := range long {
		long[i] = fmt.Sprintf("line %d", i)
	}
	a.Entries = []model.Entry{
		{Kind: model.EntryTool, Tool: "bash", Cmd: "ls -la src/", Content: "total 8\ndrwxr-xr-x app\ndrwxr-xr-x ui"},
		{Kind: model.EntryTool, Tool: "bash", Cmd: "cat big.txt", Content: strings.Join(long, "\n")},
		{Kind: model.EntryTool, Tool: "bash", Cmd: "false", Content: "$ false\n(no output)\n(exit status 1)"},
		{Kind: model.EntryTool, Tool: "bash", Cmd: "rm -rf x", Content: "the user denied this command: too risky"},
		{Kind: model.EntryTool, Tool: "bash", Cmd: "sleep 5", Streaming: true},
	}
	ui.Layout(a)
	a.Viewport.GotoTop()
	a.Viewport.SetYOffset(0)
	for _, l := range strings.Split(ansi.Strip(a.Viewport.GetContent()), "\n") {
		t.Log(l)
	}
	got := ansi.Strip(a.Viewport.GetContent())
	for _, want := range []string{"bash: ls -la src/", "drwxr-xr-x app", "+15 more lines", "bash: false", "✗", "✓", "running", "too risky"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "$ false") {
		t.Error("command echo should be stripped from output")
	}
}
