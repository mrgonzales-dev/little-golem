package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"little-golem/src/config"
)

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
	out, err := cmd.CombinedOutput()
	content := strings.TrimRight(string(out), "\n")
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
