package main

import (
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/tui"
)

// S asks for one collection; these cover what reaches the server for each
// scope. The fake library is Aoi with two albums -- Harbour holding tr-1 and
// tr-2, and Low Water holding tr-3 -- and The Bilge Pumps with one album
// holding tr-4. So an artist, an album and the whole library are three
// different numbers, which is what makes the scope worth testing.

func TestShufflingAnArtistReadsEveryAlbumTheyHave(t *testing.T) {
	a, _ := withPlaylists(t, nil)

	a, _ = runTo(t, a, tui.ShuffleThis{Scope: "artist", ID: "ar-1"})

	// Three: two albums, and the server gives up an album's tracks only when
	// asked for that album. A catalogue is the one fetch here that is not the
	// single level a screen is showing.
	if a.queue.Len() != 3 {
		t.Fatalf("shuffling an artist queued %d tracks, want their 3", a.queue.Len())
	}
	for _, id := range []string{"tr-1", "tr-2", "tr-3"} {
		if !queued(a, id) {
			t.Errorf("%s is missing from the shuffled catalogue", id)
		}
	}
	if queued(a, "tr-4") {
		t.Error("tr-4 is by another artist and should not be in the queue")
	}
	if !strings.Contains(a.ui.Status(), "Aoi") {
		t.Errorf("it said %q, which does not name the artist", a.ui.Status())
	}
	if _, playing := a.queue.Current(); !playing {
		t.Error("shuffling an artist queued tracks and played nothing")
	}
}

func TestShufflingAnAlbumReadsOnlyThatAlbum(t *testing.T) {
	a, _ := withPlaylists(t, nil)

	a, _ = runTo(t, a, tui.ShuffleThis{Scope: "album", ID: "al-1"})

	if a.queue.Len() != 2 {
		t.Fatalf("shuffling an album queued %d tracks, want Harbour's 2", a.queue.Len())
	}
	if queued(a, "tr-3") {
		t.Error("tr-3 is on another album by the same artist and should not be in the queue")
	}
	if !strings.Contains(a.ui.Status(), "Harbour") {
		t.Errorf("it said %q, which does not name the album", a.ui.Status())
	}
}

func TestShufflingAPlaylistTakesItsIdentifierRatherThanItsName(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-4"}})

	// pl-1 is the first playlist the fake creates. The name is never sent,
	// which is what makes this work where two playlists share one.
	a, _ = runTo(t, a, tui.ShuffleThis{Scope: "playlist", ID: "pl-1"})

	if a.queue.Len() != 2 {
		t.Fatalf("shuffling a playlist queued %d tracks, want its 2", a.queue.Len())
	}
	for _, id := range []string{"tr-1", "tr-4"} {
		if !queued(a, id) {
			t.Errorf("%s is missing from the shuffled playlist", id)
		}
	}
	if !strings.Contains(a.ui.Status(), "Evening") {
		t.Errorf("it said %q, which does not name the playlist", a.ui.Status())
	}
}

func TestShufflingAnEmptyAlbumSaysThereIsNothingToShuffle(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Empty": {}})

	a, _ = runTo(t, a, tui.ShuffleThis{Scope: "playlist", ID: "pl-1"})

	if !strings.Contains(a.ui.Status(), "nothing to shuffle") {
		t.Errorf("it said %q", a.ui.Status())
	}
}

func TestAScopeShantyDoesNotKnowIsReportedRatherThanIgnored(t *testing.T) {
	a, _ := withPlaylists(t, nil)

	a, _ = runTo(t, a, tui.ShuffleThis{Scope: "genre", ID: "rock"})

	if !strings.Contains(a.ui.Status(), "genre") {
		t.Errorf("it said %q, which does not name what it could not do", a.ui.Status())
	}
	if a.queue.Len() != 0 {
		t.Errorf("a scope it does not know queued %d tracks", a.queue.Len())
	}
}

// queued says whether the identifier is anywhere in the queue.
func queued(a app, id string) bool {
	for _, track := range a.queue.Tracks() {
		if track.ID == id {
			return true
		}
	}
	return false
}
