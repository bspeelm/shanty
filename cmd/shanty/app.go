package main

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/backlog"
	"github.com/bspeelm/shanty/internal/control"
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
}

// Messages the app sends itself.
type (
	playerEvent  mpv.Event
	playerGone   struct{ err error }
	scrobbled    struct{}
	detached     struct{}
	detachFailed struct{ err error }
	// savedQueue is what the server was holding when this started.
	savedQueue subsonic.PlayQueue
	// starredLoaded is what the server has starred, with the identifiers
	// gathered so that every list can mark them.
	starredLoaded struct {
		results subsonic.Results
		ids     map[string]bool
	}
)

func newApp(ctx context.Context, client *subsonic.Client, p player) app {
	return app{ui: tui.New(), client: client, player: p, volume: 100, ctx: ctx}
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
	case tui.OpenAlbum:
		return a, a.fetchAlbum(msg.ID)
	case tui.Search:
		return a, a.search(string(msg))
	case tui.Resume:
		return a, a.resumeSaved()
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
	where := "your server"
	if found.ChangedBy != "" {
		where = found.ChangedBy
	}
	return a.forward(tui.Notice(fmt.Sprintf(
		"%s left %s playing — type :resume to carry on from it",
		where, found.Songs[found.Index()].Title)))
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
