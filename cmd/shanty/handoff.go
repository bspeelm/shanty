package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/control"
	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/tui"
)

// handoff is what an interface gives a session so it can carry on playing.
//
// It names tracks by id. The session builds its own stream URLs from the
// credential it reads itself, so no credential is written down here.
type handoff struct {
	Protocol int           `json:"protocol"`
	Tracks   []queue.Track `json:"tracks"`
	At       int           `json:"at"`
	Volume   int           `json:"volume"`
	Paused   bool          `json:"paused"`
	// Position is how far into the current track playback has reached. It is
	// zero in the handoff that starts a session, which is taking over a player
	// that is already at the right place.
	Position time.Duration `json:"position,omitempty"`
}

// startSession hands the queue to a session and quits once it has it. The
// player is left running for the session to take over.
func (a app) startSession() (tea.Model, tea.Cmd) {
	if a.detach == nil {
		return a.forward(tui.Failed{Message: "this is already a session; there is no interface to leave"})
	}
	if a.queue.Done() {
		return a.forward(tui.Failed{Message: "there is nothing playing to leave behind\n\nplay something, then type :headless"})
	}

	h := a.handoff()
	// The queue belongs to the session from here, whether or not it starts.
	// Losing one report if it fails is better than making two if it works.
	a.detaching = true
	detach := a.detach
	start := func() tea.Msg {
		if err := detach(h); err != nil {
			return detachFailed{err: err}
		}
		return detached{}
	}
	ui, _ := a.ui.Update(tui.Notice("leaving the interface; the music keeps playing"))
	a.ui = ui.(tui.Model)
	return a, start
}

// handoff is what this process would give another to carry on from here.
func (a app) handoff() handoff {
	return handoff{
		Protocol: control.Protocol,
		Tracks:   a.queue.Tracks(),
		At:       a.queue.At(),
		Volume:   a.volume,
		Paused:   a.paused(),
		Position: a.ui.Position(),
	}
}

// readHandoff reads what the interface left for this session.
func readHandoff(r io.Reader) (handoff, error) {
	var h handoff
	b, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return h, err
	}
	if len(b) == 0 {
		return h, errors.New("the interface sent nothing to play")
	}
	if err := json.Unmarshal(b, &h); err != nil {
		return h, errors.New("what the interface sent could not be read")
	}
	if h.Protocol != control.Protocol {
		return h, errors.New("the interface and the session are different versions of shanty")
	}
	if len(h.Tracks) == 0 {
		return h, errors.New("the interface sent nothing to play")
	}
	return h, nil
}

// resume returns the queue the handoff describes.
func (h handoff) resume() queue.Queue { return queue.New(h.Tracks...).Jump(h.At) }

// nowPlaying describes the track the session is on, for the interface to show.
func (h handoff) nowPlaying() (tui.NowPlaying, time.Duration) {
	t := h.Tracks[min(h.At, len(h.Tracks)-1)]
	return tui.NowPlaying{Title: t.Title, Artist: t.Artist, Duration: t.Duration}, t.Duration
}

// playing reports whether a session is answering on the control socket.
func playing(env Env) bool {
	_, err := control.Send(env.Paths.ControlSocket(), control.Request{Verb: control.Status})
	return err == nil
}

// listening reports whether anything is answering on the socket.
func listening(socket string) bool {
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// takeOver asks a running session for what it is playing. The second result
// reports whether there was one.
func takeOver(env Env) (handoff, bool, error) {
	res, err := control.Send(env.Paths.ControlSocket(), control.Request{Verb: control.Attach})
	if err != nil {
		var none control.ErrNoSession
		if errors.As(err, &none) {
			return handoff{}, false, nil
		}
		return handoff{}, false, err
	}
	if !res.OK {
		return handoff{}, false, errors.New(res.Error)
	}
	h, err := readHandoff(bytes.NewReader(res.Handover))
	if err != nil {
		return handoff{}, false, err
	}
	return h, true, nil
}

// resume returns the app showing what a session was playing.
func (a app) resume(h handoff) app {
	a.queue = h.resume()
	a.volume = h.Volume
	playing, _ := h.nowPlaying()
	for _, msg := range []tea.Msg{
		playing,
		tui.Progress(h.Position),
		tui.PausedChanged(h.Paused),
		tui.VolumeChanged(h.Volume),
		tui.Notice("carried on from the session that was playing"),
	} {
		ui, _ := a.ui.Update(msg)
		a.ui = ui.(tui.Model)
	}
	return a
}
