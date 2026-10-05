// Command little-golem is a small terminal chatbot UI for a local
// llama.cpp model. It spawns `llama-server` with a fixed GGUF model and
// talks to it over the OpenAI-compatible HTTP API. The agent can call
// tools (read = file contents, grep/glob = fff-backed search, bash, edit,
// write) via OpenAI tool calling.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/app"
	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/session"
	"little-golem/src/tools"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "little-golem:", err)
		os.Exit(1)
	}
	config.WorkDir = cwd

	saved, loadErr := session.Load()
	idx := 0
	if saved != nil {
		idx = saved.ModelIdx()
	}
	srv, err := llama.StartServer(config.Models[idx].Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "little-golem:", err)
		os.Exit(1)
	}

	if err := tools.InitFFF(cwd, 30000); err != nil {
		fmt.Fprintln(os.Stderr, "little-golem: fff init:", err)
		os.Exit(1)
	}
	defer tools.ShutdownFFF()

	reg := tools.NewRegistry(tools.NewRead(), tools.NewGrep(), tools.NewGlob())

	a := app.New(srv)
	if saved != nil {
		saved.Apply(a.App)
	}
	a.Err = loadErr
	defer func() { a.Server.Stop() }() // the server may have been swapped by a model switch
	a.Tools = reg
	reg.Add(tools.NewBash(nil)) // Ask stays unset: confirmation flows through app.BashConfirm
	reg.Add(tools.NewEdit())
	reg.Add(tools.NewWrite())
	srv.ToolDefs = reg.Definitions()

	p := tea.NewProgram(a)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "little-golem:", err)
		os.Exit(1)
	}
}
