package main

import (
	"math/rand/v2"
	"reflect"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"

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

func TestAScopeOutsideTheSetIsReportedRatherThanIgnored(t *testing.T) {
	a, _ := withPlaylists(t, nil)

	a, _ = runTo(t, a, tui.ShuffleThis{Scope: tui.Scope("genre"), ID: "rock"})

	if !strings.Contains(a.ui.Status(), "genre") {
		t.Errorf("it said %q, which does not name what it could not do", a.ui.Status())
	}
	if a.queue.Len() != 0 {
		t.Errorf("a scope it does not know queued %d tracks", a.queue.Len())
	}
}

// star stars things on the server the app is talking to.
func star(t *testing.T, a app, kind subsonic.Kind, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := a.client.Star(t.Context(), kind, id, true); err != nil {
			t.Fatal(err)
		}
	}
}

// sixQueued plays the second of six tracks, so that four follow it.
func sixQueued(t *testing.T, a app) app {
	t.Helper()
	album := subsonic.Album{Name: "Six"}
	for _, id := range []string{"tr-1", "tr-2", "tr-3", "tr-4", "tr-5", "tr-6"} {
		album.Songs = append(album.Songs, subsonic.Song{ID: id, Title: id})
	}
	return settle(t, a, tui.PlayFrom{Album: album, Index: 1})
}

// ids is the identifiers in the queue, in order.
func ids(a app) []string {
	var out []string
	for _, track := range a.queue.Tracks() {
		out = append(out, track.ID)
	}
	return out
}

// TestEveryScopeIsSomethingTheAppShuffles holds every scope the interface can
// name against what the app queues for it. A scope added to the set fails
// here until it has a row.
func TestEveryScopeIsSomethingTheAppShuffles(t *testing.T) {
	want := map[tui.Scope]struct {
		id     string
		queued []string
		says   string
	}{
		tui.ScopeArtist:   {"ar-1", []string{"tr-1", "tr-2", "tr-3"}, "Aoi"},
		tui.ScopeAlbum:    {"al-1", []string{"tr-1", "tr-2"}, "Harbour"},
		tui.ScopePlaylist: {"pl-1", []string{"tr-1", "tr-4"}, "Evening"},
		tui.ScopeStarred:  {"", []string{"tr-3", "tr-4"}, "everything starred"},
		tui.ScopeQueue:    {"", []string{"tr-1", "tr-2", "tr-3", "tr-4", "tr-5", "tr-6"}, "the rest of the queue"},
	}
	for _, scope := range tui.Scopes {
		t.Run(string(scope), func(t *testing.T) {
			tc, listed := want[scope]
			if !listed {
				t.Fatalf("%s is a scope with no row here saying what the app does with it", scope)
			}
			a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-4"}})
			star(t, a, subsonic.KindSong, "tr-3", "tr-4")
			if scope == tui.ScopeQueue {
				a = sixQueued(t, a)
			}

			a = settle(t, a, tui.ShuffleThis{Scope: scope, ID: tc.id})

			got := ids(a)
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.queued) {
				t.Errorf("the queue holds %v, want %v", got, tc.queued)
			}
			if !strings.Contains(a.ui.Status(), tc.says) {
				t.Errorf("it said %q, want it to name %q", a.ui.Status(), tc.says)
			}
			if _, playing := a.queue.Current(); !playing {
				t.Error("nothing is playing")
			}
		})
	}
	if len(want) != len(tui.Scopes) {
		t.Errorf("%d rows for %d scopes", len(want), len(tui.Scopes))
	}
}

func TestShufflingEverythingStarredTakesEachTrackOnce(t *testing.T) {
	a, _ := withPlaylists(t, nil)
	// tr-4 is starred itself. Harbour holds tr-1 and tr-2, and Aoi made both
	// of those and tr-3, so two of the four are reached twice.
	star(t, a, subsonic.KindSong, "tr-4")
	star(t, a, subsonic.KindAlbum, "al-1")
	star(t, a, subsonic.KindArtist, "ar-1")

	a = settle(t, a, tui.ShuffleThis{Scope: tui.ScopeStarred})

	got := ids(a)
	sort.Strings(got)
	if want := []string{"tr-1", "tr-2", "tr-3", "tr-4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the queue holds %v, want each of %v once", got, want)
	}
}

func TestShufflingEverythingStarredWithNothingStarredSaysHowToStar(t *testing.T) {
	a, _ := withPlaylists(t, nil)

	a = settle(t, a, tui.ShuffleThis{Scope: tui.ScopeStarred})

	if !strings.Contains(a.ui.Status(), "nothing is starred") || !strings.Contains(a.ui.Status(), "*") {
		t.Errorf("it said %q", a.ui.Status())
	}
	if a.queue.Len() != 0 {
		t.Errorf("%d tracks were queued", a.queue.Len())
	}
}

func TestShufflingTheTracksOnScreenQueuesThoseAndSaysWhatTheyAre(t *testing.T) {
	a, _ := withPlaylists(t, nil)
	a = sixQueued(t, a)
	songs := []subsonic.Song{{ID: "tr-7", Title: "Watermark"}, {ID: "tr-1", Title: "Slipway"}, {ID: "tr-9", Title: "Low Water"}}

	a = settle(t, a, tui.ShuffleSongs{Songs: songs, What: `3 tracks found for "water"`})

	got := ids(a)
	sort.Strings(got)
	if want := []string{"tr-1", "tr-7", "tr-9"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the queue holds %v, want %v in place of what was playing", got, want)
	}
	if !strings.Contains(a.ui.Status(), `3 tracks found for "water"`) {
		t.Errorf("it said %q", a.ui.Status())
	}

	a = settle(t, a, tui.ShuffleSongs{What: "0 starred tracks"})
	if !strings.Contains(a.ui.Status(), "nothing to shuffle") || a.queue.Len() != 3 {
		t.Errorf("no tracks said %q and left %d queued", a.ui.Status(), a.queue.Len())
	}
}

func TestShufflingTheRestOfTheQueueLeavesTheTrackPlayingAlone(t *testing.T) {
	a, rec, _ := wired(t)
	a.random = rand.New(rand.NewPCG(1, 2))
	a = sixQueued(t, a)
	was, said := ids(a), len(rec.said())

	a = settle(t, a, tui.ShuffleThis{Scope: tui.ScopeQueue})

	got := ids(a)
	if a.queue.At() != 1 || !reflect.DeepEqual(got[:2], was[:2]) {
		t.Fatalf("the queue is %v at %d, want it to begin %v at 1", got, a.queue.At(), was[:2])
	}
	if reflect.DeepEqual(got, was) {
		t.Error("the tracks after the one playing are in the order they were")
	}
	var prefetched bool
	for _, call := range rec.said()[said:] {
		if strings.HasPrefix(call, "load ") {
			t.Errorf("the player was told to %q, which starts the track again", call)
		}
		if strings.HasPrefix(call, "prefetch ") && strings.Contains(call, got[2]) {
			prefetched = true
		}
	}
	if !prefetched {
		t.Errorf("the player was not told that %s now comes next: %v", got[2], rec.said()[said:])
	}
	if !strings.Contains(a.ui.Status(), "the rest of the queue") {
		t.Errorf("it said %q", a.ui.Status())
	}
}

func TestShufflingAQueueWithNothingToShuffleSaysSoAndChangesNothing(t *testing.T) {
	album := subsonic.Album{Songs: []subsonic.Song{{ID: "tr-1"}, {ID: "tr-2"}, {ID: "tr-3"}}}
	for name, tc := range map[string]struct {
		play  tea.Msg
		wants string
	}{
		"empty":     {nil, "the queue is empty"},
		"last":      {tui.PlayFrom{Album: album, Index: 2}, "fewer than two"},
		"one after": {tui.PlayFrom{Album: album, Index: 1}, "fewer than two"},
	} {
		t.Run(name, func(t *testing.T) {
			a, rec, _ := wired(t)
			if tc.play != nil {
				a = settle(t, a, tc.play)
			}
			was, said := ids(a), len(rec.said())

			a = settle(t, a, tui.ShuffleThis{Scope: tui.ScopeQueue})

			if !strings.Contains(a.ui.Status(), tc.wants) {
				t.Errorf("it said %q, want %q", a.ui.Status(), tc.wants)
			}
			if !reflect.DeepEqual(ids(a), was) || len(rec.said()) != said {
				t.Errorf("the queue or the player was touched: %v, %v", ids(a), rec.said()[said:])
			}
		})
	}
}

func TestAShuffleTheServerDoesNotAnswerIsReportedAndTheQueueIsKept(t *testing.T) {
	for _, scope := range []tui.Scope{tui.ScopeArtist, tui.ScopeAlbum, tui.ScopePlaylist, tui.ScopeStarred} {
		t.Run(string(scope), func(t *testing.T) {
			a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1"}})
			a = sixQueued(t, a)
			srv.Close()

			a = settle(t, a, tui.ShuffleThis{Scope: scope, ID: "pl-1"})

			if a.ui.Status() == "" || strings.Contains(a.ui.Status(), "shuffling") {
				t.Errorf("a server that is gone left %q", a.ui.Status())
			}
			if a.queue.Len() != 6 || a.queue.At() != 1 {
				t.Errorf("the queue was changed to %v at %d", ids(a), a.queue.At())
			}
		})
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
