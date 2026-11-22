//go:build replay

package sky

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// beatLift is what the low end does at one beat, in dB: punch is how far
// the band where a kick starts rises at the beat, as the plan measures it,
// low how far the lowest band rises over the same blocks, and pump how far
// the lowest band at the beat stands above itself half a beat later.
type beatLift struct {
	at               float64
	punch, low, pump float64
	kicked           bool
}

// beatLifts measures every beat of a plan against the blocks it was made
// from. The punch is measured as gridKicks measures it.
func beatLifts(p *Plan, blocks []Block) ([]beatLift, bool) {
	var at, peak, start, low []float64
	punched := false
	for _, b := range blocks {
		if len(b.Bands) < bandCount {
			continue
		}
		at = append(at, b.At)
		start, low = append(start, b.Bands[bandPunch]), append(low, b.Bands[bandKick])
		v := silentBelow
		if b.Punch > 0 {
			v, punched = math.Max(20*math.Log10(b.Punch), silentBelow), true
		}
		peak = append(peak, v)
	}
	out := make([]beatLift, 0, len(p.Beats))
	for k, t := range p.Beats {
		i, _ := slices.BinarySearch(at, t)
		if i < 3 || i+2 > len(at) {
			continue
		}
		l := beatLift{at: t, kicked: p.kicked(t)}
		if punched {
			l.punch = slices.Max(peak[i-1:i+2]) - mean(peak[i-3:i-1])
		} else {
			l.punch = mean(start[i:i+2]) - mean(start[i-2:i])
		}
		l.low = slices.Max(low[i-1:i+2]) - mean(low[i-3:i-1])
		if k+1 < len(p.Beats) {
			j, _ := slices.BinarySearch(at, (t+p.Beats[k+1])/2)
			if j+1 < len(low) {
				l.pump = slices.Max(low[i-1:i+2]) - mean(low[j-1:j+1])
			}
		}
		out = append(out, l)
	}
	return out, punched
}

// TestReplayKicksOnBeats measures, for each file named in SKY_CORPUS, what
// the low end does at every beat of its plan, and prints it for each thirty
// seconds: the share of beats the plan gives a kick, and for the beats with
// and without one the median punch, rise of the lowest band and pump, with
// the share of beats without a kick whose punch is at least the rise the
// plan asks of a kick.
func TestReplayKicksOnBeats(t *testing.T) {
	files := os.Getenv("SKY_CORPUS")
	if files == "" {
		t.Skip("SKY_CORPUS is not set")
	}
	for _, path := range strings.Split(files, ",") {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		blocks := blocksOf(t, path)
		p := NewPlan(blocks)
		lifts, punched := beatLifts(p, blocks)
		rise := kickRise
		if punched {
			rise = peakRise
		}
		kicks := 0
		for _, h := range p.Hits {
			if h.Kind == HitKick {
				kicks++
			}
		}
		fmt.Printf("\n== %s: %d beats at %.1f BPM, %d kicks among %d hits, punch measured by its peak %v, a kick asks a rise of %.1f dB\n",
			name, len(p.Beats), p.Tempo, kicks, len(p.Hits), punched, rise)
		line := func(label string, a, b float64) {
			var with, without []beatLift
			for _, l := range lifts {
				if l.at < a || l.at >= b {
					continue
				}
				if l.kicked {
					with = append(with, l)
				} else {
					without = append(without, l)
				}
			}
			med := func(v []beatLift, pick func(beatLift) float64) float64 {
				var x []float64
				for _, l := range v {
					x = append(x, pick(l))
				}
				return quantile(x, 0.5)
			}
			over := 0
			for _, l := range without {
				if l.punch >= rise {
					over++
				}
			}
			punch := func(l beatLift) float64 { return l.punch }
			low := func(l beatLift) float64 { return l.low }
			pump := func(l beatLift) float64 { return l.pump }
			fmt.Printf("  %-9s %3d beats, kick on %3.0f%%, section share %.2f | with a kick: punch %5.1f low %5.1f pump %5.1f | without: punch %5.1f low %5.1f pump %5.1f, punch over the rise on %3.0f%%\n",
				label, len(with)+len(without), 100*float64(len(with))/math.Max(float64(len(with)+len(without)), 1), p.sectionShare((a+math.Min(b, p.Length))/2),
				med(with, punch), med(with, low), med(with, pump), med(without, punch), med(without, low), med(without, pump),
				100*float64(over)/math.Max(float64(len(without)), 1))
		}
		for a := 0.0; a < p.Length; a += 30 {
			line(fmt.Sprintf("%3.0f-%3.0f", a, a+30), a, a+30)
		}
		line("whole", 0, math.Inf(1))
	}
}

// kickEveryBeat gives a plan a kick on every beat it is sure of that has
// none, as strong as the track's usual kick or 0.6 where it has none, and
// forgets what the plan had worked out from its kicks.
func kickEveryBeat(p *Plan) int {
	strength := p.kickStrength()
	if strength == 0 {
		strength = 0.6
	}
	added := 0
	for k, at := range p.Beats {
		if k < len(p.conf) && p.conf[k] >= kickConfidence && !p.kicked(at) {
			p.Hits = append(p.Hits, Hit{At: at, Kind: HitKick, Strength: strength})
			slices.SortStableFunc(p.Hits, func(a, b Hit) int { return cmp.Compare(a.At, b.At) })
			added++
		}
	}
	p.usualKick, p.edges, p.shares = 0, nil, nil
	return added
}
