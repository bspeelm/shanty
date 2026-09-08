package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// shells is every shell completions can be written for. The set is closed, and
// a name that is not here is answered with the list.
var shells = []string{"bash", "zsh", "fish"}

// following is what a command accepts after it, where that is a closed set. A
// command taking a number, a position or nothing is absent, and completes to
// nothing rather than to the names of files.
func following() map[string][]string {
	return map[string][]string{
		"doctor":      {"-json"},
		"completions": append([]string{"install"}, shells...),
	}
}

// install writes the completion script for every shell on this machine that
// reads one from a directory of its own, creating the directory if it is not
// there. It reports what it wrote.
//
// This is what the installer calls, so that completion works without anybody
// creating a directory or editing a startup file.
func install(env Env) ([]string, error) {
	var written []string
	for shell, path := range env.Paths.Completions() {
		// A shell that is not on this machine gets no file, so nothing is
		// left in a directory nothing reads.
		if _, err := env.LookPath(shell); err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, err
		}
		script := &bytes.Buffer{}
		out := env
		out.Stdout = script
		if err := runCompletions(context.Background(), out, []string{shell}); err != nil {
			return written, err
		}
		if err := os.WriteFile(path, script.Bytes(), 0o644); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	sort.Strings(written)
	return written, nil
}

// runCompletions writes a completion script for one shell.
//
// The script is built from the command table, so the commands it offers are
// the commands that exist. Nothing is checked in that could fall behind.
func runCompletions(_ context.Context, env Env, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("`shanty completions` needs `install`, or the name of a shell: %s",
			strings.Join(shells, ", "))
	}
	if args[0] == "install" {
		written, err := install(env)
		if err != nil {
			return err
		}
		if len(written) == 0 {
			fmt.Fprintln(env.Stdout, "no shell here reads completions from a directory of its own")
			return nil
		}
		for _, path := range written {
			fmt.Fprintln(env.Stdout, "wrote", path)
		}
		fmt.Fprintln(env.Stdout, "\ncompletion works in the next terminal you open")
		return nil
	}
	write, ok := map[string]func(Env){
		"bash": bashCompletions,
		"zsh":  zshCompletions,
		"fish": fishCompletions,
	}[args[0]]
	if !ok {
		return fmt.Errorf("there is no completion script for %q\n\nshanty has one for %s",
			args[0], strings.Join(shells, ", "))
	}
	write(env)
	return nil
}

// names is every command, in the order a shell should offer them.
func names() []string {
	out := make([]string, 0, len(commands()))
	for _, c := range commands() {
		out = append(out, c.name)
	}
	sort.Strings(out)
	return out
}

// quote makes a string safe inside single quotes, which is how zsh and fish
// take a description.
func quote(s string) string { return strings.ReplaceAll(s, "'", `'\''`) }

func bashCompletions(env Env) {
	fmt.Fprintf(env.Stdout, `_shanty() {
	local cur="${COMP_WORDS[COMP_CWORD]}"

	if [ "$COMP_CWORD" -eq 1 ]; then
		COMPREPLY=($(compgen -W '%s' -- "$cur"))
		return
	fi

	case "${COMP_WORDS[1]}" in
`, strings.Join(names(), " "))

	for _, name := range accepting() {
		fmt.Fprintf(env.Stdout, "\t\t%s) COMPREPLY=($(compgen -W '%s' -- \"$cur\")) ;;\n",
			name, strings.Join(following()[name], " "))
	}
	// Nothing else takes a file, so the shell is told to offer nothing rather
	// than falling back to listing the directory.
	fmt.Fprint(env.Stdout, `		*) COMPREPLY=() ;;
	esac
}
complete -F _shanty shanty
`)
}

func zshCompletions(env Env) {
	fmt.Fprint(env.Stdout, "#compdef shanty\n\n_shanty() {\n\tlocal -a subcommands\n\tsubcommands=(\n")
	for _, c := range commands() {
		// Only the first colon separates a name from its description, so one
		// inside the description needs no escaping.
		fmt.Fprintf(env.Stdout, "\t\t'%s:%s'\n", c.name, quote(c.summary))
	}
	fmt.Fprint(env.Stdout, "\t)\n\n\tif (( CURRENT == 2 )); then\n\t\t_describe 'command' subcommands\n\t\treturn\n\tfi\n\n\tcase $words[2] in\n")
	for _, name := range accepting() {
		fmt.Fprintf(env.Stdout, "\t\t%s) _values 'argument' %s ;;\n", name, strings.Join(following()[name], " "))
	}
	fmt.Fprint(env.Stdout, "\tesac\n}\n\ncompdef _shanty shanty\n")
}

func fishCompletions(env Env) {
	// -f stops fish offering files, which none of these commands takes.
	fmt.Fprint(env.Stdout, "complete -c shanty -f\n")
	for _, c := range commands() {
		fmt.Fprintf(env.Stdout, "complete -c shanty -n __fish_use_subcommand -a %s -d '%s'\n",
			c.name, quote(c.summary))
	}
	for _, name := range accepting() {
		for _, word := range following()[name] {
			// A word beginning with a dash is an option; anything else is an
			// argument, and fish spells the two differently.
			kind, value := "-a", word
			if after, isFlag := strings.CutPrefix(word, "-"); isFlag {
				kind, value = "-o", after
			}
			fmt.Fprintf(env.Stdout, "complete -c shanty -n '__fish_seen_subcommand_from %s' %s %s\n",
				name, kind, value)
		}
	}
}

// accepting is every command with a closed set after it, in a fixed order.
func accepting() []string {
	out := make([]string, 0, len(following()))
	for name := range following() {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
