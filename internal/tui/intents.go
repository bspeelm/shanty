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
	// Search asks the server for anything matching the query.
	Search string
	// Resume asks for the queue the server is holding to be taken up.
	Resume struct{}
	// Reload asks for what is on screen to be fetched from the server again.
	Reload struct{}
	// Scan asks the server to look at its music folder again.
	Scan struct{}
	// ShowPlaylists asks for the list of playlists.
	ShowPlaylists struct{}
	// Playlist is one thing to do to a playlist, named by what it is called.
	Playlist struct {
		Verb string
		Name string
	}
	// Shuffle asks for everything on the server to play in a random order.
	Shuffle struct{}
	// OpenPlaylist asks for one playlist's tracks.
	OpenPlaylist struct{ ID string }
	// EditPlaylist adds or removes the selected track from the playlist being
	// edited.
	EditPlaylist struct {
		ID     string
		SongID string
		Add    bool
	}
	// ShowWiki asks for the list of commands and what each does.
	ShowWiki struct{}
	// ShowMessages asks for what shanty has said this session. It is handled
	// by the interface itself, which is the only thing that has it.
	ShowMessages struct{}
	// ToggleStar asks for something to be starred or unstarred on the server.
	ToggleStar struct {
		Kind    Kind
		ID      string
		Starred bool
	}
)

// Inputs are what the caller sends back once it has done the work.
type (
	ArtistsLoaded []subsonic.Artist
	ArtistLoaded  subsonic.Artist
	AlbumLoaded   subsonic.Album
	// StarredChanged is everything the server has starred, and the set of
	// identifiers so that every list can mark what is in it.
	StarredChanged struct {
		Results subsonic.Results
		IDs     map[string]bool
	}
	// Confirm asks a question that must be answered before something that
	// cannot be undone is done. Do is emitted if the answer is yes.
	Confirm struct {
		Question string
		Do       tea.Msg
	}
	// PlaylistsLoaded is every playlist the server will show.
	PlaylistsLoaded []subsonic.Playlist
	// PlaylistLoaded is one playlist with its tracks. Editing reports the same
	// message, so the marks follow what the server has.
	PlaylistLoaded subsonic.Playlist
	// EditingPlaylist puts the library lists into the mode where a track can
	// be added to or removed from the named playlist.
	EditingPlaylist subsonic.Playlist
	// SearchLoaded is what a search found.
	SearchLoaded struct {
		Query   string
		Results subsonic.Results
	}
	NowPlaying struct {
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

// Kind is what a starrable thing is. It mirrors the client's own, so that the
// interface names a kind without importing the package that talks to the
// server.
type Kind string

const (
	StarArtist Kind = "artist"
	StarAlbum  Kind = "album"
	StarSong   Kind = "song"
)
