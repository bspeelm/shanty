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
