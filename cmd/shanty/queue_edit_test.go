package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/tui"
)

// calls is what the player was told after the first n calls, as the first
// word of each.
func calls(rec *recorder, after int) []string {
	var out []string
	for _, call := range rec.said()[after:] {
		out = append(out, strings.Fields(call)[0])
	}
	return out
}

func TestTakingThePlayingTrackOutOfTheQueueStartsTheOneThatFollows(t *testing.T) {
	a, rec, _ := wired(t)
	a = sixQueued(t, a)
	said := len(rec.said())

	a = settle(t, a, tui.RemoveQueued(1))

	if want := []string{"tr-1", "tr-3", "tr-4", "tr-5", "tr-6"}; !reflect.DeepEqual(ids(a), want) {
		t.Fatalf("the queue is %v, want %v", ids(a), want)
	}
	if now, _ := a.queue.Current(); now.ID != "tr-3" || a.queue.At() != 1 {
		t.Fatalf("the track current is %s at %d", now.ID, a.queue.At())
	}
	told := rec.said()[said:]
	if len(told) == 0 || !strings.HasPrefix(told[0], "load ") || !strings.Contains(told[0], "tr-3") {
		t.Errorf("the player was told %v, want the track that followed loaded", told)
	}
	if !strings.Contains(a.ui.Status(), "out of the queue") {
		t.Errorf("it said %q", a.ui.Status())
	}
}

func TestTakingTheLastAndPlayingTrackOutLeavesNothingPlaying(t *testing.T) {
	a, _, _ := wired(t)
	a = sixQueued(t, a)
	a = settle(t, a, tui.JumpTo(5))

	a = settle(t, a, tui.RemoveQueued(5))
	if _, playing := a.queue.Current(); playing || a.queue.Len() != 5 {
		t.Errorf("the queue holds %d and something is current", a.queue.Len())
	}
}

func TestTakingAnotherTrackOutOrMovingOneDoesNotStartThePlayingTrackAgain(t *testing.T) {
	for name, tc := range map[string]struct {
		edit     tui.RemoveQueued
		move     *tui.MoveQueued
		want     []string
		at       int
		prefetch bool
	}{
		"one before":      {edit: 0, want: []string{"tr-2", "tr-3", "tr-4", "tr-5", "tr-6"}, at: 0},
		"the one after":   {edit: 2, want: []string{"tr-1", "tr-2", "tr-4", "tr-5", "tr-6"}, at: 1, prefetch: true},
		"one further on":  {edit: 4, want: []string{"tr-1", "tr-2", "tr-3", "tr-4", "tr-6"}, at: 1},
		"moved to follow": {move: &tui.MoveQueued{From: 5, To: 2}, want: []string{"tr-1", "tr-2", "tr-6", "tr-3", "tr-4", "tr-5"}, at: 1, prefetch: true},
		"moved across":    {move: &tui.MoveQueued{From: 0, To: 4}, want: []string{"tr-2", "tr-3", "tr-4", "tr-5", "tr-1", "tr-6"}, at: 0},
		"the playing one": {move: &tui.MoveQueued{From: 1, To: 3}, want: []string{"tr-1", "tr-3", "tr-4", "tr-2", "tr-5", "tr-6"}, at: 3, prefetch: true},
		"further on":      {move: &tui.MoveQueued{From: 4, To: 5}, want: []string{"tr-1", "tr-2", "tr-3", "tr-4", "tr-6", "tr-5"}, at: 1},
	} {
		t.Run(name, func(t *testing.T) {
			a, rec, _ := wired(t)
			a = sixQueued(t, a)
			said := len(rec.said())

			if tc.move != nil {
				a = settle(t, a, *tc.move)
			} else {
				a = settle(t, a, tc.edit)
			}

			if !reflect.DeepEqual(ids(a), tc.want) || a.queue.At() != tc.at {
				t.Fatalf("the queue is %v at %d, want %v at %d", ids(a), a.queue.At(), tc.want, tc.at)
			}
			if now, _ := a.queue.Current(); now.ID != "tr-2" {
				t.Errorf("the track current is %s, want the one that was playing", now.ID)
			}
			told := calls(rec, said)
			for _, call := range told {
				if call == "load" {
					t.Errorf("the player was told to load, which starts the track again: %v", rec.said()[said:])
				}
			}
			if got := len(told) > 0; got != tc.prefetch {
				t.Errorf("the player was told %v; told what comes next should be %v", told, tc.prefetch)
			}
			if tc.prefetch {
				next, _ := a.queue.Upcoming()
				if last := rec.said()[len(rec.said())-1]; !strings.HasPrefix(last, "prefetch ") || !strings.Contains(last, next.ID) {
					t.Errorf("the player was told %q, want %s to come next", last, next.ID)
				}
			}
		})
	}
}

func TestTakingOutATrackThatIsNotThereSaysSoAndChangesNothing(t *testing.T) {
	a, rec, _ := wired(t)
	a = sixQueued(t, a)
	said := len(rec.said())

	a = settle(t, a, tui.RemoveQueued(9))
	a = settle(t, a, tui.MoveQueued{From: 9, To: 0})
	if a.queue.Len() != 6 || a.queue.At() != 1 || len(rec.said()) != said {
		t.Errorf("the queue is %v at %d and the player was told %v", ids(a), a.queue.At(), rec.said()[said:])
	}
}

// TestEveryScopeIsAddedToTheQueueOrRefused holds every scope the interface can
// name against what the app does when asked to add it to the queue. A scope
// added to the set fails here until it has a row.
func TestEveryScopeIsAddedToTheQueueOrRefused(t *testing.T) {
	want := map[tui.Scope]struct {
		id    string
		added []string
		says  string
	}{
		tui.ScopeArtist:   {"ar-1", []string{"tr-1", "tr-2", "tr-3"}, "Aoi, 3 tracks"},
		tui.ScopeAlbum:    {"al-1", []string{"tr-1", "tr-2"}, "Harbour, 2 tracks"},
		tui.ScopePlaylist: {"pl-1", []string{"tr-4", "tr-1"}, "Evening, 2 tracks"},
		tui.ScopeStarred:  {"", nil, "does not know how to add starred"},
		tui.ScopeQueue:    {"", nil, "does not know how to add queue"},
	}
	for _, scope := range tui.Scopes {
		tc, listed := want[scope]
		if !listed {
			t.Fatalf("%s is a scope with no row here saying what the app does with it", scope)
		}
		for _, next := range []bool{false, true} {
			a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-4", "tr-1"}})
			a = sixQueued(t, a)
			was := ids(a)

			a = settle(t, a, tui.QueueThis{Scope: scope, ID: tc.id, Next: next})

			got := ids(a)
			expect := append(append([]string{}, was...), tc.added...)
			if next {
				expect = append(append(append([]string{}, was[:2]...), tc.added...), was[2:]...)
			}
			if !reflect.DeepEqual(got, expect) || a.queue.At() != 1 {
				t.Errorf("%s, next %v: the queue is %v at %d, want %v at 1", scope, next, got, a.queue.At(), expect)
			}
			if !strings.Contains(a.ui.Status(), tc.says) {
				t.Errorf("%s, next %v: it said %q, want %q", scope, next, a.ui.Status(), tc.says)
			}
		}
	}
}

func TestAddingAnAlbumToAQueueWithNothingPlayingStartsIt(t *testing.T) {
	a, rec, _ := wired(t)

	a = settle(t, a, tui.QueueThis{Scope: tui.ScopeAlbum, ID: "al-1"})

	if now, playing := a.queue.Current(); !playing || now.ID != "tr-1" || a.queue.Len() != 2 {
		t.Fatalf("the queue is %v and %q is current", ids(a), now.ID)
	}
	if told := rec.said(); len(told) == 0 || !strings.HasPrefix(told[0], "load ") {
		t.Errorf("the player was told %v, want the first track loaded", told)
	}
}

func TestAddingWhatCannotBeReadOrHoldsNothingSaysSoAndKeepsTheQueue(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Empty": {}})
	a = sixQueued(t, a)

	a = settle(t, a, tui.QueueThis{Scope: tui.ScopePlaylist, ID: "pl-1"})
	if !strings.Contains(a.ui.Status(), "holds no tracks") || a.queue.Len() != 6 {
		t.Errorf("an empty playlist said %q and left %d queued", a.ui.Status(), a.queue.Len())
	}

	srv.Refuse("ar-1")
	a = settle(t, a, tui.QueueThis{Scope: tui.ScopeArtist, ID: "ar-1", Next: true})
	if a.ui.Status() == "" || strings.Contains(a.ui.Status(), "playing") || a.queue.Len() != 6 {
		t.Errorf("an artist that could not be read said %q and left %d queued", a.ui.Status(), a.queue.Len())
	}
}

func TestAddingAnAlbumDoesNotStartThePlayingTrackAgain(t *testing.T) {
	a, rec, _ := wired(t)
	a = sixQueued(t, a)
	said := len(rec.said())

	a = settle(t, a, tui.QueueThis{Scope: tui.ScopeAlbum, ID: "al-1", Next: true})
	told := calls(rec, said)
	if len(told) == 0 || told[len(told)-1] != "prefetch" {
		t.Errorf("the player was told %v, want only what now comes next", told)
	}
	for _, call := range told {
		if call == "load" {
			t.Errorf("the player was told to load: %v", rec.said()[said:])
		}
	}
}
