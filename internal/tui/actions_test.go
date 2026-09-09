package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// backticked is every `thing` written in the wiki page.
var backticked = regexp.MustCompile("`([^`]+)`")

// spelling is how the page writes a key that shanty names differently. The
// page is for reading, so it uses the symbol on the keyboard.
var spelling = map[string]string{
	"up": "↑", "down": "↓", "left": "←", "right": "→", " ": "space",
}

// keysPage is the wiki page that tells somebody what the keys do.
func keysPage(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "wiki", "Keys.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// TestEveryKeyIsDocumentedAndEveryDocumentedKeyIsBound is what ADR-016
// promised and did not have: the bindings and the page describing them, held
// against each other in both directions.
//
// A key that does something nobody wrote down cannot be discovered, and a page
// naming a key that does nothing is worse than saying nothing at all.
func TestEveryKeyIsDocumentedAndEveryDocumentedKeyIsBound(t *testing.T) {
	page := keysPage(t)

	// Keys the page explains in prose rather than naming in a table, because
	// they are typed as part of something rather than pressed on their own.
	prose := map[string]bool{
		"g": true,            // the prefix; the page tables what follows it
		"/": true, ":": true, // each opens a mode the page explains at length
		"%": true,            // written as {count}%
		"=": true, "_": true, // the unshifted spellings of + and -
	}

	bound := binding(Actions())
	for key, action := range bound {
		if prose[key] {
			continue
		}
		written := key
		if as, differs := spelling[key]; differs {
			written = as
		}
		if !strings.Contains(page, "`"+written+"`") {
			t.Errorf("`%s` is bound to %s and the page does not mention it", written, action.Name)
		}
	}

	// The other way round: everything the page writes as a key, and which
	// looks like one rather than a command or a two-key jump, is bound.
	asWritten := map[string]string{}
	for key := range bound {
		written := key
		if as, differs := spelling[key]; differs {
			written = as
		}
		asWritten[written] = key
	}
	// The page writes other things in the same style, and each is exempt for a
	// reason rather than because it was in the way.
	notAKey := map[string]string{
		"3j":     "an example of a count before a movement",
		"50%":    "an example of a count before %",
		"y":      "answers the question a deletion asks, which is its own mode",
		"tab":    "completes a command, which is its own mode",
		"★":      "the mark on something starred",
		"(A)":    "the mark on a track already in a playlist",
		"shanty": "the program",
		"[keys]": "the section of config.toml that binds them",
	}

	// The longest key anybody presses. Prose names tools, tags and settings in
	// the same style, and adding each to the list above as it appears is a
	// list that grows with the documentation rather than with the keys.
	const longestKey = len("backspace")

	for _, found := range backticked.FindAllStringSubmatch(page, -1) {
		written := found[1]
		if notAKey[written] != "" ||
			len(written) > longestKey || // a tool, a tag, a setting
			strings.ToUpper(written) == written && len(written) > 1 || // a tag name
			strings.HasPrefix(written, ":") || // a command
			strings.Contains(written, " ") || // a phrase, or a command with an argument
			strings.HasPrefix(written, "g") || // the two-key jumps
			strings.Contains(written, ".") || // a path or a file
			strings.Contains(written, "/") { // a path
			continue
		}
		if _, ok := asWritten[written]; !ok {
			t.Errorf("the page writes `%s` as a key and nothing is bound to it", written)
		}
	}
}

// TestEveryActionIsDescribedOnce covers the table itself: a name that appears
// twice is a binding nobody can change predictably, and a key bound to two
// actions is one of them doing nothing.
func TestEveryActionIsDescribedOnce(t *testing.T) {
	names := map[string]bool{}
	keys := map[string]string{}

	for _, a := range Actions() {
		if a.Name == "" {
			t.Error("an action has no name")
		}
		if names[a.Name] {
			t.Errorf("two actions are called %q", a.Name)
		}
		names[a.Name] = true

		if len(a.Summary) < 10 {
			t.Errorf("%s is described as %q", a.Name, a.Summary)
		}
		if a.do == nil {
			t.Errorf("%s does nothing", a.Name)
		}
		if len(a.Keys) == 0 {
			t.Errorf("%s has no key", a.Name)
		}
		for _, key := range a.Keys {
			if was, taken := keys[key]; taken {
				t.Errorf("%q is bound to both %s and %s", key, was, a.Name)
			}
			keys[key] = a.Name
		}
	}
}

// TestEveryActionIsReachableFromAKeyPress covers the dispatch. An action in
// the table that no key press reaches is one nobody can do.
func TestEveryActionIsReachableFromAKeyPress(t *testing.T) {
	m := loaded(t)
	bound := m.bound()

	for _, a := range Actions() {
		for _, key := range a.Keys {
			got, ok := bound[key]
			if !ok {
				t.Errorf("%q is listed under %s and reaches nothing", key, a.Name)
				continue
			}
			if got.Name != a.Name {
				t.Errorf("%q reaches %s, want %s", key, got.Name, a.Name)
			}
		}
	}
}

// TestBindingReplacesTheKeysAnActionCameWith covers the point of the
// configuration: a named action takes the keys it is given instead of its own.
func TestBindingReplacesTheKeysAnActionCameWith(t *testing.T) {
	keys, wrong := Bind(map[string][]string{"next": {"z"}})
	if len(wrong) != 0 {
		t.Fatalf("a good binding was refused: %v", wrong)
	}

	if got, ok := keys["z"]; !ok || got.Name != "next" {
		t.Errorf("z reaches %v, want next", got.Name)
	}
	if _, ok := keys["n"]; ok {
		t.Error("n still skips a track after being rebound")
	}
	// An action not named keeps what it came with.
	if got, ok := keys["p"]; !ok || got.Name != "previous" {
		t.Errorf("p reaches %v, want previous", got.Name)
	}
}

// TestBindingReportsEverythingWrongAtOnce covers somebody fixing a
// configuration, who wants the whole list rather than the first problem.
func TestBindingReportsEverythingWrongAtOnce(t *testing.T) {
	_, wrong := Bind(map[string][]string{
		"skip-forward": {"z"}, // not an action
		"pause":        {},    // bound to nothing
		"louder":       {"x"}, // not an action either
	})

	if len(wrong) != 3 {
		t.Fatalf("got %d complaints, want one for each mistake: %v", len(wrong), wrong)
	}
	all := strings.Join(wrong, "\n")
	for _, want := range []string{"skip-forward", "pause", "louder"} {
		if !strings.Contains(all, want) {
			t.Errorf("nothing said about %q: %v", want, wrong)
		}
	}
}

// TestARebindingThatCollidesIsReported covers two actions given the same key,
// which would otherwise leave one of them unreachable and silent.
func TestARebindingThatCollidesIsReported(t *testing.T) {
	_, wrong := Bind(map[string][]string{"next": {"x"}, "previous": {"x"}})

	if len(wrong) == 0 {
		t.Fatal("two actions were bound to one key without complaint")
	}
	if !strings.Contains(wrong[0], "both") {
		t.Errorf("the complaint reads %q", wrong[0])
	}
}

// TestARebindingOntoAKeyStillHeldIsReported covers taking a key an action
// still has, which is the commonest way to get this wrong.
func TestARebindingOntoAKeyStillHeldIsReported(t *testing.T) {
	// p still belongs to previous, so binding next to it is a collision.
	_, wrong := Bind(map[string][]string{"next": {"p"}})

	if len(wrong) == 0 {
		t.Fatal("an action took a key another still holds without complaint")
	}
	if !strings.Contains(strings.Join(wrong, " "), "previous") {
		t.Errorf("the complaint does not name what it collided with: %v", wrong)
	}
}

// TestAnUnknownActionIsAnsweredWithTheNearest covers a typo, which is answered
// with the word that was meant rather than a list to read.
func TestAnUnknownActionIsAnsweredWithTheNearest(t *testing.T) {
	_, wrong := Bind(map[string][]string{"volume": {"z"}})

	if len(wrong) != 1 {
		t.Fatalf("got %v", wrong)
	}
	if !strings.Contains(wrong[0], "volume-up") && !strings.Contains(wrong[0], "volume-down") {
		t.Errorf("the complaint does not suggest what was meant: %q", wrong[0])
	}
}

// TestABoundKeyActuallyDoesTheThing covers the whole way through: a key from
// the configuration reaching the action it names.
func TestABoundKeyActuallyDoesTheThing(t *testing.T) {
	keys, wrong := Bind(map[string][]string{"next": {"z"}})
	if len(wrong) != 0 {
		t.Fatal(wrong)
	}
	m := loaded(t).WithKeys(keys)

	if _, got := press(t, m, "z"); got == nil {
		t.Fatal("the rebound key did nothing")
	} else if _, ok := got.(SkipNext); !ok {
		t.Errorf("z emitted %#v, want SkipNext", got)
	}
	if _, got := press(t, m, "n"); got != nil {
		t.Errorf("n still emitted %#v after being rebound", got)
	}
}

// TestBindingNothingKeepsEveryKey covers the ordinary configuration, which
// says nothing about keys at all.
func TestBindingNothingKeepsEveryKey(t *testing.T) {
	keys, wrong := Bind(nil)
	if len(wrong) != 0 {
		t.Fatalf("binding nothing complained: %v", wrong)
	}
	if len(keys) != len(binding(Actions())) {
		t.Errorf("binding nothing gave %d keys, want the usual %d", len(keys), len(binding(Actions())))
	}
}

// The wiki's command table is written from the command set rather than by
// hand, and these mark the part of the page that is written.
const (
	tableStart = "<!-- commands: written by go test ./internal/tui/ -update -->"
	tableEnd   = "<!-- end commands -->"
)

// commandTable is the table wiki/Keys.md should hold.
func commandTable() string {
	var b strings.Builder
	b.WriteString(tableStart + "\n\n| Command | What it does |\n|---|---|\n")
	for _, c := range commands {
		// A vertical bar is a column separator, and `on|off` has one in it.
		name := strings.ReplaceAll(spelled(c), "|", "\\|")
		b.WriteString("| `:" + name + "` | " + c.summary + " |\n")
	}
	return b.String() + "\n" + tableEnd
}

// TestTheWikiTableIsWrittenFromTheCommands is what stops the two drifting.
//
// The table used to be typed out beside the command set, and the two grew
// apart: three lists of the same names existed in three orders, and a command
// missing from the page would have failed nothing. It is generated now, and
// this test is how it is generated.
func TestTheWikiTableIsWrittenFromTheCommands(t *testing.T) {
	path := filepath.Join("..", "..", "wiki", "Keys.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)

	from := strings.Index(page, tableStart)
	to := strings.Index(page, tableEnd)
	if from < 0 || to < 0 {
		t.Fatalf("wiki/Keys.md has no marked command table; it should hold\n%s", commandTable())
	}
	to += len(tableEnd)

	want := commandTable()
	if page[from:to] == want {
		return
	}
	if *update {
		if err := os.WriteFile(path, []byte(page[:from]+want+page[to:]), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("wrote the command table into wiki/Keys.md")
		return
	}
	t.Errorf("the wiki table is not what the commands say.\nRun: go test ./internal/tui/ -update\n\nwant:\n%s\n\ngot:\n%s",
		want, page[from:to])
}
