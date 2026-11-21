package tui

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/sky"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// Update applies a message to the model and returns any intent it produces.
// An arrangement being made of a playlist is forgotten when the message leaves
// that playlist's screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	after, cmd := m.update(msg)
	next := after.(Model)
	if next.reordering && next.screen != ScreenPlaylist {
		next.reordering, next.order, next.drawnFrom = false, nil, nil
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC && (m.mode == modeFilter || m.mode == modeCommand) {
			return m, emit(Quit{})
		}
		switch m.mode {
		case modeFilter:
			return m.filterKey(msg)
		case modeCommand:
			return m.commandKey(msg)
		case modeConfirm:
			return m.confirmKey(msg.String())
		}
		return m.key(msg)

	case ArtistsLoaded:
		m.artists, m.loading, m.status = msg, false, ""
		return m.reloaded(ScreenArtists, plural(len(msg), "artist")), nil
	case ArtistLoaded:
		artist := subsonic.Artist(msg)
		if m.keep != "" && m.screen == ScreenAlbums && artist.ID == m.artist.ID {
			m.artist, m.loading = artist, false
			return m.reloaded(ScreenAlbums, plural(len(artist.Albums), "album")), nil
		}
		m, wanted := m.arrived(artist.ID)
		if !wanted {
			return m, nil
		}
		m = m.show(ScreenAlbums)
		m.artist, m.loading = artist, false
		return m.selecting(ScreenAlbums, 0), nil
	case AlbumLoaded:
		album := subsonic.Album(msg)
		reload := m.keep != "" && m.screen == ScreenTracks && album.ID == m.album.ID
		m, wanted := m.arrived(album.ID)
		if !wanted && !reload {
			return m, nil
		}
		if album.ID != m.album.ID {
			// An album with a cover holds the rows for it straight away. The
			// picture is a request away and the list would otherwise jump
			// when it lands.
			m.art = nil
			if album.CoverArt != "" {
				m.art = m.artBlank
			}
		}
		if reload {
			m.album, m.loading = album, false
			return m.reloaded(ScreenTracks, plural(len(album.Songs), "track")), nil
		}
		m = m.show(ScreenTracks)
		m.album, m.loading = album, false
		return m.selecting(ScreenTracks, 0), nil

	case NowPlaying:
		m.nowPlaying, m.nowArtist, m.duration = msg.Title, msg.Artist, msg.Duration
		m.position = 0
		return m, nil
	case Progress:
		m.position = time.Duration(msg)
		return m, nil
	case PausedChanged:
		m.paused = bool(msg)
		// The sky holds while the music is paused and nothing asks for the
		// next frame, so resuming has to ask again.
		if m.showSky && !m.paused {
			return m, m.turnSky()
		}
		return m, nil
	case VolumeChanged:
		m.volume = int(msg)
		return m, nil
	case Failed:
		m.loading, m.status, m.statusFailed = false, msg.Message, true
		m.keep, m.wanted = "", ""
		return m.remember(msg.Message), nil
	case Notice:
		m.status, m.statusFailed = string(msg), false
		return m.remember(string(msg)), nil
	case StarredChanged:
		m.starredRows, m.starred = flatten(msg.Results), msg.IDs
		if m.screen == ScreenStarred && m.keep != "" {
			m.loading = false
			return m.reloaded(ScreenStarred, plural(len(msg.IDs), "starred thing")).ontoARow(), nil
		}
		if m.screen == ScreenStarred {
			m = m.selecting(ScreenStarred, min(m.cursor[ScreenStarred], max(0, len(m.starredRows)-1))).ontoARow()
		}
		return m, nil
	case SearchLoaded:
		m = m.show(ScreenSearch)
		m.found, m.query, m.loading = flatten(msg.Results), msg.Query, false
		return m.selecting(ScreenSearch, firstSelectable(m.found)), nil
	case Reload:
		// What was selected is remembered, so a reload leaves the cursor
		// where it was rather than at the top of a list that mostly did not
		// change.
		m.keep, m.loading, m.status = m.selectedID(), true, "asking the server again"
		if m.keep == "" {
			m.keep = " " // nothing selected, but a reload is still under way
		}
		return m, nil
	case PlaylistsLoaded:
		m.playlists, m.loading, m.status = msg, false, ""
		if msg == nil {
			// An answer holding no playlists is still an answer.
			m.playlists = []subsonic.Playlist{}
		}
		m = m.within(ScreenPlaylists, len(msg))
		if m.screen != ScreenPlaylists {
			return m, nil
		}
		return m.reloaded(ScreenPlaylists, plural(len(msg), "playlist")), nil
	case PlaylistLoaded:
		loaded := subsonic.Playlist(msg)
		// The same message reports an edit, so the marks follow the server
		// rather than what was pressed.
		if m.editing.ID == loaded.ID && m.editing.ID != "" {
			m.editing = loaded
			m.inPlaylist = holding(loaded)
			if m.screen != ScreenPlaylist {
				return m, nil
			}
		}
		if m.screen == ScreenPlaylist && loaded.ID == m.playlist.ID {
			m.playlist, m.loading, m.status = loaded, false, ""
			return m.reloaded(ScreenPlaylist, plural(len(loaded.Songs), "track")), nil
		}
		m, wanted := m.arrived(loaded.ID)
		if !wanted {
			return m, nil
		}
		m = m.show(ScreenPlaylist)
		m.playlist, m.loading = loaded, false
		return m.selecting(ScreenPlaylist, 0), nil
	case FullArt:
		m.bigArt = msg
		// A cover filling the screen and the one above the track list are the
		// same album, so the title row does not change between them. The
		// renderer writes only rows that changed, so the row carrying the
		// sequence that takes the old picture away would never be written and
		// both pictures would be on screen. A repaint writes every row.
		return m, tea.ClearScreen
	case CoverArt:
		// Art for an album that is no longer open is dropped. A slow request
		// must not draw one album's cover over another's tracks.
		if msg.Clear != "" {
			m.clearArt = msg.Clear
		}
		if len(msg.Lines) > 0 {
			m.artBlank = blankLike(msg.Lines)
		}
		if msg.AlbumID == m.album.ID && msg.AlbumID != "" {
			m.art = msg.Lines
		}
		return m, nil
	case EditingPlaylist:
		// Editing shows the library, because that is where the music is.
		m.editing, m.inPlaylist = subsonic.Playlist(msg), holding(subsonic.Playlist(msg))
		m = m.show(ScreenArtists)
		m.loading = false
		m.status = editingHint(m.editing.Name)
		return m.remember(m.status), nil
	case Confirm:
		m.mode, m.asking, m.agreed = modeConfirm, msg.Question, msg.Do
		return m, nil
	case ShowPlaylists:
		m = m.show(ScreenPlaylists)
		if m.playlists == nil {
			m.loading = true
		}
		return m, nil
	case ShowEveryAlbum:
		m = m.show(ScreenEveryAlbum)
		if len(m.albums) == 0 {
			m.loading = true
		}
		return m, nil
	case AlbumsLoaded:
		m.albums, m.loading, m.status = msg, false, ""
		m = m.within(ScreenEveryAlbum, len(msg))
		if m.screen != ScreenEveryAlbum {
			return m, nil
		}
		return m.reloaded(ScreenEveryAlbum, plural(len(msg), "album")), nil
	case ShowVisualiser:
		return m.openSky()
	case Heard:
		if m.sky != nil {
			m.sky.Hear(sky.Reading(msg))
		}
		return m, nil
	case Foresight:
		if m.sky != nil {
			m.sky.Foresee(skyTrack(msg.Title, msg.Artist), msg.Plan)
		}
		return m, nil
	case SkySeed:
		m.skySeed = uint64(msg)
		return m, nil
	case skyTurned:
		// A tick that arrives after the overlay was dismissed, or while the
		// music is paused, is the last one: nothing asks for another.
		if !m.showSky {
			return m, nil
		}
		dt := skyStep.Seconds()
		if !m.skyLast.IsZero() && !msg.at.IsZero() {
			dt = min(max(msg.at.Sub(m.skyLast).Seconds(), 0), 0.1)
		}
		m.skyLast = msg.at
		w, h := m.skySize()
		m.sky.Step(w, h, dt, msg.at, skyTrack(m.nowPlaying, m.nowArtist))
		if m.paused {
			return m, nil
		}
		return m, m.turnSky()
	case ShowWiki:
		m = m.show(ScreenWiki)
		m.topic = ""
		return m.selecting(ScreenWiki, 0), nil
	case ShowMessages:
		return m.show(ScreenMessages).selecting(ScreenMessages, 0), nil
	case QueueChanged:
		m.queued, m.queuedAt, m.queuedFrom = msg.Tracks, msg.At, msg.From
		if m.screen == ScreenQueue {
			m = m.selecting(ScreenQueue, min(m.cursor[ScreenQueue], max(0, len(msg.Tracks)-1)))
		}
		return m, nil
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// A key clears the message on the last row, unless the message is about
	// something still arriving.
	if !m.loading {
		m.status, m.statusFailed = "", false
	}

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
	repeat, counted := m.take()

	action, bound := m.bound()[key]
	if (m.showSky || len(m.bigArt) > 0) && !overCover[action.Name] {
		// The list underneath cannot be seen, so nothing acts on it. A cover
		// goes away on any key that does not work the player.
		if !m.showSky {
			m.bigArt = nil
			return m, tea.ClearScreen
		}
		return m, nil
	}
	if !bound {
		return m, nil
	}
	return action.do(m, repeat, counted)
}

// overCover is the actions that work while the visualiser or a cover fills the
// screen: leaving it, the player, the command line and quitting.
var overCover = map[string]bool{
	"back": true, "command": true, "pause": true, "next": true, "previous": true,
	"seek-forward": true, "seek-back": true, "seek-percent": true,
	"volume-up": true, "volume-down": true, "quit": true, "quit-hint": true,
}

// bound is the key to action map in force, which is the default one until
// something replaces it.
func (m Model) bound() map[string]Action {
	if m.keys != nil {
		return m.keys
	}
	return binding(Actions())
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
	m.mode, m.line, m.statusFailed = modeNormal, "", false

	name, arg := split(line)
	if name == "" {
		return m, nil
	}
	c, found := lookup(line)
	if !found {
		return m.refuse("no such command: " + Sanitise(name)), nil
	}
	intent, err := c.run(arg)
	if err != nil {
		return m.refuse(err.Error()), nil
	}
	// A rename's argument is the new name, and the playlist is the one on
	// screen -- the rule ADR-019 set for e and r. Resolved here because this
	// is the only place that has both the typed line and the screen, and
	// resolving to an identifier means a name shared by two playlists is not
	// a question anybody has to answer.
	if p, ok := intent.(Playlist); ok && p.Verb == "reorder" {
		if m.screen != ScreenPlaylist || m.playlist.ID == "" {
			return m.refuse("reorder needs a playlist on screen — gp, enter, then :playlist reorder"), nil
		}
		return m.startReorder(), nil
	}
	if p, ok := intent.(Playlist); ok && p.Verb == "delete" && p.Name == "" {
		if m.screen != ScreenPlaylist || m.playlist.ID == "" {
			return m.refuse("delete needs a playlist on screen, or a name — :playlist delete <name>"), nil
		}
		return m, emit(DeletePlaylist{ID: m.playlist.ID, Name: m.playlist.Name,
			Tracks: len(m.playlist.Songs)})
	}
	if p, ok := intent.(Playlist); ok && p.Verb == "rename" {
		if m.screen != ScreenPlaylist || m.playlist.ID == "" {
			return m.refuse("rename needs a playlist on screen — gp, enter, then :playlist rename <new name>"), nil
		}
		return m, emit(RenamePlaylist{ID: m.playlist.ID, Name: p.Name})
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

// skyStep is how long one frame of the visualiser lasts: thirty a second.
const skyStep = 33 * time.Millisecond

// skyTurned asks for the next frame of the visualiser. It is handled here and
// nowhere else, so nothing outside this package has to know the pattern moves.
type skyTurned struct{ at time.Time }

// turnSky asks for the next frame after skyStep. Only the handlers that leave
// the overlay showing call it, so closing the overlay is what stops the
// pattern rather than a flag that happens to be false.
func (m Model) turnSky() tea.Cmd {
	return tea.Tick(skyStep, func(at time.Time) tea.Msg { return skyTurned{at: at} })
}

// openSky puts the visualiser on screen with nothing heard yet and asks for
// its first frame. When a cover is on screen it also asks for a repaint,
// which takes the cover's picture away.
func (m Model) openSky() (tea.Model, tea.Cmd) {
	covered := len(m.bigArt) > 0 || len(m.artBlock(m.viewWidth())) > 0
	m.skyOpens++
	m.showSky, m.sky = true, sky.New(m.skySeed+m.skyOpens)
	m.skyLast = time.Time{}
	w, h := m.skySize()
	m.sky.Step(w, h, skyStep.Seconds(), time.Time{}, skyTrack(m.nowPlaying, m.nowArtist))
	if covered {
		return m, tea.Batch(tea.ClearScreen, m.turnSky())
	}
	return m, m.turnSky()
}

// startReorder takes a copy of the playlist on screen to move about, and
// records the order it was drawn from.
func (m Model) startReorder() Model {
	m.order = append([]subsonic.Song(nil), m.playlist.Songs...)
	m.drawnFrom = idsOf(m.order)
	m.reordering = true
	m.filter, m.status = "", ""
	return m.selecting(ScreenPlaylist, 0)
}

// moveInOrder moves the selected track by one place and follows it with the
// cursor, so the row being moved stays under the keys moving it.
func (m Model) moveInOrder(by int) (tea.Model, tea.Cmd) {
	if !m.reordering {
		return m, nil
	}
	at := m.cursor[ScreenPlaylist]
	to := at + by
	if to < 0 || to >= len(m.order) {
		return m, nil
	}
	m.order = append([]subsonic.Song(nil), m.order...)
	m.order[at], m.order[to] = m.order[to], m.order[at]
	return m.selecting(ScreenPlaylist, to), nil
}

// saveReorder asks for the arrangement to be written, carrying the order the
// screen was drawn from so the caller can refuse a playlist that changed.
func (m Model) saveReorder() (tea.Model, tea.Cmd) {
	order, was := idsOf(m.order), m.drawnFrom
	name, id := m.playlist.Name, m.playlist.ID
	m.reordering, m.order, m.drawnFrom = false, nil, nil
	if same(order, was) {
		m.status = "nothing was moved"
		return m, nil
	}
	return m, emit(ReorderPlaylist{ID: id, Name: name, Songs: order, Was: was})
}

// idsOf is the identifiers of those tracks, in order.
func idsOf(songs []subsonic.Song) []string {
	ids := make([]string, len(songs))
	for i, s := range songs {
		ids[i] = s.ID
	}
	return ids
}

// same says whether two orders are the same sequence.
func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// goTo handles the key after g. The queue, playlists and starred screens are
// bound before they exist, so the prefix is whole and says what is missing
// rather than doing nothing.
func (m Model) goTo(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "g":
		return m.moveTo(0).ontoARow(), nil
	case "l":
		return m.show(ScreenLibrary), nil
	case "a":
		return m.show(ScreenArtists), nil
	case "b":
		// The screen is shown at once and the albums are asked for, as the
		// library row does.
		next, _ := m.update(ShowEveryAlbum{})
		return next, emit(ShowEveryAlbum{})
	case "v":
		return m.openSky()
	case "p":
		next, _ := m.update(ShowPlaylists{})
		return next, emit(ShowPlaylists{})
	case "q":
		m = m.show(ScreenQueue)
		return m.selecting(ScreenQueue, max(0, min(len(m.queued)-1, m.queuedAt))), nil
	case "s":
		m = m.show(ScreenStarred)
		return m.selecting(ScreenStarred, firstSelectable(m.starredRows)), nil
	}
	return m, nil
}

// shuffleSelected asks for the collection the cursor names, or the one it is
// inside, to play in a random order.
//
// On a track row it is the album or the playlist holding the track rather than
// the track itself, because one track has no order to shuffle. :shuffle keeps
// its own meaning, which is the whole server.
func (m Model) shuffleSelected() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenArtists, ScreenAlbums:
		if id := m.selectedID(); id != "" {
			scope := "artist"
			if m.screen == ScreenAlbums {
				scope = "album"
			}
			return m, emit(ShuffleThis{Scope: scope, ID: id})
		}
	case ScreenTracks:
		if m.album.ID != "" {
			return m, emit(ShuffleThis{Scope: "album", ID: m.album.ID})
		}
	case ScreenPlaylists:
		if m.rows() > 0 {
			return m, emit(ShuffleThis{Scope: "playlist", ID: m.playlists[m.matches()[m.cursor[m.screen]]].ID})
		}
	case ScreenPlaylist:
		if m.playlist.ID != "" {
			return m, emit(ShuffleThis{Scope: "playlist", ID: m.playlist.ID})
		}
	}
	m.status = "nothing to shuffle here — press S on an artist, an album or a playlist"
	return m, nil
}

// open moves down one screen, or plays the selected track. The album travels
// with the intent.
func (m Model) open() (tea.Model, tea.Cmd) {
	if m.reordering {
		return m.saveReorder()
	}
	if m.rows() == 0 {
		return m, nil
	}
	at := m.matches()[m.cursor[m.screen]]
	switch m.screen {
	case ScreenLibrary:
		dest := libraryRows[at]
		switch dest.screen {
		case ScreenPlaylists:
			return m, emit(ShowPlaylists{})
		case ScreenEveryAlbum:
			return m, emit(ShowEveryAlbum{})
		}
		// The rest are screens a jump already reaches, and the jump places the
		// cursor as that screen wants it.
		return m.goTo(dest.key)
	case ScreenArtists:
		return m.opening("opening "+m.artists[at].Name, m.artists[at].ID), emit(OpenArtist{ID: m.artists[at].ID})
	case ScreenAlbums:
		m.fromTracks = ScreenAlbums
		return m.opening("opening "+m.artist.Albums[at].Name, m.artist.Albums[at].ID), emit(OpenAlbum{ID: m.artist.Albums[at].ID})
	case ScreenEveryAlbum:
		m.fromTracks = ScreenEveryAlbum
		return m.opening("opening "+m.albums[at].Name, m.albums[at].ID), emit(OpenAlbum{ID: m.albums[at].ID})
	case ScreenTracks:
		return m, emit(PlayFrom{Album: m.album, Index: at})
	case ScreenQueue:
		return m, emit(JumpTo(at))
	case ScreenPlaylists:
		return m.opening("opening "+m.playlists[at].Name, m.playlists[at].ID), emit(OpenPlaylist{ID: m.playlists[at].ID})
	case ScreenPlaylist:
		return m, emit(PlayFrom{Album: asAlbum(m.playlist), Index: at,
			Playlist: m.playlist.ID})
	case ScreenWiki:
		if m.topic != "" {
			return m, nil
		}
		m.topic = commands[at].name
		return m.selecting(ScreenWiki, 0), nil
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[at]
		switch r.kind {
		case kindArtist:
			return m.opening("opening "+r.name, r.id), emit(OpenArtist{ID: r.id})
		case kindAlbum:
			m.fromTracks = m.screen
			return m.opening("opening "+r.name, r.id), emit(OpenAlbum{ID: r.id})
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
	case ScreenEveryAlbum:
		kind, id = StarAlbum, m.albums[at].ID
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

// editSelected adds the selected track to the playlist being edited, or takes
// it out. The library lists are what is on screen, so the row is a track.
func (m Model) editSelected(add bool) (tea.Model, tea.Cmd) {
	if m.rows() == 0 {
		return m, nil
	}
	at := m.matches()[m.cursor[m.screen]]
	id := ""
	switch m.screen {
	case ScreenTracks:
		id = m.album.Songs[at].ID
	case ScreenQueue:
		id = m.queued[at].ID
	case ScreenSearch, ScreenStarred:
		if r := m.grouped()[at]; r.kind == kindSong {
			id = r.id
		}
	}
	if id == "" {
		m.status = "there is no track here to add; open an album"
		return m, nil
	}
	if add == m.inPlaylist[id] {
		// Already as asked. Saying so is better than a request that changes
		// nothing and looks like it worked.
		what := "already in"
		if !add {
			what = "not in"
		}
		m.status = "that track is " + what + " " + Sanitise(m.editing.Name)
		return m, nil
	}
	return m, emit(EditPlaylist{ID: m.editing.ID, SongID: id, Add: add, At: -1})
}

// holding is the identifiers of the tracks in a playlist.
func holding(p subsonic.Playlist) map[string]bool {
	out := make(map[string]bool, len(p.Songs))
	for _, s := range p.Songs {
		out[s.ID] = true
	}
	return out
}

// asAlbum gathers a playlist into something that can be played from. Its
// tracks come from several albums, so it is a list rather than one of them.
func asAlbum(p subsonic.Playlist) subsonic.Album {
	return subsonic.Album{ID: p.ID, Name: p.Name, Songs: p.Songs}
}

// back moves up one screen. At the top it does nothing.
func (m Model) back() (tea.Model, tea.Cmd) {
	if m.reordering {
		// The arrangement was never sent, so leaving it is forgetting it.
		m.reordering, m.order, m.drawnFrom = false, nil, nil
		m.status = "nothing was moved"
		return m, nil
	}
	if m.showSky {
		m.showSky = false
		return m, tea.ClearScreen
	}
	if len(m.bigArt) > 0 {
		m.bigArt = nil
		return m, tea.ClearScreen
	}
	switch m.screen {
	case ScreenTracks:
		// An album is reached from one artist's list, every album, a search or
		// the starred list, and back returns to the one it was opened from.
		up := m.fromTracks
		if up != ScreenEveryAlbum && up != ScreenSearch && up != ScreenStarred {
			up = ScreenAlbums
		}
		return m.show(up), nil
	case ScreenAlbums:
		return m.show(ScreenArtists), nil
	case ScreenPlaylist:
		return m.show(ScreenPlaylists), nil
	case ScreenWiki:
		// Reading about a command goes back to the list of them before it
		// leaves the wiki altogether.
		if m.topic != "" {
			m.topic, m.status = "", ""
			return m.selecting(ScreenWiki, 0), nil
		}
		return m.show(ScreenLibrary), nil
	case ScreenArtists, ScreenEveryAlbum, ScreenQueue, ScreenSearch, ScreenStarred,
		ScreenMessages, ScreenPlaylists:
		// Each of these is opened from the library, or reached from anywhere,
		// so back goes up to the library rather than wherever it was opened
		// from. On the library itself back does nothing, because there is
		// nothing above it: that is what stops a key held down closing the
		// program.
		return m.show(ScreenLibrary), nil
	}
	return m, nil
}

// show moves to a screen. The filter on the screen being left is dropped with
// the cursor kept on the row it was on, and an answer still on its way from
// the server no longer moves the screen when it arrives.
func (m Model) show(s Screen) Model {
	if m.filter != "" {
		if rows := m.matches(); len(rows) > 0 {
			m = m.selecting(m.screen, rows[min(m.cursor[m.screen], len(rows)-1)])
		}
		m.filter = ""
	}
	if m.wanted != "" {
		m = m.abandon()
		m.loading = false
	}
	m.screen, m.status = s, ""
	return m
}

// strayedKept is how many abandoned requests are remembered.
const strayedKept = 8

// abandon stops waiting for what was asked for, and remembers it so that its
// answer is dropped when it arrives.
func (m Model) abandon() Model {
	strayed := append(append([]string(nil), m.strayed...), m.wanted)
	if len(strayed) > strayedKept {
		strayed = strayed[len(strayed)-strayedKept:]
	}
	m.wanted, m.strayed = "", strayed
	return m
}

// opening records that the thing with this identifier was asked for, and says
// so while it arrives.
func (m Model) opening(status, id string) Model {
	if m.wanted != "" && m.wanted != id {
		m = m.abandon()
	}
	m.loading, m.status, m.wanted = true, Sanitise(status), id
	return m
}

// arrived reports whether something loaded should be shown. It should unless
// it was asked for and then abandoned, by leaving the screen that asked or by
// asking for something else.
func (m Model) arrived(id string) (Model, bool) {
	if m.wanted == id {
		m.wanted = ""
		return m, true
	}
	for i, was := range m.strayed {
		if was == id {
			m.strayed = append(append([]string(nil), m.strayed[:i]...), m.strayed[i+1:]...)
			return m, false
		}
	}
	return m, true
}

// within keeps the cursor of a screen on a row of a list that now holds n.
func (m Model) within(s Screen, n int) Model {
	if m.screen == s {
		n = m.rows()
	}
	return m.selecting(s, max(0, min(n-1, m.cursor[s])))
}

// refuse says why something asked for was not done.
func (m Model) refuse(why string) Model {
	m.status, m.statusFailed = why, true
	return m
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
	if n := m.height - m.chrome(); n > 1 {
		return n
	}
	return 1
}

// selectedID is what the cursor is on, for the screens a reload can refresh.
func (m Model) selectedID() string {
	if m.rows() == 0 {
		return ""
	}
	return m.idAt(m.matches()[m.cursor[m.screen]])
}

// reloaded puts the cursor back on what was selected before, and says what came
// back. A reload that returns the same thing looks exactly like one that did
// nothing, so it has to say.
func (m Model) reloaded(screen Screen, what string) Model {
	if m.keep == "" {
		return m
	}
	keep := m.keep
	m.keep = ""
	m.status = "reloaded: " + what

	for row, i := range m.matches() {
		was := m.screen
		m.screen = screen
		id := m.idAt(i)
		m.screen = was
		if id == keep {
			return m.selecting(screen, row)
		}
	}
	// What was selected is not there any more, and the list may be shorter
	// than the cursor. Leaving it where it was is a row that cannot be drawn
	// and an index that opening would read past the end of.
	return m.selecting(screen, max(0, min(m.rows()-1, m.cursor[screen])))
}

// idAt is the identifier of row i of the current screen.
func (m Model) idAt(i int) string {
	switch m.screen {
	case ScreenArtists:
		return m.artists[i].ID
	case ScreenAlbums:
		return m.artist.Albums[i].ID
	case ScreenEveryAlbum:
		return m.albums[i].ID
	case ScreenTracks:
		return m.album.Songs[i].ID
	case ScreenStarred:
		return m.starredRows[i].id
	case ScreenPlaylists:
		return m.playlists[i].ID
	case ScreenPlaylist:
		return m.playlist.Songs[i].ID
	}
	return ""
}

// confirmKey answers a question about something that cannot be undone.
//
// Only yes does it. Every other key says no, because a question nobody meant
// to answer should end with nothing having happened.
func (m Model) confirmKey(key string) (tea.Model, tea.Cmd) {
	agreed := m.agreed
	m.mode, m.asking, m.agreed = modeNormal, "", nil

	if key != "y" && key != "Y" {
		m.status = "nothing was changed"
		return m, nil
	}
	m.status = ""
	return m, emit(agreed)
}

// editPlaylist starts adding to a playlist, showing the library to find tracks
// in. It is what `:playlist edit` arrives at and what `e` does on a playlist.
func (m Model) editPlaylist(p subsonic.Playlist) (tea.Model, tea.Cmd) {
	m.editing, m.inPlaylist = p, holding(p)
	m = m.show(ScreenArtists)
	m.loading = false
	m.status = editingHint(p.Name)
	return m.remember(m.status), nil
}

// removeFromPlaylist takes the selected track out of the playlist on screen.
//
// The server removes by position rather than by identifier, and the position
// it means is the one in the playlist rather than the one on screen, which
// differ whenever a filter is narrowing the list.
func (m Model) removeFromPlaylist() (tea.Model, tea.Cmd) {
	if m.rows() == 0 || m.playlist.ID == "" {
		return m, nil
	}
	at := m.matches()[m.cursor[m.screen]]
	if at >= len(m.playlist.Songs) {
		return m, nil
	}
	// Nothing is said until the server has done it. Saying so first reads as
	// success, and the removal can still be refused.
	track := m.playlist.Songs[at]
	return m, emit(EditPlaylist{ID: m.playlist.ID, SongID: track.ID, Add: false, At: at})
}
