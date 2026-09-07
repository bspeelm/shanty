package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// press returns the model after a key, and whatever intent it emitted.
func press(t *testing.T, m Model, key string) (Model, tea.Msg) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	if key == "enter" {
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}
	var out tea.Msg
	if cmd != nil {
		out = cmd()
	}
	return next.(Model), out
}

func loaded(t *testing.T) Model {
	t.Helper()
	return send(t, sized(New()), ArtistsLoaded(library()))
}

// Descending emits an intent and nothing else: this package cannot fetch the
// artist it just asked for, which is the rule the whole design rests on.
func TestOpeningEmitsAnIntentRatherThanFetching(t *testing.T) {
	m := loaded(t)

	_, msg := press(t, m, "enter")
	open, ok := msg.(OpenArtist)
	if !ok {
		t.Fatalf("enter on the artist list emitted %T, want OpenArtist", msg)
	}
	if open.ID != "ar-1" {
		t.Errorf("opened %q, want ar-1", open.ID)
	}
}

func TestBrowseDescendsAndReturns(t *testing.T) {
	m := loaded(t)
	if m.Screen() != ScreenArtists {
		t.Fatalf("started on %s", m.Screen())
	}

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenAlbums {
		t.Fatalf("after ArtistLoaded the screen is %s", m.Screen())
	}
	m = send(t, m, AlbumLoaded(library()[0].Albums[0]))
	if m.Screen() != ScreenTracks {
		t.Fatalf("after AlbumLoaded the screen is %s", m.Screen())
	}

	m, _ = press(t, m, "h")
	if m.Screen() != ScreenAlbums {
		t.Errorf("back from tracks went to %s", m.Screen())
	}
	m, _ = press(t, m, "h")
	if m.Screen() != ScreenArtists {
		t.Errorf("back from albums went to %s", m.Screen())
	}
}

// Leaving a music player by pressing left one time too many is a surprise
// nobody wants mid-album.
func TestBackAtTheTopDoesNothing(t *testing.T) {
	m := loaded(t)
	for range 3 {
		var msg tea.Msg
		m, msg = press(t, m, "h")
		if msg != nil {
			t.Fatalf("back at the top emitted %T", msg)
		}
	}
	if m.Screen() != ScreenArtists {
		t.Errorf("back at the top moved to %s", m.Screen())
	}
}

// The cursor is per screen, so going back lands where the user left.
func TestTheCursorIsRememberedPerScreen(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j")
	if m.Cursor() != 2 {
		t.Fatalf("cursor is %d after two downs", m.Cursor())
	}

	m = send(t, m, ArtistLoaded(library()[0]))
	m, _ = press(t, m, "j")
	if m.Cursor() != 1 {
		t.Fatalf("the albums cursor is %d, want 1", m.Cursor())
	}

	m, _ = press(t, m, "h")
	if m.Cursor() != 2 {
		t.Errorf("returning to artists put the cursor at %d, want 2", m.Cursor())
	}
}

// The caller is a key that can be held down.
func TestTheCursorClamps(t *testing.T) {
	m := loaded(t)
	for range 20 {
		m, _ = press(t, m, "j")
	}
	if got, want := m.Cursor(), len(library())-1; got != want {
		t.Errorf("cursor ran to %d, want %d", got, want)
	}
	for range 20 {
		m, _ = press(t, m, "k")
	}
	if m.Cursor() != 0 {
		t.Errorf("cursor ran to %d going up, want 0", m.Cursor())
	}
}

// Update must not change the model it was called on: the previous frame is
// still being held by bubbletea when the next one is computed.
func TestUpdateDoesNotMutateItsReceiver(t *testing.T) {
	m := loaded(t)
	before := m.Cursor()

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	if m.Cursor() != before {
		t.Errorf("the receiver moved to %d without being reassigned", m.Cursor())
	}
}

// Every playback key produces an intent and touches nothing.
func TestPlaybackKeysEmitIntents(t *testing.T) {
	m := loaded(t)
	for _, tc := range []struct {
		key  string
		want tea.Msg
	}{
		{" ", TogglePause{}},
		{"n", SkipNext{}},
		{"p", SkipPrev{}},
		{"]", SeekBy{By: 10 * time.Second}},
		{"[", SeekBy{By: -10 * time.Second}},
		{"+", VolumeBy{Delta: 5}},
		{"-", VolumeBy{Delta: -5}},
		{"q", Quit{}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			_, got := press(t, m, tc.key)
			if got != tc.want {
				t.Errorf("%q emitted %#v, want %#v", tc.key, got, tc.want)
			}
		})
	}
}

// Playing carries the album by value, so the handler does not have to ask this
// package for state it already sent in.
func TestPlayingCarriesTheAlbumAndTheIndex(t *testing.T) {
	m := send(t, loaded(t), ArtistLoaded(library()[0]), AlbumLoaded(library()[0].Albums[0]))
	m, _ = press(t, m, "j")

	_, msg := press(t, m, "enter")
	play, ok := msg.(PlayFrom)
	if !ok {
		t.Fatalf("enter on the track list emitted %T, want PlayFrom", msg)
	}
	if play.Index != 1 {
		t.Errorf("playing index %d, want 1", play.Index)
	}
	if len(play.Album.Songs) != 2 || play.Album.Songs[1].ID != "tr-2" {
		t.Errorf("the album did not travel with the intent: %+v", play.Album)
	}
}

// An empty list must not be indexable, and pressing enter on nothing must not
// emit an intent naming a track that does not exist.
func TestAnEmptyLibraryIsSafeToPressKeysAt(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(nil))
	for _, key := range []string{"j", "k", "enter", "G", "g"} {
		var msg tea.Msg
		m, msg = press(t, m, key)
		if _, isOpen := msg.(OpenArtist); isOpen {
			t.Errorf("%q opened something in an empty library", key)
		}
	}
	if m.Cursor() != 0 {
		t.Errorf("the cursor moved to %d in an empty library", m.Cursor())
	}
}

// Screens is a closed set: a fourth screen added without a heading would
// render "unknown" to a user rather than failing here.
func TestEveryScreenHasAHeadingAndRenders(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(library()),
		ArtistLoaded(library()[0]), AlbumLoaded(library()[0].Albums[0]))

	for _, s := range Screens {
		m.screen = s
		if s.String() == "unknown" {
			t.Errorf("screen %d has no name", s)
		}
		if frame := m.View(); frame == "" {
			t.Errorf("screen %s rendered nothing", s)
		}
	}
	if len(Screens) != 3 {
		t.Errorf("Screens lists %d screens; v0.1 has three, so this is a change to the interface", len(Screens))
	}
}
