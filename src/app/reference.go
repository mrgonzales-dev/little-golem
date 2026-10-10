package app

import (
	"os"
	"regexp"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/model"
	"little-golem/src/tools"
)

const (
	refRows      = 6
	refFileBytes = 6000 // of one @reference put into the message; under the
	// read body cap so this cut (with its marker) is the one that fires
	refTotalBytes = 20000 // of all @references in one message
	refLines      = 200
)

// FindPaths lists files and folders matching what follows an @; tests
// replace it.
var FindPaths = tools.FindPaths

var refRe = regexp.MustCompile(`(?:^|\s)@(?:"([^"]+)"?|'([^']+)'?|(\S+))`)

// refToken is the @reference under the cursor, or "". Quoted references
// (@"my file... / @'my file...) may contain spaces while typed; bare
// @words still end at whitespace.
func refToken(m *model.App) string {
	lines := strings.Split(m.Input.Value(), "\n")
	if m.Input.Line() >= len(lines) {
		return ""
	}
	r := []rune(lines[m.Input.Line()])
	end := min(m.Input.Column(), len(r))
	// Find the last @ starting a token before the cursor.
	at := -1
	for i := end - 1; i >= 0 && i >= end-512; i-- {
		if r[i] == '@' && (i == 0 || unicode.IsSpace(r[i-1])) {
			at = i
			break
		}
	}
	if at < 0 {
		return ""
	}
	tok := string(r[at:end])
	if len(tok) >= 2 && (tok[1] == '"' || tok[1] == '\'') {
		// Quoted reference: spaces are part of the token while open.
		// A closer before the cursor means the reference is done.
		if strings.ContainsRune(tok[2:], rune(tok[1])) {
			return ""
		}
		return tok
	}
	if strings.ContainsAny(tok, " \t") {
		return ""
	}
	return tok
}

// refQuery strips the @-prefix (and opening quote) for the fuzzy finder.
func refQuery(tok string) string {
	if strings.HasPrefix(tok, "@\"") || strings.HasPrefix(tok, "@'") {
		q := tok[2:]
		// Drop a trailing closer if the cursor sits after it.
		q = strings.TrimSuffix(strings.TrimSuffix(q, "\""), "'")
		return q
	}
	return strings.TrimPrefix(tok, "@")
}

// RefreshRefs updates the file picker for the word under the cursor.
func RefreshRefs(m *model.App) {
	tok := refToken(m)
	if tok == "" {
		m.RefOff = ""
	}
	if tok == "" || tok == m.RefOff || m.Current != nil {
		m.Refs, m.RefTok = nil, ""
		return
	}
	if tok != m.RefTok {
		m.RefTok, m.RefSel = tok, 0
		m.Refs = FindPaths(refQuery(tok), refRows)
	}
}

// handleRefKey drives the open picker: arrows move, tab/enter insert
// the highlighted file or folder (a folder keeps the picker open to go
// deeper), esc closes it until the word changes.
func handleRefKey(m *model.App, key string) bool {
	n := len(m.Refs)
	switch key {
	case "up":
		m.RefSel = (m.RefSel + n - 1) % n
	case "down":
		m.RefSel = (m.RefSel + 1) % n
	case "tab", "enter":
		path := m.Refs[m.RefSel]
		for range len([]rune(m.RefTok)) {
			m.Input, _ = m.Input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		}
		if strings.HasSuffix(path, "/") && strings.ContainsAny(path, " \t\"'") {
			// Folder with spaces: keep it open (no closer) so the
			// picker can drill deeper.
			m.Input.InsertString("@\"" + path)
		} else if strings.ContainsAny(path, " \t\"'") {
			m.Input.InsertString("@\"" + path + "\"")
		} else {
			m.Input.InsertString("@" + path)
		}
		if !strings.HasSuffix(path, "/") {
			m.Input.InsertString(" ")
		}
		m.Refs, m.RefTok, m.RefOff = nil, "", ""
	case "esc":
		m.Refs, m.RefOff = nil, m.RefTok
	default:
		return false
	}
	return true
}

// expandRefs appends the contents of every file and the listing of every
// folder @-referenced in text, within a size budget, so the model sees
// them without a read call. Quoted references (@"my file.xlsx") may contain
// spaces; bare @words end at whitespace as before. Paths resolve without
// the workspace jail (absolute and ~/... allowed) since @ is explicit user
// intent, but this stays read-only. It returns the message to send, the
// references attached and those that could not be read.
func expandRefs(text string) (string, []string, []string) {
	var b strings.Builder
	b.WriteString(text)
	var attached, missing []string
	seen := map[string]bool{}
	budget := refTotalBytes
	for _, mt := range refRe.FindAllStringSubmatch(text, -1) {
		var ref, label string
		switch {
		case mt[1] != "":
			ref = strings.TrimSpace(mt[1])
			label = "@\"" + ref + "\""
		case mt[2] != "":
			ref = strings.TrimSpace(mt[2])
			label = "@\"" + ref + "\""
		default:
			ref = strings.TrimRight(mt[3], ".,;:!?)]}\"'")
			label = "@" + ref
		}
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		res, abs, err := tools.ReadAny(ref, refLines)
		if err != nil {
			missing = append(missing, label)
			continue
		}
		if len(attached) == 0 {
			b.WriteString("\n\nFiles and folders the user referenced with @ (already read for you):")
		}
		attached = append(attached, label)
		body, room := res.Content, min(refFileBytes, budget)
		switch {
		case room <= 0:
			body = "(not attached: no room left; use the read tool)"
		case len(body) > room:
			body = body[:strings.LastIndex(body[:room], "\n")+1] + "… [cut for length; use the read tool with offset to see more]"
		}
		budget -= len(body)
		kind := "file"
		if st, err := os.Stat(abs); err == nil && st.IsDir() {
			kind = "folder"
		}
		b.WriteString("\n\n<" + kind + " path=\"" + ref + "\">\n" + body + "\n</" + kind + ">")
	}
	return b.String(), attached, missing
}
