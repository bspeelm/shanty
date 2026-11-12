package sky

import "math"

// tempoWindow is how many steps of onset strength the tempo is found from,
// and tempoStep is the length of a step in seconds.
const (
	tempoWindow = 256
	tempoStep   = 1.0 / 30
)

// tempo finds the beat from onset strength: an autocorrelation over the last
// few seconds gives the period, and the phase is pulled towards each strong
// onset. Onsets are kept on a grid of fixed steps in time, holding the
// strongest onset heard in each step, so frames of uneven length measure the
// same period.
type tempo struct {
	onsets []float64
	steps  int
	since  float64
	peak   float64
	bpm    float64
	sure   float64
	phase  float64
	beat   int
	// misses counts strong onsets in a row that fell far from a beat, and
	// struck is whether the last frame's onset was strong.
	misses int
	struck bool
}

// tempoMisses is how many strong onsets in a row must fall far from the beat
// before the phase is moved onto them.
const tempoMisses = 4

// step takes one frame's onset strength and returns the tempo, the phase
// within the beat and the beat count.
func (t *tempo) step(onset, dt float64) (float64, float64, int) {
	t.peak = math.Max(t.peak, onset)
	for t.since += dt; t.since >= tempoStep; t.since -= tempoStep {
		t.onsets = append(t.onsets, t.peak)
		t.peak = 0
		if len(t.onsets) > tempoWindow {
			t.onsets = t.onsets[1:]
		}
		t.steps++
		if t.steps%15 == 0 && len(t.onsets) >= tempoWindow/2 {
			bpm, sure := periodOf(t.onsets, tempoStep)
			t.bpm, t.sure = sameOctave(bpm, t.bpm), sure
		}
	}
	if t.bpm == 0 || t.sure < 0.15 {
		return 0, 0, t.beat
	}
	t.phase += dt * t.bpm / 60
	if onset > 0.5 {
		// A strong onset close to a beat pulls the phase towards it. The
		// first frame of a strong onset far from any beat counts as a miss,
		// and enough misses in a row move the phase onto the onset.
		off := t.phase - math.Round(t.phase)
		if math.Abs(off) < 0.25 {
			t.phase -= off * 0.5
			t.misses = 0
		} else if !t.struck {
			t.misses++
			if t.misses >= tempoMisses {
				t.phase -= off
				t.misses = 0
			}
		}
	}
	t.struck = onset > 0.5
	for t.phase >= 1 {
		t.phase--
		t.beat++
	}
	if t.phase < 0 {
		t.phase++
	}
	return t.bpm, t.phase, t.beat
}

// sameOctave returns bpm moved onto the octave of the tempo held, when it is
// within 4% of half or double it.
func sameOctave(bpm, held float64) float64 {
	if held == 0 || bpm == 0 {
		return bpm
	}
	for _, k := range []float64{2, 0.5} {
		if math.Abs(bpm*k/held-1) < 0.04 {
			return bpm * k
		}
	}
	return bpm
}

// periodOf returns the tempo in beats per minute between 60 and 180 that best
// matches the repeats in the onset strengths, and how strongly it matches,
// from 0 to 1. Tempos near 120 are preferred when two match about equally.
func periodOf(onsets []float64, dt float64) (float64, float64) {
	mean := 0.0
	for _, v := range onsets {
		mean += v
	}
	mean /= float64(len(onsets))
	zero := 0.0
	for _, v := range onsets {
		zero += (v - mean) * (v - mean)
	}
	if zero == 0 {
		return 0, 0
	}
	corr := func(lag int) float64 {
		sum := 0.0
		for i := lag; i < len(onsets); i++ {
			sum += (onsets[i] - mean) * (onsets[i-lag] - mean)
		}
		return sum / zero
	}
	lo := int(math.Floor(60 / 180 / dt))
	hi := int(math.Ceil(60 / 60 / dt))
	best, bestLag, bestScore := 0.0, 0.0, math.Inf(-1)
	for lag := max(lo, 1); lag <= hi && lag < len(onsets)/2; lag++ {
		c := corr(lag)
		bpm := 60 / (float64(lag) * dt)
		score := c * math.Exp(-math.Pow(math.Log2(bpm/120), 2))
		if score > bestScore {
			best, bestLag, bestScore = c, float64(lag), score
		}
	}
	if bestLag == 0 || best <= 0 {
		return 0, 0
	}
	// The peak is refined between its neighbours, since a whole-frame lag is
	// coarse at thirty frames a second.
	l := int(bestLag)
	a, b, c := corr(l-1), best, corr(l+1)
	if d := a - 2*b + c; d < 0 {
		bestLag += 0.5 * (a - c) / d
	}
	return math.Min(math.Max(60/(bestLag*dt), 60), 180), clamp01(best)
}
