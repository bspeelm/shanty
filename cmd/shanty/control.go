package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bspeelm/shanty/internal/control"
	"github.com/bspeelm/shanty/internal/mpv"
)

// commanding is every subcommand that drives a session, with the verb it
// sends. They are one implementation, because a session accepts one closed set
// of verbs and the command line is a way of typing them.
var commanding = []struct {
	name    string
	verb    control.Verb
	summary string
}{
	{"pause", control.Pause, "pause or resume the session that is playing"},
	{"next", control.Next, "skip to the next track in the session"},
	{"prev", control.Prev, "go back to the previous track in the session"},
	{"vol", control.Volume, "set the volume of the session, from 0 to 100"},
	{"seek", control.Seek, "move to a position in the track, written as 1:23"},
	{"status", control.Status, "say what the session is playing"},
	{"stop", control.Stop, "end the session and the music with it"},
}

// commandSession sends one verb to the session and reports what came back.
func commandSession(verb control.Verb) func(context.Context, Env, []string) error {
	return func(_ context.Context, env Env, args []string) error {
		arg := strings.Join(args, " ")
		if control.TakesArgument(verb) && strings.TrimSpace(arg) == "" {
			return fmt.Errorf("`shanty %s` needs something after it\n\nRun `shanty help` for the list", verb)
		}
		if !control.TakesArgument(verb) && arg != "" {
			return fmt.Errorf("`shanty %s` takes nothing after it", verb)
		}

		res, err := control.Send(env.Paths.ControlSocket(), control.Request{Verb: verb, Arg: arg})
		if err != nil {
			var none control.ErrNoSession
			if errors.As(err, &none) {
				return noSession(env, verb)
			}
			return err
		}
		if !res.OK {
			return errors.New(res.Error)
		}
		fmt.Fprintln(env.Stdout, describe(verb, res.State))
		return nil
	}
}

// noSession answers a command with nothing to command. Stopping is the one
// that still has work to do: a player left by a session that was killed is
// exactly what somebody typing `shanty stop` wants gone.
func noSession(env Env, verb control.Verb) error {
	if verb != control.Stop {
		return errors.New("no session is playing\n\nRun `shanty` to start one, and `:headless` to leave it running")
	}
	if !listening(env.Paths.Socket()) {
		fmt.Fprintln(env.Stdout, "nothing was playing")
		return nil
	}
	p, err := mpv.Attach(context.Background(), env.Paths.Socket())
	if err != nil {
		return err
	}
	if err := p.Close(); err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, "stopped a player that no session owned")
	return nil
}

// describe is the line a control command prints.
func describe(verb control.Verb, s *control.State) string {
	if verb == control.Stop {
		return "stopped"
	}
	if s == nil || s.Title == "" {
		return "nothing playing"
	}
	mark := "playing"
	if s.Paused {
		mark = "paused"
	}
	return fmt.Sprintf("%s · %s — %s   %s / %s   vol %d%%   track %d of %d",
		mark, s.Title, s.Artist, s.Position, s.Duration, s.Volume, s.Track, s.Of)
}
