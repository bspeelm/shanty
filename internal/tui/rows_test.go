package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// everywhere is one model on each kind of row a key can act on, by name.
func everywhere(t *testing.T) map[string]Model {
	t.Helper()
	lists := []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}
	base := send(t, loaded(t), PlaylistsLoaded(lists), starredChanged(), AlbumsLoaded(everyAlbum()))
	aoi := library()[0]
	found := send(t, base, SearchLoaded{Query: "water", Results: results()})
	starred := onStarred(t, base)
	albums := send(t, base, ArtistLoaded(aoi))
	return map[string]Model{
		"an artist":                  base,
		"an artist found by search":  found,
		"a starred artist":           starred,
		"one of an artist's albums":  down(t, albums, 1),
		"one of every album":         down(t, send(t, base, keyOf("g"), keyOf("b")), 1),
		"an album found by search":   down(t, found, 2),
		"a starred album":            down(t, starred, 1),
		"a track on an album":        down(t, send(t, albums, AlbumLoaded(aoi.Albums[0])), 1),
		"a playlist":                 down(t, onPlaylists(t, base), 1),
		"a track in a playlist":      down(t, send(t, onPlaylists(t, base), PlaylistLoaded(eveningOf(3))), 2),
		"a track found by search":    down(t, found, 4),
		"a starred track":            down(t, starred, 2),
		"a track in the queue":       down(t, onQueue(t, send(t, base, QueueChanged{Tracks: queued()})), 1),
		"the library":                send(t, base, keyOf("g"), keyOf("l")),
		"the wiki":                   send(t, base, ShowWiki{}),
		"the messages":               send(t, base, Notice("playing Ballast next"), ShowMessages{}),
		"a list with nothing in it":  send(t, sized(New()), ArtistsLoaded(nil), keyOf("g"), keyOf("a")),
		"a playlist with no tracks":  send(t, onPlaylists(t, base), PlaylistLoaded(eveningOf(0))),
		"the queue with nothing":     onQueue(t, base),
		"a search that found nought": send(t, base, SearchLoaded{Query: "zzz"}),
	}
}

// TestTheQueueKeysAddWhatTheCursorIsOn holds a and A on every kind of row
// against what they ask for.
func TestTheQueueKeysAddWhatTheCursorIsOn(t *testing.T) {
	aoi := library()[0]
	songs := subsonic.Album{Songs: results().Songs}
	want := map[string]func(next bool) tea.Msg{
		"an artist":                 func(n bool) tea.Msg { return QueueThis{Scope: ScopeArtist, ID: "ar-1", Next: n} },
		"an artist found by search": func(n bool) tea.Msg { return QueueThis{Scope: ScopeArtist, ID: "ar-1", Next: n} },
		"a starred artist":          func(n bool) tea.Msg { return QueueThis{Scope: ScopeArtist, ID: "ar-1", Next: n} },
		"one of an artist's albums": func(n bool) tea.Msg { return QueueThis{Scope: ScopeAlbum, ID: "al-2", Next: n} },
		"one of every album":        func(n bool) tea.Msg { return QueueThis{Scope: ScopeAlbum, ID: "al-3", Next: n} },
		"an album found by search":  func(n bool) tea.Msg { return QueueThis{Scope: ScopeAlbum, ID: "al-9", Next: n} },
		"a starred album":           func(n bool) tea.Msg { return QueueThis{Scope: ScopeAlbum, ID: "al-1", Next: n} },
		"a playlist":                func(n bool) tea.Msg { return QueueThis{Scope: ScopePlaylist, ID: "pl-2", Next: n} },
		"a track on an album":       func(n bool) tea.Msg { return one(n, aoi.Albums[0], 1) },
		"a track in a playlist":     func(n bool) tea.Msg { return one(n, asAlbum(eveningOf(3)), 2) },
		"a track found by search":   func(n bool) tea.Msg { return one(n, songs, 1) },
		"a starred track":           func(n bool) tea.Msg { return one(n, subsonic.Album{Songs: starredChanged().Results.Songs}, 0) },
	}
	for name, m := range everywhere(t) {
		for key, next := range map[string]bool{"a": false, "A": true} {
			got, msg := press(t, m, key)
			if make, has := want[name]; has {
				if !reflect.DeepEqual(msg, make(next)) {
					t.Errorf("%s on %s emitted %#v, want %#v", key, name, msg, make(next))
				}
				continue
			}
			if msg != nil || got.Status() == "" {
				t.Errorf("%s on %s emitted %#v and said %q", key, name, msg, got.Status())
			}
		}
	}
}

// one is the intent that queues a single track of a list.
func one(next bool, list subsonic.Album, index int) tea.Msg {
	if next {
		return PlayNext{Album: list, Index: index}
	}
	return Enqueue{Album: list, Index: index}
}

// TestAKeyWithNothingToActOnSaysSoOnEveryScreen presses the keys that act on a
// row everywhere, and holds each to doing something or saying why not.
func TestAKeyWithNothingToActOnSaysSoOnEveryScreen(t *testing.T) {
	for name, m := range everywhere(t) {
		for _, key := range []string{"a", "A", "e", "r", "K", "J", "*", "S"} {
			got, msg := press(t, m, key)
			silent := msg == nil && got.Status() == "" && !got.Confirming() && got.Editing().ID == "" &&
				reflect.DeepEqual(got.cursor, m.cursor)
			// K on the first row and J on the last have nowhere to move to.
			if silent && !(strings.ContainsAny(key, "KJ") && name == "a track in the queue") {
				t.Errorf("%s on %s did nothing and said nothing", key, name)
			}
		}
	}
}

func TestStarringWorksOnEveryRowThatCanBeStarred(t *testing.T) {
	want := map[string]ToggleStar{
		"an artist":                 {Kind: StarArtist, ID: "ar-1"},
		"an artist found by search": {Kind: StarArtist, ID: "ar-1", Starred: true},
		"a starred artist":          {Kind: StarArtist, ID: "ar-1"},
		"one of an artist's albums": {Kind: StarAlbum, ID: "al-2", Starred: true},
		"one of every album":        {Kind: StarAlbum, ID: "al-3", Starred: true},
		"an album found by search":  {Kind: StarAlbum, ID: "al-9", Starred: true},
		"a starred album":           {Kind: StarAlbum, ID: "al-1"},
		"a track on an album":       {Kind: StarSong, ID: "tr-2"},
		"a track in a playlist":     {Kind: StarSong, ID: "tr-3", Starred: true},
		"a track found by search":   {Kind: StarSong, ID: "tr-7", Starred: true},
		"a starred track":           {Kind: StarSong, ID: "tr-2"},
		"a track in the queue":      {Kind: StarSong, ID: "tr-2"},
	}
	for name, m := range everywhere(t) {
		_, msg := press(t, m, "*")
		star, has := want[name]
		switch {
		case has && (msg == nil || msg.(ToggleStar).Kind != star.Kind || msg.(ToggleStar).ID != star.ID):
			t.Errorf("* on %s emitted %#v, want %s %s", name, msg, star.Kind, star.ID)
		case !has && msg != nil:
			t.Errorf("* on %s emitted %#v", name, msg)
		}
	}
}

func TestEnterOnATrackFoundPlaysFromTheTracksListedThere(t *testing.T) {
	m := down(t, send(t, loaded(t), SearchLoaded{Query: "water", Results: results()}), 5)
	_, msg := press(t, m, "enter")
	if want := (PlayFrom{Album: subsonic.Album{Songs: results().Songs}, Index: 1}); !reflect.DeepEqual(msg, want) {
		t.Errorf("enter on the second track found emitted %#v, want %#v", msg, want)
	}
}

func TestTheQueueIsEditedWithTheKeysThatEditAPlaylist(t *testing.T) {
	m := down(t, onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued()})), 1)

	if _, msg := press(t, m, "r"); msg != (RemoveQueued{At: 1, ID: "tr-2", Count: 1}) {
		t.Errorf("r on the second track emitted %#v", msg)
	}
	up, msg := press(t, m, "K")
	if msg != (MoveQueued{From: 1, To: 0, ID: "tr-2"}) || up.Cursor() != 0 {
		t.Errorf("K emitted %#v and left the cursor on row %d", msg, up.Cursor())
	}
	dn, msg := press(t, m, "J")
	if msg != (MoveQueued{From: 1, To: 2, ID: "tr-2"}) || dn.Cursor() != 2 {
		t.Errorf("J emitted %#v and left the cursor on row %d", msg, dn.Cursor())
	}
	if _, msg := press(t, dn, "J"); msg != nil {
		t.Errorf("J on the last row emitted %#v", msg)
	}
	if _, msg := press(t, up, "K"); msg != nil {
		t.Errorf("K on the first row emitted %#v", msg)
	}

	// Narrowed to the two tracks by one artist, the second row shown is the
	// second track of the queue, and the row after the filter is the third.
	kept := narrowed(t, onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued()})), "low")
	if kept.rows() != 1 {
		t.Fatalf("the filter leaves %d rows, want only the third track", kept.rows())
	}
	said, msg := press(t, kept, "J")
	if msg != nil || !strings.Contains(said.Status(), "narrowed") {
		t.Errorf("J on a narrowed queue emitted %#v and said %q", msg, said.Status())
	}
	if _, msg := press(t, kept, "r"); msg != (RemoveQueued{At: 2, ID: "tr-9", Count: 1}) {
		t.Errorf("r on a narrowed queue emitted %#v, want the track's own place in the queue", msg)
	}

	counted := onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued()}))
	counted, _ = press(t, counted, "2")
	if _, msg := press(t, counted, "r"); msg != (RemoveQueued{At: 0, ID: "tr-1", Count: 2}) {
		t.Errorf("2r emitted %#v, want two tracks taken from the cursor down", msg)
	}
}

func TestTakingATrackOutOfAPlaylistAsksFirst(t *testing.T) {
	m := down(t, send(t, onPlaylists(t, loaded(t)), PlaylistLoaded(eveningOf(3))), 1)

	asked, msg := press(t, m, "r")
	if msg != nil || !asked.Confirming() || !strings.Contains(asked.View(), "take Ballast out of Evening?") {
		t.Fatalf("r emitted %#v and asked:\n%s", msg, asked.View())
	}
	if _, msg := press(t, asked, "n"); msg != nil {
		t.Errorf("n emitted %#v", msg)
	}
	_, msg = press(t, asked, "y")
	if want := (EditPlaylist{ID: "pl-1", SongID: "tr-2", Add: false, At: 1}); msg != want {
		t.Errorf("y emitted %#v, want %#v", msg, want)
	}
}

func TestWhileAddingToAPlaylistTheKeysEditItOnTrackRowsWithoutAsking(t *testing.T) {
	editing := func(m Model) Model {
		m.editing, m.inPlaylist = eveningOf(1), holding(eveningOf(1))
		return m
	}
	rows := everywhere(t)
	for _, name := range []string{"a track on an album", "a track in a playlist", "a track found by search", "a track in the queue"} {
		m := editing(rows[name])
		id := m.under().id
		add, remove := press, press
		_, added := add(t, m, "a")
		_, removed := remove(t, m, "r")
		want := EditPlaylist{ID: "pl-1", SongID: id, Add: !m.inPlaylist[id], At: -1}
		got := added
		if m.inPlaylist[id] {
			got = removed
		}
		if got != want {
			t.Errorf("%s: adding to a playlist, the key emitted %#v, want %#v", name, got, want)
		}
	}
	said, msg := press(t, editing(rows["an artist"]), "a")
	if msg != nil || !strings.Contains(said.Status(), "open an album") {
		t.Errorf("a on an artist while adding to a playlist emitted %#v and said %q", msg, said.Status())
	}
}
