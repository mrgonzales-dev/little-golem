package tools

// #cgo CFLAGS: -I/home/mrg/.local/include
// #cgo LDFLAGS: -L/home/mrg/.local/lib64 -lfff_c -Wl,-rpath,/home/mrg/.local/lib64
// #include <fff.h>
// #include <stdlib.h>
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// fffIndex wraps one fff instance rooted at a base directory. All C calls
// are serialized: the library is thread-safe but the grep path is a hot
// single buffer, so a lock costs nothing measurable.
type fffIndex struct {
	mu sync.Mutex
	h  unsafe.Pointer
}

// fffShared is the process-wide instance, created by InitFFF.
var (
	fffSharedMu sync.Mutex
	fffShared   *fffIndex
)

// InitFFF creates the shared index for basePath, waiting up to timeout for
// the initial directory scan. Idempotent: subsequent calls no-op.
func InitFFF(basePath string, timeoutMs uint64) error {
	fffSharedMu.Lock()
	defer fffSharedMu.Unlock()
	if fffShared != nil {
		return nil
	}

	cBase := C.CString(basePath)
	defer C.free(unsafe.Pointer(cBase))

	opts := C.struct_FffCreateOptions{}
	opts.version = C.FFF_CREATE_OPTIONS_VERSION
	opts.base_path = cBase
	opts.enable_mmap_cache = true
	opts.enable_content_indexing = true
	opts.watch = true
	opts.ai_mode = true

	res := C.fff_create_instance_with(&opts)
	defer C.fff_free_result(res)

	if msg := C.fff_result_get_error(res); msg != nil {
		return fmt.Errorf("fff: init %s: %s", basePath, C.GoString(msg))
	}
	h := C.fff_result_get_handle(res)
	if h == nil {
		return fmt.Errorf("fff: init returned no handle")
	}
	f := &fffIndex{h: h}
	fffShared = f

	// Wait for the initial scan so the first grep sees a complete index.
	f.mu.Lock()
	defer f.mu.Unlock()
	sres := C.fff_wait_for_scan(h, C.uint64_t(timeoutMs))
	defer C.fff_free_result(sres)
	if C.fff_result_get_int_value(sres) == 0 {
		// Not fatal: the watcher keeps indexing; report and continue.
		return fmt.Errorf("fff: initial scan did not finish in %dms (continuing)", timeoutMs)
	}
	return nil
}

// WaitScan blocks until the index finishes its initial scan or timeout.
func (f *fffIndex) WaitScan(timeoutMs uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	res := C.fff_wait_for_scan(f.h, C.uint64_t(timeoutMs))
	defer C.fff_free_result(res)
	if C.fff_result_get_int_value(res) == 0 {
		return fmt.Errorf("fff: initial scan did not finish in %dms", timeoutMs)
	}
	return nil
}

// ShutdownFFF destroys the shared instance, if present.
func ShutdownFFF() {
	fffSharedMu.Lock()
	defer fffSharedMu.Unlock()
	if fffShared != nil {
		C.fff_destroy(fffShared.h)
		fffShared = nil
	}
}

// grepArgs mirrors the read tool's JSON arguments.
type grepArgs struct {
	Query      string  `json:"query"`
	Mode       *uint8  `json:"mode"`        // 0 plain, 1 regex, 2 fuzzy
	MaxResults *uint32 `json:"max_results"` // page_limit
	Before     *uint32 `json:"context"`
	After      *uint32 `json:"context_after"`
}

// Grep runs a content search and returns formatted matches.
func (f *fffIndex) Grep(a grepArgs) (string, error) {
	if strings.TrimSpace(a.Query) == "" {
		return "", fmt.Errorf("empty query")
	}
	mode := uint8(0)
	if a.Mode != nil {
		mode = *a.Mode
	}
	limit := uint32(50)
	if a.MaxResults != nil {
		limit = *a.MaxResults
	}
	before, after := uint32(0), uint32(0)
	if a.Before != nil {
		before = *a.Before
	}
	if a.After != nil {
		after = *a.After
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	q := C.CString(a.Query)
	defer C.free(unsafe.Pointer(q))

	res := C.fff_live_grep(f.h, q, C.uint8_t(mode),
		0,            // max_file_size: auto
		0,            // max_matches_per_file: unlimited
		C.bool(true), // smart_case
		0,            // file_offset
		C.uint32_t(limit),
		0, // time_budget_ms: unlimited
		C.uint32_t(before),
		C.uint32_t(after),
		C.bool(true), // classify_definitions
	)
	defer C.fff_free_result(res)

	if msg := C.fff_result_get_error(res); msg != nil {
		return "", fmt.Errorf("fff: grep: %s", C.GoString(msg))
	}
	gp := (*C.struct_FffGrepResult)(C.fff_result_get_handle(res))
	if gp == nil {
		return "(no matches)\n", nil
	}
	defer C.fff_free_grep_result(gp)

	var b strings.Builder
	items := gp.items
	n := int(gp.count)
	for i := 0; i < n; i++ {
		m := (*C.struct_FffGrepMatch)(unsafe.Pointer(uintptr(unsafe.Pointer(items)) + uintptr(i)*unsafe.Sizeof(*items)))
		path := C.GoString(m.relative_path)
		line := uint64(m.line_number)
		content := C.GoString(m.line_content)
		if m.is_definition {
			b.WriteString("def> ")
		} else {
			b.WriteString("     ")
		}
		b.WriteString(path)
		b.WriteString(":")
		b.WriteString(strconv.FormatUint(line, 10))
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteString("\n")
		for j := 0; j < int(m.context_before_count); j++ {
			cb := C.fff_grep_match_get_context_before(m, C.uint32_t(j))
			b.WriteString("      " + C.GoString(cb) + "\n")
		}
		for j := 0; j < int(m.context_after_count); j++ {
			ca := C.fff_grep_match_get_context_after(m, C.uint32_t(j))
			b.WriteString("      " + C.GoString(ca) + "\n")
		}
	}
	return b.String(), nil
}

// ReadTool is the agent's native read capability over the fff index.
type ReadTool struct{}

// NewRead returns the read tool backed by the shared index.
func NewRead() *ReadTool { return &ReadTool{} }

func (t *ReadTool) Name() string { return "read" }

func (t *ReadTool) Description() string {
	return "Search the workspace's file CONTENTS for an identifier or text and return matching lines with file paths and line numbers. Pass a bare identifier (e.g. RenderEntries), optionally prefixed by a directory ('src/ queue') or glob ('*.go queue'). Plain text beats regex."
}

func (t *ReadTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "query":       {"type": "string", "description": "Search text with optional path/glob prefix, e.g. 'src/ AppendDelta' or '*.ts parse'."},
    "max_results": {"type": "integer", "description": "Maximum matches returned (default 50)."},
    "context":     {"type": "integer", "description": "Context lines before and after each match."},
    "mode":        {"type": "integer", "description": "0=plain text (default), 1=regex, 2=fuzzy."}
  },
  "required": ["query"]
}`)
}

func (t *ReadTool) Run(ctx context.Context, raw json.RawMessage) (Result, error) {
	a, err := parseGrepArgs(raw)
	if err != nil {
		return Result{}, fmt.Errorf("%w (raw args: %s)", err, string(raw))
	}
	out, err := fffShared.Grep(a)
	if err != nil {
		return Result{}, err
	}
	return Result{Content: strings.TrimRight(out, "\n")}, nil
}

// parseGrepArgs accepts the clean JSON object plus the variants small
// models emit: text around the object, or the bare query itself.
func parseGrepArgs(raw json.RawMessage) (grepArgs, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return grepArgs{}, fmt.Errorf(`missing arguments; expected {"query": "identifier"[, "max_results": n][, "context": n][, "mode": 0|1|2]}`)
	}
	var a grepArgs
	if err := json.Unmarshal(raw, &a); err == nil && strings.TrimSpace(a.Query) != "" {
		return a, nil
	}
	// Text around a JSON object: grab from the first { to the last }.
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			if err := json.Unmarshal([]byte(s[i:j+1]), &a); err == nil && strings.TrimSpace(a.Query) != "" {
				return a, nil
			}
		}
	}
	// A quoted string containing the query, or the bare query itself.
	if q, err := strconv.Unquote(s); err == nil && strings.TrimSpace(q) != "" {
		return grepArgs{Query: q}, nil
	}
	if !strings.HasPrefix(s, "{") {
		return grepArgs{Query: s}, nil
	}
	return grepArgs{}, fmt.Errorf(`could not parse arguments; expected {"query": "identifier"[, "max_results": n][, "context": n][, "mode": 0|1|2]}`)
}
