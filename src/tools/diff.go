package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// DiffKind tags one line of a Diff.
type DiffKind uint8

const (
	DiffCtx  DiffKind = iota // unchanged line shown for context
	DiffAdd                  // line added
	DiffDel                  // line removed
	DiffSkip                 // run of unchanged lines left out; Num is how many
)

// DiffLine is one rendered line of a diff. Num is the line number in the
// file (the old file for removed lines, the new file otherwise); 0 when
// unknown.
type DiffLine struct {
	Kind DiffKind
	Num  int
	Text string
}

// Diff previews what an edit or write call does to a file.
type Diff struct {
	Path        string
	Lines       []DiffLine
	Adds, Dels  int
	Created     bool // write: the file does not exist yet
	Matches     int  // edit: occurrences of old_string in the file, 0 if unknown
	Streaming   bool // built by PartialDiff while the call is still being written
	Overwriting bool // streaming write: the file exists and will be replaced
}

const (
	diffContext = 2         // unchanged lines kept around each change
	lcsLimit    = 1_000_000 // largest n*m the line diff will compute exactly
	diffMaxOld  = 1 << 20   // larger existing files are not diffed against
)

// BuildDiff previews the edit or write call given by its JSON arguments,
// reading the current file from disk. Call it before the tool runs.
func BuildDiff(name, rawArgs string) (*Diff, error) {
	var a struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := parseArgs(json.RawMessage(rawArgs), &a); err != nil {
		return nil, err
	}
	abs, err := resolve(a.Path)
	if err != nil {
		return nil, err
	}
	cur, readErr := os.ReadFile(abs)
	d := &Diff{Path: a.Path}
	var lines []DiffLine
	switch name {
	case "edit":
		start := 0
		if readErr == nil && a.OldString != "" {
			text := string(cur)
			if i := strings.Index(text, a.OldString); i >= 0 {
				start = strings.Count(text[:i], "\n") + 1
				d.Matches = strings.Count(text, a.OldString)
			}
		}
		lines = diffLines(splitLines(a.OldString), splitLines(a.NewString), start)
	case "write":
		d.Created = readErr != nil
		old := ""
		if readErr == nil && len(cur) <= diffMaxOld {
			old = string(cur)
		}
		lines = diffLines(splitLines(old), splitLines(a.Content), 1)
	default:
		return nil, fmt.Errorf("no diff for tool %q", name)
	}
	for _, l := range lines {
		switch l.Kind {
		case DiffAdd:
			d.Adds++
		case DiffDel:
			d.Dels++
		}
	}
	d.Lines = collapse(lines)
	return d, nil
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n"), "\n")
}

// diffLines diffs two line lists. start is the file line number of a[0]
// and b[0], or 0 to leave lines unnumbered.
func diffLines(a, b []string, start int) []DiffLine {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	var ops []DiffLine
	for _, t := range a[:p] {
		ops = append(ops, DiffLine{Kind: DiffCtx, Text: t})
	}
	ops = append(ops, lcsOps(a[p:len(a)-s], b[p:len(b)-s])...)
	for _, t := range a[len(a)-s:] {
		ops = append(ops, DiffLine{Kind: DiffCtx, Text: t})
	}
	if start > 0 {
		oldN, newN := start, start
		for i := range ops {
			switch ops[i].Kind {
			case DiffDel:
				ops[i].Num = oldN
				oldN++
			case DiffAdd:
				ops[i].Num = newN
				newN++
			default:
				ops[i].Num = newN
				oldN++
				newN++
			}
		}
	}
	return ops
}

// lcsOps diffs two line lists with a longest-common-subsequence table,
// falling back to "all removed, then all added" for very large inputs.
func lcsOps(a, b []string) []DiffLine {
	var ops []DiffLine
	if len(a)*len(b) > lcsLimit {
		for _, t := range a {
			ops = append(ops, DiffLine{Kind: DiffDel, Text: t})
		}
		for _, t := range b {
			ops = append(ops, DiffLine{Kind: DiffAdd, Text: t})
		}
		return ops
	}
	n, m := len(a), len(b)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			ops = append(ops, DiffLine{Kind: DiffCtx, Text: a[i]})
			i++
			j++
		case j == m || (i < n && dp[i+1][j] >= dp[i][j+1]):
			ops = append(ops, DiffLine{Kind: DiffDel, Text: a[i]})
			i++
		default:
			ops = append(ops, DiffLine{Kind: DiffAdd, Text: b[j]})
			j++
		}
	}
	return ops
}

// collapse replaces runs of unchanged lines farther than diffContext from
// any change by a single DiffSkip line.
func collapse(lines []DiffLine) []DiffLine {
	near := make([]bool, len(lines))
	for i, l := range lines {
		if l.Kind == DiffCtx {
			continue
		}
		for j := max(0, i-diffContext); j <= min(len(lines)-1, i+diffContext); j++ {
			near[j] = true
		}
	}
	var out []DiffLine
	for i := 0; i < len(lines); {
		if near[i] {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && !near[j] {
			j++
		}
		if j-i == 1 {
			out = append(out, lines[i])
		} else {
			out = append(out, DiffLine{Kind: DiffSkip, Num: j - i})
		}
		i = j
	}
	return out
}
