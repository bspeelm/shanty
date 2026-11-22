package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
	"github.com/bspeelm/shanty/internal/tui"
)

// settle applies a message and then everything that comes back, however many
// turns that takes.
func settle(t *testing.T, a app, msg tea.Msg) app {
	t.Helper()
	waiting := []tea.Msg{msg}
	for turns := 0; len(waiting) > 0 && turns < 20; turns++ {
		var more []tea.Msg
		a, more = step(t, a, waiting[0])
		waiting = append(waiting[1:], more...)
	}
	return a
}

// asked reports whether the endpoint is among those the server was asked for.
func asked(reqs []string, endpoint string) bool {
	for _, e := range reqs {
		if e == endpoint {
			return true
		}
	}
	return false
}

func TestTheJumpsToPlaylistsAndEveryAlbumFetchWhatTheyShow(t *testing.T) {
	for _, tc := range []struct {
		key, endpoint string
		screen        tui.Screen
	}{
		{"p", "getPlaylists", tui.ScreenPlaylists},
		{"b", "getAlbumList2", tui.ScreenEveryAlbum},
	} {
		t.Run("g"+tc.key, func(t *testing.T) {
			a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2"}})
			before := len(srv.Requests())

			a = settle(t, settle(t, a, keyRunes("g")), keyRunes(tc.key))

			var endpoints []string
			for _, r := range srv.Requests()[before:] {
				endpoints = append(endpoints, r.Endpoint)
			}
			if !asked(endpoints, tc.endpoint) {
				t.Fatalf("g%s asked the server for %v, want %s", tc.key, endpoints, tc.endpoint)
			}
			if a.ui.Screen() != tc.screen {
				t.Fatalf("g%s went to %s", tc.key, a.ui.Screen())
			}
			frame := a.ui.View()
			if strings.Contains(frame, "nothing here") || strings.Contains(frame, "no playlists yet") ||
				strings.Contains(frame, "loading") {
				t.Errorf("g%s shows an empty screen for a server that has something:\n%s", tc.key, frame)
			}
		})
	}
}

func TestAReloadWithNothingToFetchIsNotMistakenForTheNextListToArrive(t *testing.T) {
	a, _, _ := wired(t)
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("q"))

	a = settle(t, a, tui.Reload{})
	if !strings.Contains(a.ui.Status(), "nothing here to fetch") {
		t.Fatalf("reloading the queue said %q", a.ui.Status())
	}
	if strings.Contains(a.ui.View(), "loading") {
		t.Errorf("the queue screen still says it is loading after a reload that fetched nothing:\n%s", a.ui.View())
	}

	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("a"))
	a = settle(t, a, a.fetchArtists()())
	if a.ui.Screen() != tui.ScreenArtists {
		t.Fatalf("the artist list is not on screen; it is %s", a.ui.Screen())
	}
	if strings.Contains(a.ui.Status(), "reloaded") {
		t.Errorf("a list that arrived afterwards was reported as the reload: %q", a.ui.Status())
	}
}

// deliver applies messages that were held back, and everything they lead to.
func deliver(t *testing.T, a app, held ...[]tea.Msg) app {
	t.Helper()
	for _, msgs := range held {
		for _, m := range msgs {
			a = settle(t, a, m)
		}
	}
	return a
}

func TestWhatAJumpAsksForCannotUndoAKeyPressedBeforeItIsAnswered(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2"}})
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("p"))
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("a"))

	// The jump and the key after it are both pressed before either is answered.
	a, _ = step(t, a, keyRunes("g"))
	a, jumped := step(t, a, keyRunes("p"))
	a, opened := step(t, a, tea.KeyMsg{Type: tea.KeyEnter})
	a = deliver(t, a, jumped, opened)
	if a.ui.Screen() != tui.ScreenPlaylist {
		t.Errorf("a playlist opened straight after gp is not shown; the screen is %s", a.ui.Screen())
	}

	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("a"))
	a, _ = step(t, a, keyRunes("g"))
	a, jumped = step(t, a, keyRunes("p"))
	a, left := step(t, a, tea.KeyMsg{Type: tea.KeyEsc})
	a = deliver(t, a, jumped, left)
	if a.ui.Screen() != tui.ScreenLibrary {
		t.Errorf("esc straight after gp was undone; the screen is %s", a.ui.Screen())
	}

	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("a"))
	a, _ = step(t, a, keyRunes("g"))
	a, jumped = step(t, a, keyRunes("b"))
	a, left = step(t, a, tea.KeyMsg{Type: tea.KeyEsc})
	a = deliver(t, a, jumped, left)
	if a.ui.Screen() != tui.ScreenLibrary {
		t.Errorf("esc straight after gb was undone; the screen is %s", a.ui.Screen())
	}
}

// requests counts the calls to one endpoint after the first n.
func requests(srv *fake.Server, after int, endpoint string) int {
	n := 0
	for _, r := range srv.Requests()[after:] {
		if r.Endpoint == endpoint {
			n++
		}
	}
	return n
}

func TestAFinishedScanAsksForTheLibraryOnce(t *testing.T) {
	a, _, srv := wired(t)
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("a"))
	a = settle(t, a, a.fetchArtists()())
	a = settle(t, a, keyRunes("j"))
	before := len(srv.Requests())

	a = settle(t, a, scanning{status: subsonic.Scan{Count: 4}})

	if n := requests(srv, before, "getArtists"); n != 1 {
		t.Errorf("a finished scan asked for the artists %d times, want once", n)
	}
	if a.ui.Cursor() != 1 || !strings.Contains(a.ui.Status(), "reloaded") {
		t.Errorf("the cursor is on row %d and the last row says %q", a.ui.Cursor(), a.ui.Status())
	}
}

func TestReloadingTwiceMakesTwoRequestsAndKeepsTheSelection(t *testing.T) {
	a, _, srv := wired(t)
	a = settle(t, a, tui.OpenArtist{ID: "ar-1"})
	a = settle(t, a, tui.OpenAlbum{ID: "al-1"})
	a = settle(t, a, keyRunes("j"))
	before := len(srv.Requests())

	a, first := step(t, a, tui.Reload{})
	a, second := step(t, a, tui.Reload{})
	a = deliver(t, a, first, second)

	if n := requests(srv, before, "getAlbum"); n != 2 {
		t.Errorf("two reloads asked for the album %d times", n)
	}
	if a.ui.Screen() != tui.ScreenTracks || a.ui.Cursor() != 1 {
		t.Errorf("after two reloads: screen %s, cursor %d", a.ui.Screen(), a.ui.Cursor())
	}
}

func TestAReloadAnsweredAfterTheScreenWasLeftDoesNotComeBackToIt(t *testing.T) {
	a, _, _ := wired(t)
	a = settle(t, a, tui.OpenArtist{ID: "ar-1"})
	a = settle(t, a, tui.OpenAlbum{ID: "al-1"})

	a, answer := step(t, a, tui.Reload{})
	a = settle(t, a, keyRunes("h"))
	a = deliver(t, a, answer)
	if a.ui.Screen() != tui.ScreenAlbums {
		t.Errorf("the reload's answer took the screen back to %s", a.ui.Screen())
	}
}

// enter is the key that opens a row.
var enter = tea.KeyMsg{Type: tea.KeyEnter}

func TestAnOpenIsShownOnlyWhileItsScreenIsStillWaitingForIt(t *testing.T) {
	a, _, _ := wired(t)
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("a"))
	a = settle(t, a, a.fetchArtists()())

	// enter, back, and enter again, all before the first answer arrives.
	a, first := step(t, a, enter)
	a = settle(t, a, keyRunes("h"))
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("a"))
	a, second := step(t, a, enter)
	a = deliver(t, a, second)
	if a.ui.Screen() != tui.ScreenAlbums {
		t.Fatalf("the artist opened again is not shown; the screen is %s", a.ui.Screen())
	}
	a, album := step(t, a, enter)
	a = deliver(t, a, album, first)
	if a.ui.Screen() != tui.ScreenTracks {
		t.Errorf("the answer to the open that was left took the screen to %s", a.ui.Screen())
	}
}

func TestAnOpenThatFailsStopsBeingWaitedForAndCanBeTriedAgain(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2", "tr-3"}})
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("p"))

	srv.Refuse("pl-1")
	a = settle(t, a, enter)
	if a.ui.Screen() != tui.ScreenPlaylists || strings.Contains(a.ui.View(), "loading") {
		t.Fatalf("after a failed open: screen %s\n%s", a.ui.Screen(), a.ui.View())
	}
	if a.ui.Status() == "" {
		t.Error("the failed open said nothing")
	}
}

func TestRemovingFromOnePlaylistAndOpeningAnotherShowsTheOther(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2", "tr-3"}, "Morning": {"tr-4"}})
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("p"))
	first := a.ui.View()
	a = settle(t, a, enter)
	opened := a.ui.Playlist()

	// r, back, down and enter, with the removal answered last.
	a, removed := step(t, a, keyRunes("r"))
	a = settle(t, a, keyRunes("h"))
	a = settle(t, a, keyRunes("j"))
	a, other := step(t, a, enter)
	a = deliver(t, a, other, removed)

	if a.ui.Screen() != tui.ScreenPlaylist || a.ui.Playlist().ID == opened.ID {
		t.Errorf("the removal's answer took the screen: %s showing %q\n%s",
			a.ui.Screen(), a.ui.Playlist().Name, first)
	}
}

func TestAddingToAPlaylistFromAnAlbumStaysOnTheAlbum(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-4"}})
	a = settle(t, a, tui.Playlist{Verb: "edit", Name: "Evening"})
	a = settle(t, a, tui.OpenArtist{ID: "ar-1"})
	a = settle(t, a, tui.OpenAlbum{ID: "al-1"})

	a, added := step(t, a, keyRunes("a"))
	a = settle(t, a, keyRunes("e"))
	a = deliver(t, a, added)
	if a.ui.Screen() != tui.ScreenTracks {
		t.Errorf("the add's answer took the screen to %s", a.ui.Screen())
	}
}

func TestAPlaylistBeingAddedToOpensFromTheList(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2", "tr-3"}})
	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("p"))
	a = settle(t, a, enter)
	a = settle(t, a, keyRunes("e"))
	if a.ui.Editing().ID == "" {
		t.Fatal("e did not start adding to the playlist on screen")
	}

	a = settle(t, settle(t, a, keyRunes("g")), keyRunes("p"))
	a = settle(t, a, enter)
	if a.ui.Screen() != tui.ScreenPlaylist {
		t.Fatalf("the playlist being added to did not open; the screen is %s", a.ui.Screen())
	}
	a = settle(t, a, keyRunes("j"))
	if strings.Contains(a.ui.View(), "opening") {
		t.Errorf("the screen still says the playlist is opening:\n%s", a.ui.View())
	}
}
