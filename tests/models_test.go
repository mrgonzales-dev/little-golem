package tests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/app"
	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
)

// fakeLoader replaces app.LoadServer: every load records the path and
// returns a server that reports healthy, except paths listed in fail.
func fakeLoader(t *testing.T, fail ...string) *[]string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(ts.Close)
	var loaded []string
	prev := app.LoadServer
	t.Cleanup(func() { app.LoadServer = prev })
	app.LoadServer = func(path string) (*llama.Server, error) {
		loaded = append(loaded, path)
		for _, f := range fail {
			if f == path {
				return nil, errors.New("no such model")
			}
		}
		return &llama.Server{Base: ts.URL}, nil
	}
	return &loaded
}

func modelApp() *model.App {
	a := app.New(&llama.Server{ToolDefs: []map[string]any{{"type": "function"}}}).App
	a.Ready, a.Width, a.Height = true, 80, 24
	a.History = []llama.ChatMessage{{Role: "user", Content: "hi"}}
	return a
}

func runSlash(a *model.App, line string) tea.Cmd {
	a.Input.SetValue(line)
	cmd, _ := app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEnter})
	return cmd
}

func TestSlashModelsSwitchesAndKeepsHistory(t *testing.T) {
	loaded := fakeLoader(t)
	a := modelApp()
	cmd := runSlash(a, "/models spark")
	if cmd == nil || a.Switching != "spark" || !a.Working() || a.Input.Value() != "" {
		t.Fatalf("switch did not start: switching=%q", a.Switching)
	}
	app.FinishSwitch(a, cmd().(app.ModelSwitchedMsg))
	if a.ModelName() != "spark" || a.Switching != "" || a.Working() || a.Err != nil {
		t.Fatalf("after switch: model=%s switching=%q err=%v", a.ModelName(), a.Switching, a.Err)
	}
	if len(*loaded) != 1 || !strings.HasSuffix((*loaded)[0], "Spark-X2.5-1.7B-Q4_K_M.gguf") {
		t.Fatalf("loaded %v", *loaded)
	}
	if len(a.Server.ToolDefs) != 1 || len(a.History) != 1 {
		t.Fatalf("tool defs / history not carried over: %v %v", a.Server.ToolDefs, a.History)
	}
	if n := a.Entries[len(a.Entries)-1]; n.Kind != model.EntryNote || !strings.Contains(n.Content, "spark") {
		t.Fatalf("no switch note: %+v", n)
	}
}

func TestCtrlMAndBareModelsCycle(t *testing.T) {
	fakeLoader(t)
	a := modelApp()
	cmd, handled := app.HandleKey(a, tea.KeyPressMsg{Code: 'm', Mod: tea.ModCtrl})
	if !handled || cmd == nil {
		t.Fatal("ctrl+m did not switch")
	}
	app.FinishSwitch(a, cmd().(app.ModelSwitchedMsg))
	if a.ModelName() != "spark" {
		t.Fatalf("ctrl+m went to %s", a.ModelName())
	}
	app.FinishSwitch(a, runSlash(a, "/models")().(app.ModelSwitchedMsg))
	if a.ModelName() != config.Models[0].Name {
		t.Fatalf("/models wrapped to %s", a.ModelName())
	}
}

func TestSwitchRefusals(t *testing.T) {
	loaded := fakeLoader(t)
	a := modelApp()
	if cmd := runSlash(a, "/models nope"); cmd != nil || !strings.Contains(a.Notice, "unknown model") {
		t.Fatalf("unknown model: cmd=%v notice=%q", cmd != nil, a.Notice)
	}
	if cmd := runSlash(a, "/models minicpm"); cmd != nil || !strings.Contains(a.Notice, "already using") {
		t.Fatalf("same model: notice=%q", a.Notice)
	}
	a.Busy = true
	if cmd := runSlash(a, "/models spark"); cmd != nil || !strings.Contains(a.Notice, "while the agent is working") {
		t.Fatalf("busy: notice=%q", a.Notice)
	}
	if len(*loaded) != 0 {
		t.Fatalf("refused switches still loaded %v", *loaded)
	}
}

func TestFailedSwitchRestoresPreviousModel(t *testing.T) {
	fakeLoader(t, config.Models[1].Path)
	a := modelApp()
	app.FinishSwitch(a, runSlash(a, "/models spark")().(app.ModelSwitchedMsg))
	if a.Err == nil || !strings.Contains(a.Err.Error(), "spark") {
		t.Fatalf("want a load error, got %v", a.Err)
	}
	if a.ModelName() != config.Models[0].Name || a.Server == nil || !a.Ready || a.Working() {
		t.Fatalf("previous model not restored: model=%s ready=%v", a.ModelName(), a.Ready)
	}
}

func TestFailedSwitchWithNoFallbackStopsApp(t *testing.T) {
	fakeLoader(t, config.Models[0].Path, config.Models[1].Path)
	a := modelApp()
	app.FinishSwitch(a, runSlash(a, "/models spark")().(app.ModelSwitchedMsg))
	if a.Err == nil || a.Ready || a.Working() {
		t.Fatalf("want error and not ready: err=%v ready=%v", a.Err, a.Ready)
	}
}
