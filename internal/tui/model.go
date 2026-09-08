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
)

// Screens lists every screen.
var Screens = []Screen{ScreenArtists, ScreenAlbums, ScreenTracks, ScreenQueue, ScreenSearch, ScreenStarred}

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
func (m Model) Screen() Screen   { return m.screen }
func (m Model) Filter() string   { return m.filter }
func (m Model) Filtering() bool  { return m.mode == modeFilter }
func (m Model) Commanding() bool { return m.mode == modeCommand }
func (m Model) Line() string     { return m.line }
func (m Model) Pending() string  { return m.pending }
func (m Model) Count() string    { return m.count }
func (m Model) Cursor() int      { return m.cursor[m.screen] }
func (m Model) Status() string   { return m.status }
func (m Model) Paused() bool     { return m.paused }

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
	}
	return ""
}
