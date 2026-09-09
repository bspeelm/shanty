package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/cover"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
	"github.com/bspeelm/shanty/internal/tui"
)

// withCovers is an app whose cover art is kept in a scratch directory and
// drawn as text, which is what a terminal that shows no pictures gets.
func withCovers(t *testing.T) (app, *fake.Server) {
	t.Helper()
	a, srv := withPlaylists(t, nil)
	a.covers = cover.NewCache(t.TempDir())
	a.art = cover.Text
	return a, srv
}

// openAlbum drives the album onto the screen the way pressing enter does.
//
// The cover is two rounds behind the album -- opening asks for the album, the
// album arriving asks for the cover -- so this keeps feeding messages back
// until there are none, where runTo stops after one round.
func openAlbum(t *testing.T, a app, id string) app {
	t.Helper()
	pending := []tea.Msg{tui.OpenAlbum{ID: id}}
	for round := 0; len(pending) > 0 && round < 8; round++ {
		var next []tea.Msg
		for _, m := range pending {
			if m == nil {
				continue
			}
			var produced []tea.Msg
			a, produced = step(t, a, m)
			next = append(next, produced...)
		}
		pending = next
	}
	return a
}

// press sends a key to the interface, which is how a screen is actually left:
// the key runs the action directly rather than emitting an intent.
func press(t *testing.T, a app, key tea.KeyMsg) app {
	t.Helper()
	next, _ := a.ui.Update(key)
	a.ui = next.(tui.Model)
	return a
}

// TestOpeningAnAlbumFetchesItsCover covers the wiring end to end: the album
// arrives, the art follows it, and the screen holds both.
func TestOpeningAnAlbumFetchesItsCover(t *testing.T) {
	a, srv := withCovers(t)

	a = openAlbum(t, a, "al-1")

	if a.ui.Album().ID != "al-1" {
		t.Fatalf("the album on screen is %q", a.ui.Album().ID)
	}
	var asked bool
	for _, r := range srv.Requests() {
		if r.Endpoint == "getCoverArt" {
			asked = true
			if r.Query.Get("id") != "mf-al-1" {
				t.Errorf("the cover asked for is %q, and the album's is mf-al-1", r.Query.Get("id"))
			}
		}
	}
	if !asked {
		t.Fatal("opening an album asked for no cover art")
	}
	if !strings.Contains(a.View(), "╭") {
		t.Errorf("the screen shows no cover:\n%s", a.View())
	}
}

// TestTheCoverIsAskedForOnceAndKept covers the cache doing its job through the
// app rather than on its own.
func TestTheCoverIsAskedForOnceAndKept(t *testing.T) {
	a, srv := withCovers(t)

	a = openAlbum(t, a, "al-1")
	a = press(t, a, tea.KeyMsg{Type: tea.KeyEsc})
	a = openAlbum(t, a, "al-1")

	var asks int
	for _, r := range srv.Requests() {
		if r.Endpoint == "getCoverArt" {
			asks++
		}
	}
	if asks != 1 {
		t.Errorf("the same cover was fetched %d times", asks)
	}
}

// TestACoverArrivingLateIsNotDrawnOverAnotherAlbum is the race this wiring can
// lose. The request is slower than the track list, so a cover can arrive after
// somebody has already opened something else.
func TestACoverArrivingLateIsNotDrawnOverAnotherAlbum(t *testing.T) {
	a, _ := withCovers(t)

	a = openAlbum(t, a, "al-1")
	before := a.View()

	// The cover for an album nobody is looking at.
	a, _ = step(t, a, tui.CoverArt{AlbumID: "al-2", Lines: []string{"WRONG ALBUM"}})

	if strings.Contains(a.View(), "WRONG ALBUM") {
		t.Error("a cover for another album was drawn over this one")
	}
	if a.View() != before {
		t.Error("a cover for another album changed the screen")
	}
}

// TestAnAlbumWithNoCoverAsksForNothing covers the album a server has no
// picture for. An empty identifier must not become a request.
func TestAnAlbumWithNoCoverAsksForNothing(t *testing.T) {
	a, srv := withCovers(t)

	if cmd := a.fetchArt(subsonic.Album{ID: "al-9"}); cmd != nil {
		t.Error("an album with no cover art id produced a request")
	}
	for _, r := range srv.Requests() {
		if r.Endpoint == "getCoverArt" {
			t.Error("a request was made for an album with no cover")
		}
	}
}

// TestASessionAsksForNoPictures covers the process with no terminal. A
// detached session would be fetching images nobody can see.
func TestASessionAsksForNoPictures(t *testing.T) {
	a, _ := withCovers(t)
	a.headless = true

	if cmd := a.fetchArt(subsonic.Album{ID: "al-1", CoverArt: "mf-al-1"}); cmd != nil {
		t.Error("a session asked for cover art")
	}
}

// TestOpeningASecondAlbumDropsTheFirstCover covers the art belonging to one
// album. Without it the previous cover sits over the new track list until the
// new one arrives.
func TestOpeningASecondAlbumDropsTheFirstCover(t *testing.T) {
	a, _ := withCovers(t)
	a = openAlbum(t, a, "al-1")

	// The album arrives before its art does, which is the ordinary case.
	next, _ := a.ui.Update(tui.AlbumLoaded(subsonic.Album{ID: "al-2", Name: "Other"}))
	a.ui = next.(tui.Model)

	if strings.Contains(a.View(), "╭") {
		t.Errorf("the first album's cover is still on screen:\n%s", a.View())
	}
}

// TestLeavingAnAlbumTakesThePictureWithIt covers what a screenshot showed: a
// kitty image is an overlay the terminal holds, so navigating out of an album
// left the cover sitting over the artist list.
func TestLeavingAnAlbumTakesThePictureWithIt(t *testing.T) {
	a, _ := withCovers(t)
	a.art = cover.Kitty
	a = openAlbum(t, a, "al-1")

	if !strings.Contains(a.View(), "\x1b_G") {
		t.Fatal("the album screen carries no picture to take away")
	}
	if strings.Contains(a.View(), "a=d") {
		t.Error("the screen showing the picture also asks for it to be deleted")
	}

	a = press(t, a, tea.KeyMsg{Type: tea.KeyEsc})

	frame := a.View()
	if !strings.Contains(frame, cover.Clear(cover.Kitty)) {
		t.Error("leaving the album did not take the picture away")
	}
	if strings.Contains(frame, "f=100") {
		t.Error("the picture is still being placed after leaving the album")
	}
}

// TestATerminalThatDrawsIntoTheScreenIsSentNothingExtra covers the other
// direction. A sequence on every frame of every screen is a cost paid by
// terminals that never needed it.
func TestATerminalThatDrawsIntoTheScreenIsSentNothingExtra(t *testing.T) {
	a, _ := withCovers(t)
	a = openAlbum(t, a, "al-1")
	a = press(t, a, tea.KeyMsg{Type: tea.KeyEsc})

	if strings.Contains(a.View(), "\x1b_G") {
		t.Error("a terminal drawing text placeholders was sent a graphics sequence")
	}
}

// TestTheRowsAreHeldBeforeThePictureArrives covers the jump. The cover is a
// request behind the track list, so a screen that grows when it lands moves
// every track under the cursor.
func TestTheRowsAreHeldBeforeThePictureArrives(t *testing.T) {
	a, _ := withCovers(t)
	a.art = cover.Kitty

	// The first album teaches the interface how big a cover is. The frame is
	// always the height of the terminal, so what moves when the list jumps is
	// where the first track sits in it.
	a = openAlbum(t, a, "al-1")
	withPicture := rowOf(t, a.View(), "Slipway")

	// The second arrives with its cover still on its way.
	a = press(t, a, tea.KeyMsg{Type: tea.KeyEsc})
	next, _ := a.ui.Update(tui.AlbumLoaded(subsonic.Album{
		ID: "al-2", Name: "Other", CoverArt: "mf-al-2",
		Songs: []subsonic.Song{{ID: "tr-9", Title: "Waiting"}},
	}))
	a.ui = next.(tui.Model)

	waiting := rowOf(t, a.View(), "Waiting")
	if waiting != withPicture {
		t.Errorf("the first track is on row %d while the cover is on its way and row %d once it lands; the list jumps by %d",
			waiting, withPicture, withPicture-waiting)
	}
	if strings.Contains(a.View(), "f=100") {
		t.Error("a picture is being placed before one has arrived")
	}
}

// rowOf is which row of a frame a piece of text is on.
func rowOf(t *testing.T, frame, text string) int {
	t.Helper()
	for i, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, text) {
			return i
		}
	}
	t.Fatalf("%q is not on the screen:\n%s", text, frame)
	return -1
}

// TestAnAlbumWithNoCoverHoldsNoRows covers the other side: a server with no
// art for an album must not leave a gap where a picture will never be, and the
// picture from the album before it has to go.
func TestAnAlbumWithNoCoverHoldsNoRows(t *testing.T) {
	a, _ := withCovers(t)
	a.art = cover.Kitty
	a = openAlbum(t, a, "al-1")

	next, _ := a.ui.Update(tui.AlbumLoaded(subsonic.Album{
		ID: "al-3", Name: "Coverless",
		Songs: []subsonic.Song{{ID: "tr-9", Title: "Nothing"}},
	}))
	a.ui = next.(tui.Model)

	frame := a.View()
	// f=100 is the placement. The delete sequence shares its prefix, so
	// looking for the prefix alone finds the thing that takes a picture away.
	if strings.Contains(frame, "f=100") {
		t.Error("an album with no cover art placed a picture")
	}
	if !strings.Contains(frame, cover.Clear(cover.Kitty)) {
		t.Error("the picture from the album before it was not taken away")
	}
}
