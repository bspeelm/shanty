package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// deepIn is the track list of Harbour, reached through the artists and Aoi.
func deepIn(t *testing.T) Model {
	t.Helper()
	aoi := library()[0]
	return send(t, loaded(t), ArtistLoaded(aoi), AlbumLoaded(aoi.Albums[0]))
}

func TestBackReturnsThroughTheScreensThatWereLeft(t *testing.T) {
	m, _ := press(t, deepIn(t), "j")
	m = onQueue(t, m)
	m = send(t, m, ShowWiki{})

	for _, want := range []Screen{ScreenQueue, ScreenTracks, ScreenAlbums, ScreenArtists, ScreenLibrary} {
		var msg tea.Msg
		m, msg = press(t, m, "esc")
		if m.Screen() != want || msg != nil {
			t.Fatalf("back went to %s and emitted %#v, want %s", m.Screen(), msg, want)
		}
		if want == ScreenTracks && (m.Cursor() != 1 || m.Album().ID != "al-1") {
			t.Errorf("the track list came back on row %d of %q", m.Cursor(), m.Album().ID)
		}
	}
	for range 5 {
		var msg tea.Msg
		m, msg = press(t, m, "esc")
		if m.Screen() != ScreenLibrary || msg != nil {
			t.Fatalf("back on the library went to %s and emitted %#v", m.Screen(), msg)
		}
	}
}

func TestBackShowsTheArtistThatWasLeftAfterAnotherWasOpened(t *testing.T) {
	aoi, pumps := library()[0], library()[1]
	pumps.Albums = []subsonic.Album{{ID: "al-3", Name: "Bail", Artist: pumps.Name}}
	m := send(t, loaded(t), SearchLoaded{Query: "bilge", Results: subsonic.Results{Artists: []subsonic.Artist{pumps}}})
	m = send(t, onArtists(t, m), ArtistLoaded(aoi))
	m, _ = press(t, m, "j")
	m = send(t, m, AlbumLoaded(aoi.Albums[1]))

	// From Low Water to the search, and into the other artist and their album.
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "/")
	if m.Screen() != ScreenSearch {
		t.Fatalf("g/ went to %s", m.Screen())
	}
	m = send(t, m, ArtistLoaded(pumps), AlbumLoaded(pumps.Albums[0]))

	m, _ = press(t, m, "esc")
	if m.Artist().ID != "ar-2" || !strings.Contains(m.View(), "The Bilge Pumps") {
		t.Fatalf("back from the album shows %q", m.Artist().Name)
	}
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenTracks || m.Album().ID != "al-2" {
		t.Fatalf("back from the search shows %s holding %q, want Low Water", m.Screen(), m.Album().ID)
	}
	m, _ = press(t, m, "esc")
	frame := m.View()
	if m.Screen() != ScreenAlbums || m.Artist().ID != "ar-1" || m.Cursor() != 1 {
		t.Fatalf("back shows %s of %q on row %d, want Aoi's albums on row 1", m.Screen(), m.Artist().Name, m.Cursor())
	}
	if !strings.Contains(frame, "artists › Aoi") || !strings.Contains(frame, "Low Water") || strings.Contains(frame, "Bail") {
		t.Errorf("the heading and the albums are not one artist's:\n%s", frame)
	}
}

func TestGoingBetweenTwoScreensDoesNotGrowWhatBackRemembers(t *testing.T) {
	m := loaded(t)
	for range 40 {
		m = onArtists(t, onQueue(t, m))
	}
	if len(m.trail) > 2 {
		t.Fatalf("forty trips between two screens are remembered as %d places", len(m.trail))
	}
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenLibrary {
		t.Errorf("two backs after going between two screens reached %s", m.Screen())
	}
}

func TestWhatBackRemembersIsBounded(t *testing.T) {
	m := loaded(t)
	for range 3 * trailKept {
		m = m.show(ScreenAlbums).show(ScreenTracks)
	}
	if len(m.trail) != trailKept {
		t.Errorf("%d places are remembered, want %d", len(m.trail), trailKept)
	}
}

func TestTheLibraryIsWhereBackEnds(t *testing.T) {
	m, _ := press(t, deepIn(t), "g")
	m, _ = press(t, m, "l")
	if m.Screen() != ScreenLibrary || len(m.trail) != 0 {
		t.Fatalf("gl left %d places to go back to", len(m.trail))
	}
	if m, _ = press(t, m, "esc"); m.Screen() != ScreenLibrary {
		t.Errorf("back on the library went to %s", m.Screen())
	}
}

func TestAJumpToAScreenLeftEarlierShowsItAsItWasAndBackReturnsToWhereTheJumpWasMade(t *testing.T) {
	m := onStarred(t, send(t, loaded(t), starredChanged()))
	m, _ = press(t, m, "j")
	at := m.Cursor()
	m = onQueue(t, m)

	m = onStarred(t, m)
	if m.Cursor() != at {
		t.Errorf("gs came back on row %d, want %d", m.Cursor(), at)
	}
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenQueue {
		t.Errorf("back from the starred list went to %s, want the queue the jump was made from", m.Screen())
	}
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenArtists {
		t.Errorf("the next back went to %s, want the artists the starred list was first reached from", m.Screen())
	}
}

func TestTheHeadingShowsThePathAndTheFilterKept(t *testing.T) {
	m := deepIn(t)
	if frame := m.View(); !strings.Contains(frame, "shanty · artists › Aoi › Harbour") {
		t.Errorf("the heading does not show the path:\n%s", frame)
	}

	m = narrowed(t, m, "slip")
	if frame := m.View(); !strings.Contains(frame, "Harbour · /slip") {
		t.Errorf("the heading does not show the filter kept:\n%s", frame)
	}

	narrow := send(t, deepIn(t), tea.WindowSizeMsg{Width: 22, Height: 12})
	first := strings.SplitN(stripSGR(narrow.View()), "\n", 2)[0]
	if !strings.Contains(first, "… › Harbour") || strings.Contains(first, "artists") {
		t.Errorf("a narrow heading does not keep the screen showing: %q", first)
	}

	if frame := onQueue(t, deepIn(t)).View(); strings.Contains(frame, "›") {
		t.Errorf("a screen reached by a jump shows a path:\n%s", frame)
	}
}

func TestAJumpBegunAndACountTypedAreShownOnTheLastRow(t *testing.T) {
	last := func(m Model) string {
		rows := strings.Split(stripSGR(m.View()), "\n")
		return rows[len(rows)-1]
	}
	m, _ := press(t, loaded(t), "g")
	if !strings.HasPrefix(last(m), "g") {
		t.Errorf("a jump begun is not shown: %q", last(m))
	}
	m, _ = press(t, loaded(t), "1")
	m, _ = press(t, m, "2")
	if !strings.HasPrefix(last(m), "12") {
		t.Errorf("a count typed is not shown: %q", last(m))
	}
	m, _ = press(t, m, "j")
	if strings.HasPrefix(last(m), "12") {
		t.Errorf("the count is still shown after it was used: %q", last(m))
	}
}

func TestTheLastSearchIsReachedFromTheLibraryAndByAJump(t *testing.T) {
	m, msg := press(t, onRow(t, sized(New()), "Search"), "enter")
	if m.Screen() != ScreenLibrary || msg != nil || !strings.Contains(m.Status(), ":search") {
		t.Fatalf("the Search row with nothing searched for went to %s saying %q", m.Screen(), m.Status())
	}

	m, _ = press(t, searched(t), "j")
	at := m.Cursor()
	m = onArtists(t, m)
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "/")
	if m.Screen() != ScreenSearch || m.Cursor() != at {
		t.Errorf("g/ went to %s on row %d, want the results on row %d", m.Screen(), m.Cursor(), at)
	}

	m = send(t, m, SearchLoaded{Query: "bail", Results: results()})
	if m.query != "bail" || !m.found[m.Cursor()].selectable() {
		t.Errorf("a new search did not replace the results: %q", m.query)
	}
}
