package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// Intents are what this package emits instead of doing. Each is a Bubble Tea
// message; the outer model in cmd/shanty intercepts them and performs the I/O,
// which is how internal/tui renders a whole client without being able to open
// a socket.
type (
	OpenArtist struct{ ID string }
	OpenAlbum  struct{ ID string }
	PlayFrom   struct {
		Album subsonic.Album
		Index int
	}
	TogglePause struct{}
	SkipNext    struct{}
	SkipPrev    struct{}
	SeekBy      struct{ By time.Duration }
	VolumeBy    struct{ Delta int }
	GoBack      struct{}
	Quit        struct{}
)

// Inputs are what the outer model sends back once it has done the work.
type (
	ArtistsLoaded []subsonic.Artist
	ArtistLoaded  subsonic.Artist
	AlbumLoaded   subsonic.Album
	NowPlaying    struct {
		Title, Artist string
		Duration      time.Duration
	}
	Progress      time.Duration
	PausedChanged bool
	VolumeChanged int
	// Failed carries a message rather than an error because the text may have
	// come from the server, and it is sanitised on the way to the screen like
	// any other string the server chose.
	Failed struct{ Message string }
)

func emit(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }
