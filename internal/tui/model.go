package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// mode is the kind of input the interface is taking.
type mode int

const (
	// modeNormal is the default: keys act rather than being typed.
	modeNormal mode = iota
	// modeFilter narrows the list on screen as characters are typed.
	modeFilter
	// modeCommand takes a line naming a command to run.
	modeCommand
	// modeConfirm waits for yes or no to something that cannot be undone.
	modeConfirm
)

// Screen identifies one of the views.
type Screen int

const (
	ScreenArtists Screen = iota
	ScreenAlbums
	ScreenTracks
	ScreenQueue
	ScreenSearch
	ScreenStarred
	ScreenMessages
	ScreenPlaylists
	ScreenPlaylist
	ScreenWiki
)

// Screens lists every screen.
var Screens = []Screen{ScreenArtists, ScreenAlbums, ScreenTracks, ScreenQueue,
	ScreenSearch, ScreenStarred, ScreenMessages, ScreenPlaylists, ScreenPlaylist, ScreenWiki}

func (s Screen) String() string {
	switch s {
	case ScreenArtists:
		return "artists"
	case ScreenAlbums:
		return "albums"
	case ScreenTracks:
		return "tracks"
	case ScreenQueue:
		return "queue"
	case ScreenSearch:
		return "search"
	case ScreenStarred:
		return "starred"
	case ScreenMessages:
		return "messages"
	case ScreenPlaylists:
		return "playlists"
	case ScreenPlaylist:
		return "playlist"
	case ScreenWiki:
		return "wiki"
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
	// art is the cover for album, already rendered, one string per row.
	art []string
	// artBlank is an empty block the shape of a cover. The rows are held from
	// the moment an album with a cover opens, so that the list does not move
	// when the picture arrives a moment later.
	artBlank []string
	// clearArt takes away a picture the terminal is still holding. It is kept
	// once it is known, because it describes the terminal rather than one
	// album, and it is sent on every screen that shows no art.
	clearArt string

	// queued is what is playing and what follows it, and queuedAt is the
	// position in it. The caller owns the queue; this is a copy to draw.
	queued   []queue.Track
	queuedAt int

	// found is what the last search returned, flattened into rows with a
	// heading before each group. query is what was asked for.
	found []result
	query string

	// starredRows is what the server has starred, in the same shape, and
	// starred says which identifiers those are so every list can mark them.
	starredRows []result
	starred     map[string]bool

	// cursor holds the selected row for each screen, as an index into the
	// filtered list rather than the whole one.
	cursor map[Screen]int

	mode mode
	// filter narrows the current screen to rows containing it.
	filter string
	// pending holds the first key of a two-key binding.
	pending string
	// count holds the digits typed before a movement, as in 3j.
	count string
	// line is the command being typed, without its leading colon.
	line string

	nowPlaying string
	nowArtist  string
	position   time.Duration
	duration   time.Duration
	paused     bool
	volume     int

	status  string
	loading bool

	// said is every message shown this session, oldest first, so that one
	// which scrolled past can be read again. It is kept in memory only.
	said []message
	// playlists is every playlist the server will show, and playlist is the
	// one being looked at.
	playlists []subsonic.Playlist
	playlist  subsonic.Playlist
	// editing is the playlist the library lists are adding to, and inPlaylist
	// says which tracks are already in it so they can be marked.
	editing    subsonic.Playlist
	inPlaylist map[string]bool
	// asking is what modeConfirm is waiting to hear about, and agreed is what
	// to do if the answer is yes.
	asking string
	agreed tea.Msg
	// keep is what was selected when a reload was asked for, so the cursor can
	// go back to it once the answer arrives.
	keep string
	// topic is the command :wiki is explaining. Empty means it is showing the
	// list of them.
	topic string
	// keys is what each key does. It is nil until something replaces the
	// default bindings.
	keys map[string]Action
	// now reads the clock. It is a field so that a test can pin what a
	// message is stamped with.
	now func() time.Time
}

// message is one thing shanty said, and when.
type message struct {
	at   time.Time
	text string
}

// remembered is how many messages are kept. Older ones go, because the recent
// ones are what somebody looking for what just happened wants.
const remembered = 200

// New returns a model with nothing loaded.
func New() Model {
	return Model{
		cursor:  map[Screen]int{},
		volume:  100,
		loading: true,
		status:  "loading the library",
		now:     time.Now,
	}
}

func (m Model) Init() tea.Cmd { return nil }

// Screen, Cursor, Status and Paused report the model’s state.
func (m Model) Screen() Screen   { return m.screen }
func (m Model) Filter() string   { return m.filter }
func (m Model) Filtering() bool  { return m.mode == modeFilter }
func (m Model) Commanding() bool { return m.mode == modeCommand }
func (m Model) Confirming() bool { return m.mode == modeConfirm }
func (m Model) Line() string     { return m.line }
func (m Model) Pending() string  { return m.pending }
func (m Model) Count() string    { return m.count }
func (m Model) Cursor() int      { return m.cursor[m.screen] }
func (m Model) Status() string   { return m.status }
func (m Model) Paused() bool     { return m.paused }

// Editing is the playlist the library lists are adding to, if any.
func (m Model) Editing() subsonic.Playlist { return m.editing }

// Artist and Album are what the screens below the artist list are showing.
func (m Model) Artist() subsonic.Artist { return m.artist }
func (m Model) Album() subsonic.Album   { return m.album }

// Position is how far into the track playback has reached.
func (m Model) Position() time.Duration { return m.position }

// rows is the number of items the current screen shows, after filtering.
func (m Model) rows() int { return len(m.matches()) }

// matches returns the indexes into the current screen's full list of the rows
// the filter selects, in order. Without a filter it is every row.
func (m Model) matches() []int {
	n := m.allRows()
	out := make([]int, 0, n)
	needle := strings.ToLower(m.filter)
	for i := range n {
		if needle == "" {
			out = append(out, i)
			continue
		}
		// A search heading names a group rather than a thing in the library,
		// so narrowing the list drops it. Otherwise typing "tracks" would
		// leave a screen holding nothing but the word TRACKS.
		if m.isGrouped() && !m.grouped()[i].selectable() {
			continue
		}
		if strings.Contains(strings.ToLower(m.rowName(i)), needle) {
			out = append(out, i)
		}
	}
	return out
}

// allRows is how many items the current screen has before filtering.
func (m Model) allRows() int {
	switch m.screen {
	case ScreenArtists:
		return len(m.artists)
	case ScreenAlbums:
		return len(m.artist.Albums)
	case ScreenTracks:
		return len(m.album.Songs)
	case ScreenQueue:
		return len(m.queued)
	case ScreenSearch, ScreenStarred:
		return len(m.grouped())
	case ScreenMessages:
		return len(m.said)
	case ScreenPlaylists:
		return len(m.playlists)
	case ScreenPlaylist:
		return len(m.playlist.Songs)
	case ScreenWiki:
		if m.topic != "" {
			return len(helpLines(m.topic, m.viewWidth()))
		}
		return len(commands)
	}
	return 0
}

// rowName is the text the filter matches against, for row i of the unfiltered
// list.
func (m Model) rowName(i int) string {
	switch m.screen {
	case ScreenArtists:
		return m.artists[i].Name
	case ScreenAlbums:
		return m.artist.Albums[i].Name
	case ScreenTracks:
		return m.album.Songs[i].Title
	case ScreenQueue:
		return m.queued[i].Title + " " + m.queued[i].Artist
	case ScreenSearch, ScreenStarred:
		return m.grouped()[i].name
	case ScreenMessages:
		return m.said[m.saidAt(i)].text
	case ScreenPlaylists:
		return m.playlists[i].Name
	case ScreenPlaylist:
		return m.playlist.Songs[i].Title
	case ScreenWiki:
		if m.topic != "" {
			return helpLines(m.topic, m.viewWidth())[i]
		}
		return commands[i].name + " " + commands[i].summary
	}
	return ""
}

// saidAt maps a row of the messages screen to a message. The newest is at the
// top, because what just happened is what somebody is looking for.
func (m Model) saidAt(row int) int { return len(m.said) - 1 - row }

// remember keeps a message so that it can be read after it has been replaced.
func (m Model) remember(text string) Model {
	if strings.TrimSpace(text) == "" {
		return m
	}
	at := time.Now()
	if m.now != nil {
		at = m.now()
	}
	said := make([]message, 0, len(m.said)+1)
	said = append(said, m.said...)
	said = append(said, message{at: at, text: text})
	if len(said) > remembered {
		said = said[len(said)-remembered:]
	}
	m.said = said
	return m
}
