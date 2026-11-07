package main

import (
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/subsonic/fake"
	"github.com/bspeelm/shanty/internal/tui"
)

// Reordering is the one change shanty makes that it checks before writing. The
// server cannot move a track, so the only way to express an order replaces the
// playlist's whole contents, and a write from a stale screen would discard
// whatever another client had added.

func TestReorderingWritesTheNewOrder(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2", "tr-3"}})

	a, _ = runTo(t, a, tui.ReorderPlaylist{
		ID:    "pl-1",
		Name:  "Evening",
		Songs: []string{"tr-3", "tr-1", "tr-2"},
		Was:   []string{"tr-1", "tr-2", "tr-3"},
	})

	if got := a.ui.Playlist().Songs; len(got) != 3 || got[0].ID != "tr-3" {
		t.Fatalf("the playlist reads back as %v", idsOf(got))
	}
	if asked := endpoints(srv); !strings.Contains(asked, "createPlaylist") {
		t.Errorf("it did not write the order; it asked for %s", asked)
	}
}

func TestReorderingAPlaylistThatChangedWritesNothing(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2", "tr-3"}})

	// The screen was drawn from an order the server no longer has: another
	// client has been at it. Was is what the screen saw.
	before := len(srv.Requests())
	a, _ = runTo(t, a, tui.ReorderPlaylist{
		ID:    "pl-1",
		Name:  "Evening",
		Songs: []string{"tr-3", "tr-2", "tr-1"},
		Was:   []string{"tr-1", "tr-2"},
	})

	if !strings.Contains(a.ui.Status(), "changed on the server") {
		t.Errorf("it did not say the playlist changed; it said %q", a.ui.Status())
	}
	if !strings.Contains(a.ui.Status(), "nothing was moved") {
		t.Errorf("it did not say nothing was moved; it said %q", a.ui.Status())
	}
	for _, r := range srv.Requests()[before:] {
		if r.Endpoint == "createPlaylist" {
			t.Fatal("it wrote the order anyway, over whatever the server had")
		}
	}
}

func TestReorderingReportsAPlaylistTheServerDoesNotHave(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2"}})

	// The read comes first, so a playlist that is not there is reported
	// rather than written to.
	a, _ = runTo(t, a, tui.ReorderPlaylist{
		ID:    "pl-404",
		Name:  "Gone",
		Songs: []string{"tr-2", "tr-1"},
		Was:   []string{"tr-1", "tr-2"},
	})

	if a.ui.Status() == "" {
		t.Error("reordering a playlist the server does not have said nothing")
	}
}

// endpoints is every endpoint the fake was asked for, as one string.
func endpoints(srv *fake.Server) string {
	var names []string
	for _, r := range srv.Requests() {
		names = append(names, r.Endpoint)
	}
	return strings.Join(names, " ")
}
