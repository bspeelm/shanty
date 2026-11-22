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
	// OpenArtist, OpenAlbum and OpenPlaylist ask for something to be fetched
	// and shown. Ask numbers the request, and comes back in Opened or Failed.
	OpenArtist struct {
		ID  string
		Ask int
	}
	OpenAlbum struct {
		ID  string
		Ask int
	}
	PlayFrom struct {
		Album subsonic.Album
		Index int
		// Playlist is the identifier of the playlist the tracks came from, and
		// is empty when they came from an album. Both arrive here as an Album,
		// so without this the caller cannot tell which it was handed.
		Playlist string
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
	// ShowArt asks for the cover of the album on screen, as large as it fits.
	ShowArt struct{}
	// ShowVisualiser asks for the visualiser to fill the screen. The interface
	// draws it; the caller sends Heard readings for as long as it is showing.
	// SkySeed is the number the visualiser's scenes are drawn from, sent once
	// when the program starts.
	ShowVisualiser struct{}
	SkySeed        uint64
	// AutoVolume turns loudness levelling on or off, and is remembered.
	AutoVolume struct{ On bool }
	// ToggleAutoVolume asks for whichever of the two it is not, because the
	// caller holds the setting and the interface does not.
	ToggleAutoVolume struct{}
	Quit             struct{}
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
	// QueueThis asks for every track of an artist, an album or a playlist to
	// be added to the queue in its own order, after the track playing when
	// Next is set and at the end otherwise.
	QueueThis struct {
		Scope Scope
		ID    string
		Next  bool
	}
	// JumpTo asks for the queue to move to one of its own tracks.
	JumpTo int
	// RemoveQueued asks for those tracks to be taken out of the queue, and
	// MoveQueued for the track at From to be put at To. Each track is named
	// by its place and by the identifier the screen showed there, and nothing
	// is done if another track is at one of those places now.
	RemoveQueued struct{ Tracks []Queued }
	Queued       struct {
		At int
		ID string
	}
	MoveQueued struct {
		From, To int
		ID       string
	}
	// FetchCover asks for the cover of an album that is on screen without
	// one.
	FetchCover struct{ Album subsonic.Album }
	// Search asks the server for anything matching the query.
	Search string
	// Resume asks for the queue the server is holding to be taken up.
	Resume struct{}
	// Reload asks for a screen to be fetched from the server again. Ask
	// numbers the reload, and comes back in the Reloaded or the Failed that
	// answers it. Screen is the screen to fetch, and ID the artist, album or
	// playlist it shows. Sent to the interface with no number, it begins a
	// reload the caller is making.
	Reload struct {
		Ask    int
		Screen Screen
		ID     string
	}
	// Scan asks the server to look at its music folder again.
	Scan struct{}
	// FetchPlaylists and FetchEveryAlbum ask for what a screen the interface
	// has already moved to shows. Nothing is sent back but the list.
	FetchPlaylists  struct{}
	FetchEveryAlbum struct{}
	// Playlist is one thing to do to a playlist, named by what it is called.
	Playlist struct {
		Verb string
		Name string
	}
	// Shuffle asks for everything on the server to play in a random order.
	Shuffle struct{}
	// ShuffleThis asks for one collection to play in a random order. Scope
	// says what kind it is and ID names the one meant, where the scope has
	// more than one.
	ShuffleThis struct {
		Scope Scope
		ID    string
	}
	// ShuffleSongs asks for these tracks to play in a random order. What says
	// what they are, in words for the message that follows.
	ShuffleSongs struct {
		Songs []subsonic.Song
		What  string
	}
	OpenPlaylist struct {
		ID  string
		Ask int
	}
	// EditPlaylist adds or removes the selected track from the playlist being
	// edited.
	EditPlaylist struct {
		ID     string
		SongID string
		Add    bool
		// At is which of the playlist's tracks to remove, where the caller
		// knows. Removing is by position, and a playlist may hold the same
		// track twice, so the position is the difference between taking out
		// the one somebody pointed at and taking out another copy of it.
		//
		// It is -1 from the library, where the position in the playlist is
		// not on screen to be pointed at.
		At int
	}
	// DeletePlaylist asks for the playlist on screen to be deleted, by
	// identifier. Tracks is how many it holds, so the question can say.
	DeletePlaylist struct {
		ID     string
		Name   string
		Tracks int
	}
	// ReorderPlaylist asks for a playlist's tracks to be put in this order.
	// Was is the order the screen was drawn from: the caller compares it to
	// what the server holds and writes nothing if they differ, because the
	// only call that can express an order replaces the whole playlist.
	//
	// Ask numbers the request, and comes back in the Failed that refuses it.
	ReorderPlaylist struct {
		ID    string
		Name  string
		Songs []string
		Was   []string
		Ask   int
	}
	// RenamePlaylist gives a playlist a new name. It carries the identifier
	// rather than the old name, because the playlist is the one on screen and
	// a server's playlist names are not unique.
	RenamePlaylist struct {
		ID   string
		Name string
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
	// AlbumsLoaded is every album the server will list.
	AlbumsLoaded []subsonic.Album
	AlbumLoaded  subsonic.Album
	// FullArt is a cover rendered to fill the screen, in answer to ShowArt.
	// It is shown over everything until a key takes it away.
	FullArt []string
	// CoverArt is an album's art, already rendered for this terminal. The
	// interface is handed lines rather than an image, because deciding what a
	// terminal can show and fetching the picture are both I/O.
	CoverArt struct {
		// AlbumID says which album the art is for. One that arrives after the
		// screen has moved on is dropped rather than drawn over the new album.
		AlbumID string
		Lines   []string
		// Clear is sent to the terminal when the art is no longer on screen.
		// Some protocols draw over the interface rather than into it, and what
		// they draw stays until it is taken away.
		Clear string
	}
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
	// ArtistLoaded, AlbumLoaded and PlaylistLoaded sent bare are shown.
	PlaylistLoaded subsonic.Playlist
	// Opened answers the open numbered Ask with what was loaded, which is an
	// ArtistLoaded, an AlbumLoaded or a PlaylistLoaded. It is shown only if
	// that open is the one being waited for, and dropped otherwise.
	Opened struct {
		Ask    int
		Loaded tea.Msg
	}
	// Reloaded answers the reload numbered Ask with what was fetched again,
	// and with More where two lists were fetched. Both replace what is held
	// and never move the screen. Only the answer whose number is waited for
	// ends the wait and puts the cursor back.
	Reloaded struct {
		Ask    int
		Loaded tea.Msg
		More   tea.Msg
	}
	// Saved answers the order numbered Ask that was written to a playlist,
	// with the playlist as the server now has it.
	Saved struct {
		Ask      int
		Playlist subsonic.Playlist
	}
	// PlaylistChanged is a playlist as the server has it after a rename, a
	// new order, or a track added or removed. It replaces the copy held and
	// the edit marks, and never moves the screen.
	PlaylistChanged subsonic.Playlist
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
		// From is the identifier of the playlist the queue was built from, and
		// is empty when it was built from anything else.
		From string
	}
	Progress      time.Duration
	PausedChanged bool
	VolumeChanged int
	// Failed carries a message to display. It is sanitised like any other text,
	// because it may have come from the server.
	//
	// Ask is the number of the open that failed, and zero when the failure
	// is not about an open.
	Failed struct {
		Message string
		Ask     int
	}
	// Notice carries a message about something in progress. Failed would clear
	// the loading flag, which is wrong while a screen is still arriving.
	Notice string
)

func emit(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }

// Scope is what a shuffle takes its tracks from.
type Scope string

const (
	// ScopeArtist is every track on every album by one artist.
	ScopeArtist Scope = "artist"
	// ScopeAlbum is one album.
	ScopeAlbum Scope = "album"
	// ScopePlaylist is one playlist.
	ScopePlaylist Scope = "playlist"
	// ScopeStarred is every starred track, and every track of every starred
	// album and artist.
	ScopeStarred Scope = "starred"
	// ScopeQueue is the tracks in the queue after the one playing.
	ScopeQueue Scope = "queue"
)

// Scopes lists every scope.
var Scopes = []Scope{ScopeArtist, ScopeAlbum, ScopePlaylist, ScopeStarred, ScopeQueue}

// Kind is what a starrable thing is. It mirrors the client's own, so that the
// interface names a kind without importing the package that talks to the
// server.
type Kind string

const (
	StarArtist Kind = "artist"
	StarAlbum  Kind = "album"
	StarSong   Kind = "song"
)
