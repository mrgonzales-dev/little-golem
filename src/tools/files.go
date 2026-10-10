package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"little-golem/src/config"
)

// expandHome expands a leading "~" or "~/" to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// resolveAny maps a user-supplied @-reference to an absolute path without
// enforcing the workspace jail. It still cleans the path and resolves
// symlinks of the deepest existing ancestor. Read-only: only expandRefs
// uses it; the read/edit/write tools stay jailed via resolve.
func resolveAny(p string) (string, error) {
	p = strings.TrimSpace(expandHome(p))
	if p == "" {
		return "", fmt.Errorf("missing path")
	}
	if !filepath.IsAbs(p) {
		root := config.WorkDir
		if root == "" {
			var err error
			if root, err = os.Getwd(); err != nil {
				return "", err
			}
		}
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)
	head, tail := p, ""
	for {
		if _, err := os.Lstat(head); err == nil {
			break
		}
		parent := filepath.Dir(head)
		if parent == head {
			break
		}
		tail = filepath.Join(filepath.Base(head), tail)
		head = parent
	}
	if head, err := filepath.EvalSymlinks(head); err == nil {
		p = filepath.Join(head, tail)
	}
	return p, nil
}

// ResolveAny is the exported workspace-unjailed resolver for @-references.
func ResolveAny(p string) (string, error) { return resolveAny(p) }

// resolve maps a model-supplied path to an absolute path inside the
// workspace, following symlinks so a link cannot lead outside it.
func resolve(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("missing path")
	}
	root := config.WorkDir
	if root == "" {
		var err error
		if root, err = os.Getwd(); err != nil {
			return "", err
		}
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)

	// Resolve the deepest existing ancestor, then re-attach the rest.
	head, tail := abs, ""
	for {
		if _, err := os.Lstat(head); err == nil {
			break
		}
		parent := filepath.Dir(head)
		if parent == head {
			break
		}
		tail = filepath.Join(filepath.Base(head), tail)
		head = parent
	}
	if head, err = filepath.EvalSymlinks(head); err != nil {
		return "", err
	}
	abs = filepath.Join(head, tail)
	if rel, err := filepath.Rel(root, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the project folder", p)
	}
	return abs, nil
}

// parseArgs decodes tool arguments, tolerating text around the JSON object
// as small models emit it.
func parseArgs(raw json.RawMessage, v any) error {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return fmt.Errorf("missing arguments")
	}
	err := json.Unmarshal(raw, v)
	if err == nil {
		return nil
	}
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		if json.Unmarshal([]byte(s[i:j+1]), v) == nil {
			return nil
		}
	}
	return fmt.Errorf("could not parse arguments: %v (raw args: %s)", err, s)
}

// WriteTool creates or overwrites a file after the user approves it.
type WriteTool struct{}

func NewWrite() *WriteTool { return &WriteTool{} }

func (t *WriteTool) Name() string { return "write" }

// RequiresApproval marks the tool as confirmation-gated.
func (t *WriteTool) RequiresApproval() bool { return true }

func (t *WriteTool) Description() string {
	return "Create a new file, or completely overwrite an existing one, with the given content. Use this whenever the user asks you to create, write, make or generate a file or script: call it directly with the full content, without listing the folder first. Parent folders are created. To change part of an existing file use edit instead. Requires user approval."
}

func (t *WriteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":    {"type": "string", "description": "File path relative to the project folder, e.g. 'src/util.go'."},
    "content": {"type": "string", "description": "The full content of the file."}
  },
  "required": ["path", "content"]
}`)
}

func (t *WriteTool) Run(_ context.Context, raw json.RawMessage) (Result, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := parseArgs(raw, &a); err != nil {
		return Result{}, err
	}
	abs, err := resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	mode, verb := os.FileMode(0o644), "created"
	if st, err := os.Stat(abs); err == nil {
		if st.IsDir() {
			return Result{}, fmt.Errorf("%s is a directory", a.Path)
		}
		mode, verb = st.Mode().Perm(), "overwrote"
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(abs, []byte(a.Content), mode); err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("%s %s (%d lines, %d bytes)", verb, a.Path, countLines(a.Content), len(a.Content))}, nil
}

// EditTool replaces exact text in an existing file after the user approves.
type EditTool struct{}

func NewEdit() *EditTool { return &EditTool{} }

func (t *EditTool) Name() string { return "edit" }

// RequiresApproval marks the tool as confirmation-gated.
func (t *EditTool) RequiresApproval() bool { return true }

func (t *EditTool) Description() string {
	return "Edit an existing file by replacing exact text. old_string must match the file exactly (whitespace included) and be unique in it; include a few surrounding lines to make it unique, or set replace_all. Requires user approval."
}

func (t *EditTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":        {"type": "string", "description": "File path relative to the project folder."},
    "old_string":  {"type": "string", "description": "The exact text to replace. Must be non-empty."},
    "new_string":  {"type": "string", "description": "The replacement text."},
    "replace_all": {"type": "boolean", "description": "Replace every occurrence instead of requiring a unique match (default false)."}
  },
  "required": ["path", "old_string", "new_string"]
}`)
}

func (t *EditTool) Run(_ context.Context, raw json.RawMessage) (Result, error) {
	var a struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := parseArgs(raw, &a); err != nil {
		return Result{}, err
	}
	if a.OldString == "" {
		return Result{}, fmt.Errorf("old_string is empty; use write to create or overwrite a file")
	}
	if a.OldString == a.NewString {
		return Result{}, fmt.Errorf("old_string and new_string are identical")
	}
	abs, err := resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return Result{}, fmt.Errorf("cannot edit %s: %v (use write to create it)", a.Path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Result{}, err
	}
	text := string(data)
	n := strings.Count(text, a.OldString)
	switch {
	case n == 0:
		return Result{}, fmt.Errorf("old_string not found in %s; read the file and copy the text exactly", a.Path)
	case n > 1 && !a.ReplaceAll:
		return Result{}, fmt.Errorf("old_string matches %d places in %s; add surrounding lines to make it unique or set replace_all", n, a.Path)
	}
	text = strings.ReplaceAll(text, a.OldString, a.NewString)
	if err := os.WriteFile(abs, []byte(text), st.Mode().Perm()); err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("edited %s (%d replaced)", a.Path, n)}, nil
}

func countLines(s string) int {
	return len(strings.Split(strings.TrimSuffix(s, "\n"), "\n")) * min(1, len(s))
}
