package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// Update applies a message to the model and returns any intent it produces.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeFilter:
			return m.filterKey(msg)
		case modeCommand:
			return m.commandKey(msg)
		}
		return m.key(msg)

	case ArtistsLoaded:
		m.artists, m.loading, m.status = msg, false, ""
		return m, nil
	case ArtistLoaded:
		m.artist, m.screen, m.loading, m.status = subsonic.Artist(msg), ScreenAlbums, false, ""
		m.filter = ""
		m.cursor[ScreenAlbums] = 0
		return m, nil
	case AlbumLoaded:
		m.album, m.screen, m.loading, m.status = subsonic.Album(msg), ScreenTracks, false, ""
		m.filter = ""
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
	case "ctrl+c":
		return m, emit(Quit{})

	case "q":
		// Quitting ends the session and is a command rather than a key
		// (ADR-016). Saying so beats doing nothing for anyone who learned q
		// somewhere else.
		m.status = "type :q to quit"
		return m, nil

	case "/":
		m.mode, m.filter = modeFilter, ""
		return m.moveTo(0), nil

	case ":":
		m.mode, m.line, m.status = modeCommand, "", ""
		return m, nil

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

// filterKey handles a keystroke while the filter is being typed. The list
// narrows as characters arrive. esc abandons the filter and restores the list;
// enter keeps it and returns to normal mode.
func (m Model) filterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode, m.filter = modeNormal, ""
		return m.moveTo(0), nil
	case tea.KeyEnter:
		m.mode = modeNormal
		return m, nil
	case tea.KeyBackspace:
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
		}
		return m.moveTo(0), nil
	case tea.KeyRunes, tea.KeySpace:
		m.filter += string(msg.Runes)
		if msg.Type == tea.KeySpace {
			m.filter += " "
		}
		return m.moveTo(0), nil
	}
	// Movement still works while typing, so a filter can be narrowed and a row
	// chosen without leaving the mode.
	switch msg.String() {
	case "up":
		return m.move(-1), nil
	case "down":
		return m.move(1), nil
	}
	return m, nil
}

// commandKey handles a keystroke while a command is being typed. The list of
// matching commands is shown while typing, tab completes as far as the matches
// agree, enter runs, and esc abandons the line.
func (m Model) commandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode, m.line = modeNormal, ""
		return m, nil
	case tea.KeyTab:
		m.line = complete(m.line)
		return m, nil
	case tea.KeyEnter:
		return m.runCommand()
	case tea.KeyBackspace:
		if m.line != "" {
			m.line = m.line[:len(m.line)-1]
		}
		return m, nil
	case tea.KeyRunes:
		m.line += string(msg.Runes)
		return m, nil
	case tea.KeySpace:
		m.line += " "
		return m, nil
	}
	return m, nil
}

// runCommand leaves the mode and acts on the line, reporting a name that is
// not a command and an argument a command could not use.
func (m Model) runCommand() (tea.Model, tea.Cmd) {
	line := m.line
	m.mode, m.line = modeNormal, ""

	name, arg := split(line)
	if name == "" {
		return m, nil
	}
	c, found := lookup(line)
	if !found {
		m.status = "no such command: " + Sanitise(name)
		return m, nil
	}
	intent, err := c.run(arg)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	return m, emit(intent)
}

// open moves down one screen, or plays the selected track. The album travels
// with the intent.
func (m Model) open() (tea.Model, tea.Cmd) {
	if m.rows() == 0 {
		return m, nil
	}
	at := m.matches()[m.cursor[m.screen]]
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

// back moves up one screen. At the top it does nothing.
func (m Model) back() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenTracks:
		m.screen, m.status, m.filter = ScreenAlbums, "", ""
	case ScreenAlbums:
		m.screen, m.status, m.filter = ScreenArtists, "", ""
	}
	return m, nil
}

func (m Model) move(by int) Model { return m.moveTo(m.cursor[m.screen] + by) }

// moveTo selects row to, clamped to the list.
func (m Model) moveTo(to int) Model {
	rows := m.rows()
	if rows == 0 {
		return m
	}
	to = max(0, min(rows-1, to))
	// The map is replaced rather than written through, because Update must not
	// modify the model it was called on.
	next := make(map[Screen]int, len(m.cursor))
	for k, v := range m.cursor {
		next[k] = v
	}
	next[m.screen] = to
	m.cursor = next
	return m
}

// page is how many rows pgup and pgdown move.
func (m Model) page() int {
	if n := m.height - chromeLines; n > 1 {
		return n
	}
	return 1
}
