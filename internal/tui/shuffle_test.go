package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// S shuffles the collection the cursor names, or the one it is inside. These
// pin which collection that is on every screen, because "the one you mean" is
// only obvious while you are looking at it.

func TestShuffleOnAnArtistAsksForTheWholeCatalogue(t *testing.T) {
	m, msg := press(t, loaded(t), "S")

	got, ok := msg.(ShuffleThis)
	if !ok {
		t.Fatalf("S on the artist list emitted %#v, want ShuffleThis", msg)
	}
	if got.Scope != "artist" || got.ID != "ar-1" {
		t.Errorf("shuffled %s %q, want artist ar-1", got.Scope, got.ID)
	}
	if m.status != "" {
		t.Errorf("S reported %q when it had something to shuffle", m.status)
	}
}

func TestShuffleOnAnAlbumRowAsksForThatAlbum(t *testing.T) {
	albums := send(t, loaded(t), ArtistLoaded(library()[0]))
	_, msg := press(t, albums, "S")

	got, ok := msg.(ShuffleThis)
	if !ok {
		t.Fatalf("S on the album list emitted %#v, want ShuffleThis", msg)
	}
	if got.Scope != "album" || got.ID != "al-1" {
		t.Errorf("shuffled %s %q, want album al-1", got.Scope, got.ID)
	}
}

// Inside an album the cursor is on a track, and one track has no order to
// shuffle, so it is the record that gets shuffled.
func TestShuffleOnATrackAsksForTheAlbumHoldingIt(t *testing.T) {
	tracks := send(t, loaded(t), ArtistLoaded(library()[0]), AlbumLoaded(library()[0].Albums[0]))
	tracks, _ = press(t, tracks, "j") // off the first row, to show the row does not matter
	_, msg := press(t, tracks, "S")

	got, ok := msg.(ShuffleThis)
	if !ok {
		t.Fatalf("S on a track emitted %#v, want ShuffleThis", msg)
	}
	if got.Scope != "album" || got.ID != "al-1" {
		t.Errorf("shuffled %s %q, want album al-1, the album on screen", got.Scope, got.ID)
	}
}

func TestShuffleOnAPlaylistRowAndInsideOneBothAskForThatPlaylist(t *testing.T) {
	list := subsonic.Playlist{ID: "pl-1", Name: "Evening", SongCount: 1, Songs: []subsonic.Song{
		{ID: "tr-1", Title: "Slipway", Artist: "Aoi", Duration: 180},
	}}
	lists := onPlaylists(t, send(t, loaded(t), PlaylistsLoaded([]subsonic.Playlist{list})))

	_, msg := press(t, lists, "S")
	got, ok := msg.(ShuffleThis)
	if !ok {
		t.Fatalf("S on the playlists screen emitted %#v, want ShuffleThis", msg)
	}
	if got.Scope != "playlist" || got.ID != "pl-1" {
		t.Errorf("shuffled %s %q, want playlist pl-1", got.Scope, got.ID)
	}

	_, msg = press(t, send(t, lists, PlaylistLoaded(list)), "S")
	inside, ok := msg.(ShuffleThis)
	if !ok {
		t.Fatalf("S inside a playlist emitted %#v, want ShuffleThis", msg)
	}
	if inside != got {
		t.Errorf("inside the playlist S asked for %+v, want the same as from the list, %+v", inside, got)
	}
}

func TestShuffleWhereThereIsNothingToShuffleSaysWherePressingItWorks(t *testing.T) {
	// The wiki and the messages hold nothing that can be played.
	for name, m := range map[string]Model{
		"wiki":     send(t, loaded(t), ShowWiki{}),
		"messages": send(t, loaded(t), Notice("playing Ballast next"), ShowMessages{}),
	} {
		m, msg := press(t, m, "S")
		if msg != nil {
			t.Errorf("S on the %s emitted %#v, want nothing", name, msg)
		}
		if !strings.Contains(m.status, "press S on") {
			t.Errorf("the status on the %s does not say where S does work: %q", name, m.status)
		}
	}
}

// narrowed types a filter and keeps it.
func narrowed(t *testing.T, m Model, by string) Model {
	t.Helper()
	m, _ = press(t, m, "/")
	m, _ = press(t, typed(t, m, by), "enter")
	return m
}

// down moves the cursor down n rows.
func down(t *testing.T, m Model, n int) Model {
	t.Helper()
	for range n {
		m, _ = press(t, m, "j")
	}
	return m
}

// TestShuffleAsksForWhatTheCursorIsOnOrTheListItIsIn holds every kind of row
// on every screen against what S asks for there.
func TestShuffleAsksForWhatTheCursorIsOnOrTheListItIsIn(t *testing.T) {
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}
	base := send(t, loaded(t), PlaylistsLoaded(lists), starredChanged(), AlbumsLoaded(everyAlbum()))
	library := send(t, base, QueueChanged{Tracks: queued()})
	library.screen = ScreenLibrary
	found := send(t, base, SearchLoaded{Query: "water", Results: results()})
	starred := onStarred(t, base)
	albums := send(t, base, ArtistLoaded(library.artists[0]))
	everyAlbums := send(t, base, keyOf("g"), keyOf("b"))
	inPlaylist := send(t, base, PlaylistLoaded(eveningOf(3)))
	onLibrary := func(row string) Model { return onRow(t, library, row) }

	for _, tc := range []struct {
		name string
		m    Model
		want any
	}{
		{"an artist", base, ShuffleThis{Scope: ScopeArtist, ID: "ar-1"}},
		{"an artist in a narrowed list", narrowed(t, base, "bilge"), ShuffleThis{Scope: ScopeArtist, ID: "ar-2"}},
		{"an artist found by a search", found, ShuffleThis{Scope: ScopeArtist, ID: "ar-1"}},
		{"a starred artist", starred, ShuffleThis{Scope: ScopeArtist, ID: "ar-1"}},
		{"one of an artist's albums", down(t, albums, 1), ShuffleThis{Scope: ScopeAlbum, ID: "al-2"}},
		{"one of every album", down(t, everyAlbums, 1), ShuffleThis{Scope: ScopeAlbum, ID: "al-3"}},
		{"one of every album, narrowed", narrowed(t, everyAlbums, "low"), ShuffleThis{Scope: ScopeAlbum, ID: "al-2"}},
		{"an album found by a search", down(t, found, 2), ShuffleThis{Scope: ScopeAlbum, ID: "al-9"}},
		{"a starred album", down(t, starred, 1), ShuffleThis{Scope: ScopeAlbum, ID: "al-1"}},
		{"a track on an album", down(t, send(t, albums, AlbumLoaded(library.artists[0].Albums[0])), 1),
			ShuffleThis{Scope: ScopeAlbum, ID: "al-1"}},
		{"a playlist", down(t, onPlaylists(t, base), 1), ShuffleThis{Scope: ScopePlaylist, ID: "pl-2"}},
		{"a track in a playlist", down(t, inPlaylist, 2), ShuffleThis{Scope: ScopePlaylist, ID: "pl-1"}},
		{"a track found by a search", down(t, found, 4),
			ShuffleSongs{Songs: results().Songs, What: `2 tracks found for "water"`}},
		{"a track found by a search, narrowed", narrowed(t, found, "slip"),
			ShuffleSongs{Songs: results().Songs[:1], What: `1 track found for "water"`}},
		{"a starred track", down(t, starred, 2),
			ShuffleSongs{Songs: starredChanged().Results.Songs, What: "1 starred track"}},
		{"the library's starred", onLibrary("Starred"), ShuffleThis{Scope: ScopeStarred}},
		{"the library's queue", onLibrary("Queue"), ShuffleThis{Scope: ScopeQueue}},
		{"the queue", onQueue(t, library), ShuffleThis{Scope: ScopeQueue}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, msg := press(t, tc.m, "S")
			if !reflect.DeepEqual(msg, tc.want) {
				t.Errorf("S emitted %#v, want %#v", msg, tc.want)
			}
			if m.Status() != "" {
				t.Errorf("S said %q when it had something to shuffle", m.Status())
			}
		})
	}
}

// TestShuffleWithNothingUnderItSaysWhatIsWrongAndWhatToDo covers the rows and
// the screens S has nothing to ask for.
func TestShuffleWithNothingUnderItSaysWhatIsWrongAndWhatToDo(t *testing.T) {
	base := loaded(t)
	library := base
	library.screen = ScreenLibrary

	for _, tc := range []struct {
		name  string
		m     Model
		wants []string
	}{
		{"the library's playlists", onRow(t, library, "Playlists"), []string{"choose a playlist", "press S"}},
		{"the library's artists", onRow(t, library, "Artists"), []string{":shuffle", "press S"}},
		{"the library's albums", onRow(t, library, "Albums"), []string{":shuffle", "press S"}},
		{"the library's queue, empty", onRow(t, library, "Queue"), []string{"queue is empty", "enter"}},
		{"the queue, empty", onQueue(t, base), []string{"queue is empty", "enter"}},
		{"the queue, finished", onQueue(t, send(t, base, QueueChanged{Tracks: queued(), At: 3})),
			[]string{"nothing is playing", "enter"}},
		{"the queue, on its last track", onQueue(t, send(t, base, QueueChanged{Tracks: queued(), At: 2})),
			[]string{"fewer than two tracks follow", "a on a track"}},
		{"the queue, with one track left", onQueue(t, send(t, base, QueueChanged{Tracks: queued(), At: 1})),
			[]string{"fewer than two tracks follow", "a on a track"}},
		{"a playlist being reordered", reordering(t, evening3(t)), []string{"being reordered", "enter saves"}},
		{"a heading", onHeading(t, searched(t)), []string{"heading", "press S"}},
		{"nothing starred", onStarred(t, base), []string{"nothing is starred", "*"}},
		{"a search that found nothing", send(t, base, SearchLoaded{Query: "zzz"}), []string{"found nothing", ":search"}},
		{"every album, with none", send(t, base, keyOf("g"), keyOf("b"), AlbumsLoaded(nil)), []string{"nothing to shuffle here", "press S on"}},
		{"a filter matching nothing", narrowed(t, base, "zzz"), []string{"nothing matches zzz", "/"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, msg := press(t, tc.m, "S")
			if msg != nil {
				t.Errorf("S emitted %#v, want nothing", msg)
			}
			for _, want := range tc.wants {
				if !strings.Contains(m.Status(), want) {
					t.Errorf("S said %q, which does not hold %q", m.Status(), want)
				}
			}
		})
	}
}

func TestShuffleOnAnEmptyScreenDoesNotReachForARowThatIsNotThere(t *testing.T) {
	// No artists loaded, so there is no row under the cursor. The guard here
	// is the difference between a message and a panic, and the panic budget
	// is zero.
	m, msg := press(t, sized(New()), "S")
	if msg != nil {
		t.Errorf("S on an empty list emitted %#v, want nothing", msg)
	}
	if m.status == "" {
		t.Error("S on an empty list said nothing at all")
	}
}

func TestShuffleEverythingIsStillItsOwnCommand(t *testing.T) {
	// :shuffle means the whole server and S means one collection. Keeping
	// them apart is the reason S exists.
	_, msg := typeCommand(t, loaded(t), "shuffle")
	if _, ok := msg.(Shuffle); !ok {
		t.Errorf(":shuffle emitted %#v, want Shuffle", msg)
	}
}

// onHeading puts the cursor on the first row of a search, which is a heading.
// No key leaves it there.
func onHeading(t *testing.T, m Model) Model {
	t.Helper()
	m = m.selecting(m.screen, 0)
	if m.grouped()[0].selectable() {
		t.Fatal("the first row is not a heading")
	}
	return m
}

func TestTheQueueIsRefusedTheSameWayHereAsByTheApp(t *testing.T) {
	m := onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued(), At: 2}))
	m, _ = press(t, m, "S")
	if !m.statusFailed {
		t.Errorf("too few tracks to shuffle is not drawn as a refusal: %q", m.Status())
	}
}
