package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// Intents are what the interface asks for. Each is a message; the caller
// performs the work and sends the result back as one of the Inputs below.
type (
	OpenArtist struct{ ID string }
	OpenAlbum  struct{ ID string }
	PlayFrom   struct {
		Album subsonic.Album
		Index int
	}
	TogglePause struct{}
	// SetPaused is an absolute pause state. TogglePause is what the space key
	// sends, where the screen says which way it will go; a command in another
	// shell has no screen and says which state it wants.
	SetPaused bool
	SkipNext  struct{}
	SkipPrev  struct{}
	SeekBy    struct{ By time.Duration }
	// SeekTo is a position in the track, where SeekBy is the relative step the
	// keys make.
	SeekTo time.Duration
	// SeekToPercent is a position in the track as a percentage of its length,
	// where SeekBy is the relative step the keys make.
	SeekToPercent int
	VolumeBy      struct{ Delta int }
	// VolumeSet is an absolute volume, from the command line, where VolumeBy
	// is the relative change the keys make.
	VolumeSet int
	GoBack    struct{}
	Quit      struct{}
	// Detach asks for the interface to end while playback carries on.
	Detach struct{}
	// PlayNext asks for the selected track to play after the current one, and
	// Enqueue for it to go on the end.
	PlayNext struct {
		Album subsonic.Album
		Index int
	}
	Enqueue struct {
		Album subsonic.Album
		Index int
	}
	// JumpTo asks for the queue to move to one of its own tracks.
	JumpTo int
)

// Inputs are what the caller sends back once it has done the work.
type (
	ArtistsLoaded []subsonic.Artist
	ArtistLoaded  subsonic.Artist
	AlbumLoaded   subsonic.Album
	NowPlaying    struct {
		Title, Artist string
		Duration      time.Duration
	}
	// QueueChanged is what is queued and where in it playback has reached. The
	// caller owns the queue; this is what the screen draws.
	QueueChanged struct {
		Tracks []queue.Track
		At     int
	}
	Progress      time.Duration
	PausedChanged bool
	VolumeChanged int
	// Failed carries a message to display. It is sanitised like any other text,
	// because it may have come from the server.
	Failed struct{ Message string }
	// Notice carries a message about something in progress. Failed would clear
	// the loading flag, which is wrong while a screen is still arriving.
	Notice string
)

func emit(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }
