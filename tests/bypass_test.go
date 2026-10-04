package tests

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/tools"
)

func newBypassApp() *model.App {
	a := app.New(nil).App
	a.Ready, a.Width, a.Height = true, 80, 24
	a.Tools = tools.NewRegistry(tools.NewBash(nil))
	return a
}

func TestBypassToggles(t *testing.T) {
	a := newBypassApp()
	app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !a.Bypass {
		t.Fatal("shift+tab did not enable bypass")
	}
	app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if a.Bypass {
		t.Fatal("shift+tab did not disable bypass")
	}
	a.Input.SetValue("/super")
	app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !a.Bypass || a.Input.Value() != "" || len(a.Entries) != 0 {
		t.Fatal("/super should toggle bypass without being sent")
	}
}

func TestBypassSkipsApproval(t *testing.T) {
	a := newBypassApp()
	a.Pending = []model.PendingCall{{Name: "bash", Arguments: `{"command":"echo hi"}`}}
	if app.RunNext(a); a.Current == nil {
		t.Fatal("without bypass the card should appear")
	}

	a = newBypassApp()
	a.Bypass = true
	a.Pending = []model.PendingCall{{Name: "bash", Arguments: `{"command":"echo hi"}`}}
	cmd := app.RunNext(a)
	if a.Current != nil || cmd == nil {
		t.Fatal("bypass should run without a card")
	}
	if msg := cmd().(app.ExecDoneMsg); msg.Content != "hi" {
		t.Fatalf("got %q", msg.Content)
	}
}
