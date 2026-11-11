package sky

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
)

// layer is one component in a scene, with its parameters and how it is
// combined with the layers under it. tone is where on the palette it leans,
// and couple how strongly it reacts to the scene's last frame.
type layer struct {
	name   string
	comp   component
	p      [4]param
	weight param
	blend  blend
	tone   float64
	couple float64
	clock  float64
	buf    []float64
	// v and w are the parameter values and weight drawn with, which glide
	// towards what the signals ask for rather than jumping there.
	v     [4]float64
	w     float64
	ready bool
	// presence is how far the layer is in the scene, multiplied into its
	// weight, and goal what presence is moving towards: 1 while it belongs,
	// 0 while it is leaving.
	presence float64
	goal     float64
}

// scene is a set of layers drawn through a set of warps, with feedback from
// its own last frame, a way of drawing cells and a palette. A scene can grow
// into the next one in place: layers and warps come and go, the palette moves
// from one to another, and the way of drawing dissolves from one to another.
type scene struct {
	layers []*layer
	warps  []*warp
	// keep, zoom and turn set how much of the last frame shows through and
	// how it is moved before it does. bend sets how far the first layer
	// pushes the coordinates of the layers drawn over it.
	keep, zoom, turn, bend param
	// fb holds the gliding values of keep, zoom, turn and bend.
	fb    [4]float64
	ready bool
	bx    []float64
	by    []float64
	// glyphs is how cells are drawn, moving to toGlyphs as glyphsAt goes
	// from 0 to 1. palette moves from fromPalette to toPalette the same way
	// with paletteAt.
	glyphs, toGlyphs       glyphs
	glyphsAt               float64
	palette                []RGB
	fromPalette, toPalette []RGB
	paletteAt              float64
	label                  string
	clock                  float64
	// char is the character the scene was made or last changed for; zero
	// when it was made before the music had been heard.
	char Character
	// glide is how quickly parameters follow their signals, morph how long
	// a change takes, lock how strongly turning steps on the beat, and hits
	// how strongly Kick and Snap move parameters.
	glide, morph, lock, hits float64
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
	var names []string
	for _, l := range s.layers {
		if l.goal > 0 {
			names = append(names, l.name)
		}
	}
	slices.Sort(names)
	return strings.Join(names, "+")
}

// newScene makes a scene in the given style. A busy or loud moment adds a
// layer.
func newScene(rng *rand.Rand, st *style, c Character, mood Signals) *scene {
	energy := clamp01(0.6*mood.Level + 0.6*mood.Busy)
	g := glyphs(pick(rng, st.glyphs[:]))
	s := &scene{glyphs: g, toGlyphs: g, glide: st.glide, morph: st.morph, lock: st.lock, hits: st.hits, char: c}
	s.palette = st.palette(rng, c)
	count := 2 + rng.IntN(2)
	if energy > 0.4 {
		count++
	}
	for len(s.layers) < count {
		s.layers = append(s.layers, newLayer(rng, pickKind(rng, st, s), st))
	}
	s.layers[0].blend = blendAdd
	for range 1 + rng.IntN(3) {
		s.warps = append(s.warps, newWarp(rng, st))
	}
	s.keep = st.param(rng)
	s.keep.base = st.keep[0] + (st.keep[1]-st.keep[0])*rng.Float64()
	s.zoom, s.turn = st.param(rng), st.param(rng)
	s.bend = st.param(rng)
	s.bend.base = 0.1 * rng.Float64()
	for _, l := range s.layers {
		l.presence = 1
	}
	for _, w := range s.warps {
		w.amount = 1
	}
	return s
}

// rareScene makes a scene of one classic algorithm on its own, named on
// screen.
func rareScene(rng *rand.Rand, st *style, c Character) *scene {
	k := rarities[rng.IntN(len(rarities))]
	l := newLayer(rng, k, st)
	l.weight = param{base: 1}
	l.blend = blendAdd
	l.presence, l.couple = 1, 0
	g := glyphHalf
	if rng.IntN(2) == 0 {
		g = glyphShade
	}
	return &scene{palette: st.palette(rng, c), glyphs: g, toGlyphs: g, label: k.name,
		layers: []*layer{l}, glide: st.glide, morph: st.morph, hits: st.hits, char: c}
}

func newLayer(rng *rand.Rand, k kind, st *style) *layer {
	l := &layer{name: k.name, comp: k.make(rng), blend: blend(rng.IntN(int(blends))),
		tone: rng.Float64(), couple: rng.Float64(), goal: 1}
	for i := range l.p {
		l.p[i] = st.param(rng)
	}
	l.weight = st.param(rng)
	l.weight.base = 0.4 + 0.4*rng.Float64()
	return l
}

func newWarp(rng *rand.Rand, st *style) *warp {
	w := &warp{kind: warpKind(pick(rng, st.warps[:])), p: [2]param{st.param(rng), st.param(rng)}, goal: 1}
	switch w.kind {
	case warpKaleido:
		w.n = 3 + rng.IntN(6)
	case warpMirror:
		w.n = rng.IntN(2)
	}
	return w
}

// pickKind draws a component the scene does not already have, weighted by
// the style.
func pickKind(rng *rand.Rand, st *style, s *scene) kind {
	weights := make([]float64, len(kinds))
	for i, k := range kinds {
		if !slices.ContainsFunc(s.layers, func(l *layer) bool { return l.name == k.name }) {
			weights[i] = st.kindWeight(k.name)
		}
	}
	return kinds[pick(rng, weights)]
}

// mutate redraws one parameter of one layer.
func (s *scene) mutate(rng *rand.Rand, st *style) {
	l := s.layers[rng.IntN(len(s.layers))]
	if i := rng.IntN(5); i < 4 {
		l.p[i] = st.param(rng)
	} else {
		base := l.weight.base
		l.weight = st.param(rng)
		l.weight.base = base
	}
}

// evolve starts the scene growing into a new one in the given style. One or
// two layers leave and one or two arrive, keeping between one and three of
// those it had, so that what stays carries on as it was. A warp may leave and
// one may arrive. The palette moves to a turned copy of itself or to a new
// one, and the way of drawing sometimes changes.
//
// When the character has moved some way from the one the scene was made for,
// only one layer stays, and the palette and the way of drawing move to the new
// character's.
func (s *scene) evolve(rng *rand.Rand, st *style, c Character) {
	far := s.char.distance(c) > farChange
	s.glide, s.morph, s.lock, s.hits, s.char = st.glide, st.morph, st.lock, st.hits, c
	var staying []*layer
	for _, l := range s.layers {
		if l.goal > 0 {
			staying = append(staying, l)
		}
	}
	leave := 1 + rng.IntN(2)
	if far {
		leave = len(staying)
	}
	leave = min(leave, len(staying)-1)
	rng.Shuffle(len(staying), func(i, j int) { staying[i], staying[j] = staying[j], staying[i] })
	for _, l := range staying[:max(leave, 0)] {
		l.goal = 0
	}
	kept := len(staying) - max(leave, 0)
	arrive := 1 + rng.IntN(2)
	if far {
		arrive = 2 + rng.IntN(2)
	}
	arrive = min(arrive, 4-kept)
	for range arrive {
		l := newLayer(rng, pickKind(rng, st, s), st)
		s.layers = append(s.layers, l)
	}

	var warps []*warp
	for _, w := range s.warps {
		if w.goal > 0 {
			warps = append(warps, w)
		}
	}
	if len(warps) > 1 && rng.IntN(2) == 0 {
		warps[rng.IntN(len(warps))].goal = 0
	}
	if len(warps) < 3 && rng.Float64() < 0.6 {
		s.warps = append(s.warps, newWarp(rng, st))
	}

	s.fromPalette = s.palette
	if rng.IntN(2) == 0 && !far {
		s.toPalette = turnPalette(s.palette, rng)
	} else {
		s.toPalette = st.palette(rng, c)
	}
	s.paletteAt = 0
	if far || rng.Float64() < 0.35 {
		s.glyphs = s.drawnGlyphs()
		s.toGlyphs, s.glyphsAt = glyphs(pick(rng, st.glyphs[:])), 0
	}
	s.keep.base = st.keep[0] + (st.keep[1]-st.keep[0])*rng.Float64()
}

// How far the character must move from the one a scene was made for before a
// change keeps only one of its layers, and before it starts afresh.
const (
	farChange   = 0.3
	freshChange = 0.6
)

// drawnGlyphs is the way of drawing the scene has mostly arrived at.
func (s *scene) drawnGlyphs() glyphs {
	if s.glyphsAt >= 0.5 {
		return s.toGlyphs
	}
	return s.glyphs
}

// changing reports whether the scene is still growing into its last change.
func (s *scene) changing() bool {
	for _, l := range s.layers {
		if math.Abs(l.presence-l.goal) > 0.02 {
			return true
		}
	}
	return s.toPalette != nil
}

// step draws the scene's next frame into value and tone.
func (s *scene) step(f *frame) {
	n := f.fw * f.fh
	if len(s.value) != n {
		s.value, s.tone, s.last = make([]float64, n), make([]float64, n), make([]float64, n)
		s.bx, s.by = make([]float64, n), make([]float64, n)
	}
	if s.glide == 0 {
		s.glide, s.morph = 0.12, 6
	}
	if s.hits == 0 {
		s.hits = 1
	}
	s.clock += f.dt
	sig, clock := f.sig, s.clock
	sig.Kick, sig.Snap = sig.Kick*s.hits, sig.Snap*s.hits
	s.advance(f.dt)

	s.wv = s.wv[:0]
	for _, w := range s.warps {
		for j := range 2 {
			target := w.p[j].value(sig, clock)
			if !w.ready {
				w.v[j] = target
			}
			w.v[j] = glide(w.v[j], target, f.dt, s.glide)
		}
		w.ready = true
		turn := (w.v[1] - 0.5) * 1.5
		if s.lock > 0.5 && sig.Tempo > 0 {
			// Turning steps a little on each beat and settles between.
			if sig.Beat != w.beat {
				w.beat = sig.Beat
				w.aim += turn * 60 / sig.Tempo * 2
			}
			w.clock = glide(w.clock, w.aim, f.dt, 0.08)
		} else {
			w.clock += f.dt * turn
			w.aim = w.clock
		}
		s.wv = append(s.wv, [4]float64{w.v[0], w.v[1], w.clock})
	}
	for i, p := range []param{s.keep, s.zoom, s.turn, s.bend} {
		target := p.value(sig, clock)
		if !s.ready {
			s.fb[i] = target
		}
		s.fb[i] = glide(s.fb[i], target, f.dt, s.glide)
	}
	s.ready = true
	f.warp(s.warps, s.wv)
	xs, ys := f.xs, f.ys
	f.env = s.last

	clear(s.value)
	clear(s.tone)
	bent := false
	for _, l := range s.layers {
		if len(l.buf) != n {
			l.buf = make([]float64, n)
		}
		for i := range l.v {
			target := l.p[i].value(sig, clock)
			if !l.ready {
				l.v[i] = target
			}
			l.v[i] = glide(l.v[i], target, f.dt, s.glide)
		}
		target := l.weight.value(sig, clock)
		switch {
		case !l.ready:
			l.w = target
		case target > l.w:
			l.w = glide(l.w, target, f.dt, math.Min(riseGlide, s.glide))
		default:
			l.w = glide(l.w, target, f.dt, math.Max(fallGlide, s.glide))
		}
		l.ready = true
		l.clock += f.dt * (0.2 + 1.8*l.v[0])
		f.couple = l.couple
		l.comp.draw(f, l.v, l.clock, l.buf)
		w := l.w * l.presence
		for i, b := range l.buf {
			b *= w
			s.value[i] = l.blend.mix(s.value[i], b)
			s.tone[i] += (l.tone - s.tone[i]) * b
		}
		if !bent && l.goal > 0 {
			s.bendFrom(f, l.buf, xs, ys)
			bent = true
		}
	}
	f.xs, f.ys, f.env = xs, ys, nil
	s.feedback(f, sig)
	s.expose(exposureFor(sig))
}

// How quickly a layer's weight follows its signals, in seconds: it rises
// faster than it falls, so that a beat still lands.
const (
	riseGlide = 0.03
	fallGlide = 0.15
)

// advance moves layers, warps, the palette and the way of drawing on through
// a change, and drops the layers and warps that have finished leaving.
func (s *scene) advance(dt float64) {
	tau := s.morph / 3
	layers := s.layers[:0]
	for _, l := range s.layers {
		l.presence = glide(l.presence, l.goal, dt, tau)
		if l.goal > 0 || l.presence > 0.01 {
			layers = append(layers, l)
		}
	}
	s.layers = layers
	warps := s.warps[:0]
	for _, w := range s.warps {
		w.amount = glide(w.amount, w.goal, dt, tau)
		if w.goal > 0 || w.amount > 0.01 {
			warps = append(warps, w)
		}
	}
	s.warps = warps
	if s.toPalette != nil {
		s.paletteAt += dt / s.morph
		s.palette = blendPalettes(s.fromPalette, s.toPalette, smooth(0, 1, s.paletteAt))
		if s.paletteAt >= 1 {
			s.palette, s.fromPalette, s.toPalette = s.toPalette, nil, nil
		}
	}
	if s.glyphs != s.toGlyphs {
		s.glyphsAt += dt / s.morph
		if s.glyphsAt >= 1 {
			s.glyphs, s.glyphsAt = s.toGlyphs, 0
		}
	}
}

// bendFrom pushes the coordinates the later layers are drawn at outward where
// the first layer is bright and inward where it is dark, so that they move
// through it rather than over it.
func (s *scene) bendFrom(f *frame, base, xs, ys []float64) {
	k := s.fb[3] * 0.6
	for i, b := range base {
		d := 1 + k*(b-0.5)
		s.bx[i], s.by[i] = xs[i]*d, ys[i]*d
	}
	f.xs, f.ys = s.bx, s.by
}

// feedback lets the last frame show through, moved by the scene's zoom and
// turn and pushed outward by a kick, wherever it is brighter than the new one.
// The breath moves keep and zoom by a few percent.
func (s *scene) feedback(f *frame, sig Signals) {
	keep := (0.3 + 0.55*s.fb[0]) * (1 + 0.06*(sig.Breath-0.5))
	if s.label != "" {
		keep = 0
	}
	zoom := (0.96 + 0.06*s.fb[1] + 0.03*sig.Kick) * (1 + 0.01*(sig.Breath-0.5))
	sn, cs := math.Sincos((s.fb[2] - 0.5) * 0.08)
	for py := range f.fh {
		for px := range f.fw {
			dx, dy := (float64(px)-f.cx)/zoom, (float64(py)-f.cy)*f.aspect/zoom
			sx := dx*cs - dy*sn + f.cx
			sy := (dx*sn+dy*cs)/f.aspect + f.cy
			i := py*f.fw + px
			s.value[i] = math.Max(s.value[i], bilinear(s.last, f.fw, f.fh, sx, sy)*keep)
		}
	}
	copy(s.last, s.value)
}

// exposure is the mean brightness a frame is scaled towards on steady music.
const exposure = 0.4

// expose scales the frame towards a mean brightness of target, moving the
// gain a little each frame so that a beat still brightens the picture.
func (s *scene) expose(target float64) {
	if s.gain == 0 {
		s.gain = 1
	}
	sum := 0.0
	for _, v := range s.value {
		sum += v
	}
	if mean := sum / float64(len(s.value)); mean > 0.001 {
		want := math.Min(math.Max(target/mean, 0.3), 5)
		s.gain += (want - s.gain) * 0.04
	}
	for i, v := range s.value {
		s.value[i] = clamp01(v * s.gain)
	}
}
