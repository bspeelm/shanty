package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// eveningOf is the playlist evening3 opens, cut to its first n tracks.
func eveningOf(n int) subsonic.Playlist {
	songs := []subsonic.Song{
		{ID: "tr-1", Title: "Slipway", Artist: "Aoi", Duration: 180},
		{ID: "tr-2", Title: "Ballast", Artist: "Aoi", Duration: 200},
		{ID: "tr-3", Title: "Low Water", Artist: "The Bilge Pumps", Duration: 220},
	}
	return subsonic.Playlist{ID: "pl-1", Name: "Evening", SongCount: n, Songs: songs[:n]}
}

func TestAPlaylistThatChangesOnTheServerWhileBeingReorderedEndsTheReorder(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j")

	m = send(t, m, PlaylistChanged(eveningOf(2)))
	if m.Reordering() {
		t.Fatal("the arrangement of a playlist that no longer holds those tracks is still being made")
	}
	if !strings.Contains(m.Status(), "changed") {
		t.Errorf("nothing says why the reorder ended: %q", m.Status())
	}
	if m.Cursor() >= 2 {
		t.Errorf("the cursor is on row %d of two", m.Cursor())
	}
	m = send(t, m, Reload{})
	_ = m.View()
}

func TestAPlaylistRenamedWhileBeingReorderedKeepsTheArrangementBeingMade(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")

	renamed := eveningOf(3)
	renamed.Name = "Dusk"
	m = send(t, m, PlaylistChanged(renamed))
	if !m.Reordering() || m.order[1].ID != "tr-1" {
		t.Errorf("a rename ended the reorder or lost the move: %v, %v", m.Reordering(), idsOf(m.order))
	}
	if m.Playlist().Name != "Dusk" || strings.Contains(m.Status(), "changed") {
		t.Errorf("the playlist is called %q and the last row says %q", m.Playlist().Name, m.Status())
	}
}

func TestReloadingAPlaylistBeingReorderedReadsTheRowsOnScreen(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")
	m = send(t, m, Reload{})
	if m.keep != "tr-1" {
		t.Errorf("the reload kept %q, want the track under the cursor in the arrangement", m.keep)
	}
}

func TestATrackIsNotTakenOutOfAPlaylistWhileItIsBeingReordered(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")

	m, msg := press(t, m, "r")
	if msg != nil {
		t.Fatalf("r while reordering emitted %#v", msg)
	}
	if !m.Reordering() || !strings.Contains(m.Status(), "reorder") {
		t.Errorf("r while reordering said %q", m.Status())
	}
}

func TestAListReplacedWhileAnotherScreenIsShowingKeepsItsCursorOnARow(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}
	few := starredChanged()
	few.Results.Albums, few.Results.Songs = nil, nil

	for _, tc := range []struct {
		name    string
		reach   func(Model) Model
		shorter tea.Msg
	}{
		{"artists", func(m Model) Model { return m }, ArtistsLoaded(library()[:1])},
		{"every album", func(m Model) Model {
			return send(t, m, keyOf("g"), keyOf("b"), AlbumsLoaded(everyAlbum()))
		}, AlbumsLoaded(everyAlbum()[:1])},
		{"queue", func(m Model) Model {
			return onQueue(t, send(t, m, QueueChanged{Tracks: queued()}))
		}, QueueChanged{Tracks: queued()[:1]}},
		{"starred", func(m Model) Model {
			return onStarred(t, send(t, m, starredChanged()))
		}, few},
		{"playlists", func(m Model) Model {
			return onPlaylists(t, send(t, m, PlaylistsLoaded(lists)))
		}, PlaylistsLoaded(lists[:1])},
		{"playlist", func(m Model) Model {
			return send(t, m, PlaylistLoaded(eveningOf(3)))
		}, PlaylistChanged(eveningOf(1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.reach(loaded(t))
			screen := m.Screen()
			m, _ = press(t, m, "end")
			if m.Cursor() == 0 {
				t.Fatal("the cursor did not leave the top, so this measures nothing")
			}

			m = send(t, m, ShowMessages{}, tc.shorter)
			if m.Screen() != ScreenMessages {
				t.Fatalf("the list arriving moved the screen to %s", m.Screen())
			}
			m.screen = screen
			if m.Cursor() >= m.rows() {
				t.Fatalf("the cursor is on row %d of %d", m.Cursor(), m.rows())
			}
			if m.isGrouped() && !m.grouped()[m.Cursor()].selectable() {
				t.Errorf("the cursor is on a heading")
			}
			_, _ = press(t, m, "enter")
			_, _ = press(t, m, "*")
			_ = m.View()
		})
	}
}

func TestEveryAlbumComingBackShorterLeavesTheCursorOnARow(t *testing.T) {
	m := send(t, loaded(t), keyOf("g"), keyOf("b"), AlbumsLoaded(everyAlbum()))
	m, _ = press(t, m, "end")
	m = send(t, m, AlbumsLoaded(everyAlbum()[:1]))
	if m.Cursor() != 0 {
		t.Fatalf("the cursor is on row %d of a list of one", m.Cursor())
	}
	if _, msg := press(t, m, "enter"); msg.(OpenAlbum).ID != "al-1" {
		t.Errorf("enter emitted %#v", msg)
	}
}

func TestAnAnswerHoldingNoPlaylistsIsDrawnAsFetched(t *testing.T) {
	m := send(t, loaded(t), PlaylistsLoaded(nil))
	if got := m.libraryCount(ScreenPlaylists); got != "0" {
		t.Errorf("the library counts %q playlists after the server said there are none", got)
	}
	m = onPlaylists(t, m)
	if frame := m.View(); strings.Contains(frame, "loading") || !strings.Contains(frame, "no playlists yet") {
		t.Errorf("no playlists is drawn as still loading:\n%s", frame)
	}
}

func TestAPlaylistOpenedStraightAfterTheJumpToThemIsShown(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}
	m := send(t, loaded(t), PlaylistsLoaded(lists))

	m, _ = press(t, m, "g")
	m, fetch := press(t, m, "p")
	if fetch != (FetchPlaylists{}) {
		t.Fatalf("gp emitted %#v", fetch)
	}
	m, ask := opened(t, m)
	// What gp asked for is answered with the list, which does not move the
	// screen or stop the open being waited for.
	m = send(t, m, PlaylistsLoaded(lists), Opened{Ask: ask, Loaded: PlaylistLoaded(eveningOf(3))})
	if m.Screen() != ScreenPlaylist {
		t.Errorf("the playlist opened after gp is not shown; the screen is %s", m.Screen())
	}
}

func TestARefusalWhileSomethingIsArrivingDoesNotColourWhatIsSaidNext(t *testing.T) {
	m, _ := press(t, loaded(t), "enter")
	m, _ = typeCommand(t, m, "nosuchthing")
	if !m.statusFailed {
		t.Fatal("the refusal is not drawn as one")
	}
	m, _ = press(t, m, "q")
	if m.statusFailed || !strings.Contains(m.Status(), ":q") {
		t.Errorf("q said %q, drawn as a failure: %v", m.Status(), m.statusFailed)
	}
}

func TestReloadingTheLibraryWithFewerArtistsThanItHasRows(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(library()))
	m, _ = press(t, m, "end")

	m = send(t, m, Reload{}, Reloaded{Loaded: ArtistsLoaded(library())})
	if m.Screen() != ScreenLibrary || m.loading() {
		t.Fatalf("the library is on %s, loading %v", m.Screen(), m.loading())
	}
	if !strings.Contains(m.Status(), "reloaded") {
		t.Errorf("the reload did not say it finished: %q", m.Status())
	}
	if m.Cursor() != len(libraryRows)-1 {
		t.Errorf("the library cursor moved to %d", m.Cursor())
	}
}

func TestLeavingAScreenDropsTheReloadItWasWaitingFor(t *testing.T) {
	m := send(t, loaded(t), keyOf("g"), keyOf("b"), AlbumsLoaded(everyAlbum()), Reload{})
	m, _ = press(t, m, "esc")
	if m.loading() || m.keep != "" {
		t.Fatalf("the library is still waiting for a reload: loading %v, keeping %q", m.loading(), m.keep)
	}
	m = send(t, onArtists(t, m), Reloaded{Loaded: ArtistsLoaded(library())}, Reloaded{Loaded: AlbumsLoaded(everyAlbum())})
	if strings.Contains(m.Status(), "reloaded") {
		t.Errorf("a list that arrived on another screen was reported as the reload: %q", m.Status())
	}
}

func TestAReloadAnsweredAfterItsScreenWasLeftDoesNotTakeTheScreen(t *testing.T) {
	artist, album := library()[0], library()[0].Albums[0]

	m := send(t, loaded(t), ArtistLoaded(artist), AlbumLoaded(album), Reload{})
	m, _ = press(t, m, "h")
	m = send(t, m, Reloaded{Loaded: AlbumLoaded(album)})
	if m.Screen() != ScreenAlbums || m.loading() {
		t.Errorf("an album reloaded and then left: screen %s, loading %v", m.Screen(), m.loading())
	}

	m = send(t, m, Reload{})
	m, _ = press(t, m, "h")
	m = send(t, m, Reloaded{Loaded: ArtistLoaded(artist)})
	if m.Screen() != ScreenArtists || m.loading() {
		t.Errorf("an artist reloaded and then left: screen %s, loading %v", m.Screen(), m.loading())
	}

	m = send(t, onStarred(t, send(t, m, starredChanged())), Reload{})
	m, _ = press(t, m, "h")
	m = send(t, m, Reloaded{Loaded: starredChanged()})
	if m.loading() || strings.Contains(m.Status(), "reloaded") {
		t.Errorf("a starred list reloaded and then left: loading %v, said %q", m.loading(), m.Status())
	}
}

func TestAReloadAnsweredOnItsScreenStillKeepsTheSelection(t *testing.T) {
	artist, album := library()[0], library()[0].Albums[0]
	m := send(t, loaded(t), ArtistLoaded(artist), AlbumLoaded(album))
	m, _ = press(t, m, "j")
	m = send(t, m, Reload{}, Reloaded{Loaded: AlbumLoaded(album)})
	if m.Screen() != ScreenTracks || m.Cursor() != 1 || !strings.Contains(m.Status(), "reloaded") {
		t.Errorf("screen %s, cursor %d, said %q", m.Screen(), m.Cursor(), m.Status())
	}
}

func TestAReloadAskedForTwiceKeepsTheSelectionThroughBothAnswers(t *testing.T) {
	artist, album := library()[0], library()[0].Albums[0]
	m := send(t, loaded(t), ArtistLoaded(artist), AlbumLoaded(album))
	m, _ = press(t, m, "j")
	m = send(t, m, Reload{}, Reload{}, Reloaded{Loaded: AlbumLoaded(album)}, Reloaded{Loaded: AlbumLoaded(album)})
	if m.Screen() != ScreenTracks || m.Cursor() != 1 || m.loading() {
		t.Errorf("tracks: screen %s, cursor %d, loading %v", m.Screen(), m.Cursor(), m.loading())
	}

	m, _ = press(t, m, "h")
	m, _ = press(t, m, "j")
	m = send(t, m, Reload{}, Reload{}, Reloaded{Loaded: ArtistLoaded(artist)}, Reloaded{Loaded: ArtistLoaded(artist)})
	if m.Screen() != ScreenAlbums || m.Cursor() != 1 {
		t.Errorf("albums: screen %s, cursor %d", m.Screen(), m.Cursor())
	}

	m = send(t, onArtists(t, m), keyOf("j"), Reload{}, Reload{},
		Reloaded{Loaded: ArtistsLoaded(library())}, Reloaded{Loaded: ArtistsLoaded(library())})
	if m.Cursor() != 1 {
		t.Errorf("artists: cursor %d", m.Cursor())
	}
}

func TestAReloadOfSomethingNoLongerHeldIsDropped(t *testing.T) {
	artist := library()[0]
	m := send(t, loaded(t), ArtistLoaded(artist), AlbumLoaded(artist.Albums[0]))
	m = send(t, m, Reloaded{Loaded: AlbumLoaded(artist.Albums[1])}, Reloaded{Loaded: ArtistLoaded(library()[1])})
	if m.Screen() != ScreenTracks || m.Album().ID != "al-1" || m.Artist().ID != "ar-1" {
		t.Errorf("screen %s, album %q, artist %q", m.Screen(), m.Album().ID, m.Artist().ID)
	}
}

func TestAPlaylistRefreshedAfterItsScreenWasLeftIsKeptWithoutGoingBackToIt(t *testing.T) {
	m, _ := press(t, evening3(t), "end")
	m, _ = press(t, m, "h")
	if m.Screen() != ScreenPlaylists {
		t.Fatalf("back from the playlist went to %s", m.Screen())
	}

	m = send(t, m, PlaylistChanged(eveningOf(2)))
	if m.Screen() != ScreenPlaylists {
		t.Fatalf("the answer to a change made before leaving went back to %s", m.Screen())
	}
	if len(m.Playlist().Songs) != 2 || m.playlists[0].SongCount != 2 {
		t.Errorf("the playlist as the server now has it was not kept: %d tracks held, %d listed",
			len(m.Playlist().Songs), m.playlists[0].SongCount)
	}
}

func TestAChangeToOnePlaylistDoesNotTakeTheScreenFromAnother(t *testing.T) {
	morning := subsonic.Playlist{ID: "pl-2", Name: "Morning", Songs: eveningOf(1).Songs}
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}
	m := send(t, loaded(t), PlaylistsLoaded(lists), PlaylistLoaded(eveningOf(3)))
	m, removed := press(t, m, "r")
	if _, ok := removed.(EditPlaylist); !ok {
		t.Fatalf("r emitted %#v", removed)
	}
	m, _ = press(t, m, "h")
	m, _ = press(t, m, "j")
	m, ask := opened(t, m)

	m = send(t, m, Opened{Ask: ask, Loaded: PlaylistLoaded(morning)}, PlaylistChanged(eveningOf(2)))
	if m.Screen() != ScreenPlaylist || m.Playlist().ID != "pl-2" {
		t.Errorf("Evening's answer took the screen: %s showing %q", m.Screen(), m.Playlist().ID)
	}
	if m.playlists[0].SongCount != 2 {
		t.Errorf("the list still counts %d tracks in Evening", m.playlists[0].SongCount)
	}
}

func TestATrackAddedWhileEditingDoesNotTakeTheScreenToThePlaylist(t *testing.T) {
	a := library()[0]
	m := send(t, loaded(t), EditingPlaylist(eveningOf(1)), ArtistLoaded(a), AlbumLoaded(a.Albums[0]))
	m, _ = press(t, m, "j")
	m, added := press(t, m, "a")
	if _, ok := added.(EditPlaylist); !ok {
		t.Fatalf("a emitted %#v", added)
	}

	stillEditing := send(t, m, PlaylistChanged(eveningOf(2)))
	if stillEditing.Screen() != ScreenTracks || !stillEditing.inPlaylist["tr-2"] {
		t.Errorf("while editing: screen %s, marked %v", stillEditing.Screen(), stillEditing.inPlaylist)
	}

	m, _ = press(t, m, "e")
	m = send(t, m, PlaylistChanged(eveningOf(2)))
	if m.Screen() != ScreenTracks {
		t.Errorf("after leaving the edit, the add's answer went to %s", m.Screen())
	}
}

func TestAnOpenTriedAgainAfterItFailedDoesNotLoseTheNextChange(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}
	m := onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists)))
	m, first := opened(t, m)
	m = send(t, m, Failed{Message: "the server did not answer", Ask: first})
	if m.loading() || m.waiting != 0 {
		t.Fatalf("the failed open is still waited for: loading %v, waiting %d", m.loading(), m.waiting)
	}
	m, second := opened(t, m)
	m = send(t, m, Opened{Ask: second, Loaded: PlaylistLoaded(eveningOf(3))})
	m, _ = press(t, m, "r")

	m = send(t, m, PlaylistChanged(eveningOf(2)))
	if len(m.Playlist().Songs) != 2 || m.Screen() != ScreenPlaylist {
		t.Errorf("the screen shows %d tracks where the server holds 2", len(m.Playlist().Songs))
	}
}

func TestTheAnswerToAnOpenThatWasLeftDoesNotComeBackLater(t *testing.T) {
	a := library()[0]
	m, first := opened(t, loaded(t))
	m, _ = press(t, m, "h")
	m, second := opened(t, onArtists(t, m))
	m = send(t, m, Opened{Ask: second, Loaded: ArtistLoaded(a)})
	m, album := opened(t, m)
	m = send(t, m, Opened{Ask: album, Loaded: AlbumLoaded(a.Albums[0])}, Opened{Ask: first, Loaded: ArtistLoaded(a)})
	if m.Screen() != ScreenTracks {
		t.Errorf("the first answer pulled the screen back to %s", m.Screen())
	}
}

func TestTheFailureOfAnOpenThatWasLeftIsKeptToReadAndChangesNothing(t *testing.T) {
	m, first := opened(t, loaded(t))
	m, _ = press(t, m, "j")
	m, second := opened(t, m)

	m = send(t, m, Failed{Message: "the first did not arrive", Ask: first})
	if !m.loading() || m.statusFailed {
		t.Errorf("the failure of the open left behind stopped the one waited for: loading %v", m.loading())
	}
	if len(m.said) == 0 || m.said[len(m.said)-1].text != "the first did not arrive" {
		t.Error("the failure was not kept to be read")
	}
	m = send(t, m, Opened{Ask: second, Loaded: ArtistLoaded(library()[1])})
	if m.Screen() != ScreenAlbums {
		t.Errorf("the open waited for was not shown; the screen is %s", m.Screen())
	}
}

func TestAFailureAboutSomethingElseDoesNotLetAnAbandonedArtistTakeTheScreen(t *testing.T) {
	m, ask := opened(t, loaded(t))
	m = send(t, m, Failed{Message: "nothing is playing to seek in"})
	m, _ = press(t, m, "h")

	m = send(t, m, Opened{Ask: ask, Loaded: ArtistLoaded(library()[0])})
	if m.Screen() != ScreenLibrary {
		t.Errorf("the artist walked away from took the screen to %s", m.Screen())
	}
}

func TestAnArtistAskedForTwiceIsNotShownAgainAfterGoingBack(t *testing.T) {
	m, first := opened(t, loaded(t))
	m, second := opened(t, m)
	m = send(t, m, Opened{Ask: first, Loaded: ArtistLoaded(library()[0])})
	if m.Screen() != ScreenArtists {
		t.Fatalf("the answer to the first of two opens was shown")
	}
	m = send(t, m, Opened{Ask: second, Loaded: ArtistLoaded(library()[0])})
	m, _ = press(t, m, "h")

	m = send(t, m, Opened{Ask: first, Loaded: ArtistLoaded(library()[0])},
		Opened{Ask: second, Loaded: ArtistLoaded(library()[0])})
	if m.Screen() != ScreenArtists {
		t.Errorf("an answer already used went to %s after back was pressed", m.Screen())
	}
}

func TestASearchOrAnEditArrivingAbandonsWhatWasBeingOpened(t *testing.T) {
	for name, arrives := range map[string]tea.Msg{
		"search": SearchLoaded{Query: "water", Results: results()},
		"edit":   EditingPlaylist(eveningOf(3)),
	} {
		m := onStarred(t, send(t, loaded(t), starredChanged()))
		m, ask := opened(t, m)
		m = send(t, m, arrives)
		was := m.Screen()
		if was == ScreenStarred {
			t.Fatalf("%s: the screen did not change", name)
		}

		m = send(t, m, Opened{Ask: ask, Loaded: ArtistLoaded(library()[0])})
		if m.Screen() != was {
			t.Errorf("%s: the artist asked for before it took the screen to %s", name, m.Screen())
		}
	}
}

func TestBackUnderTheVisualiserLeavesItBeforeItDiscardsAnArrangement(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")
	m = send(t, m, ShowVisualiser{})

	m, _ = press(t, m, "esc")
	if m.ShowingSky() {
		t.Fatal("esc left the visualiser up")
	}
	if !m.Reordering() || m.order[1].ID != "tr-1" {
		t.Error("esc discarded the arrangement instead of leaving the visualiser")
	}
}

func TestAJumpBegunBeforeACoverArrivedDoesNotMoveTheScreenUnderIt(t *testing.T) {
	m, _ := press(t, loaded(t), "g")
	m = send(t, m, FullArt{"a picture filling the screen"})

	m, _ = press(t, m, "q")
	if m.Screen() != ScreenArtists {
		t.Errorf("gq under a cover went to %s", m.Screen())
	}
	if m.Pending() != "" {
		t.Error("the jump is still pending")
	}
}

func TestACountMayBeTypedUnderACoverBecauseAPercentageSeeks(t *testing.T) {
	m := send(t, loaded(t), FullArt{"a picture filling the screen"})
	m, _ = press(t, m, "5")
	m, _ = press(t, m, "0")
	if !m.ShowingArt() || m.Count() != "50" {
		t.Fatalf("digits under the cover: showing %v, count %q", m.ShowingArt(), m.Count())
	}
	if _, msg := press(t, m, "%"); msg != SeekToPercent(50) {
		t.Errorf("50%% under the cover emitted %#v", msg)
	}
}

func TestSeekingVolumeAndTheCommandLineWorkUnderACover(t *testing.T) {
	covered := send(t, loaded(t), FullArt{"a picture filling the screen"})
	for key, want := range map[string]tea.Msg{
		"]": SeekBy{By: 10e9}, "[": SeekBy{By: -10e9},
		"+": VolumeBy{Delta: 5}, "-": VolumeBy{Delta: -5},
	} {
		m, msg := press(t, covered, key)
		if msg != want || !m.ShowingArt() {
			t.Errorf("%q under the cover emitted %#v, cover showing %v", key, msg, m.ShowingArt())
		}
	}
	if m, _ := press(t, covered, ":"); !m.Commanding() || !m.ShowingArt() {
		t.Error("the command line does not open under the cover")
	}
}

func TestTheKeysThatWorkUnderTheVisualiserFollowWhatTheyAreBoundTo(t *testing.T) {
	keys, wrong := Bind(map[string][]string{"pause": {"x"}, "move-down": {" "}})
	if len(wrong) > 0 {
		t.Fatal(wrong)
	}
	m := send(t, loaded(t).WithKeys(keys), ShowVisualiser{})

	if _, msg := press(t, m, "x"); msg != (TogglePause{}) {
		t.Errorf("the key bound to pause emitted %#v under the visualiser", msg)
	}
	next, msg := press(t, m, " ")
	if msg != nil || next.cursor[ScreenArtists] != 0 {
		t.Errorf("the key bound to move-down acted under the visualiser: %#v", msg)
	}
}

func TestAFilterBeingTypedEndsWhenTheScreenChanges(t *testing.T) {
	m, _ := press(t, loaded(t), "/")
	m = typed(t, m, "ao")

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenAlbums {
		t.Fatalf("the artist did not open; the screen is %s", m.Screen())
	}
	if m.Filtering() || m.Filter() != "" {
		t.Errorf("the filter followed to the new screen: typing %v, %q", m.Filtering(), m.Filter())
	}
}

func TestStartingAndAbandoningAFilterOnASearchLeavesTheCursorOffTheHeadings(t *testing.T) {
	m, _ := press(t, searched(t), "/")
	if !m.found[m.Cursor()].selectable() {
		t.Fatal("starting a filter put the cursor on a heading")
	}
	m = typed(t, m, "w")
	m, _ = press(t, m, "backspace")
	if !m.found[m.Cursor()].selectable() {
		t.Error("emptying the filter put the cursor on a heading")
	}
	m, _ = press(t, m, "esc")
	if !m.found[m.Cursor()].selectable() {
		t.Error("abandoning the filter put the cursor on a heading")
	}
}

func TestAWikiPageThatReflowsToFewerRowsLeavesTheCursorOnOne(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	m := send(t, next.(Model), ShowWiki{})
	for commands[m.Cursor()].name != "search" {
		m, _ = press(t, m, "j")
	}
	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "G")
	narrow := m.rows()

	m = send(t, m, tea.WindowSizeMsg{Width: 200, Height: 12})
	if m.rows() >= narrow {
		t.Fatalf("the page did not reflow to fewer rows: %d then %d", narrow, m.rows())
	}
	if m.Cursor() >= m.rows() {
		t.Fatalf("the cursor is on row %d of %d", m.Cursor(), m.rows())
	}
	_, _ = press(t, m, "*")
	_, _ = press(t, m, "enter")
	_ = m.View()
}

func TestLeavingAGroupedListWithAFilterThatMatchesNothingLeavesTheCursorOffAHeading(t *testing.T) {
	m := onStarred(t, send(t, loaded(t), starredChanged()))
	m, _ = press(t, m, "j")
	m, ask := opened(t, m)
	m, _ = press(t, m, "/")
	m = typed(t, m, "bzzz")
	if m.rows() != 0 {
		t.Fatalf("the filter still matches %d rows", m.rows())
	}

	m = send(t, m, AlbumLoaded(library()[0].Albums[0]))
	m, _ = press(t, m, "h")
	if m.Screen() != ScreenStarred || !m.starredRows[m.Cursor()].selectable() {
		t.Errorf("back on %s the cursor is on row %d, a heading", m.Screen(), m.Cursor())
	}
	_ = ask
}

func TestAScreenThatHasItsListIsNotDrawnAsLoadingBecauseAnotherIsStillArriving(t *testing.T) {
	m, _ := press(t, loaded(t), "g")
	m, _ = press(t, m, "p")
	if !m.loading() {
		t.Fatal("playlists not fetched yet are not drawn as loading")
	}
	m, _ = press(t, m, "esc")
	m = onQueue(t, m)
	if m.loading() || strings.Contains(m.View(), "loading") {
		t.Errorf("the queue says it is loading while the playlists arrive:\n%s", m.View())
	}

	m = onPlaylists(t, m)
	if !m.loading() {
		t.Error("going back to playlists that have not arrived does not say they are loading")
	}
}

func TestThePlaylistCommandWithNoNameShowsTheListAndAsksForIt(t *testing.T) {
	m, msg := typeCommand(t, loaded(t), "playlist")
	if msg != (FetchPlaylists{}) {
		t.Errorf(":playlist emitted %#v, want FetchPlaylists", msg)
	}
	if m.Screen() != ScreenPlaylists {
		t.Errorf(":playlist went to %s", m.Screen())
	}
}

func TestBackUnderACoverLeavesItBeforeItDiscardsAnArrangement(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")
	m = send(t, m, FullArt{"a picture filling the screen"})

	m, _ = press(t, m, "esc")
	if m.ShowingArt() {
		t.Fatal("esc left the cover up")
	}
	if !m.Reordering() || m.order[1].ID != "tr-1" {
		t.Error("esc discarded the arrangement instead of taking the cover away")
	}
}

// cursorsWithinTheirLists reports the first screen whose cursor is past the
// end of its list or on a heading. Every screen is checked, not only the one
// showing.
func cursorsWithinTheirLists(m Model) error {
	for _, s := range Screens {
		on := m
		if s != m.screen {
			on.screen, on.filter = s, ""
		}
		rows, at := on.rows(), on.cursor[s]
		if at < 0 || (rows > 0 && at >= rows) {
			return fmt.Errorf("the %s cursor is on row %d of %d", s, at, rows)
		}
		if on.isGrouped() && rows > 0 && !on.matchedResults()[at].selectable() {
			return fmt.Errorf("the %s cursor is on a heading", s)
		}
	}
	return nil
}

func TestAPlaylistBeingAddedToCanBeOpened(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}
	m := onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists)))
	m, ask := opened(t, m)
	m = send(t, m, Opened{Ask: ask, Loaded: PlaylistLoaded(eveningOf(3))})
	m, _ = press(t, m, "e")
	if m.Editing().ID != "pl-1" {
		t.Fatal("e did not start adding to the playlist on screen")
	}

	m, ask = opened(t, onPlaylists(t, m))
	m = send(t, m, Opened{Ask: ask, Loaded: PlaylistLoaded(eveningOf(3))})
	if m.Screen() != ScreenPlaylist {
		t.Fatalf("the playlist being added to did not open; the screen is %s", m.Screen())
	}
	if strings.Contains(m.Status(), "opening") {
		t.Errorf("the last row still says %q", m.Status())
	}
}

func TestAFailureThatIsNotAboutTheOpenDoesNotCancelIt(t *testing.T) {
	m, ask := opened(t, loaded(t))

	m = send(t, m, Failed{Message: "nothing is playing to seek in"})
	if !m.loading() {
		t.Error("the artist is no longer drawn as arriving")
	}
	m, _ = press(t, m, "j")
	if !strings.Contains(m.Status(), "opening Aoi") {
		t.Errorf("after the failure was read the last row says %q", m.Status())
	}

	m = send(t, m, Opened{Ask: ask, Loaded: ArtistLoaded(library()[0])})
	if m.Screen() != ScreenAlbums {
		t.Errorf("the artist was dropped; the screen is %s", m.Screen())
	}
}

func TestAListOrANoticeArrivingDoesNotHideWhatIsBeingOpened(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}
	for name, arrives := range map[string]tea.Msg{
		"playlists":   PlaylistsLoaded(lists),
		"artists":     ArtistsLoaded(library()),
		"every album": AlbumsLoaded(everyAlbum()),
		"starred":     starredChanged(),
		"a reload":    Reloaded{Loaded: ArtistsLoaded(library())},
	} {
		m := onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists)))
		m, ask := opened(t, m)

		m = send(t, m, arrives)
		if !m.loading() || !strings.Contains(m.Status(), "opening Evening") {
			t.Errorf("%s arriving left loading %v and %q", name, m.loading(), m.Status())
		}
		m = send(t, m, Opened{Ask: ask, Loaded: PlaylistLoaded(eveningOf(3))})
		if m.Screen() != ScreenPlaylist || m.loading() {
			t.Errorf("%s: the playlist did not open cleanly: %s, loading %v", name, m.Screen(), m.loading())
		}
	}

	m := onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists)))
	m, _ = opened(t, m)
	m = send(t, m, Notice("playing Ballast next"))
	m, _ = press(t, m, "j")
	if !m.loading() || !strings.Contains(m.Status(), "opening Evening") {
		t.Errorf("after a notice was read: loading %v, %q", m.loading(), m.Status())
	}
}
