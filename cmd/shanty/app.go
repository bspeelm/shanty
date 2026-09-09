package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/backlog"
	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/control"
	"github.com/bspeelm/shanty/internal/cover"
	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/tui"
)

// player is the part of mpv.Player the app uses.
type player interface {
	Load(ctx context.Context, url string) error
	Append(ctx context.Context, url string) error
	SetPause(ctx context.Context, paused bool) error
	Seek(ctx context.Context, d time.Duration) error
	SeekTo(ctx context.Context, d time.Duration) error
	SetVolume(ctx context.Context, percent int) error
	Prefetch(ctx context.Context, url string) error
	Observe(ctx context.Context, property string) error
	Events() <-chan mpv.Event
	Close() error
	Detach() error
	Err() error
}

// app connects the interface to the server and the player. It receives the
// intents tui emits, performs the work, and sends the results back.
type app struct {
	ui     tui.Model
	client *subsonic.Client
	player player
	queue  queue.Queue
	volume int
	ctx    context.Context

	// detach starts a session that plays on without an interface. It is nil in
	// a session, which has no interface to leave.
	detach func(handoff) error
	// starred is what the server has starred, by identifier.
	starred map[string]bool
	// saved is the queue the server is holding, kept until somebody asks for
	// it. Taking it up without being asked would surprise anyone who quit
	// deliberately.
	saved subsonic.PlayQueue
	// random is where a shuffle gets its order. It is nil outside tests.
	random *rand.Rand
	// backlog is the file holding plays the server would not accept. It is
	// empty when there is nowhere to keep them, which is how the tests that
	// have no state directory run.
	backlog string
	// headless is set in a session. It has nobody to show a message to, so it
	// ends when there is nothing left to play.
	headless bool
	// detaching is set from the moment a session is started until this process
	// quits. The session owns the queue from then on, so this one stops
	// advancing it.
	detaching bool
	// released is set once the session holds the player, and tells the caller
	// not to stop mpv on the way out.
	released bool
	// art is how this terminal shows pictures, and covers is where the ones
	// already fetched are kept. A session has no terminal, so art is Text
	// there and no cover is ever asked for.
	art    cover.Protocol
	covers cover.Cache
	// size is the terminal, remembered because `:art` renders a cover to fill
	// it and the interface does not do the rendering.
	size tea.WindowSizeMsg
}

// Cover art is drawn in a box this many cells across and down. mpv is not
// asked what shape a cell is, so a square of cells is taller than it is wide
// and the terminal is left to fit the picture inside it.
const (
	artCols = 24
	artRows = 12
	// artPixels is the longest edge asked of the server. A terminal cell is
	// around twice as tall as it is wide, so this is generous for the box
	// above and leaves the picture room on a screen with large cells.
	artPixels = 480
	// artChrome is the rows `:art` leaves for the title, the rules and the
	// last row, so that a cover filling the screen does not scroll it.
	artChrome = 5
)

// Messages the app sends itself.
type (
	playerEvent  mpv.Event
	playerGone   struct{ err error }
	scrobbled    struct{}
	detached     struct{}
	detachFailed struct{ err error }
	// deletePlaylist is agreed to after the question has been answered.
	deletePlaylist struct{ id, name string }
	// shuffled is a set of tracks to play in a random order.
	shuffled struct {
		tracks []subsonic.Song
		what   string
	}
	// savedQueue is what the server was holding when this started.
	savedQueue subsonic.PlayQueue
	// starredLoaded is what the server has starred, with the identifiers
	// gathered so that every list can mark them.
	starredLoaded struct {
		results subsonic.Results
		ids     map[string]bool
	}
)

// newApp builds the model, with the key bindings the configuration asks for.
//
// A binding that will not work is said on the last row rather than refusing to
// start: somebody with a typo in config.toml still wants their music.
func newApp(ctx context.Context, client *subsonic.Client, p player, cfg config.Config) app {
	ui := tui.New()
	if len(cfg.Keys) > 0 {
		keys, wrong := tui.Bind(cfg.Keys)
		ui = ui.WithKeys(keys)
		if len(wrong) > 0 {
			next, _ := ui.Update(tui.Failed{Message: "the key bindings in config.toml: " +
				strings.Join(wrong, "; ") + "\n\nrun `shanty doctor` for what can be bound"})
			ui = next.(tui.Model)
		}
	}
	return app{ui: ui, client: client, player: p, volume: 100, ctx: ctx}
}

func (a app) Init() tea.Cmd {
	return tea.Batch(a.fetchArtists(), a.watchPlayer(), a.observePosition(),
		a.fetchStarred(), func() tea.Msg { return a.flushBacklog() }, a.fetchSaved())
}

// View renders the interface. A session has no terminal to render to.
func (a app) View() string {
	if a.headless {
		return ""
	}
	return a.ui.View()
}

// Update acts on the intents the interface emits and passes everything else
// to it.
func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.Quit:
		// Where playback had reached goes to the server before this stops, so
		// another machine can carry on from it. One command rather than two,
		// because a batch would race the quit and the program may be gone
		// before the save runs.
		save := a.saveQueue()
		return a, func() tea.Msg {
			if save != nil {
				save()
			}
			return tea.QuitMsg{}
		}

	case tui.OpenArtist:
		return a, a.fetchArtist(msg.ID)
	case tea.WindowSizeMsg:
		a.size = msg
		return a.forward(msg)
	case tui.ShowArt:
		return a, a.fullArt()
	case tui.OpenAlbum:
		return a, a.fetchAlbum(msg.ID)
	case tui.AlbumLoaded:
		// The album is shown at once and its cover follows, because a picture
		// is worth none of the wait before a track list appears.
		next, cmd := a.forward(msg)
		return next, tea.Batch(cmd, a.fetchArt(subsonic.Album(msg)))
	case tui.Search:
		return a, a.search(string(msg))
	case tui.Resume:
		return a, a.resumeSaved()
	case tui.ShowPlaylists:
		return a.forwardAnd(msg, a.fetchPlaylists())
	case tui.OpenPlaylist:
		return a, a.fetchPlaylist(msg.ID)
	case tui.Playlist:
		return a, a.playlistCommand(msg)
	case tui.EditPlaylist:
		return a, a.editPlaylist(msg)
	case tui.Shuffle:
		return a, a.shuffleEverything()
	case deletePlaylist:
		return a, a.removePlaylist(msg)
	case shuffled:
		if len(msg.tracks) == 0 {
			return a.forward(tui.Failed{Message: "there is nothing to shuffle"})
		}
		a.queue = queueFrom(subsonic.Album{Songs: msg.tracks}).Shuffle(a.shuffler())
		next, cmd := a.forward(tui.Notice("shuffling " + msg.what))
		return next, tea.Batch(cmd, a.playCurrent(), a.queueChanged())
	case tui.Scan:
		return a.forwardAnd(msg, a.startScan())
	case scanning:
		return a.scanSaid(msg)
	case tui.Reload:
		// The interface remembers what was selected; this fetches what the
		// screen is showing.
		next, _ := a.forward(msg)
		return next, a.reload()
	case savedQueue:
		return a.offerSaved(subsonic.PlayQueue(msg))
	case tui.ToggleStar:
		return a, a.star(msg)
	case starredLoaded:
		a.starred = msg.ids
		return a.forward(tui.StarredChanged{Results: msg.results, IDs: msg.ids})

	case tui.PlayFrom:
		a.queue = queueFrom(msg.Album).Jump(msg.Index)
		return a, tea.Batch(a.playCurrent(), a.queueChanged())

	case tui.PlayNext:
		return a.enqueue(msg.Album, msg.Index, true)
	case tui.Enqueue:
		return a.enqueue(msg.Album, msg.Index, false)
	case tui.JumpTo:
		a.queue = a.queue.Jump(int(msg))
		return a, tea.Batch(a.playCurrent(), a.queueChanged())

	case tui.TogglePause:
		return a, a.setPaused(!a.paused())
	case tui.SetPaused:
		return a, a.setPaused(bool(msg))
	case tui.SkipNext:
		a.queue = a.queue.Next()
		return a, tea.Batch(a.playCurrent(), a.queueChanged())
	case tui.SkipPrev:
		a.queue = a.queue.Previous()
		return a, tea.Batch(a.playCurrent(), a.queueChanged())
	case tui.SeekBy:
		return a, a.act(func(ctx context.Context) error { return a.player.Seek(ctx, msg.By) })
	case tui.SeekTo:
		return a, a.act(func(ctx context.Context) error {
			return a.player.SeekTo(ctx, time.Duration(msg))
		})
	case tui.SeekToPercent:
		track, playing := a.queue.Current()
		if !playing || track.Duration == 0 {
			return a, emit(tui.Failed{Message: "nothing is playing to seek in"})
		}
		to := time.Duration(float64(track.Duration) * float64(msg) / 100)
		return a, a.act(func(ctx context.Context) error { return a.player.SeekTo(ctx, to) })
	case tui.VolumeBy:
		return a.setVolume(a.volume + msg.Delta)
	case tui.VolumeSet:
		return a.setVolume(int(msg))

	case tui.Detach:
		return a.startSession()
	case stateRequest:
		msg.reply <- a.state()
		return a, nil
	case handoverRequest:
		// The player goes to whoever asked, so this one must not stop it.
		a.released = true
		a.detaching = true
		msg.reply <- a.handoff()
		return a, nil

	case playerEvent:
		return a.playerSaid(mpv.Event(msg))
	case playerGone:
		if a.headless {
			// There is no screen to report this on and nothing left to play.
			return a, tea.Quit
		}
		return a.forward(tui.Failed{Message: "mpv stopped: " + msg.err.Error() + "\nrestart shanty to play again"})
	case scrobbled:
		return a, nil
	case detached:
		a.released = true
		return a, tea.Quit
	case detachFailed:
		a.detaching = false
		return a.forward(tui.Failed{Message: "the session did not start: " + msg.err.Error() + "\nnothing was interrupted; the music is still playing here"})
	}
	return a.forward(msg)
}

// forwardAnd passes a message to the interface and runs something of its own
// alongside whatever the interface asked for.
func (a app) forwardAnd(msg tea.Msg, also tea.Cmd) (tea.Model, tea.Cmd) {
	next, cmd := a.forward(msg)
	return next, tea.Batch(cmd, also)
}

// forward passes a message to the interface and returns what it asks for.
func (a app) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	ui, cmd := a.ui.Update(msg)
	a.ui = ui.(tui.Model)
	return a, cmd
}

// playerSaid handles one mpv event, advancing the queue when a track ends of
// its own accord.
func (a app) playerSaid(e mpv.Event) (tea.Model, tea.Cmd) {
	next := a.watchPlayer()
	switch {
	case e.Name == "property-change" && e.Property == "time-pos":
		if pos, ok := e.Float(); ok {
			ui, _ := a.ui.Update(tui.Progress(time.Duration(pos * float64(time.Second))))
			a.ui = ui.(tui.Model)
		}
		return a, next

	case e.Name == "end-file":
		// Only "eof" means the track finished. Any other reason means the queue has
		// already moved.
		if e.Reason != "eof" {
			return a, next
		}
		if a.detaching {
			// The session owns the queue from the moment it was started, so
			// advancing here would skip a track and report it twice.
			return a, next
		}
		finished, _ := a.queue.Current()
		a.queue = a.queue.Next()
		if a.headless && a.queue.Done() {
			return a, tea.Batch(a.scrobble(finished.ID), tea.Quit)
		}
		return a, tea.Batch(next, a.scrobble(finished.ID), a.playCurrent(),
			a.queueChanged(), a.saveQueue())
	}
	return a, next
}

func queueFrom(album subsonic.Album) queue.Queue {
	tracks := make([]queue.Track, 0, len(album.Songs))
	for _, s := range album.Songs {
		tracks = append(tracks, trackFrom(album, s))
	}
	return queue.New(tracks...)
}

// playCurrent loads the current track and appends the next, so mpv opens it
// early.
func (a app) playCurrent() tea.Cmd {
	track, ok := a.queue.Current()
	if !ok {
		return emit(tui.NowPlaying{})
	}
	upcoming, hasNext := a.queue.Upcoming()
	client, p := a.client, a.player
	return func() tea.Msg {
		ctx := a.ctx
		if err := p.Load(ctx, client.StreamURL(track.ID)); err != nil {
			return tui.Failed{Message: err.Error()}
		}
		if hasNext {
			// A failed append costs a gap between tracks and is not reported.
			_ = p.Append(ctx, client.StreamURL(upcoming.ID))
		}
		return tui.NowPlaying{Title: track.Title, Artist: track.Artist, Duration: track.Duration}
	}
}

// setVolume clamps and applies a volume, telling the screen the value it
// actually took.
func (a app) setVolume(to int) (tea.Model, tea.Cmd) {
	a.volume = max(0, min(100, to))
	volume := a.volume
	return a, tea.Batch(
		a.act(func(ctx context.Context) error { return a.player.SetVolume(ctx, volume) }),
		emit(tui.VolumeChanged(volume)))
}

func (a app) setPaused(paused bool) tea.Cmd {
	p := a.player
	return func() tea.Msg {
		if err := p.SetPause(a.ctx, paused); err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.PausedChanged(paused)
	}
}

// paused reports what the interface is currently showing.
func (a app) paused() bool { return a.ui.Paused() }

// scrobble tells the server a track was played, and keeps the report if it
// cannot be delivered.
//
// A session reports plays where nobody is watching, so a report that is
// dropped is a listening history that quietly loses days. The report is kept
// with the time it happened and sent on the next one that works.
func (a app) scrobble(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	client, path, at := a.client, a.backlog, time.Now()
	return func() tea.Msg {
		if err := client.Scrobble(a.ctx, id, true, at); err != nil {
			if path != "" {
				_ = backlog.Add(path, backlog.Play{ID: id, At: at})
			}
			return scrobbled{}
		}
		// The server is answering, so anything waiting has a chance now.
		return a.flushBacklog()
	}
}

// flushBacklog reports the plays the server would not take before, and keeps
// whatever it still will not take.
func (a app) flushBacklog() tea.Msg {
	if a.backlog == "" {
		return scrobbled{}
	}
	waiting, err := backlog.Load(a.backlog)
	if err != nil || len(waiting) == 0 {
		return scrobbled{}
	}
	var left []backlog.Play
	for i, play := range waiting {
		if err := a.client.Scrobble(a.ctx, play.ID, true, play.At); err != nil {
			// The server has stopped taking them again, so this one and
			// everything after it stays rather than being tried one by one.
			left = waiting[i:]
			break
		}
	}
	_ = backlog.Replace(a.backlog, left)
	return scrobbled{}
}

func (a app) fetchArtists() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		artists, err := client.Artists(a.ctx)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.ArtistsLoaded(artists)
	}
}

func (a app) fetchArtist(id string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		artist, err := client.Artist(a.ctx, id)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.ArtistLoaded(artist)
	}
}

func (a app) fetchAlbum(id string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		album, err := client.Album(a.ctx, id)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.AlbumLoaded(album)
	}
}

// watchPlayer waits for one mpv event and returns it. A closed channel means
// the player has stopped.
func (a app) watchPlayer() tea.Cmd {
	p := a.player
	return func() tea.Msg {
		e, ok := <-p.Events()
		if !ok {
			err := p.Err()
			if err == nil {
				err = context.Canceled
			}
			return playerGone{err: err}
		}
		return playerEvent(e)
	}
}

func (a app) observePosition() tea.Cmd {
	return a.act(func(ctx context.Context) error { return a.player.Observe(ctx, "time-pos") })
}

// act runs an operation against the player and turns a failure into a message
// on screen.
func (a app) act(do func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		if err := do(a.ctx); err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return nil
	}
}

func emit(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }

// state describes what the session is doing, for a command in another shell.
func (a app) state() control.State {
	track, playing := a.queue.Current()
	s := control.State{
		Paused:   a.paused(),
		Position: int(a.ui.Position().Seconds()),
		Volume:   a.volume,
		Track:    a.queue.At() + 1,
		Of:       a.queue.Len(),
	}
	if playing {
		s.Title, s.Artist, s.Duration = track.Title, track.Artist, int(track.Duration.Seconds())
	}
	return s
}

// clock formats a duration as minutes and seconds.
func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// enqueue adds a track to the queue, either after the one playing or at the
// end, and starts it if nothing was playing.
func (a app) enqueue(album subsonic.Album, index int, next bool) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(album.Songs) {
		return a, nil
	}
	track := trackFrom(album, album.Songs[index])
	wasIdle := a.queue.Done()

	if next {
		a.queue = a.queue.InsertNext(track)
	} else {
		a.queue = a.queue.Append(track)
	}

	// A queue with nothing playing starts on what was just added; one that is
	// playing keeps playing, and mpv is told what now comes next.
	if wasIdle {
		return a, tea.Batch(a.playCurrent(), a.queueChanged(),
			emit(tui.Notice("playing "+track.Title)))
	}
	where := "added " + track.Title + " to the end of the queue"
	if next {
		where = "playing " + track.Title + " next"
	}
	return a, tea.Batch(a.prefetch(), a.queueChanged(), emit(tui.Notice(where)))
}

// prefetch tells mpv which track follows the one playing.
func (a app) prefetch() tea.Cmd {
	upcoming, ok := a.queue.Upcoming()
	url := ""
	if ok {
		url = a.client.StreamURL(upcoming.ID)
	}
	p := a.player
	return a.act(func(ctx context.Context) error { return p.Prefetch(ctx, url) })
}

// star turns the starred state of something over on the server, and reads the
// list back so that every screen showing it agrees.
func (a app) star(msg tui.ToggleStar) tea.Cmd {
	client := a.client
	kind := map[tui.Kind]subsonic.Kind{
		tui.StarArtist: subsonic.KindArtist,
		tui.StarAlbum:  subsonic.KindAlbum,
		tui.StarSong:   subsonic.KindSong,
	}[msg.Kind]
	return func() tea.Msg {
		if err := client.Star(a.ctx, kind, msg.ID, msg.Starred); err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return a.starredMsg()
	}
}

// fetchStarred reads what the server has starred.
func (a app) fetchStarred() tea.Cmd {
	return func() tea.Msg { return a.starredMsg() }
}

// starredMsg asks the server what is starred and gathers the identifiers.
func (a app) starredMsg() tea.Msg {
	found, err := a.client.Starred(a.ctx)
	if err != nil {
		return tui.Failed{Message: err.Error()}
	}
	ids := map[string]bool{}
	for _, x := range found.Artists {
		ids[x.ID] = true
	}
	for _, x := range found.Albums {
		ids[x.ID] = true
	}
	for _, x := range found.Songs {
		ids[x.ID] = true
	}
	return starredLoaded{results: found, ids: ids}
}

// search asks the server for anything matching the query.
func (a app) search(query string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		found, err := client.Search(a.ctx, query)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.SearchLoaded{Query: query, Results: found}
	}
}

// queueChanged sends the interface the queue to draw.
func (a app) queueChanged() tea.Cmd {
	return emit(tui.QueueChanged{Tracks: a.queue.Tracks(), At: a.queue.At()})
}

// trackFrom turns a song on an album into a queue entry.
func trackFrom(album subsonic.Album, s subsonic.Song) queue.Track {
	return queue.Track{
		ID: s.ID, Title: s.Title, Album: album.Name, Artist: s.Artist,
		Duration: time.Duration(s.Duration) * time.Second,
	}
}

// reload asks the server again for what the screen is showing.
//
// Only that screen, rather than the whole library: on a large one, fetching
// everything to see a new album is slow, and the screens above are refetched
// when they are next opened anyway.
func (a app) reload() tea.Cmd {
	switch a.ui.Screen() {
	case tui.ScreenArtists:
		return a.fetchArtists()
	case tui.ScreenAlbums:
		return a.fetchArtist(a.ui.Artist().ID)
	case tui.ScreenTracks:
		return a.fetchAlbum(a.ui.Album().ID)
	case tui.ScreenStarred:
		return a.fetchStarred()
	}
	return emit(tui.Failed{Message: "there is nothing here to fetch again"})
}

// fetchSaved asks the server what queue it is holding.
func (a app) fetchSaved() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		found, err := client.PlayQueue(a.ctx)
		if err != nil {
			// Nothing to resume is the ordinary case, and a server that will
			// not say is not worth interrupting anybody about.
			return nil
		}
		return savedQueue(found)
	}
}

// offerSaved keeps the queue the server is holding and says it is there. It is
// not taken up until somebody asks, because quitting is usually deliberate.
func (a app) offerSaved(found subsonic.PlayQueue) (tea.Model, tea.Cmd) {
	if found.Empty() || !a.queue.Done() {
		return a, nil
	}
	a.saved = found

	// The server keeps one queue for the whole account rather than one per
	// machine, so what is there was left either by this program somewhere or
	// by another client. The two read differently and are worth telling apart.
	who := "you left"
	switch {
	case found.ChangedBy == "":
		who = "your server has"
	case found.ChangedBy != subsonic.ClientName:
		who = found.ChangedBy + " left"
	}
	return a.forward(tui.Notice(fmt.Sprintf(
		"%s %s playing — type :resume to carry on from it",
		who, found.Songs[found.Index()].Title)))
}

// resumeSaved takes up the queue the server was holding.
func (a app) resumeSaved() tea.Cmd {
	if a.saved.Empty() {
		return emit(tui.Failed{Message: "there is no queue saved on your server to carry on from"})
	}
	return emit(tui.PlayFrom{Album: albumOf(a.saved), Index: a.saved.Index()})
}

// albumOf gathers a saved queue into something that can be played from. The
// tracks may come from several albums, so it is a list rather than one of
// them.
func albumOf(saved subsonic.PlayQueue) subsonic.Album {
	return subsonic.Album{Songs: saved.Songs}
}

// saveQueue tells the server what is queued and where in it playback has
// reached. A queue with nothing in it is not saved, because that would wipe
// what another machine left.
func (a app) saveQueue() tea.Cmd {
	tracks := a.queue.Tracks()
	if len(tracks) == 0 {
		return nil
	}
	ids := make([]string, 0, len(tracks))
	for _, t := range tracks {
		ids = append(ids, t.ID)
	}
	current, _ := a.queue.Current()
	client, at := a.client, a.ui.Position()
	return func() tea.Msg {
		// A server that will not keep it is not worth stopping for, and the
		// next track change tries again.
		_ = client.SavePlayQueue(a.ctx, ids, current.ID, at)
		return nil
	}
}

// scanInterval is how often a running scan is asked how it is going. A scan
// takes minutes, so asking often would be noise.
var scanInterval = 2 * time.Second

// scanning is one report of how a server's scan is going.
type scanning struct {
	status subsonic.Scan
	err    error
}

// startScan asks the server to look at its music folder again.
//
// The work is the server's and takes minutes, so nothing here waits for it.
// Each answer arrives as a message and asks for the next one, the same way
// mpv's events are read, and playback carries on throughout.
func (a app) startScan() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		status, err := client.StartScan(a.ctx)
		return scanning{status: status, err: err}
	}
}

// watchScan asks again after a pause.
func (a app) watchScan() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		select {
		case <-a.ctx.Done():
			return nil
		case <-time.After(scanInterval):
		}
		status, err := client.ScanStatus(a.ctx)
		return scanning{status: status, err: err}
	}
}

// scanSaid reports how the scan is going, and reloads the library once it is
// over so the new music is there without a second command.
func (a app) scanSaid(msg scanning) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		var server *subsonic.Error
		if errors.As(msg.err, &server) && server.Unimplemented() {
			return a.forward(tui.Failed{Message: "your server does not offer to scan its library\n\nstart the scan from the server itself"})
		}
		return a.forward(tui.Failed{Message: msg.err.Error()})
	}
	if msg.status.Scanning {
		next, _ := a.forward(tui.Notice(fmt.Sprintf("scanning: %s so far", tracks(msg.status.Count))))
		return next, a.watchScan()
	}
	next, _ := a.forward(tui.Notice(fmt.Sprintf("the scan finished: %s", tracks(msg.status.Count))))
	return next, tea.Batch(emit(tui.Reload{}), a.reload())
}

// tracks counts what a scan has looked at, in words.
func tracks(n int64) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}

// fetchPlaylists reads every playlist the server will show.
func (a app) fetchPlaylists() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		found, err := client.Playlists(a.ctx)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.PlaylistsLoaded(found)
	}
}

// fetchPlaylist reads one playlist with its tracks.
func (a app) fetchPlaylist(id string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		found, err := client.Playlist(a.ctx, id)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.PlaylistLoaded(found)
	}
}

// playlistCommand does one thing to a playlist named by what it is called.
func (a app) playlistCommand(msg tui.Playlist) tea.Cmd {
	if msg.Verb == "create" {
		client := a.client
		return func() tea.Msg {
			if _, err := client.CreatePlaylist(a.ctx, msg.Name); err != nil {
				return tui.Failed{Message: err.Error()}
			}
			return tui.Notice("made " + msg.Name)
		}
	}

	client := a.client
	return func() tea.Msg {
		found, err := byName(a.ctx, client, msg.Name)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		switch msg.Verb {
		case "edit":
			// Read it with its tracks, so the marks are right from the first
			// screen rather than after the first change.
			full, err := client.Playlist(a.ctx, found.ID)
			if err != nil {
				return tui.Failed{Message: err.Error()}
			}
			return tui.EditingPlaylist(full)
		case "delete":
			// The one thing here that cannot be undone from inside shanty.
			return tui.Confirm{
				Question: fmt.Sprintf("delete %q and its %s?", found.Name, tracksIn(found)),
				Do:       deletePlaylist{id: found.ID, name: found.Name},
			}
		case "shuffle":
			full, err := client.Playlist(a.ctx, found.ID)
			if err != nil {
				return tui.Failed{Message: err.Error()}
			}
			return shuffled{tracks: full.Songs, what: full.Name}
		}
		return nil
	}
}

// byName finds the one playlist called that.
//
// A server's playlist names are not unique, only their identifiers are, so two
// with the same name is a question rather than a guess.
func byName(ctx context.Context, client *subsonic.Client, name string) (subsonic.Playlist, error) {
	found, err := client.Playlists(ctx)
	if err != nil {
		return subsonic.Playlist{}, err
	}
	var matched []subsonic.Playlist
	for _, p := range found {
		if strings.EqualFold(p.Name, name) {
			matched = append(matched, p)
		}
	}
	switch len(matched) {
	case 0:
		return subsonic.Playlist{}, fmt.Errorf("there is no playlist called %q on your server", name)
	case 1:
		return matched[0], nil
	}
	return subsonic.Playlist{}, fmt.Errorf("%d playlists are called %q, so shanty cannot tell which you mean\n\nrename one from your server", len(matched), name)
}

// editPlaylist adds a track to the playlist being edited or takes it out, and
// reads the playlist back so the marks follow the server.
func (a app) editPlaylist(msg tui.EditPlaylist) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		if msg.Add {
			if err := client.AddToPlaylist(a.ctx, msg.ID, msg.SongID); err != nil {
				return tui.Failed{Message: err.Error()}
			}
		} else {
			full, err := client.Playlist(a.ctx, msg.ID)
			if err != nil {
				return tui.Failed{Message: err.Error()}
			}
			// The server removes by position, and the same track can be in a
			// playlist more than once. The last one is what a second press
			// after an add takes back out.
			at := -1
			for i, s := range full.Songs {
				if s.ID == msg.SongID {
					at = i
				}
			}
			if at < 0 {
				return tui.Failed{Message: "that track is not in the playlist any more"}
			}
			if err := client.RemoveFromPlaylist(a.ctx, msg.ID, at); err != nil {
				return tui.Failed{Message: err.Error()}
			}
		}
		full, err := client.Playlist(a.ctx, msg.ID)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.PlaylistLoaded(full)
	}
}

// shuffleEverything plays the whole library in a random order.
func (a app) shuffleEverything() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		artists, err := client.Artists(a.ctx)
		if err != nil {
			return tui.Failed{Message: err.Error()}
		}
		var songs []subsonic.Song
		for _, artist := range artists {
			full, err := client.Artist(a.ctx, artist.ID)
			if err != nil {
				return tui.Failed{Message: err.Error()}
			}
			for _, al := range full.Albums {
				album, err := client.Album(a.ctx, al.ID)
				if err != nil {
					return tui.Failed{Message: err.Error()}
				}
				songs = append(songs, album.Songs...)
			}
		}
		return shuffled{tracks: songs, what: "everything"}
	}
}

// tracksIn counts a playlist, in words.
func tracksIn(p subsonic.Playlist) string {
	if p.SongCount == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", p.SongCount)
}

// removePlaylist deletes one, after the question has been answered.
func (a app) removePlaylist(msg deletePlaylist) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		if err := client.DeletePlaylist(a.ctx, msg.id); err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.Notice("deleted " + msg.name)
	}
}

// shuffler is where a shuffle gets its randomness. It is a field so a test can
// pin the order.
func (a app) shuffler() *rand.Rand {
	if a.random != nil {
		return a.random
	}
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}

// fetchArt gets an album's cover, from the cache if it is there, and renders
// it for this terminal.
//
// Nothing here fails loudly. A cover that cannot be had is a screen without
// one, and the placeholder holds its place.
func (a app) fetchArt(album subsonic.Album) tea.Cmd {
	if album.CoverArt == "" || a.headless {
		return nil
	}
	client, art, covers := a.client, a.art, a.covers
	return func() tea.Msg {
		data, found := covers.Get(album.CoverArt, artPixels)
		if !found {
			fetched, err := client.CoverArt(a.ctx, album.CoverArt, artPixels)
			if err != nil {
				return nil
			}
			// A cover that cannot be kept is still a cover worth showing, so
			// the write failing costs the next run a request and nothing else.
			_ = covers.Put(album.CoverArt, artPixels, fetched)
			data = fetched
		}
		return tui.CoverArt{
			AlbumID: album.ID,
			Lines:   cover.Block(art, data, artCols, artRows),
			Clear:   cover.Clear(art),
		}
	}
}

// fullArt renders the open album's cover to fill the terminal.
//
// The bytes come from the cache, which is where the cover above the track list
// put them, so looking at a picture costs no request. An album whose cover has
// not been fetched is one nobody has seen, and there is nothing to enlarge.
func (a app) fullArt() tea.Cmd {
	album := a.ui.Album()
	cols, rows := a.size.Width, a.size.Height-artChrome
	if album.CoverArt == "" || cols < 1 || rows < 1 {
		return func() tea.Msg { return tui.Failed{Message: "there is no cover to show here"} }
	}
	data, found := a.covers.Get(album.CoverArt, artPixels)
	if !found {
		return func() tea.Msg { return tui.Failed{Message: "that cover has not been fetched yet"} }
	}
	art := a.art
	return func() tea.Msg { return tui.FullArt(cover.Block(art, data, cols, rows)) }
}
