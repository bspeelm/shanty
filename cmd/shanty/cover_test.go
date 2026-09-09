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

	// Tall enough that a cover still leaves a track list worth reading. The
	// default 24 rows is not, which is the point of minTracksBesideArt.
	//
	// Sent through the app rather than to the interface, because `:art` needs
	// the size to render a cover the size of the screen.
	a, _ = step(t, a, tea.WindowSizeMsg{Width: 80, Height: 40})
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

// TestAShortTerminalKeepsTheTracksAndDropsTheCover covers what a cover costs.
// Twelve rows of picture on a short terminal leaves a handful of tracks, and
// the tracks are what somebody opened the album for.
func TestAShortTerminalKeepsTheTracksAndDropsTheCover(t *testing.T) {
	for _, tc := range []struct {
		height int
		shown  bool
	}{
		{40, true},  // room for the cover and most of a record
		{26, true},  // exactly the eight tracks the rule asks for
		{25, false}, // one short, so the cover goes
		{24, false}, // the ordinary terminal
		{10, false},
	} {
		a, _ := withCovers(t)
		a.art = cover.Kitty
		a, _ = step(t, a, tea.WindowSizeMsg{Width: 80, Height: tc.height})
		a = openAlbum(t, a, "al-1")

		frame := a.View()
		if shown := strings.Contains(frame, "f=100"); shown != tc.shown {
			t.Errorf("at %d rows the cover is shown=%v, want %v", tc.height, shown, tc.shown)
		}
		// However tall the terminal, the frame is never taller than it.
		if rows := strings.Count(frame, "\n") + 1; rows > tc.height {
			t.Errorf("at %d rows the frame is %d rows", tc.height, rows)
		}
	}
}

// TestArtFillsTheScreenAndEscPutsItAway covers the command end to end.
func TestArtFillsTheScreenAndEscPutsItAway(t *testing.T) {
	a, srv := withCovers(t)
	a.art = cover.Kitty
	a = openAlbum(t, a, "al-1")
	before := len(srv.Requests())

	a = runCommand(t, a, "art")

	frame := a.View()
	if !strings.Contains(frame, "f=100") {
		t.Fatal("the command drew no picture")
	}
	// Bigger than the twelve rows a cover takes above a track list.
	if !strings.Contains(frame, "r=35") {
		t.Errorf("the picture was not enlarged to the screen:\n%s", visible(frame))
	}
	if strings.Contains(frame, "Slipway") {
		t.Error("the track list is drawn under a cover filling the screen")
	}
	// The cache already held it, so looking at it costs no request.
	if got := len(srv.Requests()); got != before {
		t.Errorf("%d requests were made to enlarge a cover already fetched", got-before)
	}

	a = press(t, a, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(a.View(), "r=35") {
		t.Error("esc did not put the cover away")
	}
	if !strings.Contains(a.View(), "Slipway") {
		t.Error("esc did not bring the track list back")
	}
}

// TestArtWithNothingToShowSaysSo covers the two ways there is nothing to
// enlarge: no album open, and an album whose cover never arrived.
func TestArtWithNothingToShowSaysSo(t *testing.T) {
	t.Run("no album open", func(t *testing.T) {
		a, _ := withCovers(t)
		a.art = cover.Kitty

		a = runCommand(t, a, "art")

		if !strings.Contains(a.ui.Status(), "no cover") {
			t.Errorf("it said %q", a.ui.Status())
		}
	})

	t.Run("the cover was never fetched", func(t *testing.T) {
		a, _ := withCovers(t)
		a.art = cover.Kitty
		a = openAlbum(t, a, "al-1")

		// The server went away between opening the album and asking to see
		// the cover, so the cache holds nothing for it.
		a.covers = cover.NewCache(t.TempDir())

		a = runCommand(t, a, "art")

		if !strings.Contains(a.ui.Status(), "not been fetched") {
			t.Errorf("it said %q", a.ui.Status())
		}
		if strings.Contains(a.View(), "r=35") {
			t.Error("a cover was drawn from nothing")
		}
	})
}

// runCommand types a command and presses enter.
func runCommand(t *testing.T, a app, name string) app {
	t.Helper()
	next, _ := a.ui.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	a.ui = next.(tui.Model)
	for _, r := range name {
		next, _ = a.ui.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		a.ui = next.(tui.Model)
	}
	next, cmd := a.ui.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.ui = next.(tui.Model)

	// The intent reaches the app, the app answers with a message, and that
	// message is what the interface draws. Stopping after the first round
	// would report a command as done while nothing had been shown.
	pending := runAll(cmd)
	for round := 0; len(pending) > 0 && round < 8; round++ {
		var produced []tea.Msg
		for _, m := range pending {
			if m == nil {
				continue
			}
			var more []tea.Msg
			a, more = step(t, a, m)
			produced = append(produced, more...)
		}
		pending = produced
	}
	return a
}

// visible strips escape sequences so a failure prints something readable.
func visible(frame string) string {
	var b strings.Builder
	for skip := false; len(frame) > 0; frame = frame[1:] {
		switch {
		case frame[0] == 0x1b:
			skip = true
		case skip && (frame[0] == '\\' || frame[0] == '\a' || frame[0] == 'm'):
			skip = false
		case !skip:
			b.WriteByte(frame[0])
		}
	}
	return b.String()
}
