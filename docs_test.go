// Package shanty holds no code. It holds the compiler for the prose that
// describes the code, which has none of its own.
//
// The README makes claims about what commands exist and what they do. Those
// claims rot silently -- this project's own `make check` line was false within
// one commit of being written -- so the two most rot-prone of them are pinned
// here against the Makefile that actually defines them.
package shanty

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every `make <target>` the README mentions, in prose or in a fenced block.
var documentedTarget = regexp.MustCompile(`make ([a-z][a-z-]*)`)

// A Makefile rule: a target at the start of a line, followed by its
// prerequisites.
var makefileRule = regexp.MustCompile(`(?m)^([a-z][a-z-]*):(.*)$`)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func makefileTargets(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range makefileRule.FindAllStringSubmatch(read(t, "Makefile"), -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

func TestEveryDocumentedMakeTargetExists(t *testing.T) {
	targets := makefileTargets(t)

	var checked int
	for _, m := range documentedTarget.FindAllStringSubmatch(read(t, "README.md"), -1) {
		name := m[1]
		checked++
		if _, found := targets[name]; !found {
			t.Errorf("README.md documents `make %s`, which the Makefile does not define", name)
		}
	}

	// The self-check §5.3 asks for: a parser that silently stops matching
	// reports a clean run over nothing at all, which is the failure mode a
	// prose compiler is most likely to have.
	if checked < 3 {
		t.Fatalf("the README parser found %d make targets; it has stopped matching", checked)
	}
}

// The specific rot this exists for. The README describes what `make check`
// runs; the Makefile decides. When a prerequisite is added to the gate and the
// description is not updated, the README quietly understates what a green
// check proves.
func TestTheDocumentedGateMatchesTheMakefile(t *testing.T) {
	prereqs := strings.Fields(makefileTargets(t)["check"])
	if len(prereqs) == 0 {
		t.Fatal("the Makefile's check target has no prerequisites; the gate is empty")
	}

	readme := read(t, "README.md")
	line := lineContaining(t, readme, "make check")

	for _, p := range prereqs {
		// `race` is described as "race" rather than by its target name, which
		// is the one place the prose is allowed to read better than the
		// Makefile.
		if !strings.Contains(line, p) {
			t.Errorf("the Makefile runs %q as part of `make check`, which the README's description does not mention:\n  %s", p, line)
		}
	}
}

// Nothing in the README may claim the repository has no code once it does. The
// status table is the first thing a reader believes and the last thing anyone
// remembers to update.
func TestTheStatusTableDoesNotClaimAnEmptyRepository(t *testing.T) {
	readme := read(t, "README.md")
	for _, stale := range []string{"| code | none |", "Nothing is built yet"} {
		if strings.Contains(readme, stale) {
			t.Errorf("README.md still says %q, and there is code in internal/", stale)
		}
	}
}

func lineContaining(t *testing.T, text, sub string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, sub) {
			return line
		}
	}
	t.Fatalf("README.md contains no line mentioning %q", sub)
	return ""
}
