package sky

import "math"

// tempoWindow is how many steps of onset strength the tempo is found from,
// and tempoStep is the length of a step in seconds.
const (
	tempoWindow = 256
	tempoStep   = 1.0 / 30
)

// tempo finds the beat from kicks: an autocorrelation over the last few
// seconds gives the period, the way the kicks line up refines it, and a
// phase-locked loop moves the phase towards each kick by a limited amount.
// Kicks are kept on a grid of fixed steps in time, so frames of uneven length
// measure the same period.
type tempo struct {
	onsets []float64
	blur   []float64
	steps  int
	since  float64
	peak   float64
	// bpm is the tempo held, sure how strongly the last estimate matched,
	// and found the last tempoAgree estimates.
	bpm   float64
	sure  float64
	found []float64
	// conf is the confidence in the tempo held, from 0 to 1, and locked is
	// whether the tempo is reported.
	conf   float64
	locked bool
	phase  float64
	beat   int
	// struck is whether the last frame's kick was strong, and kicked the
	// beat count, with its phase, of the last kick that moved the phase.
	struck bool
	kicked float64
}

// The least strength an estimate needs to count, the least it needs when it
// agrees with the tempo held, how many estimates in a row must agree, and how
// closely, as a share of the tempo.
const (
	tempoSure  = 0.35
	tempoHold  = 0.2
	tempoAgree = 3
	tempoClose = 0.02
)

// The confidence at which the tempo is reported and below which it is
// dropped, and how far the confidence moves towards its target on each
// estimate.
const (
	tempoLock   = 0.35
	tempoUnlock = 0.15
	tempoRise   = 0.25
	tempoFall   = 0.7
)

// tempoApart is the least distance in beats between two kicks that move the
// phase.
const tempoApart = 0.5

// tempoPull is the share of a kick's distance from the beat that the phase
// moves by, and tempoGlide the most it moves on one kick, in beats.
const (
	tempoPull  = 0.3
	tempoGlide = 0.05
)

// step takes one frame's kick strength and returns the tempo, the phase within
// the beat and the beat count. The tempo is found from the kicks of 0.5 and
// above, and each kick moves the phase. The tempo is 0 while the confidence
// is too low.
func (t *tempo) step(kick, dt float64) (float64, float64, int) {
	if kick >= 0.5 {
		t.peak = 1
	}
	for t.since += dt; t.since >= tempoStep; t.since -= tempoStep {
		t.onsets = append(t.onsets, t.peak)
		t.peak = 0
		if len(t.onsets) > tempoWindow {
			t.onsets = t.onsets[1:]
		}
		t.steps++
		if t.steps%15 == 0 && len(t.onsets) >= tempoWindow/2 {
			t.estimate()
		}
	}
	if t.bpm == 0 {
		return 0, 0, t.beat
	}
	t.phase += dt * t.bpm / 60
	at := float64(t.beat) + t.phase
	if kick > 0.5 && !t.struck && at-t.kicked >= tempoApart {
		t.kicked = at
		off := t.phase - math.Round(t.phase)
		t.phase -= math.Max(-tempoGlide, math.Min(tempoGlide, off*tempoPull))
	}
	t.struck = kick > 0.5
	for t.phase >= 1 {
		t.phase--
		t.beat++
	}
	for t.phase < 0 {
		t.phase++
	}
	if !t.locked {
		return 0, 0, t.beat
	}
	return t.bpm, t.phase, t.beat
}

// estimate finds the tempo over the onsets held, spread by tempoBlur, and
// moves the confidence: towards the estimate's strength when the last
// tempoAgree estimates are strong enough, made from enough onsets, and agree,
// and down otherwise.
func (t *tempo) estimate() {
	ref := t.bpm
	if ref == 0 && len(t.found) > 0 {
		ref = t.found[len(t.found)-1]
	}
	t.blur = blur(t.blur, t.onsets, tempoBlur)
	bpm, sure := periodOf(t.blur, tempoStep)
	bpm = refine(t.onsets, tempoStep, bpm)
	bpm = sameOctave(bpm, ref)
	t.sure = sure
	held := t.bpm > 0 && math.Abs(bpm/t.bpm-1) <= tempoClose
	beats := float64(len(t.onsets)) * tempoStep * bpm / 60
	few := float64(struckIn(t.onsets)) < math.Max(tempoFewest, tempoShare*beats)
	if bpm == 0 || few || sure < tempoHold || sure < tempoSure && !held {
		t.found = t.found[:0]
	} else {
		t.found = append(t.found, bpm)
		if len(t.found) > tempoAgree {
			t.found = t.found[1:]
		}
	}
	if agree(t.found) {
		t.bpm = t.found[len(t.found)-1]
		t.conf += (clamp01(sure/0.6) - t.conf) * tempoRise
	} else {
		t.conf *= tempoFall
	}
	switch {
	case t.conf >= tempoLock:
		t.locked = true
	case t.conf < tempoUnlock:
		t.locked = false
	}
}

// refine returns the tempo within tempoSpread of bpm on which the onsets
// line up best: the period for which the onsets, placed on a circle by their
// time modulo the period, have the largest resultant.
func refine(onsets []float64, dt, bpm float64) float64 {
	if bpm == 0 {
		return 0
	}
	best, bestFit := bpm, -1.0
	for k := -tempoTries; k <= tempoTries; k++ {
		try := bpm * (1 + tempoSpread*float64(k)/tempoTries)
		step := 2 * math.Pi * dt * try / 60
		var x, y float64
		for i, v := range onsets {
			if v != 0 {
				sn, cs := math.Sincos(step * float64(i))
				x, y = x+v*cs, y+v*sn
			}
		}
		if fit := x*x + y*y; fit > bestFit {
			best, bestFit = try, fit
		}
	}
	return best
}

// tempoSpread is how far either side of an estimate refine looks, as a share
// of the tempo, and tempoTries how many tempos it tries on each side.
const (
	tempoSpread = 0.06
	tempoTries  = 30
)

// tempoBlur is how many steps either side each onset is spread over before
// the period is found.
const tempoBlur = 2

// blur sets out to in with each value spread over r steps either side,
// weighted by a triangle, and returns it.
func blur(out, in []float64, r int) []float64 {
	out = append(out[:0], make([]float64, len(in))...)
	for i, v := range in {
		if v == 0 {
			continue
		}
		for j := max(i-r, 0); j <= min(i+r, len(in)-1); j++ {
			out[j] += v * float64(r+1-abs(i-j))
		}
	}
	return out
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// tempoFewest is the least number of onsets an estimate is made from, and
// tempoShare the least share of the beats in the window that must have one.
const (
	tempoFewest = 6
	tempoShare  = 0.6
)

// struckIn counts the onsets in a run of onset strengths: the steps at or
// above 0.5 that follow one below it.
func struckIn(onsets []float64) int {
	n, was := 0, false
	for _, v := range onsets {
		if v >= 0.5 && !was {
			n++
		}
		was = v >= 0.5
	}
	return n
}

// agree reports whether there are tempoAgree estimates, all within
// tempoClose of their mean.
func agree(found []float64) bool {
	if len(found) < tempoAgree {
		return false
	}
	mean := 0.0
	for _, v := range found {
		mean += v
	}
	mean /= float64(len(found))
	for _, v := range found {
		if math.Abs(v/mean-1) > tempoClose {
			return false
		}
	}
	return true
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
