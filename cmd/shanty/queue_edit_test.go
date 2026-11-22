package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

	a = settle(t, a, tui.RemoveQueued{At: 1, ID: "tr-2"})

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

// TestWhenTheQueueEndsUnderATrackThePlayerIsStopped covers the two ways the
// queue can run out while a track is sounding. The track did not finish, so
// no play of it is reported.
func TestWhenTheQueueEndsUnderATrackThePlayerIsStopped(t *testing.T) {
	for name, ends := range map[string]tea.Msg{
		"the last and playing track is taken out": tui.RemoveQueued{At: 5, ID: "tr-6"},
		"the last track is skipped":               tui.SkipNext{},
	} {
		t.Run(name, func(t *testing.T) {
			a, rec, srv := wired(t)
			a = settle(t, sixQueued(t, a), tui.JumpTo(5))
			if !strings.Contains(a.ui.View(), "tr-6") {
				t.Fatalf("the last track is not shown as playing:\n%s", a.ui.View())
			}
			said, asked := len(rec.said()), len(srv.Requests())

			a = settle(t, a, ends)

			if _, playing := a.queue.Current(); playing {
				t.Fatal("a track is still current")
			}
			if told := calls(rec, said); len(told) != 1 || told[0] != "stop" {
				t.Errorf("the player was told %v, want it stopped and nothing else", told)
			}
			if !strings.Contains(a.ui.View(), "nothing playing") {
				t.Errorf("the screen does not say nothing is playing:\n%s", a.ui.View())
			}
			if n := requests(srv, asked, "scrobble"); n != 0 {
				t.Errorf("%d plays were reported for a track that did not finish", n)
			}
		})
	}
}

func TestASessionEndsWhenItsQueueIsSkippedPastTheEnd(t *testing.T) {
	a, rec, _ := wired(t)
	a = settle(t, sixQueued(t, a), tui.JumpTo(5))
	a.headless = true

	_, msgs := step(t, a, tui.SkipNext{})

	var quit bool
	for _, m := range msgs {
		if _, ok := m.(tea.QuitMsg); ok {
			quit = true
		}
	}
	if told := rec.said(); !quit || told[len(told)-1] != "stop" {
		t.Errorf("the session was told %v and quit is %v, want the player stopped and the session ended", msgs, quit)
	}
}

func TestTakingAnotherTrackOutOrMovingOneDoesNotStartThePlayingTrackAgain(t *testing.T) {
	for name, tc := range map[string]struct {
		edit     *tui.RemoveQueued
		move     *tui.MoveQueued
		want     []string
		at       int
		prefetch bool
	}{
		"one before":      {edit: &tui.RemoveQueued{At: 0, ID: "tr-1"}, want: []string{"tr-2", "tr-3", "tr-4", "tr-5", "tr-6"}, at: 0},
		"the one after":   {edit: &tui.RemoveQueued{At: 2, ID: "tr-3"}, want: []string{"tr-1", "tr-2", "tr-4", "tr-5", "tr-6"}, at: 1, prefetch: true},
		"one further on":  {edit: &tui.RemoveQueued{At: 4, ID: "tr-5"}, want: []string{"tr-1", "tr-2", "tr-3", "tr-4", "tr-6"}, at: 1},
		"moved to follow": {move: &tui.MoveQueued{From: 5, To: 2, ID: "tr-6"}, want: []string{"tr-1", "tr-2", "tr-6", "tr-3", "tr-4", "tr-5"}, at: 1, prefetch: true},
		"moved across":    {move: &tui.MoveQueued{From: 0, To: 4, ID: "tr-1"}, want: []string{"tr-2", "tr-3", "tr-4", "tr-5", "tr-1", "tr-6"}, at: 0},
		"the playing one": {move: &tui.MoveQueued{From: 1, To: 3, ID: "tr-2"}, want: []string{"tr-1", "tr-3", "tr-4", "tr-2", "tr-5", "tr-6"}, at: 3, prefetch: true},
		"further on":      {move: &tui.MoveQueued{From: 4, To: 5, ID: "tr-5"}, want: []string{"tr-1", "tr-2", "tr-3", "tr-4", "tr-6", "tr-5"}, at: 1},
	} {
		t.Run(name, func(t *testing.T) {
			a, rec, _ := wired(t)
			a = sixQueued(t, a)
			said := len(rec.said())

			if tc.move != nil {
				a = settle(t, a, *tc.move)
			} else {
				a = settle(t, a, *tc.edit)
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

	a = settle(t, a, tui.RemoveQueued{At: 9, ID: "tr-9"})
	a = settle(t, a, tui.MoveQueued{From: 9, To: 0, ID: "tr-9"})
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

func TestACountTakesThatManyTracksOutOfTheQueue(t *testing.T) {
	a, rec, _ := wired(t)
	a = sixQueued(t, a)
	said := len(rec.said())

	// Three from the first: the one before the playing track, the playing
	// track, and the one after it.
	a = settle(t, a, tui.RemoveQueued{At: 0, ID: "tr-1", Count: 3})

	if want := []string{"tr-4", "tr-5", "tr-6"}; !reflect.DeepEqual(ids(a), want) || a.queue.At() != 0 {
		t.Fatalf("the queue is %v at %d, want %v at 0", ids(a), a.queue.At(), want)
	}
	if told := rec.said()[said:]; len(told) == 0 || !strings.HasPrefix(told[0], "load ") || !strings.Contains(told[0], "tr-4") {
		t.Errorf("the player was told %v, want the track that followed loaded", told)
	}
	if !strings.Contains(a.ui.Status(), "took 3 tracks out of the queue") {
		t.Errorf("it said %q", a.ui.Status())
	}

	a = settle(t, a, tui.RemoveQueued{At: 1, ID: "tr-5", Count: 9})
	if want := []string{"tr-4"}; !reflect.DeepEqual(ids(a), want) {
		t.Errorf("a count past the end left %v, want %v", ids(a), want)
	}
}

func TestAnEditThatNamesATrackNoLongerAtThatPlaceIsRefused(t *testing.T) {
	a, rec, _ := wired(t)
	a = sixQueued(t, a)
	said := len(rec.said())

	for _, edit := range []tea.Msg{
		tui.RemoveQueued{At: 2, ID: "tr-6", Count: 1},
		tui.MoveQueued{From: 2, To: 3, ID: "tr-6"},
	} {
		a = settle(t, a, edit)
		if !strings.Contains(a.ui.Status(), "the queue changed since it was shown") {
			t.Errorf("%#v said %q", edit, a.ui.Status())
		}
	}
	if a.queue.Len() != 6 || a.queue.At() != 1 || len(rec.said()) != said {
		t.Errorf("the queue is %v at %d and the player was told %v", ids(a), a.queue.At(), rec.said()[said:])
	}
}

func TestAQueueWithNothingInItIsNotMarkedAsComingFromAPlaylist(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2"}})
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("p"))
	a = settle(t, settle(t, a, enter), enter)
	a = settle(t, a, keyRunes("h"))
	if !strings.Contains(a.ui.View(), "▶ Evening") {
		t.Fatalf("the playlist playing is not marked:\n%s", a.ui.View())
	}

	a = settle(t, a, tui.RemoveQueued{At: 0, ID: "tr-1", Count: 2})
	if a.queue.Len() != 0 || strings.Contains(a.ui.View(), "▶ Evening") {
		t.Errorf("with %d tracks queued the playlist is still marked:\n%s", a.queue.Len(), a.ui.View())
	}
}
