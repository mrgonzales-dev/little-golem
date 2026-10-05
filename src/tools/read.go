package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	readDefaultLimit = 250
	readMaxLimit     = 1000
	readMaxLineLen   = 500
	readMaxBytes     = 40_000
	readMaxFile      = 10 << 20
	readMaxDirItems  = 200
)

// ReadTool returns the contents of a file (or the entries of a folder).
type ReadTool struct{}

// NewRead returns the file-reading tool.
func NewRead() *ReadTool { return &ReadTool{} }

func (t *ReadTool) Name() string { return "read" }

func (t *ReadTool) Description() string {
	return "Read a file and return its contents with line numbers ('  12| text'; the 'N| ' prefix is not part of the file). Long files come back in pieces: pass offset to continue. Given a folder, lists its entries. To search many files for text use grep; to find files by name use glob."
}

func (t *ReadTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":   {"type": "string", "description": "File or folder path relative to the project folder, e.g. 'src/main.go'."},
    "offset": {"type": "integer", "description": "First line to return, 1-based (default 1)."},
    "limit":  {"type": "integer", "description": "Maximum lines to return (default 250, max 1000)."}
  },
  "required": ["path"]
}`)
}

func (t *ReadTool) Run(_ context.Context, raw json.RawMessage) (Result, error) {
	var a struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := parseArgs(raw, &a); err != nil {
		// Small models sometimes send the bare path instead of an object.
		s := strings.TrimSpace(string(raw))
		if q, e := strconv.Unquote(s); e == nil {
			s = q
		}
		if s == "" || s == "null" || strings.HasPrefix(s, "{") {
			return Result{}, err
		}
		a.Path = s
	}
	if a.Path == "" {
		a.Path = a.FilePath
	}
	abs, err := resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return Result{}, fmt.Errorf("cannot read %s: %v (use glob to find files)", a.Path, err)
	}
	if st.IsDir() {
		return listDir(abs, a.Path)
	}
	if st.Size() > readMaxFile {
		return Result{}, fmt.Errorf("%s is %d bytes, too large to read; use grep to search it", a.Path, st.Size())
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Result{}, err
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return Result{}, fmt.Errorf("%s looks like a binary file", a.Path)
	}
	if len(data) == 0 {
		return Result{Content: "(empty file)"}, nil
	}

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	start := max(a.Offset, 1)
	if start > len(lines) {
		return Result{}, fmt.Errorf("offset %d is past the end of %s (%d lines)", start, a.Path, len(lines))
	}
	limit := a.Limit
	if limit <= 0 {
		limit = readDefaultLimit
	}
	limit = min(limit, readMaxLimit)

	width := len(strconv.Itoa(len(lines)))
	var b strings.Builder
	n := start - 1
	for ; n < len(lines) && n-start+1 < limit && b.Len() < readMaxBytes; n++ {
		line := strings.TrimSuffix(lines[n], "\r")
		if len(line) > readMaxLineLen {
			line = strings.ToValidUTF8(line[:readMaxLineLen], "") + "…"
		}
		fmt.Fprintf(&b, "%*d| %s\n", width, n+1, line)
	}
	if n < len(lines) {
		fmt.Fprintf(&b, "(showing lines %d-%d of %d; call read again with offset=%d to continue)", start, n, len(lines), n+1)
	} else {
		fmt.Fprintf(&b, "(end of file, %d lines)", len(lines))
	}
	return Result{Content: b.String()}, nil
}

func listDir(abs, shown string) (Result, error) {
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Result{}, err
	}
	if len(entries) == 0 {
		return Result{Content: shown + " is an empty folder"}, nil
	}
	var b strings.Builder
	for i, e := range entries {
		if i == readMaxDirItems {
			fmt.Fprintf(&b, "(%d more entries)\n", len(entries)-i)
			break
		}
		b.WriteString(e.Name())
		if e.IsDir() {
			b.WriteString("/")
		}
		b.WriteString("\n")
	}
	return Result{Content: strings.TrimRight(b.String(), "\n")}, nil
}
