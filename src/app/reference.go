package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/config"
	"little-golem/src/model"
	"little-golem/src/tools"
)

const (
	refRows       = 6
	refFileBytes  = 8000  // of one @reference put into the message
	refTotalBytes = 20000 // of all @references in one message
	refLines      = 200
)

// FindPaths lists files and folders matching what follows an @; tests
// replace it.
var FindPaths = tools.FindPaths

var refRe = regexp.MustCompile(`(?:^|\s)@(\S+)`)

// refToken is the @word that ends at the cursor, or "".
func refToken(m *model.App) string {
	lines := strings.Split(m.Input.Value(), "\n")
	if m.Input.Line() >= len(lines) {
		return ""
	}
	r := []rune(lines[m.Input.Line()])
	end := min(m.Input.Column(), len(r))
	start := end
	for start > 0 && !unicode.IsSpace(r[start-1]) {
		start--
	}
	if tok := string(r[start:end]); strings.HasPrefix(tok, "@") {
		return tok
	}
	return ""
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
		m.Refs = FindPaths(tok[1:], refRows)
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
		m.Input.InsertString("@" + path)
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
// them without a read call. It returns the message to send, the
// references attached and those that could not be read.
func expandRefs(text string) (string, []string, []string) {
	var b strings.Builder
	b.WriteString(text)
	var attached, missing []string
	seen := map[string]bool{}
	budget := refTotalBytes
	rd := tools.NewRead()
	for _, mt := range refRe.FindAllStringSubmatch(text, -1) {
		ref := strings.TrimRight(mt[1], ".,;:!?)]}\"'")
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		args, _ := json.Marshal(map[string]any{"path": ref, "limit": refLines})
		res, err := rd.Run(context.Background(), args)
		if err != nil {
			missing = append(missing, "@"+ref)
			continue
		}
		if len(attached) == 0 {
			b.WriteString("\n\nFiles and folders the user referenced with @ (already read for you):")
		}
		attached = append(attached, "@"+ref)
		body, room := res.Content, min(refFileBytes, budget)
		switch {
		case room <= 0:
			body = "(not attached: no room left; use the read tool)"
		case len(body) > room:
			body = body[:strings.LastIndex(body[:room], "\n")+1] + "… [cut for length; use the read tool with offset to see more]"
		}
		budget -= len(body)
		kind := "file"
		if st, err := os.Stat(filepath.Join(config.WorkDir, ref)); err == nil && st.IsDir() {
			kind = "folder"
		}
		b.WriteString("\n\n<" + kind + " path=\"" + ref + "\">\n" + body + "\n</" + kind + ">")
	}
	return b.String(), attached, missing
}
