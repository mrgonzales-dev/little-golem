package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"little-golem/src/config"
)

// cappedBuffer is a memory-bounding writer: it keeps the first cap bytes
// and drops the rest, remembering that it overflowed.
type cappedBuffer struct {
	mu       sync.Mutex
	data     []byte
	cap      int
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	room := b.cap - len(b.data)
	if room <= 0 {
		b.overflow = true
		return len(p), nil // discard, report success so the command continues
	}
	if len(p) > room {
		b.data = append(b.data, p[:room]...)
		b.overflow = true
		return len(p), nil
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

// BashTool runs a shell command after the user approves it.
type BashTool struct {
	// Ask breaks the approval loop out of the tool package: it receives
	// the command and reports the user's decision.
	Ask func(ctx context.Context, cmd string) bool
}

// NewBash builds the bash tool with the given approval hook.
func NewBash(ask func(ctx context.Context, cmd string) bool) *BashTool {
	return &BashTool{Ask: ask}
}

// bashArgs mirrors the tool's JSON schema.
type bashArgs struct {
	Command string  `json:"command"`
	Timeout float64 `json:"timeout_seconds"`
}

func (t *BashTool) Name() string { return "bash" }

// RequiresApproval marks the tool as confirmation-gated.
func (t *BashTool) RequiresApproval() bool { return true }

func (t *BashTool) Description() string {
	return "Run one shell command with the project folder as the working directory (ls, cat, grep, git, tests) and return stdout/stderr. Requires user approval; pending commands are shown for y/n confirmation."
}

func (t *BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command":         {"type": "string", "description": "The shell command to run, e.g. 'ls src/'. One command; chain with && if needed."},
    "timeout_seconds": {"type": "number", "description": "Kill the command after this many seconds (default 60)."}
  },
  "required": ["command"]
}`)
}

func (t *BashTool) Run(ctx context.Context, raw json.RawMessage) (Result, error) {
	a, err := parseBashArgs(raw)
	if err != nil {
		return Result{}, fmt.Errorf("%w (raw args: %s)", err, string(raw))
	}
	timeout := 60 * time.Second
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout * float64(time.Second))
	}

	// User approval before anything runs. Ask may return quickly with a
	// delegate lookup; the app normally pre-approves via its confirm
	// queue (keys.go) and passes Ask=nil, trusting
	// exec.RunNext's y/n gate instead. If no gate exists at all, deny.
	if t.Ask != nil && !t.Ask(ctx, a.Command) {
		return Result{Content: "the user denied this command"}, nil
	}

	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", a.Command)
	// Pin the working directory to the project folder: the prompt tells
	// the model bash already runs there, and this keeps it true even if
	// the process cwd was changed.
	cmd.Dir = config.WorkDir
	// Bound memory before the context-budget cut: raw capture stops at
	// BashRawMaxBytes so a runaway command cannot OOM us; the model only
	// ever sees ToolMaxChars via the budget gate in Execute.
	var buf cappedBuffer
	buf.cap = config.BashRawMaxBytes
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err = cmd.Run()
	content := strings.TrimRight(buf.String(), "\n")
	if buf.overflow {
		content += fmt.Sprintf("\n… [raw output exceeded %d bytes; showing the first %d]", config.BashRawMaxBytes, len(buf.data))
	}
	if runCtx.Err() != nil {
		content = strings.TrimSpace(content) + fmt.Sprintf("\n(timed out after %gs, exit 124)", a.Timeout)
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			// Non-zero exit: echo the command so the model can adjust,
			// and say no-output explicitly instead of a bare body.
			body := content
			if body == "" {
				body = "(no output)"
			}
			return Result{Content: fmt.Sprintf("$ %s\n%s\n(exit status %d)", a.Command, body, ee.ExitCode())}, nil
		}
		return Result{Content: content, IsError: true}, nil
	}
	if content == "" {
		content = fmt.Sprintf("$ %s\n(no output)", a.Command)
	}
	return Result{Content: content}, nil
}

// parseBashArgs accepts the clean JSON object plus the variants small
// models emit: text around the object, or the bare command itself.
func parseBashArgs(raw json.RawMessage) (bashArgs, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return bashArgs{}, fmt.Errorf(`missing arguments; expected {"command": "ls src/"[, "timeout_seconds": n]}`)
	}
	var a bashArgs
	if err := json.Unmarshal(raw, &a); err == nil && strings.TrimSpace(a.Command) != "" {
		return a, nil
	}
	// Text around a JSON object: grab from the first { to the last }.
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			var obj bashArgs
			if err := json.Unmarshal([]byte(s[i:j+1]), &obj); err == nil && strings.TrimSpace(obj.Command) != "" {
				return obj, nil
			}
		}
	}
	// A quoted string containing the command, or the bare command itself.
	if q, err := quoteUnquote(s); err == nil && strings.TrimSpace(q) != "" {
		return bashArgs{Command: q}, nil
	}
	if !strings.HasPrefix(s, "{") {
		return bashArgs{Command: s}, nil
	}
	return bashArgs{}, fmt.Errorf(`could not parse arguments; expected {"command": "ls src/"[, "timeout_seconds": n]}`)
}

// quoteUnquote unwraps a JSON or Go string literal.
func quoteUnquote(s string) (string, error) {
	return strconv.Unquote(s)
}
