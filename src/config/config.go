// Package config holds the hardcoded runtime configuration for
// little-golem: paths to the llama.cpp binaries, the model GGUF, and the
// context window. Prompts live in prompts.go.
package config

const (
	// LlamaBin is the path to the llama-server binary.
	LlamaBin = "/home/mrg/models/little-golem/llama-server-setup/llama-server"

	// LlamaLibDir is the directory containing the llama.cpp shared libraries;
	// it is prepended to LD_LIBRARY_PATH when spawning llama-server.
	LlamaLibDir = "/home/mrg/models/little-golem/llama-server-setup"

	// ModelPath is the GGUF model file to load.
	// ModelPath = "/home/mrg/models/little-golem/model/MiniCPM5-2B-Claude-Thinking-Q4_K_M.gguf"
	ModelPath = "/home/mrg/models/little-golem/model/MiniCPM5-2B-Q4_K_M.gguf"

	// CtxSize is the llama.cpp context window in tokens.
	CtxSize = 128000
)
