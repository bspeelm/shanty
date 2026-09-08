package main

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/tui"
)

// TestASavedQueueIsOfferedRatherThanTakenUp covers the choice the issue makes.
// Quitting is usually deliberate, so starting again must not put the music
// back on by itself.
func TestASavedQueueIsOfferedRatherThanTakenUp(t *testing.T) {
	a, rec, srv := wired(t)
	srv.SetPlayQueue([]string{"tr-1", "tr-2"}, "tr-2", 83000, "a laptop")

	for _, msg := range runAll(a.Init()) {
		if saved, ok := msg.(savedQueue); ok {
			a, _ = step(t, a, saved)
		}
	}

	if !a.queue.Empty() {
		t.Error("the saved queue was taken up without being asked for")
	}
	if said := strings.Join(rec.said(), "\n"); strings.Contains(said, "load") {
		t.Errorf("starting up played something: %v", rec.said())
	}
	if status := a.ui.Status(); !strings.Contains(status, ":resume") {
		t.Errorf("nothing offered the saved queue; the screen says %q", status)
	}
	if status := a.ui.Status(); !strings.Contains(status, "a laptop") {
		t.Errorf("the offer does not say where it came from: %q", status)
	}
}

// TestResumingTakesUpTheSavedQueueWhereItWas covers the command, including
// that it starts on the track the other machine was playing.
func TestResumingTakesUpTheSavedQueueWhereItWas(t *testing.T) {
	a, rec, srv := wired(t)
	srv.SetPlayQueue([]string{"tr-1", "tr-2", "tr-3"}, "tr-2", 83000, "a laptop")
	for _, msg := range runAll(a.Init()) {
		if saved, ok := msg.(savedQueue); ok {
			a, _ = step(t, a, saved)
		}
	}

	a, msgs := step(t, a, tui.Resume{})
	for _, m := range msgs {
		if play, ok := m.(tui.PlayFrom); ok {
			a, _ = step(t, a, play)
		}
	}

	if a.queue.Len() != 3 {
		t.Fatalf("resuming queued %d tracks, want 3", a.queue.Len())
	}
	if current, _ := a.queue.Current(); current.ID != "tr-2" {
		t.Errorf("resuming started on %q, want tr-2", current.ID)
	}
	if said := strings.Join(rec.said(), "\n"); !strings.Contains(said, "tr-2") {
		t.Errorf("the player was not given the saved track: %v", rec.said())
	}
}

// TestResumingWithNothingSavedSaysSo covers the command typed when the server
// is holding nothing.
func TestResumingWithNothingSavedSaysSo(t *testing.T) {
	a, _, _ := wired(t)

	a, msgs := step(t, a, tui.Resume{})
	for _, m := range msgs {
		a, _ = step(t, a, m)
	}

	if !strings.Contains(a.ui.Status(), "no queue saved") {
		t.Errorf("the message reads %q", a.ui.Status())
	}
}

// TestNothingIsOfferedOverSomethingAlreadyPlaying covers a saved queue
// arriving after playback has started, which must not interrupt it.
func TestNothingIsOfferedOverSomethingAlreadyPlaying(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})

	a, _ = step(t, a, savedQueue(subsonic.PlayQueue{
		Songs:   []subsonic.Song{{ID: "tr-9", Title: "Something Else"}},
		Current: "tr-9", ChangedBy: "a laptop",
	}))

	if strings.Contains(a.ui.Status(), ":resume") {
		t.Errorf("a saved queue was offered over music already playing: %q", a.ui.Status())
	}
	if current, _ := a.queue.Current(); current.ID != "tr-1" {
		t.Errorf("the queue changed to %q", current.ID)
	}
}

// TestQuittingSavesWhereItGotTo covers the other half. Another machine can
// only carry on from what this one left.
func TestQuittingSavesWhereItGotTo(t *testing.T) {
	a, _, srv := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 1})
	a, _ = step(t, a, tui.Progress(45*time.Second))

	_, msgs := step(t, a, tui.Quit{})
	for _, m := range msgs {
		_, _ = step(t, a, m)
	}

	var saved url.Values
	for _, r := range srv.Requests() {
		if r.Endpoint == "savePlayQueue" {
			saved = r.Query
		}
	}
	if saved == nil {
		t.Fatal("quitting told the server nothing")
	}
	if got := saved.Get("current"); got != "tr-2" {
		t.Errorf("the server was told %q was playing, want tr-2", got)
	}
	if got := saved.Get("position"); got != "45000" {
		t.Errorf("the server was told position %q, want 45000", got)
	}
	if got := len(saved["id"]); got != 3 {
		t.Errorf("the server was sent %d tracks, want 3", got)
	}
}

// TestATrackEndingSavesWhereItGotTo covers the other moment the issue names,
// so a machine that is killed rather than quit has something recent to
// carry on from.
func TestATrackEndingSavesWhereItGotTo(t *testing.T) {
	a, _, srv := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})

	_, _ = step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}))

	for _, r := range srv.Requests() {
		if r.Endpoint == "savePlayQueue" {
			if got := r.Query.Get("current"); got != "tr-2" {
				t.Errorf("the server was told %q was playing, want the track that started", got)
			}
			return
		}
	}
	t.Error("a track ending told the server nothing")
}

// TestTheOfferSaysWhoLeftIt covers the wording. The server keeps one queue for
// the whole account rather than one per machine, so what is there was left
// either by this program somewhere or by another client, and the two read
// differently.
func TestTheOfferSaysWhoLeftIt(t *testing.T) {
	for _, tc := range []struct{ by, want string }{
		{subsonic.ClientName, "you left"},
		{"DSub", "DSub left"},
		{"", "your server has"},
	} {
		t.Run(tc.by, func(t *testing.T) {
			a, _, _ := wired(t)

			a, _ = step(t, a, savedQueue(subsonic.PlayQueue{
				Songs:   []subsonic.Song{{ID: "tr-1", Title: "Slipway"}},
				Current: "tr-1", ChangedBy: tc.by,
			}))

			if got := a.ui.Status(); !strings.HasPrefix(got, tc.want) {
				t.Errorf("a queue left by %q is offered as %q, want it to start %q", tc.by, got, tc.want)
			}
			if got := a.ui.Status(); !strings.Contains(got, "Slipway") {
				t.Errorf("the offer does not name what was playing: %q", got)
			}
		})
	}
}
