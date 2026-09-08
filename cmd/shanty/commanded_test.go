package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/control"
	"github.com/bspeelm/shanty/internal/tui"
)

// session runs a real detached loop: the same model, no renderer, no input,
// answering on a real socket. It returns the socket and the recording player.
func session(t *testing.T) (string, *recorder) {
	t.Helper()
	a, rec, _ := wired(t)
	a.headless = true
	a.queue = queueFrom(twoTracks)
	a.volume = 80

	program := tea.NewProgram(a, tea.WithContext(t.Context()),
		tea.WithoutRenderer(), tea.WithInput(nil))

	socket := filepath.Join(t.TempDir(), "control.sock")
	l, err := control.Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = control.Serve(l, commanded(program)) }()

	// The socket says a session is running, so it closes when the loop ends.
	// runSession does this in a defer; the test has to match it or stopping
	// would look like it had not worked.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = l.Close() }()
		_, _ = program.Run()
	}()
	t.Cleanup(func() {
		program.Quit()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("the session did not stop")
		}
	})

	// Wait for the loop to be answering before returning it.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := control.Send(socket, control.Request{Verb: control.Status}); err == nil {
			return socket, rec
		}
		if time.Now().After(deadline) {
			t.Fatal("the session never answered")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestEveryVerbReachesThePlayerOfARunningSession is the end-to-end claim: a
// command typed in another shell moves the music. It runs the real loop with
// no terminal, over a real socket.
func TestEveryVerbReachesThePlayerOfARunningSession(t *testing.T) {
	for _, tc := range []struct {
		verb control.Verb
		arg  string
		want string
	}{
		{control.Pause, "", "pause true"},
		{control.Volume, "40", "volume 40"},
		{control.Seek, "1:23", "seek to 1m23s"},
		{control.Next, "", "load "},
		{control.Prev, "", "load "},
	} {
		t.Run(string(tc.verb), func(t *testing.T) {
			socket, rec := session(t)

			res, err := control.Send(socket, control.Request{Verb: tc.verb, Arg: tc.arg})
			if err != nil {
				t.Fatal(err)
			}
			if !res.OK {
				t.Fatalf("the session refused: %s", res.Error)
			}

			// The work happens on a command, so give it a moment to land.
			deadline := time.Now().Add(2 * time.Second)
			for !strings.Contains(strings.Join(rec.said(), "\n"), tc.want) {
				if time.Now().After(deadline) {
					t.Fatalf("%s did not reach the player; it heard %v", tc.verb, rec.said())
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

// TestStatusDescribesWhatTheSessionIsPlaying covers the command that exists to
// answer "what is that".
func TestStatusDescribesWhatTheSessionIsPlaying(t *testing.T) {
	socket, _ := session(t)

	res, err := control.Send(socket, control.Request{Verb: control.Status})
	if err != nil {
		t.Fatal(err)
	}
	if res.State == nil {
		t.Fatal("status came back with no state")
	}
	if res.State.Title != "Slipway" {
		t.Errorf("the session says it is playing %q, want Slipway", res.State.Title)
	}
	if res.State.Of != 2 || res.State.Track != 1 {
		t.Errorf("the session says track %d of %d, want 1 of 2", res.State.Track, res.State.Of)
	}
	if res.State.Volume != 80 {
		t.Errorf("the session says volume %d, want 80", res.State.Volume)
	}
	if line := describe(control.Status, res.State); !strings.Contains(line, "Slipway") {
		t.Errorf("the printed line reads %q", line)
	}
}

// TestStoppingASessionAnswersAndThenGoes covers the command whose reply the
// session must not outrun.
func TestStoppingASessionAnswersAndThenGoes(t *testing.T) {
	socket, _ := session(t)

	res, err := control.Send(socket, control.Request{Verb: control.Stop})
	if err != nil {
		t.Fatalf("the session did not answer the command that stopped it: %v", err)
	}
	if !res.OK {
		t.Fatalf("stopping was refused: %s", res.Error)
	}

	// It is gone, so the next command finds nothing.
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := control.Send(socket, control.Request{Verb: control.Status})
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the session answered the stop and kept running")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestABadArgumentIsRefusedWithoutTouchingThePlayer covers a mistyped command.
func TestABadArgumentIsRefusedWithoutTouchingThePlayer(t *testing.T) {
	for _, tc := range []struct{ verb, arg, want string }{
		{"volume", "400", "0 to 100"},
		{"volume", "loud", "0 to 100"},
		{"seek", "half way", "1:23"},
		{"seek", "1:99", "1:23"},
	} {
		t.Run(tc.verb+" "+tc.arg, func(t *testing.T) {
			socket, rec := session(t)
			before := len(rec.said())

			res, err := control.Send(socket, control.Request{Verb: control.Verb(tc.verb), Arg: tc.arg})
			if err != nil {
				t.Fatal(err)
			}
			if res.OK {
				t.Fatalf("%s %q was accepted", tc.verb, tc.arg)
			}
			if !strings.Contains(res.Error, tc.want) {
				t.Errorf("the refusal reads %q, want it to mention %q", res.Error, tc.want)
			}
			if now := len(rec.said()); now != before {
				t.Errorf("a refused command still reached the player: %v", rec.said()[before:])
			}
		})
	}
}

// TestASessionRendersNothing covers the model with no terminal. View is called
// on every message even with no renderer, so a session must not spend its time
// drawing frames nobody sees.
func TestASessionRendersNothing(t *testing.T) {
	a, _, _ := wired(t)
	a.headless = true
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})

	if view := a.View(); view != "" {
		t.Errorf("a session rendered %d bytes of screen", len(view))
	}
	a.headless = false
	if a.View() == "" {
		t.Error("an interface rendered nothing")
	}
}
