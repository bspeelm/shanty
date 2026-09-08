package main

import (
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/control"
)

// TestEveryVerbIsASubcommandAndEverySubcommandIsAVerb holds the two closed
// sets against each other. A verb a session accepts and nobody can type is
// unreachable; a subcommand naming a verb no session knows fails when it is
// run.
func TestEveryVerbIsASubcommandAndEverySubcommandIsAVerb(t *testing.T) {
	typed := map[control.Verb]string{}
	for _, c := range commanding {
		if !control.Known(c.verb) {
			t.Errorf("`shanty %s` sends %q, which no session accepts", c.name, c.verb)
		}
		if was, seen := typed[c.verb]; seen {
			t.Errorf("%q is sent by both `shanty %s` and `shanty %s`", c.verb, was, c.name)
		}
		typed[c.verb] = c.name
	}
	for _, verb := range control.Typed() {
		if _, ok := typed[verb]; !ok {
			t.Errorf("a session accepts %q and no command sends it", verb)
		}
	}
}

// TestAControlCommandWithNoSessionSaysWhatToDo covers the common case: there
// is nothing playing. That is a state to report with a way forward, not a
// failure to diagnose.
func TestAControlCommandWithNoSessionSaysWhatToDo(t *testing.T) {
	env, _ := scratch(t)

	for _, c := range commanding {
		err := commandSession(c.verb)(t.Context(), env, argFor(c.verb))
		if err == nil {
			t.Fatalf("`shanty %s` reported success with no session running", c.name)
		}
		if !strings.Contains(err.Error(), "no session is playing") {
			t.Errorf("`shanty %s` said %q", c.name, err)
		}
		if !strings.Contains(err.Error(), "shanty") {
			t.Errorf("`shanty %s` does not say what to do: %q", c.name, err)
		}
	}
}

// TestAnArgumentIsRequiredExactlyWhereItIsUsed covers the two ways of getting
// it wrong, before anything is sent.
func TestAnArgumentIsRequiredExactlyWhereItIsUsed(t *testing.T) {
	env, _ := scratch(t)

	for _, c := range commanding {
		missing := commandSession(c.verb)(t.Context(), env, nil)
		extra := commandSession(c.verb)(t.Context(), env, []string{"40"})

		if control.TakesArgument(c.verb) {
			if missing == nil || !strings.Contains(missing.Error(), "needs something after it") {
				t.Errorf("`shanty %s` with nothing after it said %v", c.name, missing)
			}
		} else if extra == nil || !strings.Contains(extra.Error(), "takes nothing after it") {
			t.Errorf("`shanty %s 40` said %v", c.name, extra)
		}
	}
}

// TestTheSessionCommandIsNotAUserCommand covers the entry point a session is
// started through. It is not typed, so it is not in the table and not in help.
func TestTheSessionCommandIsNotAUserCommand(t *testing.T) {
	for _, c := range commands() {
		if c.name == sessionArg {
			t.Fatalf("%q is in the command table", sessionArg)
		}
	}
	if strings.HasPrefix(sessionArg, "-") || !strings.HasPrefix(sessionArg, "_") {
		t.Errorf("%q should start with an underscore so the README parser cannot match it", sessionArg)
	}
}

// argFor is a usable argument for the verbs that need one.
func argFor(v control.Verb) []string {
	if !control.TakesArgument(v) {
		return nil
	}
	if v == control.Seek {
		return []string{"1:23"}
	}
	return []string{"40"}
}
