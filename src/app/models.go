package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
	"little-golem/src/ui"
)

// LoadServer starts a llama-server on a GGUF; tests replace it.
var LoadServer = llama.StartServer

// ModelSwitchedMsg reports a finished model switch. Srv is the server now
// running (the previous model's again when the load failed, nil when even
// that failed) and Idx its index in config.Models.
type ModelSwitchedMsg struct {
	Idx int
	Srv *llama.Server
	Err error
}

// pickModel resolves a /models argument: empty cycles to the next model,
// otherwise the first model whose name starts with arg.
func pickModel(cur int, arg string) (int, bool) {
	if arg == "" {
		return (cur + 1) % len(config.Models), true
	}
	for i, md := range config.Models {
		if strings.HasPrefix(md.Name, strings.ToLower(arg)) {
			return i, true
		}
	}
	return 0, false
}

func modelNames() string {
	names := make([]string, len(config.Models))
	for i, md := range config.Models {
		names[i] = md.Name
	}
	return strings.Join(names, ", ")
}

// bootServer starts a server on path and waits until it serves requests.
func bootServer(path string) (*llama.Server, error) {
	srv, err := LoadServer(path)
	if err != nil {
		return nil, err
	}
	if err := srv.WaitReady(); err != nil {
		srv.Stop()
		return nil, err
	}
	return srv, nil
}

// SwitchModel replaces the running llama-server with one on another model
// (see pickModel for arg), keeping the chat history. The old server stops
// first so two models never sit in RAM together.
func SwitchModel(m *model.App, arg string) tea.Cmd {
	if !m.Ready || m.Working() {
		m.Notice = "can't switch models while the agent is working"
		return nil
	}
	idx, ok := pickModel(m.ModelIdx, arg)
	switch {
	case !ok:
		m.Notice = fmt.Sprintf("unknown model %q (available: %s)", arg, modelNames())
		return nil
	case idx == m.ModelIdx:
		m.Notice = "already using " + m.ModelName()
		return nil
	}

	m.Err, m.Follow = nil, true
	m.Switching = config.Models[idx].Name
	m.TurnStart, m.VerbSeed = time.Now(), config.NewVerbSeed()
	m.TurnDone, m.ReqTokens = 0, 0
	old, oldIdx := m.Server, m.ModelIdx
	ui.RenderEntries(m)
	return func() tea.Msg {
		old.Stop()
		srv, err := bootServer(config.Models[idx].Path)
		if err == nil {
			srv.ToolDefs = old.ToolDefs
			return ModelSwitchedMsg{Idx: idx, Srv: srv}
		}
		err = fmt.Errorf("loading %s: %w", config.Models[idx].Name, err)
		back, berr := bootServer(config.Models[oldIdx].Path)
		if berr != nil {
			return ModelSwitchedMsg{Idx: oldIdx, Err: fmt.Errorf("%w; reloading %s also failed: %v", err, config.Models[oldIdx].Name, berr)}
		}
		back.ToolDefs = old.ToolDefs
		return ModelSwitchedMsg{Idx: oldIdx, Srv: back, Err: err}
	}
}

// FinishSwitch installs the new server and notes the switch in the
// transcript. The context counter restarts: the new model reports usage
// with its next reply.
func FinishSwitch(m *model.App, msg ModelSwitchedMsg) {
	m.Switching = ""
	m.Err = msg.Err
	if msg.Srv == nil {
		m.Ready = false
		return
	}
	changed := msg.Idx != m.ModelIdx
	m.Server, m.ModelIdx = msg.Srv, msg.Idx
	if changed {
		m.TokenUsed = 0
		m.Entries = append(m.Entries, model.Entry{Kind: model.EntryNote, Content: "switched to " + m.ModelName()})
	}
	ui.RenderEntries(m)
}
