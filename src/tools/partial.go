package tools

import (
	"os"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// PartialFields decodes the top-level string values of a JSON object that
// may still be streaming in. A value whose closing quote has not arrived
// yet is returned as far as it has been written, and its key is reported as
// open. Only real top-level keys are matched, so text inside a value that
// looks like a key is never mistaken for one. Non-string values (booleans,
// numbers) are skipped.
func PartialFields(s string) (vals map[string]string, open string) {
	vals = map[string]string{}
	i := skipSpace(s, 0)
	if i >= len(s) || s[i] != '{' {
		return vals, ""
	}
	i++
	for {
		for i < len(s) && (isSpace(s[i]) || s[i] == ',') {
			i++
		}
		if i >= len(s) || s[i] != '"' {
			return vals, ""
		}
		key, ni, closed := scanString(s, i)
		if !closed {
			return vals, ""
		}
		i = skipSpace(s, ni)
		if i >= len(s) || s[i] != ':' {
			return vals, ""
		}
		i = skipSpace(s, i+1)
		if i >= len(s) {
			return vals, ""
		}
		if s[i] == '"' {
			val, ni, closed := scanString(s, i)
			vals[key] = val
			if !closed {
				return vals, key
			}
			i = ni
			continue
		}
		for i < len(s) && s[i] != ',' && s[i] != '}' {
			if s[i] == '"' {
				_, ni, closed := scanString(s, i)
				if !closed {
					return vals, ""
				}
				i = ni
				continue
			}
			i++
		}
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func skipSpace(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}

// scanString decodes the JSON string whose opening quote is s[i]. It
// returns the text so far, the index just past the closing quote, and
// whether the closing quote was seen. A carriage return is dropped.
func scanString(s string, i int) (string, int, bool) {
	var b strings.Builder
	for j := i + 1; j < len(s); {
		switch c := s[j]; {
		case c == '"':
			return b.String(), j + 1, true
		case c == '\\':
			if j+1 >= len(s) {
				return b.String(), len(s), false
			}
			switch e := s[j+1]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'u':
				r, n, ok := unicodeEscape(s, j)
				if !ok {
					return b.String(), len(s), false
				}
				b.WriteRune(r)
				j += n
				continue
			default:
				b.WriteByte(e)
			}
			j += 2
		default:
			r, size := utf8.DecodeRuneInString(s[j:])
			if r == utf8.RuneError && size <= 1 && !utf8.FullRuneInString(s[j:]) {
				return b.String(), len(s), false // rune cut off mid-stream
			}
			b.WriteString(s[j : j+size])
			j += size
		}
	}
	return b.String(), len(s), false
}

// unicodeEscape decodes the \uXXXX escape at s[j:], joining a surrogate
// pair when needed. ok is false while the escape is still incomplete.
func unicodeEscape(s string, j int) (r rune, n int, ok bool) {
	hex := func(at int) (rune, bool) {
		if at+6 > len(s) || s[at] != '\\' || s[at+1] != 'u' {
			return 0, false
		}
		var v rune
		for _, c := range s[at+2 : at+6] {
			switch {
			case c >= '0' && c <= '9':
				v = v<<4 | (c - '0')
			case c >= 'a' && c <= 'f':
				v = v<<4 | (c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				v = v<<4 | (c - 'A' + 10)
			default:
				return utf8.RuneError, true
			}
		}
		return v, true
	}
	hi, ok := hex(j)
	if !ok {
		return 0, 0, false
	}
	if hi < 0xD800 || hi >= 0xDC00 {
		return hi, 6, true
	}
	lo, ok := hex(j + 6)
	if !ok {
		rest := s[min(len(s), j+6):]
		if len(s) < j+12 && (strings.HasPrefix(`\u`, rest) || strings.HasPrefix(rest, `\u`)) {
			return 0, 0, false // low half of the pair has not arrived yet
		}
		return utf8.RuneError, 6, true
	}
	if d := utf16.DecodeRune(hi, lo); d != utf8.RuneError {
		return d, 12, true
	}
	return utf8.RuneError, 6, true
}

// FileSnap caches the file a streaming call targets, so the disk is read
// once per call rather than once per streamed token.
type FileSnap struct {
	path   string
	loaded bool
	Exists bool
	Text   string
}

func (f *FileSnap) load(path string) {
	if f.loaded && f.path == path {
		return
	}
	*f = FileSnap{path: path, loaded: true}
	abs, err := resolve(path)
	if err != nil {
		return
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		f.Exists = true
		if st.Size() <= diffMaxOld {
			b, _ := os.ReadFile(abs)
			f.Text = string(b)
		}
	}
}

// PartialDiff previews an edit or write call whose arguments are still
// streaming in (rawArgs is the JSON so far), so the diff can be drawn while
// the model writes it. Unlike BuildDiff it does not diff old against new:
// a write shows its content as added lines, and an edit shows old_string as
// removed lines followed by new_string as added ones. snap caches the
// target file between calls; it may be nil. It returns nil until there is
// something to show.
func PartialDiff(name, rawArgs string, snap *FileSnap) *Diff {
	vals, open := PartialFields(rawArgs)
	d := &Diff{Path: vals["path"], Streaming: true}
	known := false
	if snap != nil && open != "path" && d.Path != "" {
		snap.load(d.Path)
		known = true
	}

	var lines []DiffLine
	switch name {
	case "write":
		d.Created = known && !snap.Exists
		d.Overwriting = known && snap.Exists
		for i, t := range splitLines(vals["content"]) {
			lines = append(lines, DiffLine{Kind: DiffAdd, Num: i + 1, Text: t})
		}
	case "edit":
		old, nw := vals["old_string"], vals["new_string"]
		start := 0
		if known && snap.Exists && old != "" {
			if i := strings.Index(snap.Text, old); i >= 0 {
				start = strings.Count(snap.Text[:i], "\n") + 1
				if open != "old_string" {
					d.Matches = strings.Count(snap.Text, old)
				}
			}
		}
		number := func(i int) int {
			if start == 0 {
				return 0
			}
			return start + i
		}
		for i, t := range splitLines(old) {
			lines = append(lines, DiffLine{Kind: DiffDel, Num: number(i), Text: t})
		}
		for i, t := range splitLines(nw) {
			lines = append(lines, DiffLine{Kind: DiffAdd, Num: number(i), Text: t})
		}
	default:
		return nil
	}
	if d.Path == "" && len(lines) == 0 {
		return nil
	}
	for _, l := range lines {
		if l.Kind == DiffAdd {
			d.Adds++
		} else {
			d.Dels++
		}
	}
	d.Lines = lines
	return d
}
