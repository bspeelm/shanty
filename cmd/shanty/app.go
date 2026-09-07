package main

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/tui"
)

// player is what the app needs from mpv. An interface so the wiring can be
// tested where mpv is not installed, which is most places -- and so that
// internal/mpv's API is the one the app actually uses rather than whatever it
// happens to export.
type player interface {
	Load(ctx context.Context, url string) error
	Append(ctx context.Context, url string) error
	SetPause(ctx context.Context, paused bool) error
	Seek(ctx context.Context, d time.Duration) error
	SetVolume(ctx context.Context, percent int) error
	Observe(ctx context.Context, property string) error
	Events() <-chan mpv.Event
	Close() error
	Err() error
}

// app is the outer model. internal/tui renders and asks; this does. The split
// is what makes every screen testable without a server or a socket, and it is
// the reason tui cannot import net, os or os/exec (§4).
type app struct {
	ui     tui.Model
	client *subsonic.Client
	player player
	queue  queue.Queue
	volume int
	ctx    context.Context
}

// Messages the app sends itself. They are separate from tui's inputs because
// these are about the machinery, not about what is on screen.
type (
	playerEvent mpv.Event
	playerGone  struct{ err error }
	scrobbled   struct{}
)

func newApp(ctx context.Context, client *subsonic.Client, p player) app {
	return app{ui: tui.New(), client: client, player: p, volume: 100, ctx: ctx}
}

func (a app) Init() tea.Cmd {
	return tea.Batch(a.fetchArtists(), a.watchPlayer(), a.observePosition())
}

func (a app) View() string { return a.ui.View() }

// Update handles what the app must act on and passes everything else to the
// screen. An intent that reaches the default branch is one nobody wired up,
// which a test asserts cannot happen.
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
	case tui.VolumeBy:
		a.volume = max(0, min(100, a.volume+msg.Delta))
		volume := a.volume
		return a, tea.Batch(
			a.act(func(ctx context.Context) error { return a.player.SetVolume(ctx, volume) }),
			emit(tui.VolumeChanged(volume)))

	case playerEvent:
		return a.playerSaid(mpv.Event(msg))
	case playerGone:
		return a.forward(tui.Failed{Message: "mpv stopped: " + msg.err.Error() + "\nrestart shanty to play again"})
	case scrobbled:
		return a, nil
	}
	return a.forward(msg)
}

// forward hands a message to the screen and keeps whatever it asks for.
func (a app) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	ui, cmd := a.ui.Update(msg)
	a.ui = ui.(tui.Model)
	return a, cmd
}

// playerSaid turns mpv's notifications into what the screen understands, and
// advances the queue when a track ends of its own accord.
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
		// "eof" is the track finishing; anything else is us having already
		// moved, and advancing again would skip one.
		if e.Reason != "eof" {
			return a, next
		}
		finished, _ := a.queue.Current()
		a.queue = a.queue.Next()
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

// playCurrent loads the current track and queues the one after it, which is
// what makes mpv's prefetch gapless (§7).
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
			// Unchecked on purpose: failing to prefetch costs a gap between
			// tracks, and reporting it would replace the track that is
			// playing with an error about the one that is not.
			_ = p.Append(ctx, client.StreamURL(upcoming.ID))
		}
		return tui.NowPlaying{Title: track.Title, Artist: track.Artist, Duration: track.Duration}
	}
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

// paused reads the screen's own idea of the state, so the key toggles what the
// user can see rather than a second copy that could disagree with it.
func (a app) paused() bool { return a.ui.Paused() }

func (a app) scrobble(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	client := a.client
	return func() tea.Msg {
		// Unchecked: a scrobble that does not land is a play the server does
		// not know about, which is not worth interrupting the music for. The
		// backlog that makes it durable is v0.2.
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

// watchPlayer takes one event and re-arms. A closed channel is mpv gone, which
// is reported once and never respawned (§7).
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

// act runs something against the player and turns a failure into a line on
// screen rather than a crash.
func (a app) act(do func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		if err := do(a.ctx); err != nil {
			return tui.Failed{Message: err.Error()}
		}
		return nil
	}
}

func emit(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }
