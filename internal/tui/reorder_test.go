package tui

import (
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// evening3 is a playlist of three tracks, open on screen.
func evening3(t *testing.T) Model {
	t.Helper()
	list := subsonic.Playlist{ID: "pl-1", Name: "Evening", SongCount: 3, Songs: []subsonic.Song{
		{ID: "tr-1", Title: "Slipway", Artist: "Aoi", Duration: 180},
		{ID: "tr-2", Title: "Ballast", Artist: "Aoi", Duration: 200},
		{ID: "tr-3", Title: "Low Water", Artist: "The Bilge Pumps", Duration: 220},
	}}
	return send(t, loaded(t), PlaylistsLoaded([]subsonic.Playlist{list}), PlaylistLoaded(list))
}

// reordering enters the mode the way a person does.
func reordering(t *testing.T, m Model) Model {
	t.Helper()
	m, msg := typeCommand(t, m, "playlist reorder")
	if msg != nil {
		t.Fatalf(":playlist reorder emitted %#v, want it to be handled here", msg)
	}
	if !m.Reordering() {
		t.Fatalf("reorder mode did not start; the status says %q", m.status)
	}
	return m
}

func TestReorderNeedsAPlaylistOnScreen(t *testing.T) {
	m, msg := typeCommand(t, loaded(t), "playlist reorder")
	if msg != nil {
		t.Errorf("reorder with no playlist on screen emitted %#v", msg)
	}
	if m.Reordering() {
		t.Error("reorder started with no playlist on screen")
	}
	if !strings.Contains(m.status, "needs a playlist on screen") {
		t.Errorf("the status does not say why: %q", m.status)
	}
}

func TestMovingATrackFollowsItWithTheCursor(t *testing.T) {
	m := reordering(t, evening3(t))

	// Low Water is third. Two presses of K put it first, and the cursor
	// should be on it the whole way rather than on whatever took its place.
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j")
	for range 2 {
		m, _ = press(t, m, "K")
	}

	if got := idsOf(m.order); got[0] != "tr-3" {
		t.Errorf("the order is %v, want tr-3 first", got)
	}
	if left, _ := m.row(m.cursor[ScreenPlaylist]); !strings.Contains(left, "Low Water") {
		t.Errorf("the cursor is on %q, want it to have followed Low Water", left)
	}
	if left, _ := m.row(0); !strings.Contains(left, "⇅") {
		t.Errorf("the row being moved is not marked: %q", left)
	}
}

func TestMovingStopsAtTheEnds(t *testing.T) {
	m := reordering(t, evening3(t))

	// The first row cannot go up, and the order is untouched.
	before := idsOf(m.order)
	m, _ = press(t, m, "K")
	if got := idsOf(m.order); !same(got, before) {
		t.Errorf("moving the first row up changed the order to %v", got)
	}

	// Nor the last row down.
	for range 2 {
		m, _ = press(t, m, "j")
	}
	m, _ = press(t, m, "J")
	if got := idsOf(m.order); !same(got, before) {
		t.Errorf("moving the last row down changed the order to %v", got)
	}
}

func TestSavingAnArrangementSendsItWithWhatTheScreenWasDrawnFrom(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "K") // Ballast to the front

	m, msg := press(t, m, "enter")

	got, ok := msg.(ReorderPlaylist)
	if !ok {
		t.Fatalf("saving emitted %#v, want ReorderPlaylist", msg)
	}
	if !same(got.Songs, []string{"tr-2", "tr-1", "tr-3"}) {
		t.Errorf("it sent the order %v", got.Songs)
	}
	// The order the screen was drawn from travels with it, which is the only
	// way the caller can tell a server that changed from one that did not.
	if !same(got.Was, []string{"tr-1", "tr-2", "tr-3"}) {
		t.Errorf("it sent %v as the order it was drawn from", got.Was)
	}
	if m.Reordering() {
		t.Error("saving left the mode on")
	}
}

func TestSavingAnUnchangedArrangementSendsNothing(t *testing.T) {
	m := reordering(t, evening3(t))
	m, msg := press(t, m, "enter")

	if msg != nil {
		t.Errorf("saving an untouched playlist emitted %#v, want nothing", msg)
	}
	if !strings.Contains(m.status, "nothing was moved") {
		t.Errorf("the status says %q", m.status)
	}
}

func TestLeavingReorderForgetsTheArrangement(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "K")

	m, msg := press(t, m, "esc")

	if msg != nil {
		t.Errorf("discarding emitted %#v, want nothing sent", msg)
	}
	if m.Reordering() {
		t.Error("esc did not leave the mode")
	}
	// And it is still the playlist, showing the server's order rather than
	// the one that was abandoned.
	if m.Screen() != ScreenPlaylist {
		t.Errorf("esc also moved to %s", m.Screen())
	}
	if left, _ := m.row(0); !strings.Contains(left, "Slipway") {
		t.Errorf("the first row is %q, want the server's order back", left)
	}
}

func TestMovingDoesNothingOutsideTheMode(t *testing.T) {
	m := evening3(t)
	before, _ := m.row(0)
	m, msg := press(t, m, "K")
	if msg != nil {
		t.Errorf("K outside reorder mode emitted %#v", msg)
	}
	if after, _ := m.row(0); after != before {
		t.Errorf("K outside reorder mode changed the list: %q then %q", before, after)
	}
}

func TestTheModeSaysWhatItIsAndHowToLeave(t *testing.T) {
	frame := reordering(t, evening3(t)).View()
	for _, want := range []string{"reordering Evening", "K/J move", "enter saves", "esc discards"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the footer does not say %q:\n%s", want, frame)
		}
	}
}

func TestTheListOfPlaylistsMarksTheOneThePlayingTrackCameFrom(t *testing.T) {
	lists := []subsonic.Playlist{
		{ID: "pl-1", Name: "Evening", SongCount: 2},
		{ID: "pl-2", Name: "Long Watch", SongCount: 3},
	}
	m := onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists),
		QueueChanged{Tracks: queued(), At: 0, From: "pl-2"}))

	first, _ := m.row(0)
	second, _ := m.row(1)
	if strings.Contains(first, "▶") {
		t.Errorf("Evening is marked and the queue came from Long Watch: %q", first)
	}
	if !strings.Contains(second, "▶") {
		t.Errorf("Long Watch is not marked as the one playing: %q", second)
	}

	// A queue built from anything else leaves every row unmarked.
	m = send(t, m, QueueChanged{Tracks: queued(), At: 0})
	for i := range m.rows() {
		if left, _ := m.row(i); strings.Contains(left, "▶") {
			t.Errorf("a row is still marked after the queue came from elsewhere: %q", left)
		}
	}
}

func TestPlayingFromAPlaylistSaysWhichOne(t *testing.T) {
	m := evening3(t)
	m, msg := press(t, m, "enter")

	got, ok := msg.(PlayFrom)
	if !ok {
		t.Fatalf("playing from a playlist emitted %#v, want PlayFrom", msg)
	}
	if got.Playlist != "pl-1" {
		t.Errorf("it said the tracks came from %q, want pl-1", got.Playlist)
	}

	// An album says nothing, because it is not a playlist.
	album := send(t, loaded(t), ArtistLoaded(library()[0]), AlbumLoaded(library()[0].Albums[0]))
	_, msg = press(t, album, "enter")
	if from := msg.(PlayFrom).Playlist; from != "" {
		t.Errorf("playing from an album said it came from %q", from)
	}
}

func TestDeletingWithNoNameAsksAboutThePlaylistOnScreen(t *testing.T) {
	m, msg := typeCommand(t, evening3(t), "playlist delete")

	got, ok := msg.(DeletePlaylist)
	if !ok {
		t.Fatalf(":playlist delete emitted %#v, want DeletePlaylist", msg)
	}
	if got.ID != "pl-1" || got.Name != "Evening" || got.Tracks != 3 {
		t.Errorf("it asked about %+v, want pl-1 Evening with 3 tracks", got)
	}
	_ = m

	// Off a playlist it refuses and says both ways to mean one.
	off, msg := typeCommand(t, loaded(t), "playlist delete")
	if msg != nil {
		t.Errorf("delete with nothing on screen emitted %#v", msg)
	}
	if !strings.Contains(off.status, "or a name") {
		t.Errorf("the status does not offer the alternative: %q", off.status)
	}
}
