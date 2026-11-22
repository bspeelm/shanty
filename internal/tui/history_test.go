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

func TestMovingByPagesAndToTheEndsWorksWhileAFilterIsBeingTyped(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 64, Height: 10})
	long := tenTracks()
	m := send(t, next.(Model), AlbumLoaded(long))
	m, _ = press(t, m, "/")
	m = typed(t, m, "track")
	if !m.Filtering() || m.rows() != len(long.Songs) {
		t.Fatalf("the filter matches %d rows", m.rows())
	}

	m, _ = press(t, m, "end")
	if m.Cursor() != m.rows()-1 {
		t.Errorf("end went to row %d", m.Cursor())
	}
	m, _ = press(t, m, "home")
	if m.Cursor() != 0 {
		t.Errorf("home went to row %d", m.Cursor())
	}
	m, _ = press(t, m, "pgdown")
	down := m.Cursor()
	if down != m.page() {
		t.Errorf("pgdown went to row %d, want a page of %d", down, m.page())
	}
	m, _ = press(t, m, "pgup")
	if m.Cursor() != 0 || !m.Filtering() || m.Filter() != "track" {
		t.Errorf("pgup went to row %d, typing %v, filter %q", m.Cursor(), m.Filtering(), m.Filter())
	}
}

func TestEnterOnAFilterThatLeavesOneRowOpensIt(t *testing.T) {
	m, _ := press(t, loaded(t), "/")
	m = typed(t, m, "bilge")
	m, msg := press(t, m, "enter")
	if open, ok := msg.(OpenArtist); !ok || open.ID != "ar-2" {
		t.Fatalf("enter with one row left emitted %#v, want that artist opened", msg)
	}
	if m.Filtering() || m.Filter() != "bilge" {
		t.Errorf("the filter was not kept: typing %v, %q", m.Filtering(), m.Filter())
	}

	m, _ = press(t, send(t, loaded(t), keyOf("g"), keyOf("b"), AlbumsLoaded(everyAlbum())), "/")
	m = typed(t, m, "a")
	m, msg = press(t, m, "enter")
	if msg != nil || m.Filtering() || m.Filter() != "a" || m.rows() < 2 {
		t.Errorf("enter with %d rows left emitted %#v", m.rows(), msg)
	}
}

func TestHalfAScreenIsMovedByItsOwnKeys(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 64, Height: 12})
	m := send(t, next.(Model), AlbumLoaded(tenTracks()))
	half := m.page() / 2

	down, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if got := down.(Model).Cursor(); got != half || half < 1 {
		t.Fatalf("ctrl+d went to row %d, want half a page of %d", got, m.page())
	}
	up, _ := down.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if got := up.(Model).Cursor(); got != 0 {
		t.Errorf("ctrl+u went to row %d", got)
	}
}

func TestTheJumpToANarrowedQueueLandsOnARowOfIt(t *testing.T) {
	m := onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued(), At: 2}))
	m = narrowed(t, m, "aoi")
	if m.rows() != 2 {
		t.Fatalf("the filter leaves %d rows", m.rows())
	}
	m = onQueue(t, onArtists(t, m))
	if m.Filter() != "aoi" || m.Cursor() >= m.rows() {
		t.Fatalf("gq came back with the filter %q on row %d of %d", m.Filter(), m.Cursor(), m.rows())
	}

	m = onQueue(t, onArtists(t, send(t, m, QueueChanged{Tracks: queued(), At: 1})))
	if m.selectedIndex() != 1 {
		t.Errorf("gq is on track %d of the queue, want the one playing", m.selectedIndex())
	}
}

// selectedIndex is the place in the whole list of the row the cursor is on.
func (m Model) selectedIndex() int { return m.matches()[m.cursor[m.screen]] }
