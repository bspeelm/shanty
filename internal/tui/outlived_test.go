package tui

import (
	"fmt"
	"reflect"
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

	m = send(t, m, Reload{})
	m = send(t, m, Reloaded{Ask: m.Reloading(), Loaded: ArtistsLoaded(library())})
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
	m = send(t, m, Reload{})
	m = send(t, m, Reloaded{Ask: m.Reloading(), Loaded: AlbumLoaded(album)})
	if m.Screen() != ScreenTracks || m.Cursor() != 1 || !strings.Contains(m.Status(), "reloaded") {
		t.Errorf("screen %s, cursor %d, said %q", m.Screen(), m.Cursor(), m.Status())
	}
}

func TestAReloadAskedForTwiceKeepsTheSelectionThroughBothAnswers(t *testing.T) {
	artist, album := library()[0], library()[0].Albums[0]
	m := send(t, loaded(t), ArtistLoaded(artist), AlbumLoaded(album))
	m, _ = press(t, m, "j")
	m = send(t, m, Reload{})
	first := m.Reloading()
	m = send(t, m, Reload{})
	m = send(t, m, Reloaded{Ask: first, Loaded: AlbumLoaded(album)})
	if !m.loading() || strings.Contains(m.Status(), "reloaded") {
		t.Fatalf("the answer to the first reload ended the wait for the second: loading %v, %q", m.loading(), m.Status())
	}
	m = send(t, m, Reloaded{Ask: m.Reloading(), Loaded: AlbumLoaded(album)})
	if m.Screen() != ScreenTracks || m.Cursor() != 1 || m.loading() {
		t.Errorf("tracks: screen %s, cursor %d, loading %v", m.Screen(), m.Cursor(), m.loading())
	}

	m, _ = press(t, m, "h")
	m, _ = press(t, m, "j")
	m = send(t, m, Reload{})
	first = m.Reloading()
	m = send(t, m, Reload{})
	m = send(t, m, Reloaded{Ask: first, Loaded: ArtistLoaded(artist)}, Reloaded{Ask: m.Reloading(), Loaded: ArtistLoaded(artist)})
	if m.Screen() != ScreenAlbums || m.Cursor() != 1 {
		t.Errorf("albums: screen %s, cursor %d", m.Screen(), m.Cursor())
	}

	m = send(t, onArtists(t, m), keyOf("j"), Reload{})
	first = m.Reloading()
	m = send(t, m, Reload{})
	m = send(t, m, Reloaded{Ask: first, Loaded: ArtistsLoaded(library())}, Reloaded{Ask: m.Reloading(), Loaded: ArtistsLoaded(library())})
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
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening", SongCount: 3}}
	m := send(t, onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists))), PlaylistLoaded(eveningOf(3)))
	m, _ = press(t, m, "end")
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
	m, _ = press(t, m, "r")
	m, removed := press(t, m, "y")
	if _, ok := removed.(EditPlaylist); !ok {
		t.Fatalf("r and then y emitted %#v", removed)
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
	m, _ = press(t, m, "y")

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
		m, msg := press(t, m, "enter")
		open, ok := msg.(OpenArtist)
		if !ok || open.ID != "ar-1" {
			t.Fatalf("%s: enter on the starred artist emitted %#v", name, msg)
		}
		ask := open.Ask
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
	m := onCommand(t, send(t, next.(Model), ShowWiki{}), "search")
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

	m = send(t, m, Opened{Ask: ask, Loaded: AlbumLoaded(library()[0].Albums[0])})
	if err := cursorsWithinTheirLists(m); err != nil {
		t.Errorf("with the album open: %v", err)
	}
	// Back shows the list narrowed as it was left, and the next back widens it.
	m, _ = press(t, m, "h")
	m, _ = press(t, m, "h")
	if m.Screen() != ScreenStarred || m.Filter() != "" || !m.starredRows[m.Cursor()].selectable() {
		t.Errorf("back on %s with the filter %q the cursor is on row %d", m.Screen(), m.Filter(), m.Cursor())
	}
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

// cursorsWithinTheirLists reports the first screen whose selection is past
// the end of its list, or is on something its filter hides or on a heading
// while the filter shows a row that could be selected. Every screen is
// checked, not only the one showing.
func cursorsWithinTheirLists(m Model) error {
	for _, s := range Screens {
		on := m
		if s != m.screen {
			on.screen, on.filter = s, m.kept[s]
		}
		all, at := on.allRows(), on.cursor[s]
		if at < 0 || (all > 0 && at >= all) {
			return fmt.Errorf("the %s selection is place %d of %d", s, at, all)
		}
		rows := on.matches()
		if len(rows) == 0 {
			continue
		}
		if on.at() != at {
			return fmt.Errorf("the %s selection is place %d, which the filter %q hides", s, at, on.filter)
		}
		if on.isGrouped() && !on.grouped()[at].selectable() {
			return fmt.Errorf("the %s selection is on a heading", s)
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

func TestAnOrderJustSavedIsNotReorderedAgainUntilItIsAnswered(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")
	m, saved := press(t, m, "enter")
	order, ok := saved.(ReorderPlaylist)
	if !ok {
		t.Fatalf("enter emitted %#v", saved)
	}

	m, _ = typeCommand(t, m, "playlist reorder")
	if m.Reordering() || !m.statusFailed {
		t.Fatalf("a second reorder began before the first was answered: %q", m.Status())
	}

	now := eveningOf(3)
	now.Songs = []subsonic.Song{now.Songs[1], now.Songs[0], now.Songs[2]}
	if !same(idsOf(now.Songs), order.Songs) {
		t.Fatalf("the fixture is not the order that was saved: %v", order.Songs)
	}
	m = send(t, m, Saved{Ask: order.Ask, Playlist: now})
	if strings.Contains(m.Status(), "changed on the server") {
		t.Errorf("the order just saved was reported as someone else's change: %q", m.Status())
	}
	if m = reordering(t, m); m.order[0].ID != "tr-2" {
		t.Errorf("the next arrangement starts from %v, want the order that was saved", idsOf(m.order))
	}
}

func TestTheQueueIsRefusedInTheSameWordsWhereverItIsNoticed(t *testing.T) {
	for _, tc := range []struct {
		tracks, at int
		wants      string
	}{
		{0, 0, "the queue is empty"},
		{3, 3, "nothing is playing"},
		{3, 2, "fewer than two tracks follow"},
		{3, 1, "fewer than two tracks follow"},
		{3, 0, ""},
	} {
		why := QueueRefusal(tc.tracks, tc.at)
		if (why == "") != (tc.wants == "") || !strings.Contains(why, tc.wants) {
			t.Errorf("%d tracks at %d: %q, want %q", tc.tracks, tc.at, why, tc.wants)
		}
		m := onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued()[:tc.tracks], At: tc.at}))
		if m, _ = press(t, m, "S"); m.status != why {
			t.Errorf("%d tracks at %d: the interface says %q and the function %q", tc.tracks, tc.at, m.status, why)
		}
	}
}

func TestOnlyItsOwnAnswerEndsTheWaitForAReload(t *testing.T) {
	m, _ := press(t, loaded(t), "j")
	m, msg := typeCommand(t, m, "reload")
	asked, ok := msg.(Reload)
	if !ok || asked.Ask == 0 || asked.Ask != m.Reloading() {
		t.Fatalf(":reload emitted %#v with reload %d waited for", msg, m.Reloading())
	}

	m = send(t, m, Failed{Message: "nothing is playing to seek in"})
	m, _ = press(t, m, "5")
	if !m.loading() || !strings.Contains(m.Status(), "asking the server again") {
		t.Fatalf("a failure about something else ended the reload: loading %v, %q", m.loading(), m.Status())
	}
	m = send(t, m, Reloaded{Ask: m.Reloading(), Loaded: ArtistsLoaded(library())})
	if m.loading() || !strings.Contains(m.Status(), "reloaded: 3 artists") || m.Cursor() != 1 {
		t.Errorf("the reload's answer left loading %v, %q, row %d", m.loading(), m.Status(), m.Cursor())
	}

	m, msg = typeCommand(t, m, "reload")
	m = send(t, m, Failed{Message: "the server did not answer", Ask: msg.(Reload).Ask})
	if m.loading() || !m.statusFailed {
		t.Errorf("the reload's own failure left loading %v, %q", m.loading(), m.Status())
	}
}

func TestOnlyItsOwnAnswerLetsAnOrderJustSavedBeReorderedAgain(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening", SongCount: 3}}
	open := send(t, onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists))), PlaylistLoaded(eveningOf(3)))
	m := reordering(t, open)
	m, _ = press(t, m, "J")
	m, saved := press(t, m, "enter")
	order := saved.(ReorderPlaylist)
	refused := func(m Model) bool {
		m, _ = typeCommand(t, m, "playlist reorder")
		return !m.Reordering() && m.statusFailed
	}

	if !refused(send(t, m, Failed{Message: "nothing is playing to seek in"})) {
		t.Error("a failure about something else let the order be reordered before it was answered")
	}
	left, _ := press(t, m, "h")
	left = send(t, left, PlaylistLoaded(eveningOf(3)))
	if left.Screen() != ScreenPlaylist || !refused(left) {
		t.Error("leaving the playlist and opening it again let the order be reordered before it was answered")
	}
	if refused(send(t, m, Failed{Message: "it changed on the server", Ask: order.Ask})) {
		t.Error("the save's own failure did not end the wait")
	}
	renamed := eveningOf(3)
	renamed.Name = "Dusk"
	if !refused(send(t, m, PlaylistChanged(renamed))) {
		t.Error("the playlist renamed from elsewhere let the order be reordered before it was answered")
	}
	if !refused(send(t, m, Saved{Ask: order.Ask + 1, Playlist: eveningOf(3)})) {
		t.Error("the answer to another save ended the wait")
	}
	if refused(send(t, left, Saved{Ask: order.Ask, Playlist: eveningOf(3)})) {
		t.Error("the save's answer did not end the wait")
	}
}

func TestGoingBackToAnAlbumWhoseCoverIsNotHeldAsksForIt(t *testing.T) {
	x := subsonic.Album{ID: "al-x", Name: "X", CoverArt: "cv-x", Songs: tenTracks().Songs}
	y := subsonic.Album{ID: "al-y", Name: "Y", CoverArt: "cv-y", Songs: tenTracks().Songs}
	picture := []string{"a picture", "of a cover"}
	next, _ := New().Update(tea.WindowSizeMsg{Width: 64, Height: 40})

	// The cover of X arrives after Y was opened, and is dropped.
	m := send(t, onArtists(t, send(t, next.(Model), ArtistsLoaded(library()))), AlbumLoaded(x))
	m = send(t, onArtists(t, m), AlbumLoaded(y), CoverArt{AlbumID: "al-x", Lines: picture})
	for range 2 {
		var msg tea.Msg
		m, msg = press(t, m, "esc")
		if m.Screen() == ScreenTracks {
			if want := (FetchCover{Album: x}); !reflect.DeepEqual(msg, want) || m.Album().ID != "al-x" {
				t.Fatalf("back to %q emitted %#v, want its cover asked for", m.Album().Name, msg)
			}
		}
	}
	if m.Screen() != ScreenTracks {
		t.Fatalf("two backs reached %s", m.Screen())
	}
	m = send(t, m, CoverArt{AlbumID: "al-x", Lines: picture})
	if !strings.Contains(m.View(), "a picture") {
		t.Errorf("the cover that was asked for is not drawn:\n%s", m.View())
	}

	// A cover that is held comes back with its album and is not asked for.
	m = send(t, onArtists(t, m), AlbumLoaded(y))
	m, _ = press(t, m, "esc")
	m, msg := press(t, m, "esc")
	if msg != nil || m.Album().ID != "al-x" || !strings.Contains(m.View(), "a picture") {
		t.Errorf("back to an album whose cover is held emitted %#v:\n%s", msg, m.View())
	}
}

func TestWhileAddingToAPlaylistNeitherQueueKeyActsOnARowThatIsNotATrack(t *testing.T) {
	m := send(t, loaded(t), EditingPlaylist(eveningOf(1)))
	for _, key := range []string{"a", "A", "r"} {
		said, msg := press(t, m, key)
		if msg != nil || !strings.Contains(said.Status(), "open an album") {
			t.Errorf("%s on an artist while adding to a playlist emitted %#v and said %q", key, msg, said.Status())
		}
	}
}

func TestAddingAWholeArtistSaysSoWhileTheirTracksAreGathered(t *testing.T) {
	m, msg := press(t, loaded(t), "a")
	if _, ok := msg.(QueueThis); !ok || !strings.Contains(m.Status(), "gathering every track of Aoi") {
		t.Errorf("a on an artist emitted %#v and said %q", msg, m.Status())
	}
}

func TestTheQueueKeyOnAnEmptyQueueDoesNotSayATrackIsAlreadyInIt(t *testing.T) {
	m, msg := press(t, onQueue(t, loaded(t)), "a")
	if msg != nil || strings.Contains(m.Status(), "already") || m.Status() == "" {
		t.Errorf("a on an empty queue emitted %#v and said %q", msg, m.Status())
	}
}

func TestTheAnswerToAnEarlierReloadDoesNotEndTheWaitForALaterOne(t *testing.T) {
	m, _ := press(t, loaded(t), "j")
	m = send(t, m, Reload{})
	first := m.Reloading()
	m = send(t, m, Reload{})

	grown := append([]subsonic.Artist{{ID: "ar-0", Name: "A New Arrival"}}, library()...)
	m = send(t, m, Reloaded{Ask: first, Loaded: ArtistsLoaded(grown)})
	if !m.loading() || strings.Contains(m.Status(), "reloaded") || len(m.artists) != 4 {
		t.Fatalf("the first answer: loading %v, %q, %d artists held", m.loading(), m.Status(), len(m.artists))
	}
	m = send(t, m, Failed{Message: "the first reload failed late", Ask: first})
	if !m.loading() || m.statusFailed {
		t.Fatalf("the failure of the first reload ended the wait for the second: loading %v, %q", m.loading(), m.Status())
	}
	m = send(t, m, Reloaded{Ask: m.Reloading(), Loaded: ArtistsLoaded(grown)})
	if m.loading() || !strings.Contains(m.Status(), "reloaded: 4 artists") {
		t.Errorf("the second answer: loading %v, %q", m.loading(), m.Status())
	}
}

func TestAReloadCarriesTheScreenAndWhatItShows(t *testing.T) {
	aoi := library()[0]
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}
	for name, tc := range map[string]struct {
		m    Model
		want Reload
	}{
		"an artist's albums": {send(t, loaded(t), ArtistLoaded(aoi)), Reload{Screen: ScreenAlbums, ID: "ar-1"}},
		"an album's tracks":  {deepIn(t), Reload{Screen: ScreenTracks, ID: "al-1"}},
		"the playlists":      {onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists))), Reload{Screen: ScreenPlaylists}},
		"a playlist":         {evening3(t), Reload{Screen: ScreenPlaylist, ID: "pl-1"}},
		"the library":        {sized(New()), Reload{Screen: ScreenLibrary}},
	} {
		m, msg := typeCommand(t, tc.m, "reload")
		tc.want.Ask = m.Reloading()
		if msg != tc.want || tc.want.Ask == 0 {
			t.Errorf("%s: :reload emitted %#v, want %#v", name, msg, tc.want)
		}
	}
}

func TestReloadingThePlaylistsAndAPlaylistKeepsTheSelection(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}
	m, _ := press(t, onPlaylists(t, send(t, loaded(t), PlaylistsLoaded(lists))), "j")
	m = send(t, m, Reload{})
	more := append([]subsonic.Playlist{{ID: "pl-0", Name: "Dawn"}}, lists...)
	m = send(t, m, Reloaded{Ask: m.Reloading(), Loaded: PlaylistsLoaded(more)})
	if m.loading() || m.under().id != "pl-2" || !strings.Contains(m.Status(), "reloaded: 3 playlists") {
		t.Errorf("the playlists: loading %v, on %q, %q", m.loading(), m.under().id, m.Status())
	}

	m, _ = press(t, evening3(t), "j")
	m = send(t, m, Reload{})
	longer := eveningOf(3)
	longer.Songs = append([]subsonic.Song{{ID: "tr-0", Title: "Dawn"}}, longer.Songs...)
	m = send(t, m, Reloaded{Ask: m.Reloading(), Loaded: PlaylistLoaded(longer)})
	if m.Screen() != ScreenPlaylist || m.under().id != "tr-2" || !strings.Contains(m.Status(), "reloaded: 4 tracks") {
		t.Errorf("a playlist: on %s at %q, %q", m.Screen(), m.under().id, m.Status())
	}
}

func TestALibraryReloadWhoseSecondListFailsShowsTheFailure(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(library()), Reload{})
	m = send(t, m, Failed{Message: "the albums could not be read", Ask: m.Reloading()})
	if m.loading() || !m.statusFailed || !strings.Contains(m.Status(), "albums could not be read") {
		t.Errorf("a reload that failed: loading %v, failed %v, %q", m.loading(), m.statusFailed, m.Status())
	}

	m = send(t, sized(New()), ArtistsLoaded(library()), Reload{})
	m = send(t, m, Reloaded{Ask: m.Reloading(), More: AlbumsLoaded(everyAlbum()), Loaded: ArtistsLoaded(library())})
	if m.loading() || len(m.albums) != 3 || !strings.Contains(m.Status(), "reloaded: 3 artists") {
		t.Errorf("a reload of both lists: loading %v, %d albums, %q", m.loading(), len(m.albums), m.Status())
	}
}

// TestBackToAnAlbumOpenedAgainBeforeItsCoverCameAsksForTheCover is an album
// whose cover was held, left, and opened again: the cover is not held the
// second time until it arrives, whatever was held the first time.
func TestBackToAnAlbumOpenedAgainBeforeItsCoverCameAsksForTheCover(t *testing.T) {
	a := subsonic.Album{ID: "al-a", Name: "A", CoverArt: "cv-a", Songs: tenTracks().Songs}
	b := subsonic.Album{ID: "al-b", Name: "B", CoverArt: "cv-b", Songs: tenTracks().Songs}
	picture := []string{"a picture", "of a cover"}
	next, _ := New().Update(tea.WindowSizeMsg{Width: 64, Height: 40})
	m := onArtists(t, send(t, next.(Model), ArtistsLoaded(library()), AlbumsLoaded([]subsonic.Album{a, b})))

	// A with its cover, back, B, back, A again, and away before A's cover.
	m = send(t, m, keyOf("g"), keyOf("b"), AlbumLoaded(a), CoverArt{AlbumID: "al-a", Lines: picture}, keyOf("esc"))
	m = send(t, m, AlbumLoaded(b), keyOf("esc"), AlbumLoaded(a))
	m = send(t, onQueue(t, m), keyOf("g"), keyOf("b"), AlbumLoaded(b), CoverArt{AlbumID: "al-a", Lines: picture})

	var asked tea.Msg
	for range 3 {
		var msg tea.Msg
		m, msg = press(t, m, "esc")
		if m.Screen() == ScreenTracks && m.Album().ID == "al-a" {
			asked = msg
			break
		}
	}
	if m.Album().ID != "al-a" || !reflect.DeepEqual(asked, FetchCover{Album: a}) {
		t.Errorf("back to %q emitted %#v, want its cover asked for", m.Album().Name, asked)
	}
}
