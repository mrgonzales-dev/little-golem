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
	// compacted automatically: 80% of the window.
	CompactAt = CtxSize * 8 / 10

	// ToolMaxChars caps a single tool result fed to the model, in chars.
	// ~8000 chars ≈ 2k tokens, leaving room for multi-tool rounds inside
	// the 16k window before auto-compaction fires.
	ToolMaxChars = 8000

	// ToolMaxLines caps lines in a single tool result.
	ToolMaxLines = 400

	// BashRawMaxBytes bounds raw bash capture in memory before truncation.
	// The model only ever sees ToolMaxChars of it.
	BashRawMaxBytes = 128 << 10

	// GrepMaxResults clamps model-requested max_results.
	GrepMaxResults = 200

	// GlobMaxResults clamps model-requested max_results.
	GlobMaxResults = 200

	// GrepMaxContext clamps context lines before/after each match.
	GrepMaxContext = 5

	// GrepMaxLineChars truncates a single matched or context line.
	GrepMaxLineChars = 500
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
