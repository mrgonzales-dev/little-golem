package ui

import (
	"charm.land/glamour/v2"
)

// md is the shared glamour renderer; per-call renderers are used when a
// specific width is required (user bubble fit-content, assistant fit-view).
var md *glamour.TermRenderer

func init() {
	md = mustRenderer("dark", 80)
}

func mustRenderer(style string, width int) *glamour.TermRenderer {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	return r
}

// mdCache memoizes RenderMarkdown so animation frames do not re-run glamour
// over unchanged messages. It is dropped on wrap changes and when it grows.
var mdCache = map[string]string{}

// RenderMarkdown renders s at the current wrap width.
func RenderMarkdown(s string) string {
	if md == nil || s == "" {
		return s
	}
	if out, ok := mdCache[s]; ok {
		return out
	}
	out, err := md.Render(s)
	if err != nil {
		return s
	}
	if len(mdCache) > 256 {
		clear(mdCache)
	}
	mdCache[s] = out
	return out
}

// RenderMarkdownWidth renders s wrapped to the given width.
func RenderMarkdownWidth(s string, width int) string {
	if s == "" {
		return s
	}
	r := mustRenderer("dark", width)
	if r == nil {
		return s
	}
	out, err := r.Render(s)
	if err != nil {
		return s
	}
	return out
}

// SetWrap re-creates the shared renderer with a new word wrap width. Use
// during layout to keep markdown output aligned with the viewport.
func SetWrap(width int) {
	r := mustRenderer("dark", width)
	if r != nil {
		md = r
		clear(mdCache)
	}
}
