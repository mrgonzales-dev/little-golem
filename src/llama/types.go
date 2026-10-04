// Package llama wraps a spawned llama-server child process and talks to it
// over HTTP using the OpenAI-compatible chat completions API.
package llama

import (
	"time"
)

// LoadTimeout is how long we wait for llama-server to report /health.
const LoadTimeout = 3 * time.Minute

// ChatMessage is a single message in the OpenAI chat format.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is one function call requested by the model.
type ToolCall struct {
	// ID is empty for streamed deltas; server fills it at assembly time.
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // raw JSON string per OpenAI spec
	} `json:"function"`
}

// ToolChunk is one streamed tool_calls fragment: the SSE delta may carry
// an index, the function name (first fragment), or an argument piece.
type ToolChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// StreamEvent is one decoded SSE chunk (or a terminal error) from the
// completions stream.
type StreamEvent struct {
	Reasoning string
	Content   string
	ToolDelta *ToolDelta
	Usage     *Usage
	Err       error
}

// Usage carries the token totals reported by llama-server for the request.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolDelta is one incremental piece of a streamed tool call: either the
// function name or an argument fragment.
type ToolDelta struct {
	Index int
	Name  string
	Args  string
}
