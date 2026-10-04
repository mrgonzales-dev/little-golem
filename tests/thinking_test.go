package tests

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/ui"
)

func TestThinkingStreamToggle(t *testing.T) {
	a := app.New(nil).App
	a.Ready, a.Width, a.Height, a.Busy, a.Follow = true, 90, 30, true, true
	a.Entries = []model.Entry{
		{Kind: model.EntryUser, Content: "hi"},
		{Kind: model.EntryAssistant, Streaming: true, Started: time.Now(), Reasoning: "let me work out the answer"},
	}
	ui.Layout(a)
	view := func() string { return ansi.Strip(a.Viewport.GetContent()) }
	if strings.Contains(view(), "work out the answer") {
		t.Fatal("thinking should be hidden by default")
	}
	app.HandleKey(a, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if !strings.Contains(view(), "work out the answer") {
		t.Fatal("ctrl+o should stream the reasoning live")
	}
	app.AppendDelta(a, " step by step", "", nil)
	if !strings.Contains(view(), "step by step") {
		t.Fatal("new reasoning tokens should appear live")
	}
	app.HandleKey(a, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if strings.Contains(view(), "step by step") {
		t.Fatal("ctrl+o again should hide it")
	}
}
