package sky

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
)

// layer is one component in a scene, with its parameters and how it is
// combined with the layers under it. tone is where on the palette it leans.
type layer struct {
	name   string
	comp   component
	p      [4]param
	weight param
	blend  blend
	tone   float64
	clock  float64
	buf    []float64
}

// scene is a set of layers drawn through a set of warps, with feedback from
// its own last frame, a way of drawing cells and a palette.
type scene struct {
	layers []*layer
	warps  []warp
	// keep, zoom and turn set how much of the last frame shows through and
	// how it is moved before it does.
	keep, zoom, turn param
	glyphs           glyphs
	palette          []RGB
	label            string
	clock            float64
	// gain scales the frame so that its mean brightness stays near
	// exposure, whatever the layers add up to.
	gain        float64
	value, tone []float64
	last        []float64
	wv          [][4]float64
}

// combo names the components in a scene, so that the director can tell two
// scenes made of the same things apart from two that differ.
func (s *scene) combo() string {
	names := make([]string, len(s.layers))
	for i, l := range s.layers {
		names[i] = l.name
	}
	slices.Sort(names)
	return strings.Join(names, "+")
}

// newScene makes a scene from rng. A busy or loud moment leans towards
// components that fill the frame and fast warps; a quiet one towards points
// and lines and long trails.
func newScene(rng *rand.Rand, mood Signals) *scene {
	energy := clamp01(0.6*mood.Level + 0.6*mood.Busy)
	s := &scene{palette: newPalette(rng), glyphs: pickGlyphs(rng)}
	count := 2 + rng.IntN(2)
	if energy > 0.4 {
		count++
	}
	for len(s.layers) < count {
		k := pickKind(rng, energy)
		if slices.ContainsFunc(s.layers, func(l *layer) bool { return l.name == k.name }) {
			continue
		}
		s.layers = append(s.layers, newLayer(rng, k))
	}
	s.layers[0].blend = blendAdd
	for range 1 + rng.IntN(3) {
		s.warps = append(s.warps, newWarp(rng))
	}
	s.keep = randomParam(rng)
	s.keep.base = 0.3 + 0.5*(1-energy)
	s.zoom, s.turn = randomParam(rng), randomParam(rng)
	return s
}

// rareScene makes a scene of one classic algorithm on its own, named on
// screen.
func rareScene(rng *rand.Rand) *scene {
	k := rarities[rng.IntN(len(rarities))]
	l := newLayer(rng, k)
	l.weight = param{base: 1}
	l.blend = blendAdd
	s := &scene{palette: newPalette(rng), glyphs: glyphHalf, label: k.name, layers: []*layer{l}}
	if rng.IntN(2) == 0 {
		s.glyphs = glyphShade
	}
	return s
}

func newLayer(rng *rand.Rand, k kind) *layer {
	l := &layer{name: k.name, comp: k.make(rng), blend: blend(rng.IntN(int(blends))), tone: rng.Float64()}
	for i := range l.p {
		l.p[i] = randomParam(rng)
	}
	l.weight = randomParam(rng)
	l.weight.base = 0.4 + 0.4*rng.Float64()
	return l
}

func newWarp(rng *rand.Rand) warp {
	w := warp{kind: warpKind(rng.IntN(int(warpKinds))), p: [2]param{randomParam(rng), randomParam(rng)}}
	switch w.kind {
	case warpKaleido:
		w.n = 3 + rng.IntN(6)
	case warpMirror:
		w.n = rng.IntN(2)
	}
	return w
}

// pickKind draws a component, weighting those that fill the frame by energy
// and the rest by its absence.
func pickKind(rng *rand.Rand, energy float64) kind {
	weights := make([]float64, len(kinds))
	total := 0.0
	for i, k := range kinds {
		w := 1.5 - energy
		if k.dense {
			w = 0.5 + energy
		}
		if k.classic {
			w *= 0.7
		}
		weights[i] = w
		total += w
	}
	r := rng.Float64() * total
	for i, w := range weights {
		if r < w {
			return kinds[i]
		}
		r -= w
	}
	return kinds[len(kinds)-1]
}

// mutate redraws one parameter of one layer.
func (s *scene) mutate(rng *rand.Rand) {
	l := s.layers[rng.IntN(len(s.layers))]
	if i := rng.IntN(5); i < 4 {
		l.p[i] = randomParam(rng)
	} else {
		base := l.weight.base
		l.weight = randomParam(rng)
		l.weight.base = base
	}
}

// step draws the scene's next frame into value and tone.
func (s *scene) step(f *frame) {
	n := f.fw * f.fh
	if len(s.value) != n {
		s.value, s.tone, s.last = make([]float64, n), make([]float64, n), make([]float64, n)
	}
	s.clock += f.dt
	sig, clock := f.sig, s.clock
	s.wv = s.wv[:0]
	for i := range s.warps {
		w := &s.warps[i]
		v0, v1 := w.p[0].value(sig, clock), w.p[1].value(sig, clock)
		w.clock += f.dt * (v1 - 0.5) * 1.5
		s.wv = append(s.wv, [4]float64{v0, v1, w.clock})
	}
	f.warp(s.warps, s.wv)

	clear(s.value)
	clear(s.tone)
	for _, l := range s.layers {
		if len(l.buf) != n {
			l.buf = make([]float64, n)
		}
		var v [4]float64
		for i := range v {
			v[i] = l.p[i].value(sig, clock)
		}
		l.clock += f.dt * (0.2 + 1.8*v[0])
		l.comp.draw(f, v, l.clock, l.buf)
		w := l.weight.value(sig, clock)
		for i, b := range l.buf {
			b *= w
			s.value[i] = l.blend.mix(s.value[i], b)
			s.tone[i] += (l.tone - s.tone[i]) * b
		}
	}
	s.feedback(f, sig)
	s.expose()
}

// exposure is the mean brightness a frame is scaled towards.
const exposure = 0.4

// expose scales the frame towards a mean brightness of exposure, moving the
// gain a little each frame so that a beat still brightens the picture.
func (s *scene) expose() {
	if s.gain == 0 {
		s.gain = 1
	}
	sum := 0.0
	for _, v := range s.value {
		sum += v
	}
	if mean := sum / float64(len(s.value)); mean > 0.001 {
		want := math.Min(math.Max(exposure/mean, 0.3), 5)
		s.gain += (want - s.gain) * 0.04
	}
	for i, v := range s.value {
		s.value[i] = clamp01(v * s.gain)
	}
}

// feedback lets the last frame show through, moved by the scene's zoom and
// turn, wherever it is brighter than the new one.
func (s *scene) feedback(f *frame, sig Signals) {
	keep := 0.3 + 0.55*s.keep.value(sig, s.clock)
	if s.label != "" {
		keep = 0
	}
	zoom := 0.96 + 0.06*s.zoom.value(sig, s.clock)
	sn, cs := math.Sincos((s.turn.value(sig, s.clock) - 0.5) * 0.08)
	for py := range f.fh {
		for px := range f.fw {
			dx, dy := (float64(px)-f.cx)/zoom, (float64(py)-f.cy)*f.aspect/zoom
			sx := dx*cs - dy*sn + f.cx
			sy := (dx*sn+dy*cs)/f.aspect + f.cy
			ix, iy := int(sx+0.5), int(sy+0.5)
			i := py*f.fw + px
			if ix >= 0 && iy >= 0 && ix < f.fw && iy < f.fh {
				s.value[i] = math.Max(s.value[i], s.last[iy*f.fw+ix]*keep)
			}
		}
	}
	copy(s.last, s.value)
}
