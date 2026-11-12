//go:build replay

package sky

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/mpv"
)

// planOf measures a file with the analysing mpv and returns its plan.
func planOf(t *testing.T, path string) *Plan {
	t.Helper()
	blocks, err := mpv.Analyse(context.Background(), mpv.Options{Socket: filepath.Join(t.TempDir(), "s")}, path)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]Block, len(blocks))
	for i, b := range blocks {
		ch := make([]Channel, len(b.Channels))
		for j, c := range b.Channels {
			ch[j] = Channel{Loudness: c.Loudness, Difference: c.Difference, Crossings: c.Crossings}
		}
		out[i] = Block{At: b.At.Seconds(), Samples: b.Samples, Channels: ch, Bands: b.Bands, Punch: b.Punch, Side: b.Side, Rate: b.Rate, Scale: b.Scale}
	}
	return NewPlan(out)
}

// played is what a visualiser did over a whole track: each frame's heard
// position, kick and heaviness, and the position of each cut.
type played struct {
	pos, kick, heavy []float64
	cuts             []float64
}

// playWhole plays a file's live readings through a visualiser that is given
// the file's plan two seconds in, as the app gives it.
func playWhole(m measured, p *Plan) played {
	s := New(5)
	var out played
	last := 0.0
	for _, r := range liveReadings(m, 0.27, 1) {
		pos := r.Position.Seconds()
		if pos >= 2 && s.plan() == nil {
			s.Foresee("track", p)
		}
		was := s.next
		s.Hear(r)
		s.Step(100, 30, pos-last, r.At, "track")
		last = pos
		sig := s.Signals()
		out.pos, out.kick, out.heavy = append(out.pos, s.lis.ear.now), append(out.kick, sig.Kick), append(out.heavy, sig.Heavy)
		if s.next != nil && s.next != was && s.cutting {
			out.cuts = append(out.cuts, s.lis.ear.now)
		}
	}
	return out
}

// onBeats reports how the kick answered the beats from a to b: the share of
// them with a kick of at least 0.6 beginning within 60 ms before or 150 ms
// after, and the mean and spread, in milliseconds, of how long after the
// beat it began.
func (d played) onBeats(beats []float64, a, b float64) (share, lag, spread float64) {
	var lags []float64
	n := 0
	for _, at := range beats {
		if at < a || at >= b {
			continue
		}
		n++
		best, rise, when := 0.0, 0.0, 0.0
		for k := sort.SearchFloat64s(d.pos, at-0.06); k > 0 && k < len(d.pos) && d.pos[k] <= at+0.15; k++ {
			best = math.Max(best, d.kick[k])
			if up := d.kick[k] - d.kick[k-1]; up > rise {
				rise, when = up, d.pos[k]-at
			}
		}
		if rise >= 0.1 && best >= 0.6 {
			lags = append(lags, 1000*when)
		}
	}
	if n == 0 || len(lags) == 0 {
		return 0, 0, 0
	}
	for _, l := range lags {
		lag += l / float64(len(lags))
	}
	for _, l := range lags {
		spread += (l - lag) * (l - lag) / float64(len(lags))
	}
	return float64(len(lags)) / float64(n), lag, math.Sqrt(spread)
}

// kickedShare returns the share of a plan's beats from a to b that have a
// kick beside them.
func kickedShare(p *Plan, a, b float64) float64 {
	beats := between(p.Beats, a, b)
	n := 0
	for _, at := range beats {
		if p.kicked(at) {
			n++
		}
	}
	return float64(n) / float64(max(len(beats), 1))
}

// TestReplayEndToEnd plays each file named in SKY_CORPUS, separated by
// commas, through the whole chain: the analysing mpv, the plan, the live
// readings rebuilt from the same audio, and the visualiser. For every thirty
// seconds of each track it prints the share of beats that carry a kick in
// the plan, the share the visualiser answered with a kick and how long after
// the beat, and for each track its tempo, character, mean heaviness and
// cuts. Measures are cached in SKY_CACHE.
func TestReplayEndToEnd(t *testing.T) {
	files := os.Getenv("SKY_CORPUS")
	if files == "" {
		t.Skip("SKY_CORPUS is not set")
	}
	cache := os.Getenv("SKY_CACHE")
	if cache == "" {
		cache = t.TempDir()
	}
	for _, path := range strings.Split(files, ",") {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		p := planOf(t, path)
		d := playWhole(measureFile(t, path, cache), p)
		heavy := 0.0
		for _, h := range d.heavy {
			heavy += h / float64(len(d.heavy))
		}
		c := p.Character()
		fmt.Printf("%s: %d beats at %.1f BPM, pulse %.2f, character calm %.2f drive %.2f grit %.2f flow %.2f, heavy %.2f\n",
			name, len(p.Beats), p.Tempo, p.Pulse, c.Calm, c.Drive, c.Grit, c.Flow, heavy)
		for a := 0.0; a+30 <= p.Length; a += 30 {
			beats := between(p.Beats, a, a+30)
			if len(beats) == 0 {
				continue
			}
			share, lag, spread := d.onBeats(p.Beats, a, a+30)
			fmt.Printf("  %3.0f-%3.0f s: %2d beats, planned kicks on %3.0f%%, answered %3.0f%%, %+5.1f ms after the beat (spread %4.1f)\n",
				a, a+30, len(beats), 100*kickedShare(p, a, a+30), 100*share, lag, spread)
		}
		var cues []string
		for _, cue := range p.Cues {
			if (cue.Kind == CueSection || cue.Kind == CueDrop) && p.cutDue(cue) {
				cues = append(cues, fmt.Sprintf("%.1f", cue.At))
			}
		}
		fmt.Printf("  cues to cut to: %s\n  cuts made at:   %.1f\n", strings.Join(cues, " "), d.cuts)
	}
}
