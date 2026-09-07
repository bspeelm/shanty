package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// Update is the whole interaction. It changes the model and returns an intent
// for anything that needs the world touched.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)

	case ArtistsLoaded:
		m.artists, m.loading, m.status = msg, false, ""
		return m, nil
	case ArtistLoaded:
		m.artist, m.screen, m.loading, m.status = subsonic.Artist(msg), ScreenAlbums, false, ""
		m.cursor[ScreenAlbums] = 0
		return m, nil
	case AlbumLoaded:
		m.album, m.screen, m.loading, m.status = subsonic.Album(msg), ScreenTracks, false, ""
		m.cursor[ScreenTracks] = 0
		return m, nil

	case NowPlaying:
		m.nowPlaying, m.nowArtist, m.duration = msg.Title, msg.Artist, msg.Duration
		m.position, m.paused = 0, false
		return m, nil
	case Progress:
		m.position = time.Duration(msg)
		return m, nil
	case PausedChanged:
		m.paused = bool(msg)
		return m, nil
	case VolumeChanged:
		m.volume = int(msg)
		return m, nil
	case Failed:
		m.loading, m.status = false, msg.Message
		return m, nil
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, emit(Quit{})

	case "up", "k":
		return m.move(-1), nil
	case "down", "j":
		return m.move(1), nil
	case "home", "g":
		return m.moveTo(0), nil
	case "end", "G":
		return m.moveTo(m.rows() - 1), nil
	case "pgup":
		return m.move(-m.page()), nil
	case "pgdown":
		return m.move(m.page()), nil

	case "enter", "l", "right":
		return m.open()
	case "esc", "backspace", "h", "left":
		return m.back()

	case " ":
		return m, emit(TogglePause{})
	case "n":
		return m, emit(SkipNext{})
	case "p":
		return m, emit(SkipPrev{})
	case "]":
		return m, emit(SeekBy{By: 10 * time.Second})
	case "[":
		return m, emit(SeekBy{By: -10 * time.Second})
	case "+", "=":
		return m, emit(VolumeBy{Delta: 5})
	case "-", "_":
		return m, emit(VolumeBy{Delta: -5})
	}
	return m, nil
}

// open descends one level, or plays. The intent carries the album by value so
// the handler does not have to ask this package for state it already sent.
func (m Model) open() (tea.Model, tea.Cmd) {
	if m.rows() == 0 {
		return m, nil
	}
	at := m.cursor[m.screen]
	switch m.screen {
	case ScreenArtists:
		m.loading, m.status = true, "opening "+Sanitise(m.artists[at].Name)
		return m, emit(OpenArtist{ID: m.artists[at].ID})
	case ScreenAlbums:
		m.loading, m.status = true, "opening "+Sanitise(m.artist.Albums[at].Name)
		return m, emit(OpenAlbum{ID: m.artist.Albums[at].ID})
	case ScreenTracks:
		return m, emit(PlayFrom{Album: m.album, Index: at})
	}
	return m, nil
}

// back climbs one level. From the top it does nothing rather than quitting:
// leaving a music player by pressing left twice is a surprise nobody wants
// mid-album.
func (m Model) back() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenTracks:
		m.screen, m.status = ScreenAlbums, ""
	case ScreenAlbums:
		m.screen, m.status = ScreenArtists, ""
	}
	return m, nil
}

func (m Model) move(by int) Model { return m.moveTo(m.cursor[m.screen] + by) }

// moveTo clamps. The caller is a key that can be held down, and there is no
// useful error for pressing down at the bottom of a list.
func (m Model) moveTo(to int) Model {
	rows := m.rows()
	if rows == 0 {
		return m
	}
	to = max(0, min(rows-1, to))
	// The map is shared with the model this one was copied from, so it is
	// replaced rather than written through: an Update must not change the
	// model it was called on.
	next := make(map[Screen]int, len(m.cursor))
	for k, v := range m.cursor {
		next[k] = v
	}
	next[m.screen] = to
	m.cursor = next
	return m
}

// page is how far pgup and pgdown move, leaving the chrome out of the count.
func (m Model) page() int {
	if n := m.height - chromeLines; n > 1 {
		return n
	}
	return 1
}
