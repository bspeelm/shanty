package main

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/control"
	"github.com/bspeelm/shanty/internal/tui"
)

// stateRequest asks the session what it is doing. The answer comes back on the
// channel, because the model is only reachable through its own messages.
type stateRequest struct{ reply chan control.State }

// answerTimeout bounds how long a control command waits for the model. A
// session busy with the server must not hold up the shell that asked.
const answerTimeout = 2 * time.Second

// commanded turns a verb into the message the interface would have sent for
// the same key, so a session does the same thing whether it is driven by a
// keyboard or from another shell.
func commanded(program *tea.Program) control.Handler {
	return func(req control.Request) control.Response {
		switch req.Verb {
		case control.Status:
			state, ok := askState(program)
			if !ok {
				return control.Response{Error: "the session did not answer"}
			}
			return control.Response{OK: true, State: &state}

		case control.Stop:
			// The answer is written before the session acts on it.
			return control.Response{OK: true, After: func() { program.Send(tui.Quit{}) }}

		case control.Pause:
			program.Send(tui.TogglePause{})
		case control.Next:
			program.Send(tui.SkipNext{})
		case control.Prev:
			program.Send(tui.SkipPrev{})

		case control.Volume:
			n, err := strconv.Atoi(strings.TrimSpace(req.Arg))
			if err != nil || n < 0 || n > 100 {
				return control.Response{Error: "the volume is a number from 0 to 100, not " + strconv.Quote(req.Arg)}
			}
			program.Send(tui.VolumeSet(n))

		case control.Seek:
			d, err := parseClock(req.Arg)
			if err != nil {
				return control.Response{Error: err.Error()}
			}
			program.Send(tui.SeekTo(d))
		}

		state, ok := askState(program)
		if !ok {
			return control.Response{Error: "the session did not answer"}
		}
		return control.Response{OK: true, State: &state}
	}
}

// askState reads the session's state out of the model.
func askState(program *tea.Program) (control.State, bool) {
	reply := make(chan control.State, 1)
	program.Send(stateRequest{reply: reply})
	select {
	case state := <-reply:
		return state, true
	case <-time.After(answerTimeout):
		return control.State{}, false
	}
}

// parseClock reads a position written as minutes and seconds, or as seconds on
// their own.
func parseClock(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	mins, secs, split := strings.Cut(s, ":")
	if !split {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return 0, errBadPosition(s)
		}
		return time.Duration(n) * time.Second, nil
	}
	m, errM := strconv.Atoi(mins)
	sec, errS := strconv.Atoi(secs)
	if errM != nil || errS != nil || m < 0 || sec < 0 || sec > 59 {
		return 0, errBadPosition(s)
	}
	return time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, nil
}

func errBadPosition(s string) error {
	return &badPosition{given: s}
}

type badPosition struct{ given string }

func (e *badPosition) Error() string {
	return "a position is written as 1:23 or as a number of seconds, not " + strconv.Quote(e.given)
}
