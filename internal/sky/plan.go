package sky

import (
	"cmp"
	"math"
	"slices"
)

// Block is the measuring filter's totals for one block of a track, measured
// ahead of playback with the totals restarting every block.
type Block struct {
	// At is where the block starts, in seconds into the track.
	At float64
	// Samples is how many samples per channel the block holds.
	Samples float64
	// Channels holds the totals for each channel.
	Channels []Channel
	// Bands holds the RMS level in dB relative to full scale of each band, in
	// the order bandKick, bandLow, bandMid, bandHigh. It is empty for a block
	// measured without bands.
	Bands []float64
	// Rate is the sample rate, and Scale is the value of a full-scale sample
	// in the units of Channel.Difference.
	Rate  int
	Scale float64
}

// CueKind is what happens in the music at a cue.
type CueKind int

const (
	// CueSection is a boundary between two sections that sound different.
	CueSection CueKind = iota
	// CueBuild is the start of a sustained rise.
	CueBuild
	// CueDrop is a sudden rise at the end of a build.
	CueDrop
	// CueClimax is the centre of the loudest sustained stretch.
	CueClimax
	// CueHush is the start of a stretch well below the track's mean level.
	CueHush
)

func (k CueKind) String() string {
	names := []string{"section", "build", "drop", "climax", "hush"}
	if k < 0 || int(k) >= len(names) {
		return "unknown"
	}
	return names[k]
}

// Cue is one event in a track: when it happens in seconds, what it is, and
// how strong it is, from 0 to 1.
type Cue struct {
	At       float64
	Kind     CueKind
	Strength float64
}

// Moment is the sound of a track at one point. Every field runs from 0 to 1,
// with the same meaning as the field of the same name in Signals.
type Moment struct {
	Level, Bass, Treble, Air float64
}

// Plan is a map of a whole track: its curves, its cues, its tempo, its beats
// and its hits.
type Plan struct {
	// Cues are the events in the track, in order of time.
	Cues []Cue
	// Tempo is the beats per minute of the track, or 0, and Sure is how
	// strongly the onsets repeat at it, from 0 to 1. For a track measured in
	// bands they come from its beats, and Sure is Pulse.
	Tempo, Sure float64
	// Length is the length of the track in seconds.
	Length float64
	// Beats are the times of the beats in seconds, in order, and Downbeats
	// are the beats that start a bar. Meter is the number of beats in a bar.
	// A track measured without bands, or without a steady beat, has none.
	Beats, Downbeats []float64
	Meter            int
	// Pulse is how strong and steady the beat is across the track, from 0 to
	// 1. A track with a Pulse below beatlessBelow has no beats.
	Pulse float64
	// Hits are the kicks, snares and hats of the track, in order of time.
	Hits   []Hit
	curve  []Moment
	builds [][2]float64
	banded bool
	bar    []int
	char   Character
}

// The planner's measures. Times are in seconds and levels are on the 0 to 1
// scale of Moment.Level.
const (
	// planStep is the time between two points on the curves.
	planStep = 0.5
	// noveltyWindow is the length of the windows before and after a point
	// that are compared, and noveltyMin is the least difference between them
	// that marks a section.
	noveltyWindow = 4
	noveltyMin    = 0.15
	// A build rises by at least buildRise at a rate above buildSlope a
	// second, for between buildMin and buildMax seconds. A drop rises by at
	// least dropJump from the second before it to the second after.
	buildSlope = 0.015
	buildRise  = 0.1
	buildMin   = 4
	buildMax   = 16
	dropJump   = 0.1
	// climaxWindow is the stretch over which the loudest part is found, and
	// the climax spans the time around it within climaxBand of its level.
	climaxWindow = 8
	climaxBand   = 0.06
	// A hush is at least hushMin long and hushBelow under the mean level.
	hushMin   = 4
	hushBelow = 0.15
	// release is how long tension takes to fall after a build ends.
	release = 4
)

// NewPlan maps a track from the totals of all its blocks.
func NewPlan(blocks []Block) *Plan {
	p := &Plan{}
	for _, b := range blocks {
		if b.Rate > 0 {
			p.Length = math.Max(p.Length, b.At+b.Samples/float64(b.Rate))
		}
	}
	n := int(math.Round(p.Length / planStep))
	if n == 0 {
		return p
	}
	type sums struct{ samples, energy, diff, cross, rate float64 }
	bins := make([]sums, n)
	var onsets []float64
	var last slice
	for _, b := range blocks {
		if b.Rate <= 0 || b.Samples <= 0 || len(b.Channels) == 0 {
			continue
		}
		rate, scale := float64(b.Rate), b.Scale
		if scale <= 0 {
			scale = 1
		}
		var energy, diff, cross float64
		for _, c := range b.Channels {
			d := c.Difference / scale
			energy += power(c.Loudness)
			diff += d * d
			cross += c.Crossings
		}
		k := float64(len(b.Channels))
		energy, diff, cross = energy/k, diff/k, cross/k
		s := measure(energy, diff, cross/b.Samples, rate)
		onsets = append(onsets, math.Max(0, s.bass-last.bass)+math.Max(0, s.treble-last.treble))
		last = s

		bin := &bins[min(int((b.At+b.Samples/rate/2)/planStep), n-1)]
		bin.samples += b.Samples
		bin.energy += energy * b.Samples
		bin.diff += diff * b.Samples
		bin.cross += cross
		bin.rate = rate
	}
	p.curve = make([]Moment, n)
	for i, b := range bins {
		if b.samples > 0 {
			s := measure(b.energy/b.samples, b.diff/b.samples, b.cross/b.samples, b.rate)
			p.curve[i] = Moment{Level: s.level, Bass: s.bass, Treble: s.treble, Air: s.air}
		}
	}
	if len(onsets) > 1 {
		p.Tempo, p.Sure = periodOf(onsets, p.Length/float64(len(onsets)))
	}
	p.beats(blocks)

	level := make([]float64, n)
	for i, m := range p.curve {
		level[i] = m.Level
	}
	p.sections()
	p.findBuilds(level)
	p.climax(level)
	p.hushes(level)
	slices.SortStableFunc(p.Cues, func(a, b Cue) int { return cmp.Compare(a.At, b.At) })
	return p
}

// steps is the number of points on the curves in d seconds.
func steps(d float64) int { return int(d / planStep) }

// planBlur returns the mean of v over the k points either side of each point.
func planBlur(v []float64, k int) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		lo, hi := max(i-k, 0), min(i+k+1, len(v))
		for _, x := range v[lo:hi] {
			out[i] += x
		}
		out[i] /= float64(hi - lo)
	}
	return out
}

// sections marks the peaks of novelty: the difference between the mean sound
// of the window before a point and the window after it. A peak must be the
// highest within a window either side, and above both noveltyMin and
// half a standard deviation over the mean novelty.
func (p *Plan) sections() {
	w, n := steps(noveltyWindow), len(p.curve)
	if n < 2*w+1 {
		return
	}
	novelty := make([]float64, n)
	var sum, sq float64
	for i := w; i <= n-w; i++ {
		var before, after Moment
		for j := range w {
			before = before.plus(p.curve[i-w+j])
			after = after.plus(p.curve[i+j])
		}
		d := before.distance(after) / float64(w)
		novelty[i] = d
		sum, sq = sum+d, sq+d*d
	}
	count := float64(n - 2*w + 1)
	mean := sum / count
	floor := math.Max(noveltyMin, mean+math.Sqrt(math.Max(0, sq/count-mean*mean))/2)
	for i := w; i <= n-w; i++ {
		peak := novelty[i] >= floor
		for j := max(i-w, 0); j <= min(i+w, n-1) && peak; j++ {
			peak = j == i || novelty[j] < novelty[i] || (j > i && novelty[j] == novelty[i])
		}
		if peak {
			p.Cues = append(p.Cues, Cue{At: float64(i) * planStep, Kind: CueSection, Strength: clamp01(novelty[i])})
		}
	}
}

func (m Moment) plus(o Moment) Moment {
	return Moment{m.Level + o.Level, m.Bass + o.Bass, m.Treble + o.Treble, m.Air + o.Air}
}

func (m Moment) distance(o Moment) float64 {
	return math.Abs(m.Level-o.Level) + math.Abs(m.Bass-o.Bass) +
		math.Abs(m.Treble-o.Treble) + math.Abs(m.Air-o.Air)
}

// findBuilds marks builds and the drops that end them. A build is a run of
// the smoothed level rising faster than buildSlope, gaps of a second
// allowed. It starts at the last low point before the run, and ends at the
// largest jump in level from buildMin after its start to four seconds after
// the run, which is a drop when it is at least dropJump. A build that ends in
// a drop and is longer than buildMax keeps its last buildMax seconds. Its
// rise is the rise from start to end less the largest jump within it, so
// that a single step up is not a build.
func (p *Plan) findBuilds(x []float64) {
	n := len(x)
	xs := planBlur(x, 2)
	rising := func(i int) bool {
		return i < n && (xs[min(i+2, n-1)]-xs[max(i-2, 0)])/(4*planStep) > buildSlope
	}
	mean := func(lo, hi int) float64 {
		lo, hi = max(lo, 0), min(hi, n)
		if hi <= lo {
			return 0
		}
		s := 0.0
		for _, v := range x[lo:hi] {
			s += v
		}
		return s / float64(hi-lo)
	}
	for a := 0; a < n; a++ {
		if !rising(a) {
			continue
		}
		b := a
		for j := a + 1; j < n && j <= b+3; j++ {
			if rising(j) {
				b = j
			}
		}
		low := max(a-4, 0)
		for j := low; j <= min(a+1, n-1); j++ {
			if x[j] <= x[low] {
				low = j
			}
		}
		jumpAt := func(j int) float64 { return mean(j, j+2) - mean(j-2, j) }
		start, end, jump := low+1, b+1, 0.0
		for j := start + steps(buildMin); j <= min(b+8, n-2); j++ {
			if d := jumpAt(j); d > jump {
				end, jump = j, d
			}
		}
		drop := jump >= dropJump
		if !drop {
			end = b + 1
		} else if end-start > steps(buildMax) {
			start = end - steps(buildMax)
		}
		steepest := 0.0
		for j := start + 2; j <= end-2; j++ {
			steepest = math.Max(steepest, jumpAt(j))
		}
		rise := mean(end-2, end) - mean(start, start+2) - steepest
		a = max(a, b, end)
		if end-start < steps(buildMin) || end-start > steps(buildMax) || rise < buildRise {
			continue
		}
		from, to := float64(start)*planStep, float64(end)*planStep
		p.builds = append(p.builds, [2]float64{from, to})
		p.Cues = append(p.Cues, Cue{At: from, Kind: CueBuild, Strength: clamp01(rise / 0.4)})
		if drop {
			p.Cues = append(p.Cues, Cue{At: to, Kind: CueDrop, Strength: clamp01(jump / 0.4)})
		}
	}
}

// climax marks the centre of the loudest stretch: the time around the
// loudest climaxWindow during which the smoothed level stays within
// climaxBand of that window's mean.
func (p *Plan) climax(x []float64) {
	n := len(x)
	if n < steps(climaxWindow) {
		return
	}
	wide, xs := planBlur(x, steps(climaxWindow)/2), planBlur(x, 2)
	c := 0
	for i, v := range wide {
		if v > wide[c] {
			c = i
		}
	}
	lo, hi := c, c
	for lo > 0 && xs[lo-1] >= wide[c]-climaxBand {
		lo--
	}
	for hi < n-1 && xs[hi+1] >= wide[c]-climaxBand {
		hi++
	}
	at := float64(lo+hi+1) * planStep / 2
	p.Cues = append(p.Cues, Cue{At: at, Kind: CueClimax, Strength: clamp01((wide[c] - meanLevel(x)) / 0.3)})
}

// hushes marks the start of each stretch at least hushMin long whose smoothed
// level stays hushBelow under the mean level of the track's sound. A stretch
// that starts the track is not a hush.
func (p *Plan) hushes(x []float64) {
	n := len(x)
	mean := meanLevel(x)
	under := mean - hushBelow
	xs := planBlur(x, 2)
	for a := 0; a < n; a++ {
		if xs[a] >= under {
			continue
		}
		b, sum := a, 0.0
		for b < n && xs[b] < under {
			sum += x[b]
			b++
		}
		start := a
		for start > 0 && x[start-1] < under {
			start--
		}
		if b-a >= steps(hushMin) && start > 0 {
			p.Cues = append(p.Cues, Cue{At: float64(start) * planStep, Kind: CueHush,
				Strength: clamp01((mean - sum/float64(b-a)) / 0.4)})
		}
		a = b
	}
}

// meanLevel is the mean of the levels that are not silent.
func meanLevel(x []float64) float64 {
	sum, n := 0.0, 0
	for _, v := range x {
		if v > 0 {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// Next returns the first cue of a kind at or after t seconds, and false when
// there is none.
func (p *Plan) Next(t float64, kind CueKind) (Cue, bool) {
	if p == nil {
		return Cue{}, false
	}
	for _, c := range p.Cues {
		if c.At >= t && c.Kind == kind {
			return c, true
		}
	}
	return Cue{}, false
}

// Tension is how far t seconds is through a build, from 0 at its start to 1
// at its end, falling back to 0 over the release seconds after it. Outside
// builds and their release it is 0.
func (p *Plan) Tension(t float64) float64 {
	if p == nil {
		return 0
	}
	out := 0.0
	for _, b := range p.builds {
		switch {
		case t >= b[0] && t < b[1]:
			out = math.Max(out, (t-b[0])/(b[1]-b[0]))
		case t >= b[1] && t < b[1]+release:
			out = math.Max(out, 1-(t-b[1])/release)
		}
	}
	return out
}

// At returns the sound of the track at t seconds, interpolated between the
// points of the curves.
func (p *Plan) At(t float64) Moment {
	if p == nil || len(p.curve) == 0 {
		return Moment{}
	}
	u := t/planStep - 0.5
	i := int(math.Floor(u))
	if i < 0 {
		return p.curve[0]
	}
	if i >= len(p.curve)-1 {
		return p.curve[len(p.curve)-1]
	}
	f := u - float64(i)
	a, b := p.curve[i], p.curve[i+1]
	mix := func(x, y float64) float64 { return x + (y-x)*f }
	return Moment{mix(a.Level, b.Level), mix(a.Bass, b.Bass), mix(a.Treble, b.Treble), mix(a.Air, b.Air)}
}
