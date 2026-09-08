package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// Screen identifies one of the three views.
type Screen int

const (
	ScreenArtists Screen = iota
	ScreenAlbums
	ScreenTracks
)

// Screens lists every screen.
var Screens = []Screen{ScreenArtists, ScreenAlbums, ScreenTracks}

func (s Screen) String() string {
	switch s {
	case ScreenArtists:
		return "artists"
	case ScreenAlbums:
		return "albums"
	case ScreenTracks:
		return "tracks"
	}
	return "unknown"
}

// Model is the state of the interface. It renders itself and reports intents;
// it cannot fetch, play or write anything.
type Model struct {
	screen        Screen
	width, height int

	artists []subsonic.Artist
	artist  subsonic.Artist
	album   subsonic.Album

	// cursor holds the selected row for each screen.
	cursor map[Screen]int

	nowPlaying string
	nowArtist  string
	position   time.Duration
	duration   time.Duration
	paused     bool
	volume     int

	status  string
	loading bool
}

// New returns a model with nothing loaded.
func New() Model {
	return Model{
		cursor:  map[Screen]int{},
		volume:  100,
		loading: true,
		status:  "loading the library",
	}
}

func (m Model) Init() tea.Cmd { return nil }

// Screen, Cursor, Status and Paused report the model’s state.
func (m Model) Screen() Screen { return m.screen }
func (m Model) Cursor() int    { return m.cursor[m.screen] }
func (m Model) Status() string { return m.status }
func (m Model) Paused() bool   { return m.paused }

// rows is the number of items on the current screen.
func (m Model) rows() int {
	switch m.screen {
	case ScreenArtists:
		return len(m.artists)
	case ScreenAlbums:
		return len(m.artist.Albums)
	case ScreenTracks:
		return len(m.album.Songs)
	}
	return 0
}
