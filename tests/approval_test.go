package tests

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/ui"
)

func key(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	return tea.KeyPressMsg{Code: map[string]rune{"down": tea.KeyDown, "up": tea.KeyUp, "enter": tea.KeyEnter, "esc": tea.KeyEscape}[s]}
}

func screen(a *model.App) []string {
	return strings.Split(fmt.Sprint(ui.View(a).Content), "\n")
}

func TestApprovalFlow(t *testing.T) {
	m := app.New(nil)
	a := m.App
	a.Ready, a.Width, a.Height = true, 80, 24
	ui.Layout(a)
	base := a.Viewport.Height()

	a.Current = &model.PendingCall{Name: "bash", Arguments: `{"command":"git status --short && ls -la /home/mrg/models/little-golem/src"}`}
	ui.Layout(a)
	if a.Viewport.Height() >= base {
		t.Fatal("viewport did not shrink for card")
	}
	ls := screen(a)
	if len(ls) != 24 {
		t.Fatalf("height %d", len(ls))
	}
	t.Log("\n" + ansi.Strip(strings.Join(ls[len(ls)-12:], "\n")))

	app.HandleKey(a, key("down"))
	app.HandleKey(a, key("down"))
	if a.ApprovalSel != 2 {
		t.Fatalf("sel %d", a.ApprovalSel)
	}
	app.HandleKey(a, key("up"))
	app.HandleKey(a, key("enter")) // sel 1 = allow for session
	if a.Current != nil || !a.Approved[`git status --short && ls -la /home/mrg/models/little-golem/src`] {
		t.Fatal("session approval not recorded")
	}
	if a.Viewport.Height() != base {
		t.Fatal("viewport not restored")
	}

	// reason flow
	a.Current = &model.PendingCall{Name: "bash", Arguments: `{"command":"rm -rf x"}`}
	ui.Layout(a)
	app.HandleKey(a, key("r"))
	if !a.Reasoning {
		t.Fatal("reason mode not entered")
	}
	for _, c := range "too risky" {
		app.HandleKey(a, key(string(c)))
	}
	t.Log("\n" + ansi.Strip(strings.Join(screen(a)[16:], "\n")))
	cmd := func() tea.Cmd { c, _ := app.HandleKey(a, key("enter")); return c }()
	msg := cmd().(app.ExecDoneMsg)
	if msg.Content != "the user denied this command: too risky" {
		t.Fatalf("got %q", msg.Content)
	}
	if a.Denied["rm -rf x"] {
		t.Fatal("reason denial should not be remembered")
	}
}
