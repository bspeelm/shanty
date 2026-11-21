package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
