package tests

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"little-golem/src/config"
	"little-golem/src/ui"
)

func TestVerbsRotate(t *testing.T) {
	seen := map[string]bool{}
	for i := range 10 {
		seen[config.VerbAt(3, time.Duration(i)*config.VerbEvery)] = true
	}
	if len(seen) < 8 {
		t.Fatalf("verbs barely rotate: %v", seen)
	}
	if config.VerbAt(3, 0) != config.VerbAt(3, config.VerbEvery-time.Millisecond) {
		t.Fatal("verb changed within its window")
	}
}

func TestShimmerAnimates(t *testing.T) {
	a, b := ui.Shimmer("Golem walking…", 0), ui.Shimmer("Golem walking…", 5)
	if a == b {
		t.Fatal("frames render identically")
	}
	if ansi.Strip(a) != "Golem walking…" || ansi.Strip(b) != "Golem walking…" {
		t.Fatal("shimmer altered the text")
	}
}
