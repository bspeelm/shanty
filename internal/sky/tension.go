package sky

import "math"

// tensionMeter follows the shape of the music over seconds: how it builds,
// when a build lets go, and when it falls quiet.
type tensionMeter struct {
	clock float64
	// fast, slow and short follow the drive, a mix of level, busyness and
	// treble, over about 1, 6 and 0.3 seconds. quiet follows the level over
	// about a second, and avg over about 20.
	fast, slow, short float64
	quiet, avg        float64
	// peak is the highest tension reached while rising since the last release.
	peak float64
	// lift is how far the music is above its running average, from 0 to 1.
	lift float64
	// phase is how far through a breath the music is, in breaths.
	phase float64
}

// tensionWarmUp is how long after it starts the meter measures before it
// reports anything, in seconds.
const tensionWarmUp = 3.0

// step moves tension, release, hush and breath on by dt seconds from the
// frame's other signals.
func (m *tensionMeter) step(s *Signals, dt float64) {
	m.clock += dt
	drive := 0.5*s.Level + 0.25*s.Busy + 0.25*s.Treble
	m.breathe(s, dt)
	if m.clock < tensionWarmUp {
		m.fast, m.slow, m.short = drive, drive, drive
		m.quiet, m.avg = s.Level, s.Level
		return
	}
	m.fast = glide(m.fast, drive, dt, 1)
	m.slow = glide(m.slow, drive, dt, 6)
	m.short = glide(m.short, drive, dt, 0.3)
	m.quiet = glide(m.quiet, s.Level, dt, 1)
	m.avg = glide(m.avg, s.Level, dt, 20)

	climb := clamp01((m.fast - m.slow) * 4)
	rising := climb > s.Tension
	s.Tension = glide(s.Tension, climb, dt, 1.5)
	if rising {
		m.peak = math.Max(m.peak, s.Tension)
	}

	s.Release *= math.Exp(-dt / 0.7)
	fell := m.peak > 0.35 && s.Tension < m.peak*0.75
	dropped := m.peak > 0.2 && m.short-m.fast > 0.12
	if fell || dropped {
		s.Release = math.Max(s.Release, clamp01(0.4+m.peak))
		m.peak = 0
	}
	if dropped {
		m.fast, m.slow = m.short, m.short
	}

	hush := 0.0
	if m.avg > 0.02 {
		hush = clamp01((0.8 - m.quiet/m.avg) / 0.45)
	}
	s.Hush = glide(s.Hush, hush, dt, 1.5)
	m.lift = glide(m.lift, smooth(0.05, 0.2, m.quiet-m.avg), dt, 0.5)
}

// breathe moves Breath on: a slow rise and fall over four bars, or eight when
// a bar is shorter than 1.5 seconds, and over breathFree seconds while the
// tempo is not known.
func (m *tensionMeter) breathe(s *Signals, dt float64) {
	if s.Tempo > 0 {
		beats := breathBars(s.Tempo) * 4
		m.phase = (float64(s.Beat%beats) + s.Phase) / float64(beats)
	} else {
		m.phase = math.Mod(m.phase+dt/breathFree, 1)
	}
	s.Breath = glide(s.Breath, 0.5-0.5*math.Cos(2*math.Pi*m.phase), dt, 0.5)
}

// breathFree is the length of a breath while the tempo is not known, in
// seconds.
const breathFree = 6.0

// breathBars is how many bars of four beats a breath lasts at a tempo.
func breathBars(bpm float64) int {
	if 4*60/bpm < 1.5 {
		return 8
	}
	return 4
}

// exposureFor is the mean brightness a frame is scaled towards: 0.4 on steady
// music, falling towards 0.2 in a hush, rising to 0.45 on a release, and
// moving a few percent with the breath.
func exposureFor(s Signals) float64 {
	return (exposure - 0.2*s.Hush + 0.05*s.Release) * (1 + 0.08*(s.Breath-0.5))
}

// post is the light laid over a composited frame: a glow around its bright
// parts and a soft darkening towards its edges.
type post struct {
	// radial is each pixel's distance from the middle of the field, where 1
	// is the edge of the shorter side.
	radial []float64
	bright []float64
	tmp    []float64
	// glow and depth glide towards the bloom's strength and the vignette's
	// depth; reach is the distance at which the vignette starts.
	glow, depth, reach float64
	ready              bool
}

// resize sets the post for a field.
func (p *post) resize(f *frame) {
	n := f.fw * f.fh
	p.radial, p.bright, p.tmp = make([]float64, n), make([]float64, n), make([]float64, n)
	for py := range f.fh {
		for px := range f.fw {
			x := (float64(px) - f.cx) / f.radius
			y := (float64(py) - f.cy) * f.aspect / f.radius
			p.radial[py*f.fw+px] = math.Hypot(x, y)
		}
	}
}

// apply blooms and vignettes value in place. lift is how far the music is
// above its running average.
func (p *post) apply(f *frame, value []float64, s Signals, lift, dt float64) {
	glow := 0.12 + 0.5*math.Max(s.Release, lift)
	depth := clamp01(0.1 + 0.25*s.Tension - 0.1*s.Release)
	reach := 0.7 - 0.25*s.Tension + 0.3*s.Release
	if !p.ready {
		p.glow, p.depth, p.reach, p.ready = glow, depth, reach, true
	}
	p.glow = glide(p.glow, glow, dt, 0.25)
	p.depth = glide(p.depth, depth, dt, 0.4)
	p.reach = glide(p.reach, reach, dt, 0.4)
	rx := 3
	ry := max(1, int(math.Round(3/f.aspect)))
	bloom(value, p.bright, p.tmp, f.fw, f.fh, rx, ry, p.glow)
	for i, r := range p.radial {
		value[i] *= 1 - p.depth*smooth(p.reach, p.reach+0.9, r)
	}
}

// bloom adds to value a blur of its parts brighter than 0.6, rx pixels across
// and ry pixels down, at strength glow. bright and tmp are scratch space the
// size of value.
func bloom(value, bright, tmp []float64, w, h, rx, ry int, glow float64) {
	for i, v := range value {
		bright[i] = math.Max(v-0.6, 0) / 0.4
	}
	for y := range h {
		boxBlur(bright[y*w:(y+1)*w], tmp[y*w:(y+1)*w], 1, w, rx)
	}
	for x := range w {
		boxBlur(tmp[x:], bright[x:], w, h, ry)
	}
	for i, b := range bright {
		value[i] = clamp01(value[i] + glow*b)
	}
}

// boxBlur sets n samples of out, stride apart, to the mean of the samples of
// in within r of each, treating samples past the ends as 0.
func boxBlur(in, out []float64, stride, n, r int) {
	sum := 0.0
	for i := 0; i < min(r, n); i++ {
		sum += in[i*stride]
	}
	k := 1 / float64(2*r+1)
	for i := range n {
		if j := i + r; j < n {
			sum += in[j*stride]
		}
		if j := i - r - 1; j >= 0 {
			sum -= in[j*stride]
		}
		out[i*stride] = sum * k
	}
}
