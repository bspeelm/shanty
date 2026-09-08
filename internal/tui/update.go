package tui

import (
	"strconv"
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
		m = m.selecting(ScreenAlbums, 0)
		m.filter = ""
		return m, nil
	case AlbumLoaded:
		m.album, m.screen, m.loading, m.status = subsonic.Album(msg), ScreenTracks, false, ""
		m = m.selecting(ScreenTracks, 0)
		m.filter = ""
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
	case Notice:
		m.status = string(msg)
		return m, nil
	case StarredChanged:
		m.starredRows, m.starred = flatten(msg.Results), msg.IDs
		if m.screen == ScreenStarred {
			m = m.selecting(ScreenStarred, min(m.cursor[ScreenStarred], max(0, len(m.starredRows)-1))).ontoARow()
		}
		return m, nil
	case SearchLoaded:
		m.found, m.query = flatten(msg.Results), msg.Query
		m.screen, m.loading, m.status, m.filter = ScreenSearch, false, "", ""
		return m.selecting(ScreenSearch, firstSelectable(m.found)), nil
	case QueueChanged:
		m.queued, m.queuedAt = msg.Tracks, msg.At
		if m.screen == ScreenQueue {
			m = m.selecting(ScreenQueue, min(m.cursor[ScreenQueue], max(0, len(msg.Tracks)-1)))
		}
		return m, nil
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// A pending prefix takes the next key, whatever it is, so an unrecognised
	// second key cancels the prefix rather than acting on its own.
	if m.pending == "g" {
		m.pending = ""
		return m.goTo(key)
	}
	// Digits gather into a count. A leading zero is not a count, because it
	// would make 0 mean something different depending on what came before it.
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' && !(key == "0" && m.count == "") {
		m.count += key
		return m, nil
	}
	// Everything below consumes the count, whether or not it uses it.
	repeat, hadCount := m.take()

	switch key {
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

	case "g":
		m.pending = "g"
		return m, nil

	case "%":
		if !hadCount {
			m.status = "type a percentage first, as in 50%"
			return m, nil
		}
		return m, emit(SeekToPercent(min(100, repeat)))

	case "up", "k":
		return m.move(-repeat), nil
	case "down", "j":
		return m.move(repeat), nil
	case "home":
		return m.moveTo(0).ontoARow(), nil
	case "end", "G":
		return m.moveTo(m.rows() - 1), nil
	case "pgup":
		return m.move(-m.page() * repeat), nil
	case "pgdown":
		return m.move(m.page() * repeat), nil

	case "*":
		return m.starSelected()
	case "a":
		return m.queueSelected(false)
	case "A":
		return m.queueSelected(true)

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

// take returns the count typed before this key, defaulting to one, and clears
// it. The second result says whether a count was actually typed, which is what
// separates 50% from a bare %.
func (m *Model) take() (int, bool) {
	if m.count == "" {
		return 1, false
	}
	n, err := strconv.Atoi(m.count)
	m.count = ""
	if err != nil || n < 1 {
		return 1, false
	}
	return n, true
}

// goTo handles the key after g. The queue, playlists and starred screens are
// bound before they exist, so the prefix is whole and says what is missing
// rather than doing nothing.
func (m Model) goTo(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "g":
		return m.moveTo(0).ontoARow(), nil
	case "a":
		m.screen, m.status, m.filter = ScreenArtists, "", ""
		return m, nil
	case "q":
		m.screen, m.status, m.filter = ScreenQueue, "", ""
		return m.selecting(ScreenQueue, max(0, min(len(m.queued)-1, m.queuedAt))), nil
	case "p":
		m.status = "the playlists screen is not built yet"
		return m, nil
	case "s":
		m.screen, m.status, m.filter = ScreenStarred, "", ""
		return m.selecting(ScreenStarred, firstSelectable(m.starredRows)), nil
	}
	return m, nil
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
	case ScreenQueue:
		return m, emit(JumpTo(at))
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[at]
		switch r.kind {
		case kindArtist:
			m.loading, m.status = true, "opening "+Sanitise(r.name)
			return m, emit(OpenArtist{ID: r.id})
		case kindAlbum:
			m.loading, m.status = true, "opening "+Sanitise(r.name)
			return m, emit(OpenAlbum{ID: r.id})
		case kindSong:
			return m, emit(PlayFrom{Album: r.album, Index: 0})
		}
		return m, nil
	}
	return m, nil
}

// queueSelected asks for the selected track to be queued, either after the one
// playing or at the end. It works on the track list, which is the only screen
// where a row is one track.
func (m Model) queueSelected(next bool) (tea.Model, tea.Cmd) {
	if m.screen != ScreenQueue && m.screen != ScreenTracks {
		m.status = "there is nothing to queue here; open an album first"
		return m, nil
	}
	if m.screen == ScreenQueue {
		m.status = "that is already in the queue"
		return m, nil
	}
	if m.rows() == 0 {
		return m, nil
	}
	at := m.matches()[m.cursor[ScreenTracks]]
	if next {
		return m, emit(PlayNext{Album: m.album, Index: at})
	}
	return m, emit(Enqueue{Album: m.album, Index: at})
}

// starSelected asks for the starred state of the selected row to be turned
// over. Every list holds something that can be starred, so this works on all
// of them.
func (m Model) starSelected() (tea.Model, tea.Cmd) {
	if m.rows() == 0 {
		return m, nil
	}
	at := m.matches()[m.cursor[m.screen]]
	kind, id := Kind(""), ""
	switch m.screen {
	case ScreenArtists:
		kind, id = StarArtist, m.artists[at].ID
	case ScreenAlbums:
		kind, id = StarAlbum, m.artist.Albums[at].ID
	case ScreenTracks:
		kind, id = StarSong, m.album.Songs[at].ID
	case ScreenQueue:
		kind, id = StarSong, m.queued[at].ID
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[at]
		switch r.kind {
		case kindArtist:
			kind, id = StarArtist, r.id
		case kindAlbum:
			kind, id = StarAlbum, r.id
		case kindSong:
			kind, id = StarSong, r.id
		}
	}
	if id == "" {
		return m, nil
	}
	return m, emit(ToggleStar{Kind: kind, ID: id, Starred: !m.starred[id]})
}

// back moves up one screen. At the top it does nothing.
func (m Model) back() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenTracks:
		m.screen, m.status, m.filter = ScreenAlbums, "", ""
	case ScreenAlbums:
		m.screen, m.status, m.filter = ScreenArtists, "", ""
	case ScreenQueue, ScreenSearch, ScreenStarred:
		// Both are reached from anywhere, so back leaves for the one screen
		// that is always there rather than wherever it was opened from.
		m.screen, m.status, m.filter = ScreenArtists, "", ""
	}
	return m, nil
}

func (m Model) move(by int) Model {
	next := m.moveTo(m.cursor[m.screen] + by)
	// A heading is a row nothing can be done with, so the cursor carries on
	// past it in the direction it was already going.
	if m.isGrouped() && by != 0 {
		dir := 1
		if by < 0 {
			dir = -1
		}
		return next.selecting(m.screen, stepOver(m.matchedResults(), next.cursor[m.screen], dir))
	}
	return next
}

// ontoARow moves the cursor forward off a heading. Only the top of a search
// screen can be one: a heading is written only when its group has something in
// it, so the last row is always a result. Every other screen has no headings
// at all and is left alone.
func (m Model) ontoARow() Model {
	if !m.isGrouped() {
		return m
	}
	return m.selecting(m.screen, stepOver(m.matchedResults(), m.cursor[m.screen], 1))
}

// matchedResults is the rows the search screen is showing, which is what the
// cursor moves through once a filter has narrowed them.
func (m Model) matchedResults() []result {
	rows := m.grouped()
	out := make([]result, 0, len(rows))
	for _, i := range m.matches() {
		out = append(out, rows[i])
	}
	return out
}

// moveTo selects row to, clamped to the list.
func (m Model) moveTo(to int) Model {
	rows := m.rows()
	if rows == 0 {
		return m
	}
	return m.selecting(m.screen, max(0, min(rows-1, to)))
}

// selecting returns a model with the cursor of one screen moved.
//
// The map is replaced rather than written through, because Model is a value
// and Update must not modify the one it was called on. Writing through it
// would reach every copy, including ones already rendered.
func (m Model) selecting(screen Screen, to int) Model {
	next := make(map[Screen]int, len(m.cursor)+1)
	for k, v := range m.cursor {
		next[k] = v
	}
	next[screen] = to
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
