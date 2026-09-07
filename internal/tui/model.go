package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// Screen is which of the three v0.1 views is showing.
type Screen int

const (
	ScreenArtists Screen = iota
	ScreenAlbums
	ScreenTracks
)

// Screens is every screen, so a test can assert the set is closed rather than
// checking the three somebody remembered.
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

// Model is the whole interface as data. Nothing here can fetch, play or write
// anything: it renders what it has been given and says what it would like done.
type Model struct {
	screen        Screen
	width, height int

	artists []subsonic.Artist
	artist  subsonic.Artist
	album   subsonic.Album

	// cursor per screen, so going back lands where the user left.
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

// New builds an empty model waiting for its first artists.
func New() Model {
	return Model{
		cursor:  map[Screen]int{},
		volume:  100,
		loading: true,
		status:  "loading the library",
	}
}

func (m Model) Init() tea.Cmd { return nil }

// Screen, Cursor and Status exist so tests can assert on state without parsing
// the rendered frame, which is the golden files' job.
func (m Model) Screen() Screen { return m.screen }
func (m Model) Cursor() int    { return m.cursor[m.screen] }
func (m Model) Status() string { return m.status }

// rows is how many items the current screen lists.
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
