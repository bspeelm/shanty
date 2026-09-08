package tui

import (
	"strings"
	"testing"
)

// TestEveryCommandIsExplainedAndEveryExplanationIsACommand holds the file
// against the command table in both directions.
//
// A command nobody wrote about is one :wiki cannot explain, and a section for
// a command that does not exist is a page nobody can reach.
func TestEveryCommandIsExplainedAndEveryExplanationIsACommand(t *testing.T) {
	explained := map[string]bool{}
	for _, name := range helpTopics() {
		if explained[name] {
			t.Errorf("%q is explained twice", name)
		}
		explained[name] = true
	}

	exists := map[string]bool{}
	for _, c := range commands {
		exists[c.name] = true
		if !explained[c.name] {
			t.Errorf(":%s is a command and nothing explains it", c.name)
		}
	}
	for name := range explained {
		if !exists[name] {
			t.Errorf("%q is explained and is not a command", name)
		}
	}
}

// TestAnExplanationSaysMoreThanTheSummary covers the point of the page. A
// description that repeats the one-line summary is not worth opening.
func TestAnExplanationSaysMoreThanTheSummary(t *testing.T) {
	for _, c := range commands {
		body := helpFor(c.name)
		if body == "" {
			t.Errorf(":%s has no explanation", c.name)
			continue
		}
		if len(body) < len(c.summary)*2 {
			t.Errorf(":%s is explained in %d characters and summarised in %d", c.name, len(body), len(c.summary))
		}
	}
}

// TestACommandThatTakesAnArgumentShowsOne covers the "how to use it" half:
// anything with an argument has an example somebody can copy.
func TestACommandThatTakesAnArgumentShowsOne(t *testing.T) {
	for _, c := range commands {
		if c.argument == "" {
			continue
		}
		body := helpFor(c.name)
		if !strings.Contains(body, "    :"+c.name) {
			t.Errorf(":%s takes an argument and its page shows no example of one:\n%s", c.name, body)
		}
	}
}

// TestAnExplanationIsWrappedToTheWidth covers the rendering, including the
// indented examples, which are typed as written rather than reflowed.
func TestAnExplanationIsWrappedToTheWidth(t *testing.T) {
	lines := helpLines("volume", 40)

	if len(lines) < 3 {
		t.Fatalf("the page came out as %d lines", len(lines))
	}
	for _, line := range lines {
		if len(line) > 40 {
			t.Errorf("a line is %d characters wide, want at most 40: %q", len(line), line)
		}
	}
	var example bool
	for _, line := range lines {
		if strings.Contains(line, ":volume 40") {
			example = true
			if !strings.HasPrefix(line, "      ") {
				t.Errorf("the example lost its indent: %q", line)
			}
		}
	}
	if !example {
		t.Error("the example is missing from the page")
	}
}

// TestAskingAboutSomethingUnwrittenSaysSo covers a topic with no section,
// which a command added without its page would produce.
func TestAskingAboutSomethingUnwrittenSaysSo(t *testing.T) {
	if got := helpFor("nothing-like-this"); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
	lines := helpLines("nothing-like-this", 60)
	if len(lines) != 1 || !strings.Contains(lines[0], "nothing is written") {
		t.Errorf("the page reads %v", lines)
	}
}

// TestAPageStopsAtTheNextCommand covers the boundary between sections. A page
// that runs on shows another command's explanation under this one's heading,
// and the extra scrolls off the bottom where nobody sees it.
func TestAPageStopsAtTheNextCommand(t *testing.T) {
	topics := helpTopics()
	if len(topics) < 2 {
		t.Fatal("there is only one page to check")
	}

	for i, name := range topics[:len(topics)-1] {
		body := helpFor(name)
		if body == "" {
			t.Errorf("%q has no page", name)
			continue
		}
		if strings.Contains(body, "## ") {
			t.Errorf("the page for %q runs into another heading", name)
		}
		// The first line of the page that follows must not be in this one.
		next := helpFor(topics[i+1])
		if first, _, _ := strings.Cut(next, "\n"); first != "" && strings.Contains(body, first) {
			t.Errorf("the page for %q contains the start of %q", name, topics[i+1])
		}
	}

	// And the last page stops at the end of the file rather than running past it.
	if last := helpFor(topics[len(topics)-1]); strings.Contains(last, "## ") {
		t.Errorf("the last page runs into a heading: %q", last)
	}
}
