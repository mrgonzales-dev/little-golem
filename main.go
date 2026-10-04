// Command little-golem is a small terminal chatbot UI for a local
// llama.cpp model. It spawns `llama-server` with a fixed GGUF model and
// talks to it over the OpenAI-compatible HTTP API. The agent can call
// tools (read = fff-backed content search) via OpenAI tool calling.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/app"
	"little-golem/src/llama"
	"little-golem/src/tools"
)

func main() {
	srv, err := llama.StartServer()
	if err != nil {
		fmt.Fprintln(os.Stderr, "little-golem:", err)
		os.Exit(1)
	}
	defer srv.Stop()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "little-golem:", err)
		os.Exit(1)
	}
	if err := tools.InitFFF(cwd, 30000); err != nil {
		fmt.Fprintln(os.Stderr, "little-golem: fff init:", err)
		os.Exit(1)
	}
	defer tools.ShutdownFFF()

	reg := tools.NewRegistry(tools.NewRead())

	a := app.New(srv)
	a.Tools = reg
	reg.Add(tools.NewBash(nil)) // Ask stays unset: confirmation flows through app.BashConfirm
	srv.ToolDefs = reg.Definitions()

	p := tea.NewProgram(a)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "little-golem:", err)
		os.Exit(1)
	}
}