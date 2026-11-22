//go:build replay

package sky

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestReplayIntensity plays each file named in SKY_CORPUS, separated by
// commas, through the whole chain once for each seed in SKY_SEEDS, and
// prints how hard each run is on the eye: flashes on the guard's grid and a
// finer one, the change and churn of the cells, how far the picture was
// thrown, and how the kicks and cuts fell, for the whole track and for its
// drummed and undrummed stretches. Each figure but the flashes is given as
// its mean, its 95th percentile, its largest and its largest mean over ten
// seconds. SKY_RUNS names runs, separated by commas, in which one of the
// things that move the picture is taken away, or "all"; they are made with
// the first SKY_RUNS_SEEDS seeds, one unless set, for the files whose name
// contains SKY_RUNS_FOR. Measures are cached in SKY_CACHE.
func TestReplayIntensity(t *testing.T) {
	files := os.Getenv("SKY_CORPUS")
	if files == "" {
		t.Skip("SKY_CORPUS is not set")
	}
	cache := os.Getenv("SKY_CACHE")
	if cache == "" {
		cache = t.TempDir()
	}
	seeds := []uint64{5, 6, 7}
	if list := os.Getenv("SKY_SEEDS"); list != "" {
		seeds = nil
		for _, f := range strings.Split(list, ",") {
			n, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			seeds = append(seeds, n)
		}
	}
	runs := tweaksNamed(t, os.Getenv("SKY_RUNS"))
	tweaked := 1
	if n, err := strconv.Atoi(os.Getenv("SKY_RUNS_SEEDS")); err == nil {
		tweaked = n
	}
	const w, h = 100, 30
	slots := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, path := range strings.Split(files, ",") {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		p := planOf(t, path)
		m := measureFile(t, path, cache)
		// A second is drummed when at least drummedFrom of the beats in the
		// eight seconds around it carry a kick.
		drummed := make([]bool, int(p.Length)+2)
		for i := range drummed {
			at := float64(i)
			drummed[i] = len(between(p.Beats, at-4, at+4)) > 0 && kickedShare(p, at-4, at+4) >= drummedFrom
		}
		on := func(f seen) bool { return drummed[min(max(int(f.at), 0), len(drummed)-1)] }
		var kicked, cues []float64
		for _, at := range p.Beats {
			if p.kicked(at) {
				kicked = append(kicked, at)
			}
		}
		for _, cue := range p.Cues {
			if (cue.Kind == CueSection || cue.Kind == CueDrop) && p.cutDue(cue) {
				cues = append(cues, cue.At)
			}
		}
		c := p.Character()
		fmt.Printf("\n== %s: %.0f s, %d beats at %.1f BPM, pulse %.2f, arranged %v, character calm %.2f drive %.2f grit %.2f flow %.2f, %d cues to cut to\n",
			name, p.Length, len(p.Beats), p.Tempo, p.Pulse, p.arranged, c.Calm, c.Drive, c.Grit, c.Flow, len(cues))

		type job struct {
			seed uint64
			run  string
		}
		var jobs []job
		for i, seed := range seeds {
			for _, run := range runs {
				if run != "as it is" && (i >= tweaked || !strings.Contains(name, os.Getenv("SKY_RUNS_FOR"))) {
					continue
				}
				jobs = append(jobs, job{seed, run})
			}
		}
		out := make([]string, len(jobs))
		var wg sync.WaitGroup
		for i, j := range jobs {
			wg.Go(func() {
				slots <- struct{}{}
				defer func() { <-slots }()
				d, look := watchRun(New(j.seed), liveReadings(m, 0.27, j.seed), p, 2, w, h, tweaks[j.run])
				share, lag, spread := d.onBeats(kicked, 0, math.Inf(1))
				off, n := 0.0, 0
				for _, cue := range cues {
					if at := nearest(d.cuts, cue); at <= 1 {
						off, n = off+at, n+1
					}
				}
				var b strings.Builder
				fmt.Fprintf(&b, "-- %s seed %d, %s\n", name, j.seed, j.run)
				fmt.Fprintf(&b, "   timing: %d kicked beats answered %.1f%%, %+.1f ms after the beat (spread %.1f); %d cuts for %d cues, %d within a second, mean %.0f ms from the cue\n",
					len(kicked), 100*share, lag, spread, len(d.cuts), len(cues), n, 1000*off/math.Max(float64(n), 1))
				fmt.Fprintf(&b, "   %s\n", look.flashLine())
				if p.Tempo > 0 {
					for _, cue := range cues {
						fmt.Fprintf(&b, "   lift %s\n", look.lifts(cue, 60/p.Tempo))
					}
				}
				fmt.Fprintf(&b, "   %s\n", look.report("whole", nil))
				fmt.Fprintf(&b, "   %s\n", look.report("drummed", on))
				fmt.Fprintf(&b, "   %s\n", look.report("not drummed", func(f seen) bool { return !on(f) }))
				out[i] = b.String()
			})
		}
		wg.Wait()
		for _, s := range out {
			fmt.Print(s)
		}
	}
}
