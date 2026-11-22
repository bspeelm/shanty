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
		// A page of the wiki has as many rows as its text takes at this width.
		for _, s := range Screens {
			m = m.settled(s)
		}
		return m, nil
	case tea.KeyMsg:
		if m.mode != modeNormal {
			return m.modeKey(msg)
		}
		return m.key(msg)

	case ArtistsLoaded:
		m.artists, m.starting = msg, false
		if msg == nil {
			// The list is empty rather than nil once the server has answered.
			m.artists = []subsonic.Artist{}
		}
		return m.say("").settled(ScreenArtists), nil
	case Opened:
		if msg.Ask == 0 || msg.Ask != m.waiting {
			return m, nil
		}
		m.waiting = 0
		return m.update(msg.Loaded)
	case Reloaded:
		return m.refetched(msg.Loaded), nil
	case ArtistLoaded:
		m = m.show(ScreenAlbums)
		m.artist = subsonic.Artist(msg)
		return m.selecting(ScreenAlbums, 0), nil
	case AlbumLoaded:
		album := subsonic.Album(msg)
		if album.ID != m.album.ID {
			// An album with a cover holds the rows for it straight away. The
			// picture is a request away and the list would otherwise jump
			// when it lands.
			m.art = nil
			if album.CoverArt != "" {
				m.art = m.artBlank
			}
		}
		m = m.show(ScreenTracks)
		m.album = album
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
		// The failure of an open that is no longer waited for is kept to be
		// read, and changes nothing on screen.
		if msg.Ask != 0 && msg.Ask != m.waiting {
			return m.remember(msg.Message), nil
		}
		if msg.Ask != 0 {
			m.waiting = 0
		}
		m.keep, m.starting, m.stalled, m.saving = "", false, true, false
		return m.refuse(msg.Message).remember(msg.Message), nil
	case Notice:
		return m.say(string(msg)).remember(string(msg)), nil
	case StarredChanged:
		m.starredRows, m.starred = flatten(msg.Results), msg.IDs
		return m.settled(ScreenStarred), nil
	case SearchLoaded:
		m = m.show(ScreenSearch)
		m.found, m.query, m.filter = flatten(msg.Results), msg.Query, ""
		return m.selecting(ScreenSearch, firstSelectable(m.found)), nil
	case Reload:
		// What was selected is remembered, so a reload leaves the cursor
		// where it was rather than at the top of a list that mostly did not
		// change.
		m.keep = m.selectedID()
		if m.keep == "" {
			m.keep = " " // nothing selected, but a reload is still under way
		}
		return m.say(""), nil
	case PlaylistsLoaded:
		m.playlists = msg
		if msg == nil {
			// The list is empty rather than nil once the server has answered.
			m.playlists = []subsonic.Playlist{}
		}
		// A playlist the server no longer lists is not gone back to.
		m = m.remembering(func(p place) (place, bool) {
			for _, list := range m.playlists {
				if list.ID == p.playlist.ID {
					return p, true
				}
			}
			return p, p.screen != ScreenPlaylist
		})
		return m.say("").settled(ScreenPlaylists), nil
	case PlaylistLoaded:
		m = m.show(ScreenPlaylist)
		m.playlist = subsonic.Playlist(msg)
		m.reordering, m.order, m.drawnFrom = false, nil, nil
		return m.selecting(ScreenPlaylist, 0), nil
	case PlaylistChanged:
		loaded := subsonic.Playlist(msg)
		if m.editing.ID == loaded.ID && m.editing.ID != "" {
			m.editing, m.inPlaylist = loaded, holding(loaded)
		}
		for i, p := range m.playlists {
			if p.ID == loaded.ID {
				m.playlists = append([]subsonic.Playlist(nil), m.playlists...)
				m.playlists[i].Name, m.playlists[i].SongCount = loaded.Name, len(loaded.Songs)
			}
		}
		m = m.remembering(func(p place) (place, bool) {
			if p.playlist.ID == loaded.ID {
				p.playlist = loaded
			}
			return p, true
		})
		if loaded.ID != m.playlist.ID {
			return m, nil
		}
		return m.refreshed(loaded), nil
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
		return m.say(editingHint(m.editing.Name)).remember(editingHint(m.editing.Name)), nil
	case Confirm:
		m.mode, m.asking, m.agreed = modeConfirm, msg.Question, msg.Do
		return m, nil
	case AlbumsLoaded:
		m.albums = msg
		if msg == nil {
			m.albums = []subsonic.Album{}
		}
		return m.say("").settled(ScreenEveryAlbum), nil
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
		m.topic, m.filter = "", ""
		return m.selecting(ScreenWiki, 0).ontoARow(), nil
	case ShowMessages:
		m = m.show(ScreenMessages)
		m.filter = ""
		return m.selecting(ScreenMessages, 0), nil
	case QueueChanged:
		m.queued, m.queuedAt, m.queuedFrom = msg.Tracks, msg.At, msg.From
		return m.settled(ScreenQueue), nil
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// A key clears the message on the last row.
	m = m.say("")
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
	if _, ok := intent.(listPlaylists); ok {
		return m.goTo("p")
	}
	if p, ok := intent.(Playlist); ok && p.Verb == "reorder" {
		if m.screen != ScreenPlaylist || m.playlist.ID == "" {
			return m.refuse("reorder needs a playlist on screen — gp, enter, then :playlist reorder"), nil
		}
		if m.saving {
			return m.refuse("the order just saved has not been answered yet — reorder again once it has"), nil
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
	// A rename's argument is the new name. The playlist renamed is the one on
	// screen, named by its identifier.
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
	m.saving = true
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

// under is what the cursor is on.
type under struct {
	// kind is what the row is. It is kindHeading when there is no row, or the
	// row is a heading.
	kind kind
	id   string
	name string
	// list is the tracks a track row sits among, and index is its place in
	// them. On a search and on the starred screen they are the tracks listed.
	list  subsonic.Album
	index int
	// scope and holder name the collection a track row belongs to, where it
	// belongs to one: an album or a playlist.
	scope  Scope
	holder string
}

// under reports what the cursor is on. The library, the wiki and the messages
// report no row.
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
		return under{kind: kindSong, id: m.album.Songs[at].ID, name: m.album.Songs[at].Title,
			list: m.album, index: at, scope: ScopeAlbum, holder: m.album.ID}
	case ScreenPlaylists:
		return under{kind: kindPlaylist, id: m.playlists[at].ID, name: m.playlists[at].Name}
	case ScreenPlaylist:
		list := asAlbum(m.playlist)
		if m.reordering {
			list.Songs = m.order
		}
		return under{kind: kindSong, id: list.Songs[at].ID, name: list.Songs[at].Title,
			list: list, index: at, scope: ScopePlaylist, holder: m.playlist.ID}
	case ScreenQueue:
		return under{kind: kindSong, id: m.queued[at].ID, name: m.queued[at].Title, index: at, scope: ScopeQueue}
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[at]
		u := under{kind: r.kind, id: r.id, name: r.name}
		if r.kind != kindSong {
			return u
		}
		for _, i := range m.matches() {
			if other := m.grouped()[i]; other.kind == kindSong {
				if i == at {
					u.index = len(u.list.Songs)
				}
				u.list.Songs = append(u.list.Songs, other.album.Songs...)
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
// On the library the Starred and Queue rows are shuffled, and the other rows
// say where to press S.
func (m Model) shuffleSelected() (tea.Model, tea.Cmd) {
	if m.reordering {
		return m.say("nothing is shuffled while the playlist is being reordered — enter saves, esc discards"), nil
	}
	switch m.screen {
	case ScreenLibrary:
		if m.rows() == 0 {
			break
		}
		switch libraryRows[m.matches()[m.cursor[m.screen]]].screen {
		case ScreenStarred:
			return m, emit(ShuffleThis{Scope: ScopeStarred})
		case ScreenQueue:
			return m.shuffleQueue()
		case ScreenPlaylists:
			return m.say("choose a playlist to shuffle — enter lists them, then press S on one"), nil
		}
		return m.say(":shuffle plays everything on the server — or enter lists these, then press S on one"), nil
	case ScreenQueue:
		return m.shuffleQueue()
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
		return m, emit(ShuffleSongs{Songs: u.list.Songs,
			What: fmt.Sprintf("%s found for %q", plural(len(u.list.Songs), "track"), m.query)})
	case u.kind == kindSong:
		return m, emit(ShuffleSongs{Songs: u.list.Songs, What: plural(len(u.list.Songs), "starred track")})
	}

	switch {
	case m.rows() > 0:
		return m.say("that row is a heading — move to an artist, an album or a track and press S"), nil
	case m.filter != "":
		return m.say("nothing matches " + Sanitise(m.filter) + ", so there is nothing to shuffle — / narrows the list again"), nil
	case m.screen == ScreenStarred:
		return m.say("nothing is starred, so there is nothing to shuffle — press * on an artist, an album or a track"), nil
	case m.screen == ScreenSearch:
		return m.say("the search found nothing to shuffle — :search <words> looks again"), nil
	}
	return m.say("nothing to shuffle here — press S on an artist, an album, a playlist or a track"), nil
}

// shuffleQueue asks for the tracks after the one playing to be put in a random
// order, or refuses and says why.
func (m Model) shuffleQueue() (tea.Model, tea.Cmd) {
	if why := QueueRefusal(len(m.queued), m.queuedAt); why != "" {
		return m.refuse(why), nil
	}
	return m, emit(ShuffleThis{Scope: ScopeQueue})
}

// QueueRefusal says why a queue of that many tracks, playing the one at that
// position, has nothing after it to shuffle. It is empty when there is.
func QueueRefusal(tracks, at int) string {
	switch after := tracks - at - 1; {
	case tracks == 0:
		return "the queue is empty, so there is nothing to shuffle — enter on a track plays it"
	case after < 0:
		return "nothing is playing, so there is nothing after it to shuffle — enter on a track plays it"
	case after < 2:
		return "fewer than two tracks follow the one playing, so there is nothing to shuffle — a on a track adds it to the queue"
	}
	return ""
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
		m = m.opening("opening " + m.artists[at].Name)
		return m, emit(OpenArtist{ID: m.artists[at].ID, Ask: m.waiting})
	case ScreenAlbums:
		m = m.opening("opening " + m.artist.Albums[at].Name)
		return m, emit(OpenAlbum{ID: m.artist.Albums[at].ID, Ask: m.waiting})
	case ScreenEveryAlbum:
		m = m.opening("opening " + m.albums[at].Name)
		return m, emit(OpenAlbum{ID: m.albums[at].ID, Ask: m.waiting})
	case ScreenTracks:
		return m, emit(PlayFrom{Album: m.album, Index: at})
	case ScreenQueue:
		return m, emit(JumpTo(at))
	case ScreenPlaylists:
		m = m.opening("opening " + m.playlists[at].Name)
		return m, emit(OpenPlaylist{ID: m.playlists[at].ID, Ask: m.waiting})
	case ScreenPlaylist:
		return m, emit(PlayFrom{Album: asAlbum(m.playlist), Index: at,
			Playlist: m.playlist.ID})
	case ScreenWiki:
		if m.topic != "" {
			return m, nil
		}
		r := m.grouped()[at]
		if r.kind != kindCommand {
			return m.say("that is a key, not a page — press it on the screen it works on"), nil
		}
		m.topic = r.id
		return m.selecting(ScreenWiki, 0), nil
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[at]
		switch r.kind {
		case kindArtist:
			m = m.opening("opening " + r.name)
			return m, emit(OpenArtist{ID: r.id, Ask: m.waiting})
		case kindAlbum:
			m = m.opening("opening " + r.name)
			return m, emit(OpenAlbum{ID: r.id, Ask: m.waiting})
		case kindSong:
			u := m.under()
			return m, emit(PlayFrom{Album: u.list, Index: u.index})
		}
		return m, nil
	}
	return m, nil
}

// queueSelected asks for what the cursor is on to be queued, after the track
// playing or at the end: a track, or every track of an album, an artist or a
// playlist in its own order.
func (m Model) queueSelected(next bool) (tea.Model, tea.Cmd) {
	switch u := m.under(); {
	case m.screen == ScreenQueue:
		return m.say("that is already in the queue"), nil
	case u.kind == kindSong && next:
		return m, emit(PlayNext{Album: u.list, Index: u.index})
	case u.kind == kindSong:
		return m, emit(Enqueue{Album: u.list, Index: u.index})
	case u.kind == kindArtist:
		return m, emit(QueueThis{Scope: ScopeArtist, ID: u.id, Next: next})
	case u.kind == kindAlbum:
		return m, emit(QueueThis{Scope: ScopeAlbum, ID: u.id, Next: next})
	case u.kind == kindPlaylist:
		return m, emit(QueueThis{Scope: ScopePlaylist, ID: u.id, Next: next})
	}
	return m.say("nothing here to add to the queue — press a or A on a track, an album, an artist or a playlist"), nil
}

// starSelected asks for the starred state of the selected row to be turned
// over.
func (m Model) starSelected() (tea.Model, tea.Cmd) {
	u := m.under()
	kind := map[kind]Kind{kindArtist: StarArtist, kindAlbum: StarAlbum, kindSong: StarSong}[u.kind]
	if kind == "" {
		return m.say("nothing here can be starred — press * on an artist, an album or a track"), nil
	}
	return m, emit(ToggleStar{Kind: kind, ID: u.id, Starred: !m.starred[u.id]})
}

// editSelected adds the selected track to the playlist being edited, or takes
// it out.
func (m Model) editSelected(add bool) (tea.Model, tea.Cmd) {
	u := m.under()
	if u.kind != kindSong {
		return m.say("there is no track here to add or take out — open an album and press a or r on a track"), nil
	}
	if add == m.inPlaylist[u.id] {
		// Already as asked. Saying so is better than a request that changes
		// nothing and looks like it worked.
		what := "already in"
		if !add {
			what = "not in"
		}
		return m.say("that track is " + what + " " + Sanitise(m.editing.Name)), nil
	}
	return m, emit(EditPlaylist{ID: m.editing.ID, SongID: u.id, Add: add, At: -1})
}

// removeSelected takes the selected track out of the playlist being edited,
// the queue, or the playlist on screen. A playlist on screen is asked about
// first.
func (m Model) removeSelected() (tea.Model, tea.Cmd) {
	u := m.under()
	switch {
	case m.editing.ID != "":
		return m.editSelected(false)
	case m.reordering:
		return m.say("a track cannot be taken out while the playlist is being reordered — enter saves, esc discards"), nil
	case u.scope == ScopeQueue:
		return m, emit(RemoveQueued(u.index))
	case u.scope == ScopePlaylist:
		m.mode, m.asking = modeConfirm, "take "+u.name+" out of "+m.playlist.Name+"?"
		m.agreed = EditPlaylist{ID: m.playlist.ID, SongID: u.id, Add: false, At: u.index}
		return m, nil
	}
	return m.say("r takes a track out of the queue or a playlist — press it on a track in one of them"), nil
}

// moveSelected moves the selected track one place: in the arrangement of a
// playlist being reordered, or in the queue. The cursor follows the track.
func (m Model) moveSelected(by int) (tea.Model, tea.Cmd) {
	switch {
	case m.reordering:
		return m.moveInOrder(by)
	case m.screen != ScreenQueue || m.rows() == 0:
		return m.say("K and J move a track in the queue, or in a playlist once :playlist reorder has begun"), nil
	case m.filter != "":
		return m.say("the queue cannot be rearranged while it is narrowed — esc shows all of it"), nil
	}
	at := m.cursor[ScreenQueue]
	to := at + by
	if to < 0 || to >= len(m.queued) {
		return m, nil
	}
	return m.selecting(ScreenQueue, to), emit(MoveQueued{From: at, To: to})
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

// place is a screen that was left, with what it was showing.
type place struct {
	screen Screen
	// cursor and filter are the row selected and the filter kept there.
	cursor int
	filter string
	// artist, album, art and playlist are what was held when the screen was
	// left. Each is shown again only on the screen that shows it.
	artist   subsonic.Artist
	album    subsonic.Album
	art      []string
	playlist subsonic.Playlist
	// up is the places this one was opened through, the nearest last.
	up []place
}

// trailKept is how many screens back are remembered, and how many a screen
// can have been opened through.
const trailKept = 32

// here is the screen showing, as a place to come back to.
func (m Model) here() place {
	return place{m.screen, m.cursor[m.screen], m.filter, m.artist, m.album, m.art, m.playlist, m.up}
}

// returned is the model showing a place that was left. What that place held
// is put back unless the same thing is still held.
func (m Model) returned(p place) Model {
	m.screen, m.filter, m.up = p.screen, p.filter, p.up
	switch {
	case p.screen == ScreenAlbums && p.artist.ID != m.artist.ID:
		m.artist = p.artist
	case p.screen == ScreenTracks && p.album.ID != m.album.ID:
		m.album, m.art = p.album, p.art
	case p.screen == ScreenPlaylist && p.playlist.ID != m.playlist.ID:
		m.playlist = p.playlist
	}
	return m.say("").selecting(p.screen, p.cursor).settled(p.screen)
}

// each is those places and the places each was opened through, with f applied
// to every one. A place f does not keep is left out.
func each(places []place, f func(place) (place, bool)) []place {
	var out []place
	for _, p := range places {
		p.up = each(p.up, f)
		if p, keep := f(p); keep {
			out = append(out, p)
		}
	}
	return out
}

// remembering applies f to every place there is a way back to.
func (m Model) remembering(f func(place) (place, bool)) Model {
	m.trail, m.up = each(m.trail, f), each(m.up, f)
	return m
}

// unfiltered drops the filter and leaves the cursor on the row it was on.
func (m Model) unfiltered() Model {
	if rows := m.matches(); len(rows) > 0 {
		m = m.selecting(m.screen, rows[min(m.cursor[m.screen], len(rows)-1)])
	}
	m.filter = ""
	return m.settled(m.screen)
}

// left is the model once the screen showing has stopped waiting: an open
// asked for from it is no longer shown when it arrives, a reload of it is
// forgotten, and its filter is put away.
func (m Model) left() Model {
	m.waiting, m.keep, m.stalled, m.saving = 0, "", false, false
	if m.mode == modeFilter {
		m.mode = modeNormal
	}
	return m.unfiltered()
}

// back returns to where the screen showing was opened from, and from a list
// to the screen that was showing before it. A filter kept on the screen
// showing is dropped first. On the library, with nowhere to go back to, it
// does nothing.
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
	if m.filter != "" {
		return m.unfiltered().say(""), nil
	}
	if m.screen == ScreenWiki && m.topic != "" {
		m.topic = ""
		return m.say("").selecting(ScreenWiki, 0).ontoARow(), nil
	}
	p := place{screen: ScreenLibrary, cursor: m.cursor[ScreenLibrary]}
	if n := len(m.up); n > 0 {
		p = m.up[n-1]
	} else if n := len(m.trail); n > 0 {
		p, m.trail = m.trail[n-1], m.trail[:n-1:n-1]
	}
	return m.left().returned(p), nil
}

// show moves to a screen. One artist's albums, a track list and a playlist
// are opened from the screen showing, which back returns to. Any other screen
// is a list reached from anywhere: the screen being left is remembered for
// back, the list is shown with the filter kept on it, and it is remembered
// once however often it is reached. Moving to the library forgets them all.
func (m Model) show(s Screen) Model {
	if s == m.screen {
		return m.unfiltered().say("")
	}
	from := m.here()
	m = m.left()
	m.screen = s
	if deep(s) {
		m.up = append(from.up[:len(from.up):len(from.up)], from)
		m.up = m.up[max(0, len(m.up)-trailKept):]
		return m.say("")
	}
	var trail []place
	m.up, m.filter = nil, ""
	for _, p := range m.trail {
		if p.screen == s {
			m.filter = p.filter
			continue
		}
		trail = append(trail, p)
	}
	trail = append(trail, from)
	m.trail = trail[max(0, len(trail)-trailKept):]
	if s == ScreenLibrary {
		m.trail = nil
	}
	return m.say("").settled(s)
}

// opening gives the open being asked for the next number, waits for that
// number alone, and keeps what to say while it arrives.
func (m Model) opening(what string) Model {
	m.asks++
	m.waiting, m.opens = m.asks, Sanitise(what)
	return m.say("")
}

// loading reports whether the screen showing is waiting for something: an
// open, a reload, or its own list where that has not been fetched.
func (m Model) loading() bool {
	if m.waiting != 0 || m.keep != "" {
		return true
	}
	switch m.screen {
	case ScreenLibrary, ScreenArtists:
		return m.starting
	case ScreenPlaylists:
		return m.playlists == nil && !m.stalled
	case ScreenEveryAlbum:
		return m.albums == nil && !m.stalled
	}
	return false
}

// arriving is what to say on the last row about something still on its way,
// when there is no message there.
func (m Model) arriving() string {
	switch {
	case m.waiting != 0:
		return m.opens
	case m.keep != "":
		return "asking the server again"
	}
	return ""
}

// refetched takes what a reload fetched in place of what is held. One artist
// or album is taken only if it is the one held. Where the reload was asked
// for from the screen showing, the cursor goes back to what was selected and
// the last row says what came back.
func (m Model) refetched(loaded tea.Msg) Model {
	switch loaded := loaded.(type) {
	case ArtistsLoaded:
		next, _ := m.update(loaded)
		m = next.(Model)
		if m.screen == ScreenLibrary && m.keep != "" {
			m.keep = ""
			return m.say("reloaded: " + plural(len(loaded), "artist"))
		}
		return m.reloaded(ScreenArtists, plural(len(loaded), "artist"))
	case AlbumsLoaded:
		next, _ := m.update(loaded)
		return next.(Model).reloaded(ScreenEveryAlbum, plural(len(loaded), "album"))
	case StarredChanged:
		next, _ := m.update(loaded)
		return next.(Model).reloaded(ScreenStarred, plural(len(loaded.IDs), "starred thing")).settled(ScreenStarred)
	case ArtistLoaded:
		m = m.remembering(func(p place) (place, bool) {
			if p.artist.ID == loaded.ID {
				p.artist = subsonic.Artist(loaded)
			}
			return p, true
		})
		if loaded.ID != m.artist.ID {
			return m
		}
		m.artist = subsonic.Artist(loaded)
		return m.reloaded(ScreenAlbums, plural(len(loaded.Albums), "album")).settled(ScreenAlbums)
	case AlbumLoaded:
		m = m.remembering(func(p place) (place, bool) {
			if p.album.ID == loaded.ID {
				p.album = subsonic.Album(loaded)
			}
			return p, true
		})
		if loaded.ID != m.album.ID {
			return m
		}
		m.album = subsonic.Album(loaded)
		return m.reloaded(ScreenTracks, plural(len(loaded.Songs), "track")).settled(ScreenTracks)
	}
	return m
}

// refreshed takes a playlist as the server now has it in place of the one
// held. An arrangement being made of it is dropped if its tracks changed.
// Where its screen is showing, a reload waiting for it is finished.
func (m Model) refreshed(loaded subsonic.Playlist) Model {
	m.playlist, m.saving = loaded, false
	if m.reordering && !same(idsOf(loaded.Songs), m.drawnFrom) {
		m.reordering, m.order, m.drawnFrom = false, nil, nil
		if m.screen == ScreenPlaylist {
			m = m.say(Sanitise(loaded.Name) + " changed on the server, so the arrangement was dropped")
		}
	}
	if m.screen == ScreenPlaylist {
		m = m.reloaded(ScreenPlaylist, plural(len(loaded.Songs), "track"))
	}
	return m.settled(ScreenPlaylist)
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

// answer ends a question about something that cannot be undone. Only yes
// does what was asked about.
func (m Model) answer(yes bool) (tea.Model, tea.Cmd) {
	agreed := m.agreed
	m.mode, m.asking, m.agreed = modeNormal, "", nil
	if !yes {
		return m.say("nothing was changed"), nil
	}
	return m.say(""), emit(agreed)
}

// editPlaylist starts adding to a playlist, showing the library to find tracks
// in. It is what `:playlist edit` arrives at and what `e` does on a playlist.
func (m Model) editPlaylist(p subsonic.Playlist) (tea.Model, tea.Cmd) {
	m.editing, m.inPlaylist = p, holding(p)
	m = m.show(ScreenArtists)
	return m.say(editingHint(p.Name)).remember(editingHint(p.Name)), nil
}
