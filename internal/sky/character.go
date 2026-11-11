package sky

import "math"

// Character is how the music has sounded over the last several seconds, as
// four weights that add up to 1. They are not genres. Calm is high for few
// hits, little change and no beat, as in ambient music; Drive for a steady
// tempo and regular kicks, as in techno; Grit for bright, hissing, dense and
// irregular hits, as in rock; and Flow for wide dynamics, swells and soft
// attacks, as in classical music.
type Character struct {
	Calm, Drive, Grit, Flow float64
}

// axes returns the four weights in a fixed order.
func (c Character) axes() [4]float64 { return [4]float64{c.Calm, c.Drive, c.Grit, c.Flow} }

// sharpened returns the character with each weight squared and the four
// scaled to add up to 1 again, so that the strongest leads more clearly. A
// character not yet measured, with every weight 0, is returned as an even
// blend.
func (c Character) sharpened() Character {
	a := c.axes()
	sum := 0.0
	for i := range a {
		a[i] *= a[i]
		sum += a[i]
	}
	if sum < 1e-9 {
		return Character{0.25, 0.25, 0.25, 0.25}
	}
	return Character{a[0] / sum, a[1] / sum, a[2] / sum, a[3] / sum}
}

// distance is how far apart two characters are, from 0 to 2.
func (c Character) distance(o Character) float64 {
	a, b := c.axes(), o.axes()
	d := 0.0
	for i := range a {
		d += math.Abs(a[i] - b[i])
	}
	return d
}

// The window the character is measured over, in seconds, and how often the
// level is sampled within it. warmUp is how long after a track starts the
// measuring begins, and settleAfter how long after that the character is
// taken to describe the music.
const (
	characterWindow = 12.0
	characterStep   = 0.1
	warmUp          = 3.0
	settleAfter     = 13.0
)

// characterMeter keeps what the character is measured from.
type characterMeter struct {
	char    Character
	levels  []float64
	slow    float64
	swell   float64
	kicks   []float64
	sharp   float64
	clock   float64
	sampled float64
	fresh   float64
}

// step takes one frame's signals, the tempo's confidence and the strength of
// any onset, and moves the character towards what they describe. For ten
// seconds after restart it moves four times as fast.
func (c *characterMeter) step(sig Signals, sure, kick, onset, dt float64) Character {
	c.clock += dt
	c.fresh += dt
	// A kick is counted once, when it starts, and not again for 0.15 s.
	if kick > 0.3 && (len(c.kicks) == 0 || c.clock-c.kicks[len(c.kicks)-1] > 0.15) {
		c.kicks = append(c.kicks, c.clock)
	}
	for len(c.kicks) > 0 && c.kicks[0] < c.clock-characterWindow {
		c.kicks = c.kicks[1:]
	}
	if onset > 0.3 {
		c.sharp += (onset - c.sharp) * 0.1
	}
	c.sharp *= math.Exp(-dt / 6)
	before := c.slow
	c.slow = glide(c.slow, sig.Level, dt, 1)
	// The first seconds of a track are its level rising from nothing, which
	// is not a swell, so the dynamics are measured only after them.
	if c.fresh < warmUp {
		return c.char
	}
	c.swell = glide(c.swell, math.Abs(c.slow-before)/dt, dt, 4)
	c.sampled += dt
	for c.sampled >= characterStep {
		c.sampled -= characterStep
		c.levels = append(c.levels, c.slow)
		if len(c.levels) > int(characterWindow/characterStep) {
			c.levels = c.levels[1:]
		}
	}

	m := measures{
		steady:   clamp01(sure * 1.5),
		regular:  regularity(c.kicks),
		busy:     sig.Busy,
		bright:   clamp01(0.5 + (sig.Treble+sig.Air-sig.Bass)*0.6),
		dynamics: clamp01(spread(c.levels) * 5),
		swell:    clamp01(c.swell * 6),
		sharp:    clamp01(c.sharp * 1.2),
		loud:     c.slow,
	}
	target := m.character()
	tau := 8.0
	if c.fresh < settleAfter {
		tau = 2
	}
	c.char = Character{
		Calm:  glide(c.char.Calm, target.Calm, dt, tau),
		Drive: glide(c.char.Drive, target.Drive, dt, tau),
		Grit:  glide(c.char.Grit, target.Grit, dt, tau),
		Flow:  glide(c.char.Flow, target.Flow, dt, tau),
	}
	return c.char
}

// restart starts measuring again, for a new track, keeping the character
// until the new one has been heard.
func (c *characterMeter) restart() {
	c.fresh, c.levels, c.kicks, c.swell = 0, nil, nil, 0
}

// settled reports whether the music has been heard for long enough that the
// character describes it.
func (c *characterMeter) settled() bool { return c.fresh > settleAfter }

// measures are what the character is worked out from, each from 0 to 1.
type measures struct {
	steady, regular, busy, bright, dynamics, swell, sharp, loud float64
}

// character weighs the measures into the four axes and scales them to add up
// to 1.
func (m measures) character() Character {
	c := Character{
		Calm:  (1 - m.busy) * (1 - m.steady) * (1 - 0.7*m.dynamics) * (1 - 0.6*m.sharp),
		Drive: m.steady * (0.3 + 0.7*m.regular) * (1 - 0.6*m.dynamics) * (0.4 + 0.6*m.busy),
		Grit:  m.bright * (0.3 + 0.7*m.busy) * (0.4 + 0.6*m.sharp) * (1 - 0.5*m.regular*m.steady),
		Flow:  clamp01(0.7*m.dynamics+0.7*m.swell) * (1 - 0.5*m.sharp) * (1 - 0.5*m.steady),
	}
	sum := c.Calm + c.Drive + c.Grit + c.Flow
	if sum < 1e-6 {
		return Character{Calm: 1}
	}
	return Character{c.Calm / sum, c.Drive / sum, c.Grit / sum, c.Flow / sum}
}

// regularity is how evenly spaced a list of times is: 1 for perfectly even,
// falling towards 0 as the gaps vary. Fewer than four times is 0.
func regularity(times []float64) float64 {
	if len(times) < 4 {
		return 0
	}
	gaps := make([]float64, len(times)-1)
	for i := range gaps {
		gaps[i] = times[i+1] - times[i]
	}
	mean := 0.0
	for _, g := range gaps {
		mean += g
	}
	mean /= float64(len(gaps))
	return clamp01(1 - spread(gaps)/mean*2)
}

// spread is the standard deviation of a list of numbers.
func spread(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	mean := 0.0
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))
	sum := 0.0
	for _, x := range v {
		sum += (x - mean) * (x - mean)
	}
	return math.Sqrt(sum / float64(len(v)))
}
