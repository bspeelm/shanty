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
	}

	for _, found := range backticked.FindAllStringSubmatch(page, -1) {
		written := found[1]
		if notAKey[written] != "" ||
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
