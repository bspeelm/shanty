package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Prose has no compiler, and the README's command list is the first thing a
// reader believes. Held against the command table in both directions.
//
// Only the fenced blocks count. A sentence that happens to begin "shanty needs
// mpv" is prose, and reading it as a command list made this test fail on a
// correct README -- a parser too eager is a gate that gets worked around.
var (
	fenced     = regexp.MustCompile("(?s)```sh\n(.*?)```")
	documented = regexp.MustCompile(`(?m)^shanty ([a-z-]+)`)
)

// commandBlocks is every fenced shell block in the README, joined.
func commandBlocks(readme string) string {
	var out strings.Builder
	for _, block := range fenced.FindAllStringSubmatch(readme, -1) {
		out.WriteString(block[1])
	}
	return out.String()
}

func TestEveryCommandIsDocumentedAndEveryDocumentedCommandExists(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(raw)

	exists := map[string]bool{}
	for _, c := range commands() {
		exists[c.name] = true
		if !strings.Contains(readme, "shanty "+c.name) {
			t.Errorf("`shanty %s` exists and the README does not mention it", c.name)
		}
	}

	var checked int
	for _, m := range documented.FindAllStringSubmatch(commandBlocks(readme), -1) {
		checked++
		if !exists[m[1]] {
			t.Errorf("the README documents `shanty %s`, which is not a command", m[1])
		}
	}
	if checked < len(commands()) {
		t.Fatalf("the README parser found %d commands and there are %d; it has stopped matching",
			checked, len(commands()))
	}
}

// Every command's summary is a sentence a user can act on, not a label.
func TestEveryCommandHasASummary(t *testing.T) {
	for _, c := range commands() {
		if len(c.summary) < 10 {
			t.Errorf("`shanty %s` has no useful summary: %q", c.name, c.summary)
		}
		if c.run == nil {
			t.Errorf("`shanty %s` is listed with nothing to run", c.name)
		}
	}
}

// wikiPage reads one of the published pages.
func wikiPage(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "wiki", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// counted finds "reports on N things" in a page, however it is phrased around
// it.
var counted = regexp.MustCompile(`reports on ([a-z]+) things`)

// numbers are the counts the pages spell out.
var numbers = map[string]int{
	"four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
	"nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
}

// TestThePagesSayHowManyChecksThereAre covers a number three pages repeat and
// nothing was holding. Two checks were added and the pages went on saying
// eight and nine.
func TestThePagesSayHowManyChecksThereAre(t *testing.T) {
	want := len(checkIDs)
	var found int

	for _, page := range []string{"The-doctor.md", "Commands.md", "Installing.md"} {
		body := wikiPage(t, page)
		for _, m := range counted.FindAllStringSubmatch(body, -1) {
			found++
			got, known := numbers[m[1]]
			if !known {
				t.Errorf("%s says %q things, which is not a number this test knows", page, m[1])
				continue
			}
			if got != want {
				t.Errorf("%s says the doctor reports on %s things; there are %d", page, m[1], want)
			}
		}
	}
	if found == 0 {
		t.Fatal("no page says how many checks there are; this test has stopped matching")
	}
}

// TestEveryCheckIsDescribedOnItsPage holds the doctor's closed set against the
// page that explains it, in both directions. A check nobody wrote about is one
// whose failure means nothing to the reader.
func TestEveryCheckIsDescribedOnItsPage(t *testing.T) {
	page := wikiPage(t, "The-doctor.md")

	for _, id := range checkIDs {
		if !strings.Contains(page, "**`"+id+"`**") {
			t.Errorf("the doctor reports %q and The-doctor.md does not describe it", id)
		}
	}

	described := regexp.MustCompile("(?m)^\\*\\*`([a-z-]+)`\\*\\*")
	exists := map[string]bool{}
	for _, id := range checkIDs {
		exists[id] = true
	}
	for _, m := range described.FindAllStringSubmatch(page, -1) {
		if !exists[m[1]] {
			t.Errorf("The-doctor.md describes %q, which the doctor does not report", m[1])
		}
	}
}
