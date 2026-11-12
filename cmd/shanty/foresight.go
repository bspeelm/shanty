package main

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/sky"
	"github.com/bspeelm/shanty/internal/tui"
)

// analyser measures a whole track ahead of playback.
type analyser interface {
	Analyse(ctx context.Context, url string) ([]mpv.Block, error)
}

// silentMpv is an analyser that starts a second, silent mpv for each track.
type silentMpv mpv.Options

func (o silentMpv) Analyse(ctx context.Context, url string) ([]mpv.Block, error) {
	return mpv.Analyse(ctx, mpv.Options(o), url)
}

// foresight is what has been measured ahead of playback this session, by
// track identifier. A track whose measuring failed holds a nil plan and is
// not measured again. order is the identifiers in plans, oldest first.
// running is the track being measured, job counts the measurings started,
// and stop cancels the one under way.
type foresight struct {
	plans   map[string]*sky.Plan
	order   []string
	running string
	job     int
	stop    context.CancelFunc
}

// foresightKept is how many tracks' plans are kept. A track whose plan has
// been dropped is measured again when it is next wanted.
const foresightKept = 200

// keep holds the plan of a track, dropping the oldest once more than
// foresightKept are held.
func (f *foresight) keep(id string, plan *sky.Plan) {
	if _, held := f.plans[id]; !held {
		f.order = append(f.order, id)
	}
	f.plans[id] = plan
	for len(f.order) > foresightKept {
		delete(f.plans, f.order[0])
		f.order = f.order[1:]
	}
}

// foreseen is the result of measuring one track, with the number of the
// measuring that produced it.
type foreseen struct {
	id, title string
	job       int
	plan      *sky.Plan
	err       error
}

// foresee sends the visualiser the plans of the track playing and the one
// after it, and starts measuring the first of them not yet measured. It
// measures one track at a time, and cancels the measuring of a track that is
// neither of them. It does nothing while the visualiser is closed.
func (a app) foresee() tea.Cmd {
	f := a.foresight
	if a.analyser == nil || f == nil || !a.ui.ShowingSky() {
		return nil
	}
	var tracks []queue.Track
	if t, ok := a.queue.Current(); ok {
		tracks = append(tracks, t)
	}
	if t, ok := a.queue.Upcoming(); ok {
		tracks = append(tracks, t)
	}
	wanted := false
	for _, t := range tracks {
		wanted = wanted || t.ID == f.running
	}
	if !wanted {
		f.cancel()
	}
	var cmds []tea.Cmd
	for _, t := range tracks {
		plan, done := f.plans[t.ID]
		switch {
		case done && plan != nil:
			cmds = append(cmds, emit(tui.Foresight{Title: t.Title, Artist: t.Artist, Plan: plan}))
		case !done && f.running == "":
			ctx, stop := context.WithCancel(a.ctx)
			f.job++
			f.running, f.stop = t.ID, stop
			cmds = append(cmds, a.measure(ctx, t, f.job))
		}
	}
	return tea.Batch(cmds...)
}

// cancel stops the measuring under way, if there is one.
func (f *foresight) cancel() {
	if f.running != "" {
		f.stop()
		f.running = ""
	}
}

// measure measures one track and builds its plan.
func (a app) measure(ctx context.Context, t queue.Track, job int) tea.Cmd {
	url, an := a.client.StreamURL(t.ID), a.analyser
	return func() tea.Msg {
		blocks, err := an.Analyse(ctx, url)
		out := foreseen{id: t.ID, title: t.Title, job: job, err: err}
		if err == nil && len(blocks) > 0 {
			out.plan = sky.NewPlan(planBlocks(blocks))
		}
		return out
	}
}

// sawAhead keeps the plan of a measured track and moves on to the next. A
// cancelled measuring is not kept, so the track is measured again when it is
// next wanted.
func (a app) sawAhead(msg foreseen) (tea.Model, tea.Cmd) {
	f := a.foresight
	if msg.job == f.job {
		f.cancel()
	}
	if !errors.Is(msg.err, context.Canceled) {
		f.keep(msg.id, msg.plan)
	}
	return a, a.foresee()
}

// planBlocks converts measured blocks into the form the planner takes.
func planBlocks(blocks []mpv.Block) []sky.Block {
	out := make([]sky.Block, len(blocks))
	for i, b := range blocks {
		channels := make([]sky.Channel, len(b.Channels))
		for j, c := range b.Channels {
			channels[j] = sky.Channel{Loudness: c.Loudness, Difference: c.Difference, Crossings: c.Crossings}
		}
		out[i] = sky.Block{At: b.At.Seconds(), Samples: b.Samples, Channels: channels, Bands: b.Bands, Punch: b.Punch, Rate: b.Rate, Scale: b.Scale}
	}
	return out
}
