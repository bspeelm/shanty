package main

import (
	"encoding/json"
	"errors"
	"io"
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

	h := handoff{
		Protocol: control.Protocol,
		Tracks:   a.queue.Tracks(),
		At:       a.queue.At(),
		Volume:   a.volume,
		Paused:   a.paused(),
	}
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
