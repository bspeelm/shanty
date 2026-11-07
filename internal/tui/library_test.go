package tui

import (
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// The library is the screen the program starts on and the one above the artist
// list. These cover what each row opens, what the counts say before they are
// known, and that the way out of the program has not changed.

func TestTheProgramStartsOnTheLibrary(t *testing.T) {
	m := sized(New())
	if m.Screen() != ScreenLibrary {
		t.Fatalf("a new model starts on %s, want the library", m.Screen())
	}
	// The artists arriving does not move the screen: they fill the list
	// behind the library, and the count with it.
	m = send(t, m, ArtistsLoaded(library()))
	if m.Screen() != ScreenLibrary {
		t.Errorf("loading the artists moved to %s", m.Screen())
	}
	if frame := m.View(); !strings.Contains(frame, "shanty · library") {
		t.Errorf("the heading does not name the library:\n%s", frame)
	}
}

func TestEveryLibraryRowOpensItsScreen(t *testing.T) {
	// Each row, by the name on it, so that reordering the table does not
	// quietly change what this covers.
	for _, tc := range []struct {
		row    string
		screen Screen
		// intent is the message the row emits rather than moving by itself,
		// because the screen it opens has to be fetched first.
		intent any
	}{
		{"Playlists", ScreenPlaylists, ShowPlaylists{}},
		{"Artists", ScreenArtists, nil},
		{"Albums", ScreenEveryAlbum, ShowEveryAlbum{}},
		{"Starred", ScreenStarred, nil},
		{"Queue", ScreenQueue, nil},
	} {
		t.Run(tc.row, func(t *testing.T) {
			m := send(t, sized(New()), ArtistsLoaded(library()))
			m = onRow(t, m, tc.row)

			m, msg := press(t, m, "enter")

			if tc.intent != nil {
				if msg != tc.intent {
					t.Fatalf("%s emitted %#v, want %#v", tc.row, msg, tc.intent)
				}
				// The emitted intent is what moves the screen, so send it on.
				m = send(t, m, msg)
			}
			if m.Screen() != tc.screen {
				t.Errorf("%s opened %s, want %s", tc.row, m.Screen(), tc.screen)
			}
		})
	}
}

// onRow puts the cursor on the library row with that name.
func onRow(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, row := range libraryRows {
		if row.name != name {
			continue
		}
		for range i {
			m, _ = press(t, m, "j")
		}
		return m
	}
	t.Fatalf("the library has no %q row", name)
	return m
}

func TestLeavingTheArtistListGoesUpToTheLibrary(t *testing.T) {
	m, msg := press(t, loaded(t), "esc")
	if msg != nil {
		t.Errorf("leaving the artist list emitted %#v", msg)
	}
	if m.Screen() != ScreenLibrary {
		t.Errorf("esc from the artist list went to %s", m.Screen())
	}
}

func TestTheJumpsReachTheLibraryAndEveryAlbum(t *testing.T) {
	for key, want := range map[string]Screen{
		"l": ScreenLibrary,
		"b": ScreenEveryAlbum,
	} {
		m := loaded(t)
		m, _ = press(t, m, "g")
		m, _ = press(t, m, key)
		if m.Screen() != want {
			t.Errorf("g%s went to %s, want %s", key, m.Screen(), want)
		}
	}
}

func TestACountNotInYetIsBlankRatherThanAZero(t *testing.T) {
	// Nothing fetched. Playlists, artists, albums and starred are each a
	// request that has not come back; the queue is not, so nothing queued is
	// a real count of nothing.
	m := sized(New())
	for _, row := range libraryRows {
		got := m.libraryCount(row.screen)
		if row.screen == ScreenQueue {
			if got != "0" {
				t.Errorf("the queue count is %q before anything is queued, want 0", got)
			}
			continue
		}
		if got != "" {
			t.Errorf("%s shows %q before it has been fetched, want blank", row.name, got)
		}
	}

	// And once they are in, they are the lengths.
	m = send(t, m, ArtistsLoaded(library()), AlbumsLoaded(everyAlbum()),
		PlaylistsLoaded([]subsonic.Playlist{{ID: "pl-1", Name: "Evening"}}))
	if got := m.libraryCount(ScreenArtists); got != "3" {
		t.Errorf("the artist count is %q, want 3", got)
	}
	if got := m.libraryCount(ScreenEveryAlbum); got != "3" {
		t.Errorf("the album count is %q, want 3", got)
	}
	if got := m.libraryCount(ScreenPlaylists); got != "1" {
		t.Errorf("the playlist count is %q, want 1", got)
	}
}

func TestAnAlbumReturnsToTheListItWasOpenedFrom(t *testing.T) {
	// From every album.
	m := send(t, sized(New()), ArtistsLoaded(library()),
		ShowEveryAlbum{}, AlbumsLoaded(everyAlbum()))
	m, _ = press(t, m, "enter")
	m = send(t, m, AlbumLoaded(library()[0].Albums[0]))
	if m.Screen() != ScreenTracks {
		t.Fatalf("opening an album from every album went to %s", m.Screen())
	}
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenEveryAlbum {
		t.Errorf("leaving an album opened from every album went to %s", m.Screen())
	}

	// And from one artist's own list, which is the other way in.
	m = send(t, loaded(t), ArtistLoaded(library()[0]))
	m, _ = press(t, m, "enter")
	m = send(t, m, AlbumLoaded(library()[0].Albums[0]))
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenAlbums {
		t.Errorf("leaving an album opened from an artist went to %s", m.Screen())
	}
}

func TestEveryAlbumCarriesTheArtistOnEachRow(t *testing.T) {
	m := send(t, sized(New()), ShowEveryAlbum{}, AlbumsLoaded(everyAlbum()))

	// One artist's album list has the artist in the heading, so the rows do
	// not repeat it. This screen has no such heading.
	_, right := m.row(1)
	if right != "The Bilge Pumps" {
		t.Errorf("the second row's right half is %q, want the artist", right)
	}
	if frame := m.View(); !strings.Contains(frame, "every album · 3 albums") {
		t.Errorf("the heading does not count the albums:\n%s", frame)
	}
}

func TestFilteringTheLibraryMatchesTheRowNames(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(library()))
	m, _ = press(t, m, "/")
	for _, c := range "queue" {
		m, _ = press(t, m, string(c))
	}
	if m.rows() != 1 {
		t.Fatalf("filtering the library for queue left %d rows, want 1", m.rows())
	}
	if left, _ := m.row(0); !strings.Contains(left, "Queue") {
		t.Errorf("the remaining row is %q", left)
	}
}

func TestEveryAlbumIsFilteredByItsArtistToo(t *testing.T) {
	m := send(t, sized(New()), ShowEveryAlbum{}, AlbumsLoaded(everyAlbum()))
	m, _ = press(t, m, "/")
	for _, c := range "bilge" {
		m, _ = press(t, m, string(c))
	}
	// Bail is by The Bilge Pumps and its own name does not contain "bilge",
	// so this only passes if the artist is part of what the filter reads.
	if m.rows() != 1 {
		t.Fatalf("filtering every album by artist left %d rows, want 1", m.rows())
	}
	if left, _ := m.row(0); !strings.Contains(left, "Bail") {
		t.Errorf("the remaining row is %q", left)
	}
}
