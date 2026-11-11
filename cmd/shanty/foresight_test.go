package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/tui"
)

// seer is a fake analyser. It records the URL and context of each track it
// is asked to measure, and either answers at once with ten seconds of steady
// sound or, when hold is set, waits for the context to end.
type seer struct {
	mu   sync.Mutex
	urls []string
	ctxs []context.Context
	hold bool
}

func (s *seer) Analyse(ctx context.Context, url string) ([]mpv.Block, error) {
	s.mu.Lock()
	s.urls = append(s.urls, url)
	s.ctxs = append(s.ctxs, ctx)
	hold := s.hold
	s.mu.Unlock()
	if hold {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	var out []mpv.Block
	for i := range 430 {
		c := mpv.Channel{Loudness: -20, Difference: 100, Crossings: 50}
		out = append(out, mpv.Block{At: time.Duration(i) * 1024 * time.Second / 44100,
			Samples: 1024, Channels: []mpv.Channel{c, c}, Rate: 44100, Scale: 1 << 15})
	}
	return out, nil
}

func (s *seer) asked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.urls...)
}

// lastContext waits for the seer to be asked to measure, and returns the
// context of the latest track it was asked for.
func (s *seer) lastContext(t *testing.T) context.Context {
	t.Helper()
	for range 400 {
		s.mu.Lock()
		n := len(s.ctxs)
		s.mu.Unlock()
		if n > 0 {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.ctxs[n-1]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("nothing was measured")
	return nil
}

// playingWithASeer is an app playing the first of three tracks, with a fake
// analyser.
func playingWithASeer(t *testing.T, hold bool) (app, *seer) {
	t.Helper()
	a, _, _ := wired(t)
	s := &seer{hold: hold}
	a.analyser = s
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})
	return a, s
}

// plansFor returns the tracks named by the Foresight messages among msgs.
func plansFor(msgs []tea.Msg) []string {
	var got []string
	for _, m := range msgs {
		if f, ok := m.(tui.Foresight); ok && f.Plan != nil {
			got = append(got, f.Track)
		}
	}
	return got
}

func TestOpeningTheVisualiserMeasuresTheTrackPlayingAndTheNext(t *testing.T) {
	a, s := playingWithASeer(t, false)
	a, out := step(t, a, tui.ShowVisualiser{})
	first := only[foreseen](t, out)
	if first.id != "tr-1" || first.plan == nil {
		t.Fatalf("the first track measured was %q with plan %v", first.id, first.plan)
	}
	a, out = step(t, a, first)
	if got := plansFor(out); len(got) != 1 || got[0] != "Slipway" {
		t.Errorf("after the first measuring the visualiser was sent plans for %v", got)
	}
	second := only[foreseen](t, out)
	if second.id != "tr-2" {
		t.Fatalf("the second track measured was %q, want tr-2", second.id)
	}
	_, out = step(t, a, second)
	if got := plansFor(out); len(got) != 2 {
		t.Errorf("after both were measured the visualiser was sent plans for %v", got)
	}
	asked := s.asked()
	if len(asked) != 2 || !strings.Contains(asked[0], "id=tr-1") || !strings.Contains(asked[1], "id=tr-2") {
		t.Errorf("the analyser was asked for %v", asked)
	}
}

func TestATrackIsMeasuredOncePerSession(t *testing.T) {
	a, s := playingWithASeer(t, false)
	a, out := step(t, a, tui.ShowVisualiser{})
	a, out = step(t, a, only[foreseen](t, out))
	a, _ = step(t, a, only[foreseen](t, out))
	a, _ = step(t, a, tea.KeyMsg{Type: tea.KeyEsc})

	_, out = step(t, a, tui.ShowVisualiser{})
	for _, m := range out {
		if _, ok := m.(foreseen); ok {
			t.Errorf("opening the visualiser again measured a track again: %v", s.asked())
		}
	}
	if got := plansFor(out); len(got) != 2 {
		t.Errorf("opening it again sent the plans for %v, want both kept from before", got)
	}
	if n := len(s.asked()); n != 2 {
		t.Errorf("the analyser was asked %d times, want 2", n)
	}
}

func TestClosingTheVisualiserCancelsTheMeasuring(t *testing.T) {
	a, s := playingWithASeer(t, true)
	next, cmd := a.Update(tui.ShowVisualiser{})
	results := make(chan []tea.Msg, 1)
	go func() { results <- runAll(cmd) }()
	ctx := s.lastContext(t)

	a, _ = step(t, next.(app), tea.KeyMsg{Type: tea.KeyEsc})
	if ctx.Err() == nil {
		t.Fatal("closing the visualiser left the measuring running")
	}
	cancelled := only[foreseen](t, <-results)
	a, out := step(t, a, cancelled)
	if len(plansFor(out)) != 0 {
		t.Errorf("a cancelled measuring sent a plan: %v", out)
	}

	// The cancelled track was not kept, so it is measured when the
	// visualiser opens again.
	s.mu.Lock()
	s.hold = false
	s.mu.Unlock()
	_, out = step(t, a, tui.ShowVisualiser{})
	if again := only[foreseen](t, out); again.id != "tr-1" {
		t.Errorf("reopening measured %q, want tr-1 again", again.id)
	}
}

func TestSkippingCancelsTheMeasuringOfATrackNoLongerWanted(t *testing.T) {
	a, s := playingWithASeer(t, true)
	next, cmd := a.Update(tui.ShowVisualiser{})
	go runAll(cmd)
	ctx := s.lastContext(t)

	a = next.(app)
	a, _ = step(t, a, tui.SkipNext{})
	a, _ = step(t, a, tui.NowPlaying{Title: "Ballast"})
	if ctx.Err() == nil {
		t.Error("the track skipped past is still being measured")
	}
	if a.foresight.running != "tr-2" {
		t.Errorf("the track being measured is %q, want tr-2", a.foresight.running)
	}
	a.foresight.cancel()
}

func TestWithoutAnAnalyserNothingIsMeasured(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})
	_, out := step(t, a, tui.ShowVisualiser{})
	for _, m := range out {
		switch m.(type) {
		case foreseen, tui.Foresight:
			t.Errorf("with no analyser the app sent %#v", m)
		}
	}
}
