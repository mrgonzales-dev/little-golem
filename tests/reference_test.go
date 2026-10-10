package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/ui"
)

func typeKeys(a *model.App, s string) {
	for _, r := range s {
		a.Input, _ = a.Input.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		app.RefreshRefs(a)
	}
}

func fakeFinder(t *testing.T, files ...string) *[]string {
	t.Helper()
	var queries []string
	prev := app.FindPaths
	t.Cleanup(func() { app.FindPaths = prev })
	app.FindPaths = func(q string, _ int) []string { queries = append(queries, q); return files }
	return &queries
}

func TestAtOpensPickerAndEnterInsertsFile(t *testing.T) {
	queries := fakeFinder(t, "src/main.go", "src/mod.go")
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))

	typeKeys(a, "look at @ma")
	if len(a.Refs) != 2 || (*queries)[len(*queries)-1] != "ma" {
		t.Fatalf("picker: %v queries %v", a.Refs, *queries)
	}
	if !strings.Contains(stripANSI(ui.RefBlock(a)), "› src/main.go") {
		t.Fatalf("picker not drawn: %q", ui.RefBlock(a))
	}

	press := func(k tea.KeyPressMsg) { app.HandleKey(a, k); app.RefreshRefs(a) }
	press(tea.KeyPressMsg{Code: tea.KeyDown})
	if a.RefSel != 1 {
		t.Fatalf("sel %d", a.RefSel)
	}
	press(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := a.Input.Value(); got != "look at @src/mod.go " || len(a.Refs) != 0 || a.Busy {
		t.Fatalf("input %q refs %v busy %v", got, a.Refs, a.Busy)
	}
}

func TestEscClosesPickerUntilWordChanges(t *testing.T) {
	fakeFinder(t, "a.go")
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	typeKeys(a, "@a")
	app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEscape})
	app.RefreshRefs(a)
	if len(a.Refs) != 0 || a.Input.Value() != "@a" {
		t.Fatalf("not closed: %v %q", a.Refs, a.Input.Value())
	}
	typeKeys(a, "b")
	if len(a.Refs) != 1 {
		t.Fatal("picker did not reopen")
	}
}

func TestNoPickerForEmailOrPlainWords(t *testing.T) {
	fakeFinder(t, "a.go")
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	typeKeys(a, "mail me a@b")
	if len(a.Refs) != 0 {
		t.Fatalf("picker opened: %v", a.Refs)
	}
}

func TestSendAttachesReferencedFiles(t *testing.T) {
	dir := inTempProject(t)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc A() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat(strings.Repeat("x", 100)+"\n", 150)), 0o644)
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	a.History = nil

	a.Input.SetValue("explain @a.go, and @big.txt and @nope.go but not x@a.go")
	app.Send(a)

	sent := a.History[0].Content
	if !strings.HasPrefix(sent, "explain @a.go, and @big.txt") || !strings.Contains(sent, "<file path=\"a.go\">") ||
		!strings.Contains(sent, "func A() {}") || strings.Contains(sent, "nope.go\">") {
		t.Fatalf("message: %.300q", sent)
	}
	if !strings.Contains(sent, "cut for length") || len(sent) > 12000 {
		t.Fatalf("big file not cut: %d bytes", len(sent))
	}
	if a.Entries[0].Content != "explain @a.go, and @big.txt and @nope.go but not x@a.go" {
		t.Fatalf("transcript must show what was typed: %q", a.Entries[0].Content)
	}
	if n := a.Entries[1]; n.Kind != model.EntryNote || n.Content != "attached @a.go @big.txt" {
		t.Fatalf("note %+v", n)
	}
	if a.Notice != "not found: @nope.go" {
		t.Fatalf("notice %q", a.Notice)
	}
}

func TestPickerKeepsSpacesAndInsertsQuotes(t *testing.T) {
	queries := fakeFinder(t, "my file.txt")
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))

	typeKeys(a, `@"my `)
	if len(a.Refs) != 1 || (*queries)[len(*queries)-1] != "my " {
		t.Fatalf("picker query: %v queries %v", a.Refs, *queries)
	}

	app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := a.Input.Value(); got != `@"my file.txt" ` {
		t.Fatalf("input %q", got)
	}
}

func TestSendOutsideProjectIsAttached(t *testing.T) {
	inTempProject(t)
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	a.History = nil
	a.Input.SetValue("see @/etc/hostname")
	app.Send(a)
	if !strings.Contains(a.History[0].Content, "<file") || a.Notice != "" {
		t.Fatalf("%q / %q", a.History[0].Content, a.Notice)
	}
}

func TestSendQuotedReferenceWithSpaces(t *testing.T) {
	dir := inTempProject(t)
	name := "SweldoMo DTR Export - Sample.txt"
	os.WriteFile(filepath.Join(dir, name), []byte("hello holiday\n"), 0o644)
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	a.History = nil

	a.Input.SetValue(`explain @"` + name + `" please`)
	app.Send(a)

	sent := a.History[0].Content
	if !strings.Contains(sent, `<file path="`+name+`">`) || !strings.Contains(sent, "hello holiday") {
		t.Fatalf("message: %q", sent)
	}
	if n := a.Entries[1]; n.Kind != model.EntryNote || n.Content != `attached @"`+name+`"` {
		t.Fatalf("note %+v", n)
	}
	if a.Notice != "" {
		t.Fatalf("notice %q", a.Notice)
	}
}

func TestFolderReferenceKeepsPickerOpenAndListsEntries(t *testing.T) {
	dir := inTempProject(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "x.go"), []byte("x"), 0o644)
	queries := fakeFinder(t, "src/")
	var bodies []string
	a := compactApp(fakeLlama(t, "s", false, &bodies))
	a.History = nil

	typeKeys(a, "@sr")
	app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyTab})
	app.RefreshRefs(a)
	if a.Input.Value() != "@src/" || (*queries)[len(*queries)-1] != "src/" || len(a.Refs) == 0 {
		t.Fatalf("input %q queries %v refs %v", a.Input.Value(), *queries, a.Refs)
	}

	a.Input.SetValue("what is in @src/")
	app.Send(a)
	sent := a.History[0].Content
	if !strings.Contains(sent, `<folder path="src/">`) || !strings.Contains(sent, "x.go") {
		t.Fatalf("message: %q", sent)
	}
}
