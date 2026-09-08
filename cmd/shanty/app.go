package main

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
)

func newApp(ctx context.Context, client *subsonic.Client, p player) app {
	return app{ui: tui.New(), client: client, player: p, volume: 100, ctx: ctx}
}

func (a app) Init() tea.Cmd {
	return tea.Batch(a.fetchArtists(), a.watchPlayer(), a.observePosition())
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
		return a, tea.Quit

	case tui.OpenArtist:
		return a, a.fetchArtist(msg.ID)
	case tui.OpenAlbum:
		return a, a.fetchAlbum(msg.ID)

	case tui.PlayFrom:
		a.queue = queueFrom(msg.Album).Jump(msg.Index)
		return a, a.playCurrent()

	case tui.TogglePause:
		return a, a.togglePause()
	case tui.SkipNext:
		a.queue = a.queue.Next()
		return a, a.playCurrent()
	case tui.SkipPrev:
		a.queue = a.queue.Previous()
		return a, a.playCurrent()
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
		return a, tea.Batch(next, a.scrobble(finished.ID), a.playCurrent())
	}
	return a, next
}

func queueFrom(album subsonic.Album) queue.Queue {
	tracks := make([]queue.Track, 0, len(album.Songs))
	for _, s := range album.Songs {
		tracks = append(tracks, queue.Track{
			ID: s.ID, Title: s.Title, Album: album.Name, Artist: s.Artist,
			Duration: time.Duration(s.Duration) * time.Second,
		})
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

func (a app) togglePause() tea.Cmd {
	p := a.player
	paused := !a.paused()
	return func() tea.Msg {
		if err := p.SetPause(a.ctx, paused); err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return tui.PausedChanged(paused)
	}
}

// paused reports what the interface is currently showing.
func (a app) paused() bool { return a.ui.Paused() }

func (a app) scrobble(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	client := a.client
	return func() tea.Msg {
		// A failed report is not surfaced. A durable queue for them comes later.
		_ = client.Scrobble(a.ctx, id, true)
		return scrobbled{}
	}
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
		Position: clock(a.ui.Position()),
		Volume:   a.volume,
		Track:    a.queue.At() + 1,
		Of:       a.queue.Len(),
	}
	if playing {
		s.Title, s.Artist, s.Duration = track.Title, track.Artist, clock(track.Duration)
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
