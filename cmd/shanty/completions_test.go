package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// generate writes the completion script for one shell.
func generate(t *testing.T, shell string) string {
	t.Helper()
	env, _ := scratch(t)
	out := &bytes.Buffer{}
	env.Stdout = out
	if err := runCompletions(t.Context(), env, []string{shell}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// offered pulls the words the bash script completes a first argument with.
var offered = regexp.MustCompile(`compgen -W '([^']*)' -- "\$cur"\)\)\n\t\treturn`)

// TestEveryCommandCompletesAndOnlyCommandsDo holds the scripts against the
// command table in both directions. A command a shell will not complete is
// one nobody discovers; a word a shell offers that is not a command sends the
// reader somewhere that does not exist.
func TestEveryCommandCompletesAndOnlyCommandsDo(t *testing.T) {
	exists := map[string]bool{}
	for _, c := range commands() {
		exists[c.name] = true
	}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			script := generate(t, shell)
			for name := range exists {
				if !strings.Contains(script, name) {
					t.Errorf("`shanty %s` exists and %s will not complete it", name, shell)
				}
			}
		})
	}

	// The bash script names its words in one place, so they can be read back
	// and checked the other way round.
	found := offered.FindStringSubmatch(generate(t, "bash"))
	if found == nil {
		t.Fatal("the bash script no longer offers a word list; this test has stopped matching")
	}
	words := strings.Fields(found[1])
	if len(words) != len(exists) {
		t.Errorf("bash offers %d words and there are %d commands", len(words), len(exists))
	}
	for _, word := range words {
		if !exists[word] {
			t.Errorf("bash completes %q, which is not a command", word)
		}
	}
}

// TestCompletionOffersNothingRatherThanFiles covers the thing that made this
// worth building. Without it a shell falls back to listing the directory,
// which is never what follows any of these commands.
func TestCompletionOffersNothingRatherThanFiles(t *testing.T) {
	bash := generate(t, "bash")
	if strings.Contains(bash, "-o default") || strings.Contains(bash, "-o filenames") {
		t.Error("the bash script lets the shell fall back to listing files")
	}
	if !strings.Contains(bash, "*) COMPREPLY=() ;;") {
		t.Error("the bash script does not answer an unknown command with nothing")
	}
	if fish := generate(t, "fish"); !strings.Contains(fish, "complete -c shanty -f\n") {
		t.Error("the fish script does not turn file completion off")
	}
}

// TestAShellWithNoScriptIsToldWhichHaveOne covers the mistake of asking for a
// shell shanty does not write for.
func TestAShellWithNoScriptIsToldWhichHaveOne(t *testing.T) {
	env, _ := scratch(t)

	err := runCompletions(t.Context(), env, []string{"nushell"})
	if err == nil {
		t.Fatal("a shell with no script was accepted")
	}
	for _, shell := range shells {
		if !strings.Contains(err.Error(), shell) {
			t.Errorf("the message does not mention %s: %q", shell, err)
		}
	}

	if err := runCompletions(t.Context(), env, nil); err == nil {
		t.Error("naming no shell at all was accepted")
	}
}

// TestADescriptionCannotBreakOutOfItsQuotes covers a summary containing the
// character the shells quote with. Nothing in the table does today, and this
// is what keeps that from mattering.
func TestADescriptionCannotBreakOutOfItsQuotes(t *testing.T) {
	got := quote("a shell's description")
	if !strings.Contains(got, `'\''`) {
		t.Errorf("quote(%q) = %q, which ends the quoting early", "a shell's description", got)
	}
}

// TestEveryShellProducesAScript covers the closed set, so a shell named in the
// list but not built cannot be offered.
func TestEveryShellProducesAScript(t *testing.T) {
	for _, shell := range shells {
		if script := generate(t, shell); len(script) < 100 {
			t.Errorf("the %s script is %d bytes", shell, len(script))
		}
	}
}

// TestAClosedSetAfterACommandIsCompleted covers the arguments that are worth
// offering. Most commands take a number, a position or nothing, and complete
// to nothing; the two with a fixed list of answers offer it.
func TestAClosedSetAfterACommandIsCompleted(t *testing.T) {
	exists := map[string]bool{}
	for _, c := range commands() {
		exists[c.name] = true
	}

	for name, words := range following() {
		if !exists[name] {
			t.Errorf("%q accepts a list of words and is not a command", name)
		}
		for _, shell := range shells {
			script := generate(t, shell)
			for _, word := range words {
				// Each shell spells an option its own way -- fish writes
				// "-o json" where bash writes "-json" -- so what is held
				// against the script is the word itself.
				if !strings.Contains(script, strings.TrimPrefix(word, "-")) {
					t.Errorf("%s does not offer %q after `shanty %s`", shell, word, name)
				}
			}
		}
	}

	// Every shell shanty writes a script for is a shell it will complete.
	for _, shell := range shells {
		if !slices.Contains(following()["completions"], shell) {
			t.Errorf("`shanty completions` writes a %s script and will not complete its name", shell)
		}
	}
}

// documentedFlag finds an option written after a command in the README.
var documentedFlag = regexp.MustCompile(`(?m)^shanty ([a-z-]+) (-[a-z][a-z-]*)`)

// TestEveryDocumentedOptionIsCompleted holds the completion set against the
// README. Checking only what the set already contains cannot notice something
// missing from it, which is how `shanty doctor -json` went uncovered.
func TestEveryDocumentedOptionIsCompleted(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	found := documentedFlag.FindAllStringSubmatch(commandBlocks(string(readme)), -1)
	if len(found) == 0 {
		t.Fatal("the README documents no options at all; this test has stopped matching")
	}
	for _, m := range found {
		name, flag := m[1], m[2]
		if !slices.Contains(following()[name], flag) {
			t.Errorf("the README documents `shanty %s %s` and no shell completes it", name, flag)
		}
	}
}

// TestInstallingCompletionsWritesOnlyWhereAShellReads covers what the
// installer does. Nobody creates a directory and no startup file is touched:
// a shell reads completions from a directory of its own, so the file goes
// there and the shell finds it by itself.
func TestInstallingCompletionsWritesOnlyWhereAShellReads(t *testing.T) {
	env, _ := scratch(t)
	home := os.Getenv("HOME")
	// Both shells are present, so both files are expected.
	env.LookPath = func(string) (string, error) { return "/usr/bin/anything", nil }

	written, err := install(env)
	if err != nil {
		t.Fatal(err)
	}

	want := env.Paths.Completions()
	if len(written) != len(want) {
		t.Fatalf("the installer wrote %d files and there are %d shells: %v", len(written), len(want), written)
	}
	for shell, path := range want {
		if !slices.Contains(written, path) {
			t.Errorf("no %s completion was written", shell)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if !strings.Contains(string(body), "shanty") {
			t.Errorf("the %s file does not look like a completion script", shell)
		}
	}

	// Nothing was added to a startup file.
	for _, name := range []string{".bashrc", ".bash_profile", ".profile", ".zshrc", ".config/fish/config.fish"} {
		if _, err := os.Stat(filepath.Join(home, name)); err == nil {
			t.Errorf("the installer created or touched %s", name)
		}
	}
}

// TestACompletionIsNotWrittenForAShellThatIsNotHere covers the machine with
// one shell on it. A file in a directory nothing reads is litter.
func TestACompletionIsNotWrittenForAShellThatIsNotHere(t *testing.T) {
	env, _ := scratch(t)
	env.LookPath = func(name string) (string, error) {
		if name == "bash" {
			return "/usr/bin/bash", nil
		}
		return "", errors.New("not here")
	}

	written, err := install(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 || written[0] != env.Paths.Completions()["bash"] {
		t.Errorf("the installer wrote %v, want only the bash completion", written)
	}
}

// TestUninstallTakesTheCompletionsBack covers the other half of the promise.
// The installer put these outside the four directories, so uninstall is what
// leaves nothing behind.
func TestUninstallTakesTheCompletionsBack(t *testing.T) {
	env, out := scratch(t)
	home := os.Getenv("HOME")
	env.LookPath = func(string) (string, error) { return "/usr/bin/anything", nil }

	written, err := install(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := runUninstall(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}

	for _, path := range written {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", path)
		}
		if !strings.Contains(out.String(), path) {
			t.Errorf("uninstall removed %s without saying so", path)
		}
	}

	var left []string
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			left = append(left, path)
		}
		return nil
	})
	if len(left) != 0 {
		t.Errorf("uninstall left %v", left)
	}
}
