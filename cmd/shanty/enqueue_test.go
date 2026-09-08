package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/tui"
)

// threeTracks is an album with room to insert into the middle of it.
var threeTracks = subsonic.Album{ID: "al-2", Name: "Harbour", Artist: "Aoi",
	Songs: []subsonic.Song{
		{ID: "tr-1", Title: "Slipway", Duration: 180},
		{ID: "tr-2", Title: "Ballast", Duration: 200},
		{ID: "tr-3", Title: "Low Water", Duration: 220},
	}}

// TestPlayingNextTellsMpvWhatNowComesNext is the correctness this feature
// turns on. mpv is given the next track early so an album plays without a gap,
// so putting a different track next has to change what mpv is holding.
// Otherwise the track that used to be next plays for a moment when the current
// one ends.
func TestPlayingNextTellsMpvWhatNowComesNext(t *testing.T) {
	a, rec, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})
	before := len(rec.said())

	// Queue the third track to play after the first.
	a, _ = step(t, a, tui.PlayNext{Album: threeTracks, Index: 2})

	said := strings.Join(rec.said()[before:], "\n")
	if !strings.Contains(said, "prefetch") {
		t.Fatalf("mpv was not told what now comes next; it heard %v", rec.said()[before:])
	}
	if !strings.Contains(said, "tr-3") {
		t.Errorf("mpv was told to prefetch something other than the inserted track: %s", said)
	}
	if up, _ := a.queue.Upcoming(); up.ID != "tr-3" {
		t.Errorf("the queue plays %s next, want tr-3", up.ID)
	}
	if current, _ := a.queue.Current(); current.ID != "tr-1" {
		t.Errorf("queueing moved playback to %s", current.ID)
	}
}

// TestAddingToTheEndDoesNotDisturbWhatComesNext is the other key. It changes
// the end of the queue, so what mpv holds is still right.
func TestAddingToTheEndDoesNotDisturbWhatComesNext(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})

	a, _ = step(t, a, tui.Enqueue{Album: threeTracks, Index: 2})

	if up, _ := a.queue.Upcoming(); up.ID != "tr-2" {
		t.Errorf("adding to the end changed what plays next to %s", up.ID)
	}
	if a.queue.Len() != 4 {
		t.Errorf("the queue holds %d tracks, want 4", a.queue.Len())
	}
	if last := a.queue.Tracks()[3]; last.ID != "tr-3" {
		t.Errorf("the track went to position 3 as %s", last.ID)
	}
}

// TestQueueingWithNothingPlayingStartsIt covers the empty queue. Asked to play
// something next when nothing is playing, the only sensible answer is to play
// it.
func TestQueueingWithNothingPlayingStartsIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{"play next", tui.PlayNext{Album: threeTracks, Index: 1}},
		{"add to the end", tui.Enqueue{Album: threeTracks, Index: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, rec, _ := wired(t)

			a, _ = step(t, a, tc.msg)

			if current, ok := a.queue.Current(); !ok || current.ID != "tr-2" {
				t.Fatalf("nothing is playing after queueing into an empty queue: %v", current)
			}
			if said := strings.Join(rec.said(), "\n"); !strings.Contains(said, "load") {
				t.Errorf("the track was queued and not played; mpv heard %v", rec.said())
			}
		})
	}
}

// TestTheQueueScreenFollowsTheQueue covers what the screen is told. It draws a
// copy, so every change to the queue has to send it one.
func TestTheQueueScreenFollowsTheQueue(t *testing.T) {
	a, _, _ := wired(t)

	for _, tc := range []struct {
		name string
		msg  tea.Msg
		want int
	}{
		{"playing an album", tui.PlayFrom{Album: threeTracks, Index: 0}, 3},
		{"adding a track", tui.Enqueue{Album: threeTracks, Index: 0}, 4},
		{"skipping", tui.SkipNext{}, 4},
		{"a track ending", playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}), 4},
	} {
		var msgs []tea.Msg
		a, msgs = step(t, a, tc.msg)
		var sent *tui.QueueChanged
		for _, m := range msgs {
			if q, ok := m.(tui.QueueChanged); ok {
				sent = &q
			}
		}
		if sent == nil {
			t.Fatalf("%s did not tell the screen about the queue", tc.name)
		}
		if len(sent.Tracks) != tc.want {
			t.Errorf("%s sent %d tracks, want %d", tc.name, len(sent.Tracks), tc.want)
		}
		if sent.At != a.queue.At() {
			t.Errorf("%s sent position %d and the queue is at %d", tc.name, sent.At, a.queue.At())
		}
	}
}
