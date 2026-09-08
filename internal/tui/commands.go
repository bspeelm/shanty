package tui

import (
	"strconv"
	"strings"
)

// command is one entry in the command line.
type command struct {
	name    string
	summary string
	// argument names what follows the command, and is empty for commands that
	// take none.
	argument string
	// run turns the argument into an intent. An error is shown to the user.
	run func(arg string) (any, error)
}

// commands is every command, in the order the completion list shows them.
// ADR-017: a command exists because it takes an argument or because it is
// rare. Anything that is neither is a key.
var commands = []command{
	{
		name:    "q",
		summary: "quit",
		run:     func(string) (any, error) { return Quit{}, nil },
	},
	{
		name:     "search",
		summary:  "find artists, albums and tracks on the server",
		argument: "words",
		run: func(arg string) (any, error) {
			arg = strings.TrimSpace(arg)
			if arg == "" {
				return nil, errBadArgument{"search", arg, "something to look for"}
			}
			return Search(arg), nil
		},
	},
	{
		name:    "messages",
		summary: "show what shanty has said this session",
		run:     func(string) (any, error) { return ShowMessages{}, nil },
	},
	{
		name:    "resume",
		summary: "carry on from the queue saved on the server",
		run:     func(string) (any, error) { return Resume{}, nil },
	},
	{
		name:    "headless",
		summary: "leave the interface and keep playing",
		run:     func(string) (any, error) { return Detach{}, nil },
	},
	{
		name:     "volume",
		summary:  "set the volume",
		argument: "0-100",
		// A number outside the range is reported rather than clamped. Holding
		// a volume key down should stop at the end; typing 400 is a mistake
		// worth being told about.
		run: func(arg string) (any, error) {
			n, err := strconv.Atoi(strings.TrimSpace(arg))
			if err != nil || n < 0 || n > 100 {
				return nil, errBadArgument{"volume", arg, "a number from 0 to 100"}
			}
			return VolumeSet(n), nil
		},
	},
}

// errBadArgument reports an argument the command could not use, and says what
// it wanted instead.
type errBadArgument struct {
	command, got, want string
}

func (e errBadArgument) Error() string {
	return ":" + e.command + " wants " + e.want + ", not " + strconv.Quote(e.got)
}

// split separates a typed line into the command name and its argument.
func split(line string) (name, arg string) {
	name, arg, _ = strings.Cut(strings.TrimSpace(line), " ")
	return name, strings.TrimSpace(arg)
}

// matching returns the commands whose names begin with the name typed so far.
// An empty line matches everything, so pressing : shows the whole set.
func matching(line string) []command {
	name, _ := split(line)
	var out []command
	for _, c := range commands {
		if strings.HasPrefix(c.name, name) {
			out = append(out, c)
		}
	}
	return out
}

// lookup finds the command a line names exactly.
func lookup(line string) (command, bool) {
	name, _ := split(line)
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// complete returns the line with the command name extended as far as every
// match agrees on. A single match gains a trailing space when it takes an
// argument, so the cursor lands where the argument goes.
func complete(line string) string {
	m := matching(line)
	if len(m) == 0 {
		return line
	}
	prefix := m[0].name
	for _, c := range m[1:] {
		for !strings.HasPrefix(c.name, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	if len(m) == 1 && m[0].argument != "" {
		return prefix + " "
	}
	return prefix
}
