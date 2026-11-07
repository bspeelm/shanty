package tui

import (
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
	// The queue is reached from anywhere and holds tracks rather than a
	// collection with an order of its own.
	m := onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued(), At: 0}))
	m, msg := press(t, m, "S")

	if msg != nil {
		t.Errorf("S on the queue emitted %#v, want nothing", msg)
	}
	if !strings.Contains(m.status, "press S on") {
		t.Errorf("the status does not say where S does work: %q", m.status)
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
