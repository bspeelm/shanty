package tui

import (
	"fmt"
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
		m.artists, m.loading = msg, false
		m = m.say("").settled(ScreenArtists)
		if m.screen == ScreenLibrary && m.keep != "" {
			m.keep = ""
			return m.say("reloaded: " + plural(len(msg), "artist")), nil
		}
		return m.reloaded(ScreenArtists, plural(len(msg), "artist")), nil
	case ArtistLoaded:
		artist := subsonic.Artist(msg)
		m, how := m.arrived(artist.ID)
		if how == abandoned {
			return m, nil
		}
		if m.keep != "" && m.screen == ScreenAlbums && artist.ID == m.artist.ID {
			m.artist, m.loading = artist, false
			return m.reloaded(ScreenAlbums, plural(len(artist.Albums), "album")), nil
		}
		m = m.show(ScreenAlbums)
		m.artist, m.loading = artist, false
		return m.selecting(ScreenAlbums, 0), nil
	case AlbumLoaded:
		album := subsonic.Album(msg)
		m, how := m.arrived(album.ID)
		if how == abandoned {
			return m, nil
		}
		reload := m.keep != "" && m.screen == ScreenTracks && album.ID == m.album.ID
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
		m.loading, m.keep = false, ""
		return m.refuse(msg.Message).remember(msg.Message), nil
	case Notice:
		return m.say(string(msg)).remember(string(msg)), nil
	case StarredChanged:
		m.starredRows, m.starred = flatten(msg.Results), msg.IDs
		if m.screen == ScreenStarred && m.keep != "" {
			m.loading = false
			m = m.reloaded(ScreenStarred, plural(len(msg.IDs), "starred thing"))
		}
		return m.settled(ScreenStarred), nil
	case SearchLoaded:
		m = m.show(ScreenSearch)
		m.found, m.query, m.loading = flatten(msg.Results), msg.Query, false
		return m.selecting(ScreenSearch, firstSelectable(m.found)), nil
	case Reload:
		// What was selected is remembered, so a reload leaves the cursor
		// where it was rather than at the top of a list that mostly did not
		// change.
		m.keep, m.loading = m.selectedID(), true
		if m.keep == "" {
			m.keep = " " // nothing selected, but a reload is still under way
		}
		// The answer for one artist or album is expected like one that was
		// opened, so that leaving the screen abandons it.
		switch m.screen {
		case ScreenAlbums:
			m.flying = kept(append(m.flying[:len(m.flying):len(m.flying)], m.artist.ID))
		case ScreenTracks:
			m.flying = kept(append(m.flying[:len(m.flying):len(m.flying)], m.album.ID))
		}
		return m.say("asking the server again"), nil
	case PlaylistsLoaded:
		m.playlists, m.loading = msg, false
		if msg == nil {
			// The list is empty rather than nil once the server has answered.
			m.playlists = []subsonic.Playlist{}
		}
		m = m.say("").settled(ScreenPlaylists)
		return m.reloaded(ScreenPlaylists, plural(len(msg), "playlist")), nil
	case PlaylistLoaded:
		loaded := subsonic.Playlist(msg)
		// The same message reports an edit, so the marks follow the server
		// rather than what was pressed.
		editing := m.editing.ID == loaded.ID && m.editing.ID != ""
		if editing {
			m.editing, m.inPlaylist = loaded, holding(loaded)
		}
		m, how := m.arrived(loaded.ID)
		if how == abandoned {
			return m, nil
		}
		// An answer nobody opened a screen for is a change to the playlist
		// already held. It is kept, and shown only if that playlist is showing.
		if loaded.ID == m.playlist.ID && (how == unasked || m.screen == ScreenPlaylist) {
			m.playlist = loaded
			if m.screen != ScreenPlaylist {
				return m.settled(ScreenPlaylist), nil
			}
			m.loading = false
			m = m.say("")
			if m.reordering && !same(idsOf(loaded.Songs), m.drawnFrom) {
				m.reordering, m.order, m.drawnFrom = false, nil, nil
				m = m.say(Sanitise(loaded.Name) + " changed on the server, so the arrangement was dropped")
			}
			return m.reloaded(ScreenPlaylist, plural(len(loaded.Songs), "track")).settled(ScreenPlaylist), nil
		}
		if editing && how == unasked {
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
		return m.say(editingHint(m.editing.Name)).remember(editingHint(m.editing.Name)), nil
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
		m.albums, m.loading = msg, false
		m = m.say("").settled(ScreenEveryAlbum)
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
		return m.settled(ScreenQueue), nil
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// A key clears the message on the last row, unless the message is about
	// something still arriving.
	if !m.loading {
		m = m.say("")
	}
	digit := len(key) == 1 && key[0] >= '0' && key[0] <= '9'

	// While the visualiser or a cover fills the screen, only the actions in
	// overCover and the digits of a count are taken. Any other key takes a
	// cover away, and does nothing under the visualiser.
	// A jump begun before the screen was covered is dropped.
	if m.showSky || len(m.bigArt) > 0 {
		m.pending = ""
		if !digit && !overCover[m.bound()[key].Name] {
			m.count = ""
			if !m.showSky {
				m.bigArt = nil
				return m, tea.ClearScreen
			}
			return m, nil
		}
	}

	// A pending prefix takes the next key, whatever it is, so an unrecognised
	// second key cancels the prefix rather than acting on its own.
	if m.pending == "g" {
		m.pending = ""
		return m.goTo(key)
	}
	// Digits gather into a count. A leading zero is not a count, because it
	// would make 0 mean something different depending on what came before it.
	if digit && !(key == "0" && m.count == "") {
		m.count += key
		return m, nil
	}
	// Everything below consumes the count, whether or not it uses it.
	repeat, counted := m.take()

	action, bound := m.bound()[key]
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
		return m.moveTo(0).ontoARow(), nil
	case tea.KeyEnter:
		m.mode = modeNormal
		return m, nil
	case tea.KeyBackspace:
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
		}
		return m.moveTo(0).ontoARow(), nil
	case tea.KeyRunes, tea.KeySpace:
		m.filter += string(msg.Runes)
		if msg.Type == tea.KeySpace {
			m.filter += " "
		}
		return m.moveTo(0).ontoARow(), nil
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
	if _, ok := intent.(ShowPlaylists); ok {
		return m.goTo("p")
	}
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
	m.filter = ""
	return m.say("").selecting(ScreenPlaylist, 0)
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
		return m.say("nothing was moved"), nil
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
		// The screen is shown here and the albums are asked for.
		next, _ := m.update(ShowEveryAlbum{})
		return next, emit(FetchEveryAlbum{})
	case "v":
		return m.openSky()
	case "p":
		next, _ := m.update(ShowPlaylists{})
		return next, emit(FetchPlaylists{})
	case "q":
		m = m.show(ScreenQueue)
		return m.selecting(ScreenQueue, max(0, min(len(m.queued)-1, m.queuedAt))), nil
	case "s":
		m = m.show(ScreenStarred)
		return m.selecting(ScreenStarred, firstSelectable(m.starredRows)), nil
	}
	return m, nil
}

// under is what the cursor is on.
type under struct {
	// kind is what the row is. It is kindHeading when there is no row.
	kind kind
	id   string
	// name is what the row is called.
	name string
	// songs is the list a track row sits in and index is its place in it. On
	// a search and on the starred screen the list is the tracks shown there.
	songs subsonic.Album
	index int
	// scope and holder name the collection a track row belongs to, where it
	// belongs to one: an album, a playlist or the queue.
	scope  Scope
	holder string
}

// under reports what the cursor is on. The library, the wiki and the messages
// hold nothing that can be played, so they report no row.
func (m Model) under() under {
	if m.rows() == 0 {
		return under{}
	}
	at := m.matches()[m.cursor[m.screen]]
	switch m.screen {
	case ScreenArtists:
		return under{kind: kindArtist, id: m.artists[at].ID, name: m.artists[at].Name}
	case ScreenAlbums:
		return under{kind: kindAlbum, id: m.artist.Albums[at].ID, name: m.artist.Albums[at].Name}
	case ScreenEveryAlbum:
		return under{kind: kindAlbum, id: m.albums[at].ID, name: m.albums[at].Name}
	case ScreenTracks:
		song := m.album.Songs[at]
		return under{kind: kindSong, id: song.ID, name: song.Title, songs: m.album, index: at,
			scope: ScopeAlbum, holder: m.album.ID}
	case ScreenPlaylists:
		return under{kind: kindPlaylist, id: m.playlists[at].ID, name: m.playlists[at].Name}
	case ScreenPlaylist:
		list := asAlbum(m.playlist)
		if m.reordering {
			list.Songs = m.order
		}
		return under{kind: kindSong, id: list.Songs[at].ID, name: list.Songs[at].Title, songs: list,
			index: at, scope: ScopePlaylist, holder: m.playlist.ID}
	case ScreenQueue:
		return under{kind: kindSong, id: m.queued[at].ID, name: m.queued[at].Title, index: at,
			scope: ScopeQueue}
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[at]
		u := under{kind: r.kind, id: r.id, name: r.name}
		if r.kind != kindSong {
			return u
		}
		for _, i := range m.matches() {
			if other := m.grouped()[i]; other.kind == kindSong {
				if i == at {
					u.index = len(u.songs.Songs)
				}
				u.songs.Songs = append(u.songs.Songs, other.album.Songs...)
			}
		}
		return u
	}
	return under{}
}

// shuffleSelected asks for what the cursor is on, or the list it is in, to
// play in a random order.
//
// An artist, an album or a playlist row is that collection. A track row is the
// album or the playlist holding it, and on a search or the starred screen it
// is every track listed there. The queue is the tracks after the one playing.
// A library row is what the row opens. :shuffle keeps its own meaning, which
// is the whole server.
func (m Model) shuffleSelected() (tea.Model, tea.Cmd) {
	queue := emit(ShuffleThis{Scope: ScopeQueue})
	if len(m.queued) == 0 {
		queue = nil
	}
	switch m.screen {
	case ScreenLibrary:
		if m.rows() == 0 {
			break
		}
		switch libraryRows[m.matches()[m.cursor[m.screen]]].screen {
		case ScreenArtists, ScreenEveryAlbum:
			return m, emit(Shuffle{})
		case ScreenStarred:
			return m, emit(ShuffleThis{Scope: ScopeStarred})
		case ScreenQueue:
			if queue != nil {
				return m, queue
			}
			return m.say("the queue is empty, so there is nothing to shuffle — enter on a track plays it"), nil
		}
		return m.say("choose a playlist to shuffle — enter lists them, then press S on one"), nil
	case ScreenQueue:
		if queue != nil {
			return m, queue
		}
		return m.say("the queue is empty, so there is nothing to shuffle — enter on a track plays it"), nil
	case ScreenWiki, ScreenMessages:
		return m.say("nothing to shuffle here — press S on an artist, an album, a playlist or a track"), nil
	}

	switch u := m.under(); {
	case u.kind == kindArtist:
		return m, emit(ShuffleThis{Scope: ScopeArtist, ID: u.id})
	case u.kind == kindAlbum:
		return m, emit(ShuffleThis{Scope: ScopeAlbum, ID: u.id})
	case u.kind == kindPlaylist:
		return m, emit(ShuffleThis{Scope: ScopePlaylist, ID: u.id})
	case u.kind == kindSong && u.holder != "":
		return m, emit(ShuffleThis{Scope: u.scope, ID: u.holder})
	case u.kind == kindSong && m.screen == ScreenSearch:
		return m, emit(ShuffleSongs{Songs: u.songs.Songs,
			What: fmt.Sprintf("%s found for %q", plural(len(u.songs.Songs), "track"), m.query)})
	case u.kind == kindSong:
		return m, emit(ShuffleSongs{Songs: u.songs.Songs,
			What: plural(len(u.songs.Songs), "starred track")})
	}

	switch {
	case m.filter != "":
		return m.say("nothing matches " + Sanitise(m.filter) + ", so there is nothing to shuffle — / narrows the list again"), nil
	case m.screen == ScreenStarred:
		return m.say("nothing is starred, so there is nothing to shuffle — press * on an artist, an album or a track"), nil
	case m.screen == ScreenSearch:
		return m.say("the search found nothing to shuffle — :search <words> looks again"), nil
	}
	return m.say("nothing to shuffle here — press S on an artist, an album, a playlist or a track"), nil
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
		// Each row is a screen a jump reaches, and the jump places the cursor as
		// that screen wants it and asks for what the screen shows.
		return m.goTo(libraryRows[at].key)
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
		return m.say("there is nothing to queue here; open an album first"), nil
	}
	if m.screen == ScreenQueue {
		return m.say("that is already in the queue"), nil
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
		return m.say("there is no track here to add; open an album"), nil
	}
	if add == m.inPlaylist[id] {
		// Already as asked. Saying so is better than a request that changes
		// nothing and looks like it worked.
		what := "already in"
		if !add {
			what = "not in"
		}
		return m.say("that track is " + what + " " + Sanitise(m.editing.Name)), nil
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
	if m.showSky {
		m.showSky = false
		return m, tea.ClearScreen
	}
	if len(m.bigArt) > 0 {
		m.bigArt = nil
		return m, tea.ClearScreen
	}
	if m.reordering {
		// The arrangement was never sent, so leaving it is forgetting it.
		m.reordering, m.order, m.drawnFrom = false, nil, nil
		return m.say("nothing was moved"), nil
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
			m.topic = ""
			return m.say("").selecting(ScreenWiki, 0), nil
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
// the cursor kept on the row it was on. Leaving a screen abandons what it was
// waiting for: the answers to what was opened from it are dropped when they
// arrive, and a reload of it is forgotten.
func (m Model) show(s Screen) Model {
	if m.filter != "" {
		if rows := m.matches(); len(rows) > 0 {
			m = m.selecting(m.screen, rows[min(m.cursor[m.screen], len(rows)-1)])
		}
		m.filter = ""
	}
	if s != m.screen {
		if len(m.flying) > 0 || m.keep != "" {
			m.loading = false
		}
		m.strayed = kept(append(m.strayed[:len(m.strayed):len(m.strayed)], m.flying...))
		m.flying, m.keep = nil, ""
		if m.mode == modeFilter {
			m.mode = modeNormal
		}
	}
	m.screen = s
	return m.say("")
}

// requestsKept is how many requests in flight, and how many abandoned, are
// remembered.
const requestsKept = 8

// kept is the most recent requestsKept of those identifiers.
func kept(ids []string) []string {
	if len(ids) > requestsKept {
		return ids[len(ids)-requestsKept:]
	}
	return ids
}

// without is those identifiers with the first one equal to id taken out, and
// whether there was one.
func without(ids []string, id string) ([]string, bool) {
	for i, was := range ids {
		if was == id {
			return append(append([]string(nil), ids[:i]...), ids[i+1:]...), true
		}
	}
	return ids, false
}

// opening records that the thing with this identifier was asked for, and says
// so while it arrives. Anything else in flight is abandoned, and the
// identifier is no longer counted as abandoned.
func (m Model) opening(status, id string) Model {
	var flying, strayed []string
	for _, was := range m.strayed {
		if was != id {
			strayed = append(strayed, was)
		}
	}
	for _, was := range m.flying {
		if was == id {
			flying = append(flying, was)
		} else {
			strayed = append(strayed, was)
		}
	}
	m.flying, m.strayed, m.loading = kept(append(flying, id)), kept(strayed), true
	return m.say(Sanitise(status))
}

// arrival is how an answer from the server stands with what was asked for.
type arrival int

const (
	// unasked is an answer to nothing this model asked for.
	unasked arrival = iota
	// asked is an answer to a request in flight.
	asked
	// abandoned is an answer to a request given up on, which is dropped.
	abandoned
)

// arrived takes one request for the identifier out of those in flight, or
// failing that out of those abandoned, and reports which it was.
func (m Model) arrived(id string) (Model, arrival) {
	if flying, was := without(m.flying, id); was {
		m.flying = flying
		return m, asked
	}
	if strayed, was := without(m.strayed, id); was {
		m.strayed = strayed
		return m, abandoned
	}
	return m, unasked
}

// settled keeps the cursor of a screen on a row of its list and off a heading.
func (m Model) settled(s Screen) Model {
	on := m
	if s != m.screen {
		on.screen, on.filter = s, ""
	}
	rows := on.rows()
	at := max(0, min(rows-1, m.cursor[s]))
	if on.isGrouped() && rows > 0 {
		at = stepOver(on.matchedResults(), at, 1)
	}
	return m.selecting(s, at)
}

// say puts a message on the last row that is not a failure.
func (m Model) say(what string) Model {
	m.status, m.statusFailed = what, false
	return m
}

// refuse puts a message on the last row saying why something was not done.
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

// reloaded puts the cursor back on what was selected before a reload of the
// screen showing, and says what came back. It does nothing when no reload is
// waiting or another screen is showing.
func (m Model) reloaded(screen Screen, what string) Model {
	if m.keep == "" || m.screen != screen {
		return m
	}
	keep := m.keep
	m.keep = ""
	m = m.say("reloaded: " + what)

	for row, i := range m.matches() {
		if m.idAt(i) == keep {
			return m.selecting(screen, row)
		}
	}
	return m.settled(screen)
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
		if m.reordering {
			return m.order[i].ID
		}
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
		return m.say("nothing was changed"), nil
	}
	return m.say(""), emit(agreed)
}

// editPlaylist starts adding to a playlist, showing the library to find tracks
// in. It is what `:playlist edit` arrives at and what `e` does on a playlist.
func (m Model) editPlaylist(p subsonic.Playlist) (tea.Model, tea.Cmd) {
	m.editing, m.inPlaylist = p, holding(p)
	m = m.show(ScreenArtists)
	m.loading = false
	return m.say(editingHint(p.Name)).remember(editingHint(p.Name)), nil
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
	if m.reordering {
		return m.say("a track cannot be taken out while the playlist is being reordered — enter saves, esc discards"), nil
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
