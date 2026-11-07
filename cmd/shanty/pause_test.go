package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/tui"
)

// pauseEvent is the property-change mpv sends for an observed pause.
func pauseEvent(paused bool) playerEvent {
	raw, _ := json.Marshal(paused)
	return playerEvent(mpv.Event{Name: "property-change", Property: "pause", Data: raw})
}

// Both properties are observed, or a pause made in mpv never reaches the
// interface and the two drift apart for the rest of the session.
func TestPauseIsObservedAlongsideThePosition(t *testing.T) {
	a, rec, _ := wired(t)

	runAll(a.observePosition())

	said := strings.Join(rec.said(), "\n")
	for _, want := range []string{"observe time-pos", "observe pause"} {
		if !strings.Contains(said, want) {
			t.Errorf("%q is not among %q", want, said)
		}
	}
}

// What the interface shows is what mpv reports, rather than what it was last
// told. A pause made anywhere reaches it.
func TestTheInterfaceFollowsMpvsPause(t *testing.T) {
	a, rec, _ := wired(t)

	a, out := step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})
	a, _ = step(t, a, only[tui.NowPlaying](t, out))

	a, _ = step(t, a, pauseEvent(true))
	if !a.paused() {
		t.Error("mpv said it was paused and the interface did not follow")
	}
	a, _ = step(t, a, pauseEvent(false))
	if a.paused() {
		t.Error("mpv said it had resumed and the interface did not follow")
	}
	// Following is not answering: being told must not send a command back.
	if strings.Contains(strings.Join(rec.said(), "\n"), "pause") {
		t.Error("following mpv's pause sent a pause command back to it")
	}
}

// The bug as the maintainer met it: pause a track, play a different one, and
// it does not play. mpv keeps the pause across the load, so loading resumes.
func TestPlayingAnotherTrackWhilePausedPlaysIt(t *testing.T) {
	a, rec, _ := wired(t)

	a, out := step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})
	a, _ = step(t, a, only[tui.NowPlaying](t, out))
	a, _ = step(t, a, pauseEvent(false))

	// One press of space pauses it.
	a, _ = step(t, a, tui.TogglePause{})
	a, _ = step(t, a, pauseEvent(true))
	if !a.paused() {
		t.Fatal("one press of space did not pause")
	}

	// Playing another track resumes, and the interface is told so by mpv
	// rather than assuming it.
	before := len(rec.said())
	a, out = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 1})
	a, _ = step(t, a, only[tui.NowPlaying](t, out))

	after := strings.Join(rec.said()[before:], "\n")
	if !strings.Contains(after, "load") {
		t.Fatalf("playing another track loaded nothing: %q", after)
	}
	a, _ = step(t, a, pauseEvent(false))
	if a.paused() {
		t.Error("the interface says paused after a track was played")
	}
}

// Shanty used to tell the user that a track sitting still pointed at the audio
// output or the server. It was shanty's own pause, and the guard that should
// have caught it read a belief rather than the player.
func TestAPauseOfItsOwnIsNotBlamedOnTheServer(t *testing.T) {
	// The stall check is delivered rather than waited for, so this does not
	// depend on a grace period and does not shorten the one every other test
	// in this package shares.
	a, _, _ := wired(t)

	a, out := step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})
	a, _ = step(t, a, only[tui.NowPlaying](t, out))

	// mpv is paused and says so. Nothing moves after that, which is exactly
	// what a stall looks like from the outside.
	a, _ = step(t, a, pauseEvent(true))
	a, msgs := step(t, a, stallCheck{watch: a.watch, at: 0})

	for _, msg := range msgs {
		if failed, ok := msg.(tui.Failed); ok {
			t.Errorf("a paused player was reported as a fault: %q", failed.Message)
		}
	}
	if strings.Contains(a.ui.Status(), "server") {
		t.Errorf("the status blames the server for a pause: %q", a.ui.Status())
	}
}
