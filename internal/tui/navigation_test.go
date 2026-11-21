package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// ctrlC is the key that quits.
var ctrlC = tea.KeyMsg{Type: tea.KeyCtrlC}

// typed sends a word one character at a time, as a keyboard does.
func typed(t *testing.T, m Model, word string) Model {
	t.Helper()
	for _, c := range word {
		m, _ = press(t, m, string(c))
	}
	return m
}

func TestTheJumpToPlaylistsAsksTheServerForThem(t *testing.T) {
	m, _ := press(t, loaded(t), "g")
	m, msg := press(t, m, "p")

	if _, ok := msg.(FetchPlaylists); !ok {
		t.Fatalf("gp emitted %T, want FetchPlaylists so the playlists are fetched", msg)
	}
	if m.Screen() != ScreenPlaylists {
		t.Fatalf("gp went to %s", m.Screen())
	}
	if frame := m.View(); !strings.Contains(frame, "loading") || strings.Contains(frame, "no playlists yet") {
		t.Errorf("playlists not fetched yet are shown as none rather than as loading:\n%s", frame)
	}
}

func TestTheJumpToEveryAlbumAsksTheServerForThem(t *testing.T) {
	m, _ := press(t, loaded(t), "g")
	m, msg := press(t, m, "b")

	if _, ok := msg.(FetchEveryAlbum); !ok {
		t.Fatalf("gb emitted %T, want FetchEveryAlbum so the albums are fetched", msg)
	}
	if m.Screen() != ScreenEveryAlbum {
		t.Fatalf("gb went to %s", m.Screen())
	}
	if frame := m.View(); !strings.Contains(frame, "loading") {
		t.Errorf("albums not fetched yet are not shown as loading:\n%s", frame)
	}
}

func TestTheJumpsToPlaylistsAndEveryAlbumKeepTheRowSelected(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}
	m := send(t, loaded(t), PlaylistsLoaded(lists), AlbumsLoaded(everyAlbum()))

	m = onPlaylists(t, m)
	m, _ = press(t, m, "j")
	m = onPlaylists(t, onArtists(t, m))
	if m.Cursor() != 1 {
		t.Errorf("coming back to the playlists put the cursor on row %d, want 1", m.Cursor())
	}

	m, _ = press(t, m, "g")
	m, _ = press(t, m, "b")
	m, _ = press(t, m, "j")
	m = onArtists(t, m)
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "b")
	if m.Cursor() != 1 {
		t.Errorf("coming back to every album put the cursor on row %d, want 1", m.Cursor())
	}
}

func TestAListOfPlaylistsThatCameBackShorterLeavesTheCursorOnARow(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}
	m := onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists)))
	m, _ = press(t, m, "j")

	m = send(t, m, PlaylistsLoaded(lists[:1]))
	if m.Cursor() != 0 {
		t.Fatalf("the cursor is on row %d of a list of one", m.Cursor())
	}
	_, msg := press(t, m, "enter")
	if open, ok := msg.(OpenPlaylist); !ok || open.ID != "pl-1" {
		t.Errorf("enter emitted %#v, want the one playlist left opened", msg)
	}
}

func TestAnAlbumOpenedFromASearchGoesBackToTheSearch(t *testing.T) {
	m, _ := press(t, searched(t), "j")

	m, msg := press(t, m, "enter")
	open, ok := msg.(OpenAlbum)
	if !ok {
		t.Fatalf("enter on the album result emitted %T", msg)
	}
	at := m.Cursor()
	m = send(t, m, AlbumLoaded(subsonic.Album{ID: open.ID, Name: "Low Water", Artist: "Aoi"}))
	if m.Screen() != ScreenTracks {
		t.Fatalf("the album did not open; the screen is %s", m.Screen())
	}

	m, _ = press(t, m, "h")
	if m.Screen() != ScreenSearch {
		t.Fatalf("back from an album opened from a search went to %s", m.Screen())
	}
	if m.Cursor() != at {
		t.Errorf("the cursor is on row %d of the results, want %d", m.Cursor(), at)
	}
}

func TestAnAlbumOpenedFromTheStarredListGoesBackToIt(t *testing.T) {
	m := onStarred(t, send(t, loaded(t), starredChanged()))
	m, _ = press(t, m, "j")

	m, msg := press(t, m, "enter")
	open, ok := msg.(OpenAlbum)
	if !ok {
		t.Fatalf("enter on the starred album emitted %T", msg)
	}
	m = send(t, m, AlbumLoaded(subsonic.Album{ID: open.ID, Name: "Harbour", Artist: "Aoi"}))

	m, _ = press(t, m, "h")
	if m.Screen() != ScreenStarred {
		t.Errorf("back from an album opened from the starred list went to %s", m.Screen())
	}
}

func TestLeavingAPlaylistBeingReorderedForgetsTheArrangement(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")

	m = onQueue(t, m)
	if m.Reordering() {
		t.Fatal("the playlist is still being reordered on the queue screen")
	}
	if strings.Contains(m.View(), "reordering") {
		t.Errorf("the queue screen still says a playlist is being reordered:\n%s", m.View())
	}
	if _, msg := press(t, m, "enter"); msg != nil {
		if _, saved := msg.(ReorderPlaylist); saved {
			t.Error("enter on another screen saved the arrangement that was left")
		}
	}
}

func TestAScreenShownFromOutsideEndsAReorderToo(t *testing.T) {
	m := send(t, reordering(t, evening3(t)), ShowWiki{})
	if m.Reordering() {
		t.Error("the playlist is still being reordered behind the wiki")
	}
}

func TestAListBeingReorderedCannotBeNarrowed(t *testing.T) {
	m, _ := press(t, reordering(t, evening3(t)), "/")

	if m.Filtering() {
		t.Fatal("a filter was started over a list whose rows are being moved")
	}
	if !m.Reordering() {
		t.Error("asking for a filter ended the reorder")
	}
	if !strings.Contains(m.Status(), "reordered") {
		t.Errorf("the refusal does not say why: %q", m.Status())
	}
}

// underSky is the artist list with the visualiser over it.
func underSky(t *testing.T) Model {
	t.Helper()
	next, _ := loaded(t).Update(ShowVisualiser{})
	m := next.(Model)
	if !m.ShowingSky() {
		t.Fatal("the visualiser did not open")
	}
	return m
}

func TestAKeyForTheListDoesNothingUnderTheVisualiser(t *testing.T) {
	m := underSky(t)

	m, _ = press(t, m, "j")
	if m.cursor[ScreenArtists] != 0 {
		t.Errorf("j moved the hidden list to row %d", m.cursor[ScreenArtists])
	}
	for _, key := range []string{"enter", "?", "S", "*", "a", "/"} {
		next, msg := press(t, m, key)
		if msg != nil {
			t.Errorf("%q under the visualiser emitted %T", key, msg)
		}
		if next.Screen() != ScreenArtists || next.Filtering() || !next.ShowingSky() {
			t.Errorf("%q under the visualiser changed what is underneath", key)
		}
	}

	m, _ = press(t, m, "g")
	m, _ = press(t, m, "q")
	if m.Screen() != ScreenArtists || !m.ShowingSky() {
		t.Errorf("gq under the visualiser went to %s", m.Screen())
	}
}

func TestThePlayerKeysStillWorkUnderTheVisualiser(t *testing.T) {
	m := underSky(t)

	if _, msg := press(t, m, " "); msg != (TogglePause{}) {
		t.Errorf("space under the visualiser emitted %T", msg)
	}
	if _, msg := press(t, m, "n"); msg != (SkipNext{}) {
		t.Errorf("n under the visualiser emitted %T", msg)
	}
	m, _ = press(t, m, "5")
	m, _ = press(t, m, "0")
	if _, msg := press(t, m, "%"); msg != SeekToPercent(50) {
		t.Errorf("50%% under the visualiser emitted %#v", msg)
	}
	if next, _ := press(t, m, ":"); !next.Commanding() {
		t.Error("the command line does not open under the visualiser")
	}
}

func TestAKeyForTheListTakesTheFullCoverAwayAndDoesNothingElse(t *testing.T) {
	covered := send(t, loaded(t), FullArt{"a picture filling the screen"})

	for _, key := range []string{"j", "enter", "x", "?"} {
		m, msg := press(t, covered, key)
		if m.ShowingArt() {
			t.Errorf("%q left the cover up", key)
		}
		if !reflect.DeepEqual(msg, tea.ClearScreen()) {
			t.Errorf("%q under the cover emitted %T, want only a repaint", key, msg)
		}
		if m.cursor[ScreenArtists] != 0 || m.Screen() != ScreenArtists {
			t.Errorf("%q under the cover acted on the hidden list", key)
		}
	}

	m, msg := press(t, covered, " ")
	if !m.ShowingArt() || msg != (TogglePause{}) {
		t.Errorf("space under the cover emitted %T and left the cover showing: %v", msg, m.ShowingArt())
	}
}

func TestControlCQuitsWhileAFilterIsBeingTyped(t *testing.T) {
	m, _ := press(t, loaded(t), "/")
	m = typed(t, m, "bil")

	_, cmd := m.Update(ctrlC)
	if cmd == nil {
		t.Fatal("ctrl+c did nothing while a filter was being typed")
	}
	if msg := cmd(); msg != (Quit{}) {
		t.Errorf("ctrl+c in the filter emitted %T, want Quit", msg)
	}
}

func TestControlCQuitsWhileACommandIsBeingTyped(t *testing.T) {
	m, _ := press(t, loaded(t), ":")
	m = typed(t, m, "sear")

	_, cmd := m.Update(ctrlC)
	if cmd == nil {
		t.Fatal("ctrl+c did nothing while a command was being typed")
	}
	if msg := cmd(); msg != (Quit{}) {
		t.Errorf("ctrl+c on the command line emitted %T, want Quit", msg)
	}
}

func TestGoingBackToAListThatWasNarrowedLandsOnTheRowThatWasOpened(t *testing.T) {
	m, _ := press(t, loaded(t), "/")
	m = typed(t, m, "bilge")
	m, _ = press(t, m, "enter")

	m, msg := press(t, m, "enter")
	open, ok := msg.(OpenArtist)
	if !ok || open.ID != "ar-2" {
		t.Fatalf("enter on the narrowed list emitted %#v, want ar-2 opened", msg)
	}
	m = send(t, m, ArtistLoaded(library()[1]))

	m, _ = press(t, m, "h")
	if m.Screen() != ScreenArtists || m.Filter() != "" {
		t.Fatalf("back went to %s with the filter %q", m.Screen(), m.Filter())
	}
	if got := m.artists[m.Cursor()].ID; got != "ar-2" {
		t.Errorf("the cursor is on %s, want the artist that was opened", got)
	}
}

func TestAnArtistThatArrivesAfterItsScreenWasLeftDoesNotTakeTheScreen(t *testing.T) {
	m, msg := press(t, loaded(t), "enter")
	if _, ok := msg.(OpenArtist); !ok {
		t.Fatalf("enter emitted %T", msg)
	}
	m, _ = press(t, m, "h")
	if m.loading {
		t.Error("the library still says it is loading what was walked away from")
	}

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenLibrary {
		t.Fatalf("an artist asked for and then left moved the screen to %s", m.Screen())
	}

	// The same artist asked for again is shown.
	m, _ = press(t, onArtists(t, m), "enter")
	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenAlbums {
		t.Errorf("an artist asked for again was not shown; the screen is %s", m.Screen())
	}
}

func TestOfTwoArtistsAskedForOnlyTheLastIsShown(t *testing.T) {
	m, _ := press(t, loaded(t), "enter")
	m, _ = press(t, m, "j")
	m, msg := press(t, m, "enter")
	if open, ok := msg.(OpenArtist); !ok || open.ID != "ar-2" {
		t.Fatalf("the second enter emitted %#v", msg)
	}

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenArtists {
		t.Fatalf("the first artist asked for took the screen to %s", m.Screen())
	}
	m = send(t, m, ArtistLoaded(library()[1]))
	if m.Screen() != ScreenAlbums || m.Artist().ID != "ar-2" {
		t.Errorf("the last artist asked for is not shown: %s, %q", m.Screen(), m.Artist().ID)
	}
}

func TestAnAlbumAndAPlaylistThatArriveAfterTheirScreenWasLeftAreDropped(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}
	m := send(t, loaded(t), ArtistLoaded(library()[0]))
	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "h")
	m = send(t, m, AlbumLoaded(library()[0].Albums[0]))
	if m.Screen() != ScreenArtists {
		t.Errorf("an album asked for and then left moved the screen to %s", m.Screen())
	}

	m = onPlaylists(t, send(t, m, PlaylistsLoaded(lists)))
	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "h")
	m = send(t, m, PlaylistLoaded(lists[0]))
	if m.Screen() != ScreenLibrary {
		t.Errorf("a playlist asked for and then left moved the screen to %s", m.Screen())
	}
}

func TestAPageIsTheRowsOnScreenWhenACoverTakesSomeOfThem(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 64, Height: 30})
	long := tenTracks()
	long.Songs = append(long.Songs, append(tenTracks().Songs, tenTracks().Songs...)...)
	m := send(t, next.(Model), AlbumLoaded(long))
	bare := m.page()

	art := make([]string, 6)
	for i := range art {
		art[i] = strings.Repeat(" ", 12)
	}
	m = send(t, m, CoverArt{AlbumID: "al-7", Lines: art})
	if len(m.artBlock(m.viewWidth())) == 0 {
		t.Fatal("the cover is not on screen, so this measures nothing")
	}

	_, visible := m.skySize()
	if m.page() != visible || m.page() >= bare {
		t.Errorf("a page is %d rows with %d on screen under the cover, and %d without it",
			m.page(), visible, bare)
	}
	m, _ = press(t, m, "pgdown")
	if m.Cursor() != visible {
		t.Errorf("pgdown moved to row %d, want %d", m.Cursor(), visible)
	}
}

func TestACommandThatIsRefusedIsDrawnAsAFailure(t *testing.T) {
	m, _ := typeCommand(t, loaded(t), "nosuchthing")
	if !m.statusFailed || !strings.Contains(m.Status(), "no such command") {
		t.Fatalf("an unknown command left %q, failed %v", m.Status(), m.statusFailed)
	}
	m, _ = typeCommand(t, loaded(t), "playlist reorder")
	if !m.statusFailed {
		t.Errorf("a reorder with no playlist on screen is not drawn as a failure: %q", m.Status())
	}

	m = send(t, m, Notice("playing Ballast next"))
	if m.statusFailed {
		t.Error("a notice after a refusal is still drawn as a failure")
	}
}

func TestWhatAKeySaysAfterAFailureIsNotDrawnAsOne(t *testing.T) {
	m := send(t, loaded(t), Failed{Message: "the server did not answer"})
	m, _ = press(t, m, "q")
	if m.statusFailed || !strings.Contains(m.Status(), ":q") {
		t.Errorf("q after a failure left %q, failed %v", m.Status(), m.statusFailed)
	}
}

func TestAMessageIsGoneAfterTheNextKey(t *testing.T) {
	m := send(t, loaded(t), Notice("added Slipway to the end of the queue"))
	m, _ = press(t, m, "j")
	if m.Status() != "" {
		t.Errorf("the message is still on the last row after a key: %q", m.Status())
	}
	if !strings.Contains(m.View(), "move") {
		t.Errorf("the key help did not come back:\n%s", m.View())
	}
}

func TestAMessageAboutSomethingStillArrivingStaysWhileKeysArePressed(t *testing.T) {
	m, _ := press(t, loaded(t), "enter")
	was := m.Status()
	m, _ = press(t, m, "j")
	if m.Status() != was || was == "" {
		t.Errorf("the message went from %q to %q before the artist arrived", was, m.Status())
	}
}

func TestAReloadTheServerRefusedLeavesNoReloadWaiting(t *testing.T) {
	m := send(t, onQueue(t, loaded(t)), Reload{}, Failed{Message: "there is nothing here to fetch again"})
	if m.loading || m.keep != "" {
		t.Fatalf("a refused reload is still waiting: loading %v, keeping %q", m.loading, m.keep)
	}

	m = send(t, onArtists(t, m), ArtistsLoaded(library()))
	if strings.Contains(m.Status(), "reloaded") {
		t.Errorf("a list that arrived later was reported as the reload: %q", m.Status())
	}
}

func TestReloadingEveryAlbumKeepsTheSelection(t *testing.T) {
	m := send(t, loaded(t), ShowEveryAlbum{}, AlbumsLoaded(everyAlbum()))
	m, _ = press(t, m, "j")
	was := m.albums[m.Cursor()].ID

	grown := append([]subsonic.Album{{ID: "al-0", Name: "A New Arrival", Artist: "Aoi"}}, everyAlbum()...)
	m = send(t, m, Reload{}, AlbumsLoaded(grown))
	if got := m.albums[m.Cursor()].ID; got != was {
		t.Errorf("the cursor moved to %q, want it to stay on %q", got, was)
	}
	if !strings.Contains(m.Status(), "reloaded") {
		t.Errorf("the reload did not say it finished: %q", m.Status())
	}
}

func TestReloadingTheStarredListKeepsTheSelection(t *testing.T) {
	m := onStarred(t, send(t, loaded(t), starredChanged()))
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j")
	was := m.starredRows[m.Cursor()].id
	if was != "tr-2" {
		t.Fatalf("the cursor is on %q, want the starred track", was)
	}

	grown := starredChanged()
	grown.Results.Artists = append([]subsonic.Artist{{ID: "ar-3", Name: "坂本龍一"}}, grown.Results.Artists...)
	grown.IDs["ar-3"] = true
	m = send(t, m, Reload{}, grown)

	if m.loading {
		t.Error("the reload never finished")
	}
	if got := m.starredRows[m.Cursor()].id; got != was {
		t.Errorf("the cursor moved to %q, want it to stay on %q", got, was)
	}
}
