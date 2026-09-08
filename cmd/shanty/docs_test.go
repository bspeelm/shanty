package main

import (
	"os"
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
