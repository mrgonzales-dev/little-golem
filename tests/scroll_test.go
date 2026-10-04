package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/ui"
)

func TestScrollFollow(t *testing.T) {
	m := app.New(nil)
	a := m.App
	a.Ready = true
	a.Width, a.Height = 100, 30
	for i := 0; i < 12; i++ {
		a.Entries = append(a.Entries,
			model.Entry{Kind: model.EntryUser, Content: fmt.Sprintf("question %d", i)},
			model.Entry{Kind: model.EntryAssistant, Content: "answer line one\n\nsecond paragraph here with some words", Started: time.Now(), Ended: time.Now()})
	}
	ui.Layout(a)
	out := ui.View(a)
	s := fmt.Sprint(out.Content)
	ls := strings.Split(s, "\n")
	if len(ls) != 30 {
		t.Errorf("height %d", len(ls))
	}
	for i, l := range ls {
		if w := lipgloss.Width(l); w != 100 {
			t.Errorf("line %d width %d", i, w)
		}
	}
	if !a.Viewport.AtBottom() || !a.Follow {
		t.Fatal("not following")
	}
	// wheel up
	app.Scroll(a, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if a.Follow || a.Viewport.AtBottom() {
		t.Fatal("wheel up did not unfollow")
	}
	off := a.Viewport.YOffset()
	a.Busy = true
	a.TurnStart = time.Now().Add(-32 * time.Second)
	a.TurnDone = 695
	a.TokenUsed = 12400
	ui.RenderEntries(a)
	if a.Viewport.YOffset() != off {
		t.Fatal("offset moved while not following")
	}
	for _, l := range strings.Split(fmt.Sprint(ui.View(a).Content), "\n") {
		if ansi.StringWidth(l) != 100 {
			t.Errorf("width %d", ansi.StringWidth(l))
		}
	}
	app.Scroll(a, tea.KeyPressMsg{Code: tea.KeyPgDown})
	app.Scroll(a, tea.KeyPressMsg{Code: tea.KeyEnd, Text: ""})
	if !a.Follow {
		t.Fatal("end did not refollow")
	}
}
