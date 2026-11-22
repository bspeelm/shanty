package tui

import (
	_ "embed"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpText is what :wiki shows, a section per command.
//
// It is a file rather than strings in the table so that the prose sits where
// prose belongs. A test holds its sections against the commands.
//
//go:embed help.md
var helpText string

// help returns the paragraphs describing one command, or nothing if it has
// none.
func helpFor(name string) string {
	want := "## " + name + "\n"
	at := strings.Index(helpText, want)
	if at < 0 {
		return ""
	}
	rest := helpText[at+len(want):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	return strings.TrimSpace(rest)
}

// helpTopics is every command the file describes.
func helpTopics() []string {
	var out []string
	for _, line := range strings.Split(helpText, "\n") {
		if name, found := strings.CutPrefix(line, "## "); found {
			out = append(out, strings.TrimSpace(name))
		}
	}
	return out
}

// helpLines is one command's description, wrapped to the width and split into
// rows, so the screen scrolls it the way it scrolls a list.
func helpLines(name string, w int) []string {
	body := helpFor(name)
	if body == "" {
		return []string{"  nothing is written about that yet"}
	}
	var out []string
	for _, para := range strings.Split(body, "\n\n") {
		if strings.HasPrefix(para, "    ") {
			// An indented block is typed as it is written, not reflowed.
			for _, line := range strings.Split(para, "\n") {
				out = append(out, "  "+strings.TrimRight(line, " "))
			}
			out = append(out, "")
			continue
		}
		out = append(append(out, wrapped(para, w)...), "")
	}
	if len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// wrapped is a paragraph as rows no wider than w, each indented as a page of
// the wiki is.
func wrapped(para string, w int) []string {
	var out []string
	text := lipgloss.NewStyle().Width(max(20, w-4)).Render(strings.Join(strings.Fields(para), " "))
	for _, line := range strings.Split(text, "\n") {
		out = append(out, "  "+line)
	}
	return out
}
