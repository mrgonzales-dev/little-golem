package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/ui"
)

func selectFixture(t *testing.T) *model.App {
	t.Helper()
	m := app.New(nil)
	a := m.App
	a.Ready = true
	a.Width, a.Height = 100, 30
	for i := 0; i < 12; i++ {
		a.Entries = append(a.Entries,
			model.Entry{Kind: model.EntryUser, Content: fmt.Sprintf("question %d", i)},
			model.Entry{Kind: model.EntryAssistant, Content: "answer line one\n\nsecond paragraph here with some words", Started: time.Now(), Ended: time.Now()})
	}
	ui.Layout(a)
	if a.Viewport.TotalLineCount() == 0 {
		t.Fatal("no viewport content")
	}
	return a
}

func TestSelectDragKeepsRange(t *testing.T) {
	a := selectFixture(t)
	off := a.Viewport.YOffset()

	if _, ok := app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft}); !ok {
		t.Fatal("click not handled")
	}
	if !a.SelActive || !a.SelDragging || a.SelAnchor != off+1 {
		t.Fatalf("anchor=%d want %d active=%v dragging=%v", a.SelAnchor, off+1, a.SelActive, a.SelDragging)
	}
	if a.Follow {
		t.Fatal("Follow should be off during selection")
	}
	if _, ok := app.Scroll(a, tea.MouseMotionMsg{X: 5, Y: 3, Button: tea.MouseLeft}); !ok {
		t.Fatal("drag not handled")
	}
	lo, hi, _ := a.SelRange()
	if hi-lo != 2 {
		t.Fatalf("range %d-%d want span 2", lo, hi)
	}
	text, ok := ui.SelectedText(a)
	if !ok || strings.TrimSpace(text) == "" {
		t.Fatal("selected text empty")
	}
	cmd, ok := app.Scroll(a, tea.MouseReleaseMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	if !ok {
		t.Fatal("release not handled")
	}
	if cmd == nil {
		t.Fatal("release should copy selection to clipboard")
	}
	if a.SelActive || a.SelDragging {
		t.Fatal("selection should auto-release after copy")
	}
	if a.Toast != "Copied to clipboard" {
		t.Fatalf("toast=%q want copy confirmation", a.Toast)
	}
	if _, ok := ui.SelectedText(a); ok {
		t.Fatal("cleared selection should yield no text")
	}
}

func TestSelectBareClickClears(t *testing.T) {
	a := selectFixture(t)
	app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	cmd, _ := app.Scroll(a, tea.MouseReleaseMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	if a.SelActive {
		t.Fatal("bare click should not leave a selection")
	}
	if cmd != nil || a.Toast != "" {
		t.Fatal("bare click must not copy or toast")
	}
}

func TestSelectRightClickCopies(t *testing.T) {
	a := selectFixture(t)
	app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	app.Scroll(a, tea.MouseMotionMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	// Right-click mid-drag copies the in-progress selection.
	cmd, ok := app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseRight})
	if !ok || cmd == nil {
		t.Fatal("right-click should copy selection")
	}
	if a.SelActive {
		t.Fatal("selection should auto-release after copy")
	}
}

func TestSelectCtrlCCopiesInsteadOfQuit(t *testing.T) {
	a := selectFixture(t)
	app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	app.Scroll(a, tea.MouseMotionMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	// ctrl+c mid-drag (before release-copy) copies instead of quitting.
	cmd, handled := app.HandleKey(a, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !handled || cmd == nil {
		t.Fatal("ctrl+c with selection should copy")
	}
	if a.SelActive {
		t.Fatal("selection should clear after ctrl+c copy")
	}
	if a.Toast != "Copied to clipboard" {
		t.Fatalf("toast=%q want copy confirmation", a.Toast)
	}
	if a.ConfirmQuit {
		t.Fatal("ctrl+c copy must not arm quit")
	}
}

func TestSelectEscClears(t *testing.T) {
	a := selectFixture(t)
	app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	app.Scroll(a, tea.MouseMotionMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	// Esc mid-drag (before release-copy) drops the highlight with no copy.
	if _, handled := app.HandleKey(a, tea.KeyPressMsg{Code: tea.KeyEscape}); !handled {
		t.Fatal("esc not handled")
	}
	if a.SelActive {
		t.Fatal("esc should clear selection")
	}
	if a.Toast != "" {
		t.Fatal("esc-cleared selection must not copy or toast")
	}
}

func TestMouseDisabledFallsBackToNative(t *testing.T) {
	a := selectFixture(t)
	a.MouseEnabled = false
	if v := ui.View(a); v.MouseMode != tea.MouseModeNone {
		t.Fatal("disabled mouse should report MouseModeNone")
	}
	if _, ok := app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft}); ok {
		t.Fatal("clicks should pass through when mouse is disabled")
	}
	a.MouseEnabled = true
	if v := ui.View(a); v.MouseMode != tea.MouseModeCellMotion {
		t.Fatal("enabled mouse should report CellMotion for wheel scroll")
	}
}

func TestToastRendersTopRight(t *testing.T) {
	a := selectFixture(t)
	app.Scroll(a, tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	app.Scroll(a, tea.MouseMotionMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	app.Scroll(a, tea.MouseReleaseMsg{X: 5, Y: 3, Button: tea.MouseLeft})

	s := fmt.Sprint(ui.View(a).Content)
	plain := ansi.Strip(s)
	if !strings.Contains(plain, "Copied to clipboard") {
		t.Fatal("toast text missing from view")
	}
	lines := strings.Split(s, "\n")
	found := -1
	for i, l := range lines {
		if strings.Contains(ansi.Strip(l), "Copied to clipboard") {
			found = i
			break
		}
	}
	if found < 0 || found > 3 {
		t.Fatalf("toast on line %d, want pinned near top", found)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 100 {
			t.Fatalf("line %d width %d, want 100", i, w)
		}
	}
	// Toast box sits at the right edge: the toast line's visible text
	// should end near the right margin, not at the left.
	toastLine := ansi.Strip(lines[found])
	if idx := strings.Index(toastLine, "Copied to clipboard"); idx < 50 {
		t.Fatalf("toast starts at col %d, want top-right placement", idx)
	}
}

func TestToastExpires(t *testing.T) {
	a := selectFixture(t)
	a.Toast, a.ToastAt = "Copied to clipboard", time.Now().Add(-10*time.Second)
	if a.ToastVisible() {
		t.Fatal("stale toast should not be visible")
	}
	if strings.Contains(ansi.Strip(fmt.Sprint(ui.View(a).Content)), "Copied to clipboard") {
		t.Fatal("expired toast should not render")
	}

	a.ToastAt = time.Now()
	if !a.ToastVisible() {
		t.Fatal("fresh toast should be visible")
	}
	m := &app.M{App: a}
	updated, _ := m.Update(app.ClearToastMsg{})
	if updated.(*app.M).Toast != "" {
		t.Fatal("ClearToastMsg should dismiss the toast")
	}
}
