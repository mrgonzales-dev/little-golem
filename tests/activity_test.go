package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"little-golem/src/app"
	"little-golem/src/model"
	"little-golem/src/ui"
)

func TestWrappedActivityLineGrowsUpwardNotDownward(t *testing.T) {
	a := app.New(nil).App
	a.Ready, a.Width, a.Height = true, 60, 24
	ui.Layout(a)
	idleH := a.Viewport.Height()

	check := func(label string) []string {
		t.Helper()
		ls := screen(a)
		if len(ls) != a.Height {
			t.Fatalf("%s: screen is %d rows, want %d (the bottom bar was pushed off)", label, len(ls), a.Height)
		}
		if last := ansi.Strip(ls[len(ls)-1]); !strings.Contains(last, "little-golem") {
			t.Fatalf("%s: last row is %q, want the header bar", label, last)
		}
		if in := ansi.Strip(ls[len(ls)-2]); !strings.Contains(in, "Ask anything") {
			t.Fatalf("%s: input is not directly above the header: %q", label, in)
		}
		return ls
	}
	check("idle")

	// A turn with a long thought: the stats no longer fit one row at width 60.
	now := time.Now()
	a.Busy = true
	a.TurnStart = now.Add(-2*time.Minute - 18*time.Second)
	a.TurnDone = 71
	a.Entries = []model.Entry{{
		Kind: model.EntryAssistant, Reasoning: "hmm", Streaming: true,
		Started: now.Add(-66 * time.Second), ThinkEnd: now, Content: "x",
	}}
	ls := check("working, wrapped")
	if a.ActivityH != 2 || a.Viewport.Height() != idleH-1 {
		t.Fatalf("activity=%d rows, viewport %d (idle %d): transcript should have given up exactly one row", a.ActivityH, a.Viewport.Height(), idleH)
	}
	// The second activity row sits right above the input, the first above it.
	if !strings.Contains(ansi.Strip(ls[len(ls)-4]), "esc to interrupt") {
		t.Fatalf("activity line did not extend upward:\n%s", ansi.Strip(strings.Join(ls[len(ls)-6:], "\n")))
	}

	// Done: the row is handed back to the transcript.
	a.Busy = false
	a.Entries = nil
	check("idle again")
	if a.ActivityH != 1 || a.Viewport.Height() != idleH {
		t.Fatalf("viewport not restored: activity=%d viewport=%d want %d", a.ActivityH, a.Viewport.Height(), idleH)
	}
}
