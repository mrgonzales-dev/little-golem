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

	// CtxSize is the llama.cpp context window in tokens.
	CtxSize = 16384

	// ThinkBudget caps the thinking tokens per reply; llama.cpp forces the
	// end-of-thinking tag once it is spent.
	ThinkBudget = 512

	// ThinkBudgetMessage is injected before the end tag when the budget is
	// spent, so the model answers instead of resuming mid-thought.
	ThinkBudgetMessage = "... enough thinking, answering now."

	// CompactAt is the context size in tokens at which the history is
	// compacted automatically: 60% of the window.
	CompactAt = CtxSize * 6 / 10
)

// Model is one selectable GGUF. Name is the short id used by /models and the
// reply footer; Label is shown in the header.
type Model struct{ Name, Label, Path string }

// Models lists the selectable models; the first one loads at startup. Switch
// at runtime with ctrl+m or /models. Both share the CtxSize window.
// Another MiniCPM variant: model/MiniCPM5-2B-Claude-Thinking-Q4_K_M.gguf
var Models = []Model{
	{"minicpm", "minicpm-2b", "/home/mrg/models/little-golem/model/MiniCPM5-2B-Q4_K_M.gguf"},
	{"spark", "spark-x2.5-1.7b", "/home/mrg/models/little-golem/model/Spark-X2.5-1.7B-Q4_K_M.gguf"},
}
