package tui

import (
	"fmt"
	"math/rand/v2"
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

	m = send(t, m, PlaylistLoaded(eveningOf(2)))
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

func TestAPlaylistThatComesBackTheSameKeepsTheArrangementBeingMade(t *testing.T) {
	m := reordering(t, evening3(t))
	m, _ = press(t, m, "J")

	m = send(t, m, PlaylistLoaded(eveningOf(3)))
	if !m.Reordering() || m.order[1].ID != "tr-1" {
		t.Errorf("an unchanged playlist ended the reorder or lost the move: %v, %v", m.Reordering(), idsOf(m.order))
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
			return send(t, m, ShowEveryAlbum{}, AlbumsLoaded(everyAlbum()))
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
		}, PlaylistLoaded(eveningOf(1))},
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
	m := send(t, loaded(t), ShowEveryAlbum{}, AlbumsLoaded(everyAlbum()))
	m, _ = press(t, m, "end")
	m = send(t, m, AlbumsLoaded(everyAlbum()[:1]))
	if m.Cursor() != 0 {
		t.Fatalf("the cursor is on row %d of a list of one", m.Cursor())
	}
	if _, msg := press(t, m, "enter"); msg != (OpenAlbum{ID: "al-1"}) {
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

func TestAReturningJumpCannotUndoAKeyPressedSince(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}
	m := send(t, loaded(t), PlaylistsLoaded(lists))

	m, _ = press(t, m, "g")
	m, asked := press(t, m, "p")
	m, msg := press(t, m, "enter")
	if _, ok := msg.(OpenPlaylist); !ok {
		t.Fatalf("enter emitted %T", msg)
	}
	// The interface is handed nothing back for the jump, so the playlist that
	// was opened is shown when it arrives.
	if asked == nil {
		t.Fatal("gp asked for nothing")
	}
	m = send(t, m, PlaylistLoaded(eveningOf(3)))
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

	m = send(t, m, Reload{}, ArtistsLoaded(library()))
	if m.Screen() != ScreenLibrary || m.loading {
		t.Fatalf("the library is on %s, loading %v", m.Screen(), m.loading)
	}
	if !strings.Contains(m.Status(), "reloaded") {
		t.Errorf("the reload did not say it finished: %q", m.Status())
	}
	if m.Cursor() != len(libraryRows)-1 {
		t.Errorf("the library cursor moved to %d", m.Cursor())
	}
}

func TestLeavingAScreenDropsTheReloadItWasWaitingFor(t *testing.T) {
	m := send(t, loaded(t), ShowEveryAlbum{}, AlbumsLoaded(everyAlbum()), Reload{})
	m, _ = press(t, m, "esc")
	if m.loading || m.keep != "" {
		t.Fatalf("the library is still waiting for a reload: loading %v, keeping %q", m.loading, m.keep)
	}
	m = send(t, onArtists(t, m), ArtistsLoaded(library()), AlbumsLoaded(everyAlbum()))
	if strings.Contains(m.Status(), "reloaded") {
		t.Errorf("a list that arrived on another screen was reported as the reload: %q", m.Status())
	}
}

func TestAReloadAnsweredAfterItsScreenWasLeftDoesNotTakeTheScreen(t *testing.T) {
	artist, album := library()[0], library()[0].Albums[0]

	m := send(t, loaded(t), ArtistLoaded(artist), AlbumLoaded(album), Reload{})
	m, _ = press(t, m, "h")
	m = send(t, m, AlbumLoaded(album))
	if m.Screen() != ScreenAlbums || m.loading {
		t.Errorf("an album reloaded and then left: screen %s, loading %v", m.Screen(), m.loading)
	}

	m = send(t, m, Reload{})
	m, _ = press(t, m, "h")
	m = send(t, m, ArtistLoaded(artist))
	if m.Screen() != ScreenArtists || m.loading {
		t.Errorf("an artist reloaded and then left: screen %s, loading %v", m.Screen(), m.loading)
	}

	m = send(t, onStarred(t, send(t, m, starredChanged())), Reload{})
	m, _ = press(t, m, "h")
	m = send(t, m, starredChanged())
	if m.loading || strings.Contains(m.Status(), "reloaded") {
		t.Errorf("a starred list reloaded and then left: loading %v, said %q", m.loading, m.Status())
	}
}

func TestAReloadAnsweredOnItsScreenStillKeepsTheSelection(t *testing.T) {
	artist, album := library()[0], library()[0].Albums[0]
	m := send(t, loaded(t), ArtistLoaded(artist), AlbumLoaded(album))
	m, _ = press(t, m, "j")
	m = send(t, m, Reload{}, AlbumLoaded(album))
	if m.Screen() != ScreenTracks || m.Cursor() != 1 || !strings.Contains(m.Status(), "reloaded") {
		t.Errorf("screen %s, cursor %d, said %q", m.Screen(), m.Cursor(), m.Status())
	}
}

func TestAPlaylistRefreshedAfterItsScreenWasLeftIsKeptWithoutGoingBackToIt(t *testing.T) {
	m, _ := press(t, evening3(t), "end")
	m, _ = press(t, m, "h")
	if m.Screen() != ScreenPlaylists {
		t.Fatalf("back from the playlist went to %s", m.Screen())
	}

	m = send(t, m, PlaylistLoaded(eveningOf(2)))
	if m.Screen() != ScreenPlaylists {
		t.Fatalf("the answer to a change made before leaving went back to %s", m.Screen())
	}
	if len(m.Playlist().Songs) != 2 {
		t.Errorf("the playlist as the server now has it was not kept")
	}
}

func TestAFailureAboutSomethingElseDoesNotLetAnAbandonedArtistTakeTheScreen(t *testing.T) {
	m, _ := press(t, loaded(t), "enter")
	m = send(t, m, Failed{Message: "nothing is playing to seek in"})
	m, _ = press(t, m, "h")

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenLibrary {
		t.Errorf("the artist walked away from took the screen to %s", m.Screen())
	}
}

func TestAnArtistAskedForTwiceIsNotShownAgainAfterGoingBack(t *testing.T) {
	m, _ := press(t, loaded(t), "enter")
	m, _ = press(t, m, "enter")
	m = send(t, m, ArtistLoaded(library()[0]))
	m, _ = press(t, m, "h")

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenArtists {
		t.Errorf("the second answer went to %s after back was pressed", m.Screen())
	}
}

func TestTheRequestsRememberedAsAbandonedAreBounded(t *testing.T) {
	m := loaded(t)
	for i := range 50 {
		m = m.opening("opening", fmt.Sprintf("ar-%d", i))
		m = m.show(ScreenLibrary).show(ScreenArtists)
	}
	if len(m.strayed) > requestsKept || len(m.flying) > requestsKept {
		t.Errorf("%d abandoned and %d in flight are remembered, want at most %d of each",
			len(m.strayed), len(m.flying), requestsKept)
	}
}

func TestASearchOrAnEditArrivingAbandonsWhatWasBeingOpened(t *testing.T) {
	for name, arrives := range map[string]tea.Msg{
		"search": SearchLoaded{Query: "water", Results: results()},
		"edit":   EditingPlaylist(eveningOf(3)),
	} {
		m := onStarred(t, send(t, loaded(t), starredChanged()))
		m, msg := press(t, m, "enter")
		if msg != (OpenArtist{ID: "ar-1"}) {
			t.Fatalf("%s: enter on the starred artist emitted %#v", name, msg)
		}
		m = send(t, m, arrives)
		was := m.Screen()
		if was == ScreenStarred {
			t.Fatalf("%s: the screen did not change", name)
		}

		m = send(t, m, ArtistLoaded(library()[0]))
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

// randomMessage is a key or a message the program could plausibly receive.
func randomMessage(r *rand.Rand) tea.Msg {
	keys := []string{"j", "k", "g", "g", "l", "a", "b", "p", "q", "s", "v", "enter", "esc", "h",
		"end", "home", "pgup", "pgdown", "/", ":", "x", "w", "r", "e", "S", "*", "A", "J", "K", "?",
		"5", "%", " ", "n", "y", "backspace", "tab", "G"}
	artists := library()
	ids := []string{"ar-1", "ar-2", "al-1", "al-2", "pl-1", "pl-2", "zz"}
	id := ids[r.IntN(len(ids))]
	starred := starredChanged()
	starred.Results.Albums = starred.Results.Albums[:r.IntN(2)]
	starred.Results.Songs = starred.Results.Songs[:r.IntN(2)]
	found := results()
	found.Albums = found.Albums[:r.IntN(3)]
	found.Songs = found.Songs[:r.IntN(3)]
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}

	switch r.IntN(26) {
	case 0:
		return ArtistsLoaded(artists[:r.IntN(len(artists)+1)])
	case 1:
		a := subsonic.Artist{ID: id, Name: "Aoi", Albums: artists[0].Albums[:r.IntN(3)]}
		return ArtistLoaded(a)
	case 2:
		return AlbumsLoaded(everyAlbum()[:r.IntN(4)])
	case 3:
		a := tenTracks()
		a.ID, a.Songs = id, a.Songs[:r.IntN(len(a.Songs)+1)]
		return AlbumLoaded(a)
	case 4:
		return QueueChanged{Tracks: queued()[:r.IntN(4)], At: r.IntN(4)}
	case 5:
		return SearchLoaded{Query: "water", Results: found}
	case 6:
		return starred
	case 7:
		return PlaylistsLoaded(lists[:r.IntN(3)])
	case 8:
		p := eveningOf(r.IntN(4))
		p.ID = id
		return PlaylistLoaded(p)
	case 9:
		return Failed{Message: "the server did not answer"}
	case 10:
		return Notice("playing Ballast next")
	case 11:
		return Reload{}
	case 12:
		return FullArt{"a picture filling the screen"}
	case 13:
		return ShowVisualiser{}
	case 14:
		return ShowWiki{}
	case 15:
		return ShowMessages{}
	case 16:
		return EditingPlaylist(eveningOf(r.IntN(4)))
	case 17:
		return tea.WindowSizeMsg{Width: 20 + r.IntN(80), Height: 4 + r.IntN(30)}
	case 18:
		return Confirm{Question: "delete it?", Do: Notice("deleted")}
	case 19:
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	key := keys[r.IntN(len(keys))]
	if named, ok := namedKeys[key]; ok {
		return tea.KeyMsg{Type: named}
	}
	if key == " " {
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// TestNoRunOfKeysAndMessagesLeavesTheCursorOffItsList drives the interface with
// keys and messages drawn from a seeded generator. After each one the frame is
// drawn and the cursor of the screen showing is checked against its rows.
func TestNoRunOfKeysAndMessagesLeavesTheCursorOffItsList(t *testing.T) {
	for seed := uint64(1); seed <= 6; seed++ {
		r := rand.New(rand.NewPCG(seed, 7))
		m := sized(New())
		var last tea.Msg
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("seed %d: %v after %#v on the %s screen", seed, p, last, m.Screen())
				}
			}()
			for step := range 300 {
				last = randomMessage(r)
				next, _ := m.Update(last)
				m = next.(Model)
				if !m.ShowingSky() {
					_ = m.View()
				}
				rows, at := m.rows(), m.Cursor()
				if at < 0 || (rows > 0 && at >= rows) {
					t.Fatalf("seed %d, step %d: the cursor is on row %d of %d on the %s screen after %#v",
						seed, step, at, rows, m.Screen(), last)
				}
				if m.isGrouped() && rows > 0 && !m.matchedResults()[at].selectable() {
					t.Fatalf("seed %d, step %d: the cursor is on a heading of the %s screen after %#v",
						seed, step, m.Screen(), last)
				}
			}
		}()
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
