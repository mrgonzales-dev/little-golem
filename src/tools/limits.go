package tools

import (
	"fmt"
	"strings"

	"little-golem/src/config"
)

// EstimateTokens converts chars to a rough token count (~4 chars/token).
func EstimateTokens(nChars int) int {
	if nChars <= 0 {
		return 0
	}
	return nChars / 4
}

// ClipLine cuts one line to max chars, marking the cut. Valid UTF-8 safe.
func ClipLine(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.ToValidUTF8(s[:max], "") + "…"
}

// TruncateResult enforces the per-result character budget: head-only keep
// plus a notice reporting what was omitted and how to re-run narrower.
// hint names the recovery (e.g. "re-run with '| head -n 200'"); empty hint
// falls back to a generic narrowing suggestion. It returns the possibly
// shortened content, whether it truncated, and omitted char/line counts.
func TruncateResult(content, hint string) (string, bool, int, int) {
	max := config.ToolMaxChars
	if len(content) <= max {
		if strings.Count(content, "\n")+1 <= config.ToolMaxLines {
			return content, false, 0, 0
		}
		return truncateLines(content, hint), true, 0, 0
	}
	head := strings.ToValidUTF8(content[:max], "")
	// Avoid cutting mid-line when there is a recent newline: prefer a clean
	// break so the notice starts on its own line.
	if i := strings.LastIndex(head, "\n"); i > max-500 {
		head = head[:i]
	}
	omittedChars := len(content) - len(head)
	omittedLines := strings.Count(content[len(head):], "\n")
	if hint == "" {
		hint = "narrow the arguments and try again"
	}
	notice := fmt.Sprintf("\n… [truncated: %d chars", omittedChars)
	if omittedLines > 0 {
		notice += fmt.Sprintf(", ~%d lines", omittedLines)
	}
	notice += fmt.Sprintf(" omitted; %s]", hint)
	out := head + notice
	// The notice itself is small (<200 chars); head was <= max so out can
	// exceed max only by the notice length, which is intentional so the
	// omitted counts survive. Clamp defensively anyway.
	if len(out) > max+512 {
		out = strings.ToValidUTF8(out[:max+512], "")
	}
	return out, true, omittedChars, omittedLines
}

// truncateLines handles the rare case of many short lines exceeding the line
// budget while staying under the char budget.
func truncateLines(content, hint string) string {
	lines := strings.Split(content, "\n")
	keep := min(len(lines), config.ToolMaxLines)
	head := strings.Join(lines[:keep], "\n")
	if hint == "" {
		hint = "narrow the arguments and try again"
	}
	return fmt.Sprintf("%s\n… [truncated: ~%d lines omitted; %s]", head, len(lines)-keep, hint)
}

// defaultHintFor suggests a recovery per tool for truncation notices.
func defaultHintFor(name string) string {
	switch name {
	case "bash":
		return "re-run with '2>&1 | head -n 200' or a narrower command"
	case "read":
		return "read again with offset= to continue"
	case "grep", "glob":
		return "narrow the query/pattern or lower max_results"
	default:
		return "narrow the arguments and try again"
	}
}

// applyBudget truncates content to the tool budget with the tool's hint.
func applyBudget(toolName, content string) (string, bool) {
	out, truncated, _, _ := TruncateResult(content, defaultHintFor(toolName))
	return out, truncated
}
