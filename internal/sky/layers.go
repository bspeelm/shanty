package sky

import (
	"math"
	"math/rand/v2"
)

// component is one thing a scene can draw. draw fills buf with one value from
// 0 to 1 per field pixel, given the component's four parameter values and its
// clock. v[0] always sets how fast the clock runs.
type component interface {
	draw(f *frame, v [4]float64, clock float64, buf []float64)
}

// kind describes a component: its name, whether it fills the frame or draws
// points and lines, and how to make one.
type kind struct {
	name    string
	dense   bool
	classic bool
	make    func(rng *rand.Rand) component
}

// kinds is every component a scene can be built from.
var kinds = []kind{
	{"spiral", true, false, func(r *rand.Rand) component { return spiral{arms: float64(2 + r.IntN(4))} }},
	{"rings", false, false, func(r *rand.Rand) component { return &rings{onSnap: r.IntN(2) == 0} }},
	{"plasma", true, false, func(r *rand.Rand) component { return plasma{seed: r.Float64() * 10} }},
	{"tunnel", true, false, func(r *rand.Rand) component { return tunnel{sides: float64(3 + r.IntN(6))} }},
	{"stars", false, false, func(r *rand.Rand) component { return &stars{} }},
	{"orbits", false, false, func(r *rand.Rand) component { return &orbits{a: float64(1 + r.IntN(4)), b: float64(2 + r.IntN(4))} }},
	{"cells", true, false, func(r *rand.Rand) component { return &cells{} }},
	{"lattice", true, false, func(r *rand.Rand) component { return lattice{} }},
	{"terrain", true, false, func(r *rand.Rand) component { return terrain{mirror: r.IntN(2) == 0} }},
	{"sun", false, false, func(r *rand.Rand) component { return sun{rays: float64(4 + 2*r.IntN(5))} }},
	{"life", true, true, func(r *rand.Rand) component { return &life{} }},
	{"julia", true, true, func(r *rand.Rand) component { return &julia{} }},
	{"lorenz", false, true, func(r *rand.Rand) component { return &lorenz{} }},
	{"fire", true, true, func(r *rand.Rand) component { return &fire{} }},
	{"rule", true, true, func(r *rand.Rand) component { return &automaton{rule: pickRule(r)} }},
	{"ant", false, true, func(r *rand.Rand) component { return &ants{} }},
	{"reaction", true, true, func(r *rand.Rand) component { return &reaction{} }},
	{"chaos", false, true, func(r *rand.Rand) component { return &chaos{fern: r.IntN(2) == 0} }},
	{"boids", false, true, func(r *rand.Rand) component { return &flock{} }},
	{"epicycles", false, true, func(r *rand.Rand) component { return &epicycles{} }},
	{"flow", false, false, func(r *rand.Rand) component { return &flow{} }},
	{"grid", true, false, func(r *rand.Rand) component { return &beatGrid{} }},
	{"sparks", false, false, func(r *rand.Rand) component { return &sparks{} }},
	{"night lake", true, false, func(r *rand.Rand) component { return newLandscape(r, settings["night lake"]) }},
	{"dawn sea", true, false, func(r *rand.Rand) component { return newLandscape(r, settings["dawn sea"]) }},
	{"storm", true, false, func(r *rand.Rand) component { return newLandscape(r, settings["storm"]) }},
	{"aurora", true, false, func(r *rand.Rand) component { return newLandscape(r, settings["aurora"]) }},
}

type spiral struct{ arms float64 }

func (s spiral) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	twist := 1 + 3*v[1]
	f.fill(buf, func(x, y float64) float64 {
		r := math.Hypot(x, y)
		w := math.Sin(s.arms*math.Atan2(y, x)+twist*math.Log(r+0.05)*2-clock*2) +
			0.6*math.Sin(r*6-clock*3)
		return clamp01((w/2 + 0.5) / (1 + r*(1.5-v[2])))
	})
}

// rings sends a ring outward on each kick, or each snap. A ring starts in the
// middle, or, as often as the layer is coupled, at the brightest point of the
// last frame.
type rings struct {
	onSnap bool
	ages   [][3]float64
	since  float64
}

func (g *rings) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	hit := f.sig.Kick
	if g.onSnap {
		hit = f.sig.Snap
	}
	g.since += f.dt
	if hit > 0.55 && g.since > 0.12 {
		var cx, cy float64
		if f.rng.Float64() < f.couple {
			cx, cy = brightest(f)
		}
		g.ages, g.since = append(g.ages, [3]float64{0, cx, cy}), 0
	}
	kept := g.ages[:0]
	for _, a := range g.ages {
		if a[0]+f.dt < 3 {
			a[0] += f.dt
			kept = append(kept, a)
		}
	}
	g.ages = kept
	speed, width := 0.4+1.2*v[0], 0.03+0.15*v[1]
	f.fill(buf, func(x, y float64) float64 {
		sum := 0.0
		for _, a := range g.ages {
			d := (math.Hypot(x-a[1], y-a[2]) - a[0]*speed) / width
			sum += math.Exp(-d*d) * math.Exp(-a[0]*1.2)
		}
		return clamp01(sum)
	})
}

type plasma struct{ seed float64 }

func (p plasma) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	k := 1.5 + 5*v[1]
	c := clock + p.seed
	f.fill(buf, func(x, y float64) float64 {
		s := math.Sin(x*k+c) + math.Sin(y*k*0.8-c*0.7) +
			math.Sin((x+y)*k*0.6+c*1.3) + math.Sin(math.Hypot(x, y)*k*1.4-c)
		return clamp01(math.Pow(s/8+0.5, 1+2*v[2]))
	})
}

type tunnel struct{ sides float64 }

func (t tunnel) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	f.fill(buf, func(x, y float64) float64 {
		r := math.Hypot(x, y)
		depth := 1/(r+0.08) + clock*2
		a := math.Atan2(y, x) * t.sides / math.Pi
		check := math.Sin(depth*(2+4*v[1])) * math.Sin(a*math.Pi/2+clock*v[2])
		return clamp01((check*0.5 + 0.5) * smooth(0.02, 0.5, r))
	})
}

// splat is a picture made of points, in unwarped field pixels. It fades a
// little each frame, so moving points leave trails. Where the last frame was
// bright, a coupled splat fades more slowly, so its trails gather there.
type splat struct{ v []float64 }

func (s *splat) begin(f *frame, keep float64) {
	if len(s.v) != f.fw*f.fh {
		s.v = make([]float64, f.fw*f.fh)
	}
	env := len(f.env) == len(s.v) && f.couple > 0
	for i := range s.v {
		k := keep
		if env {
			k += (1 - keep) * f.couple * f.env[i] * 0.6
		}
		s.v[i] *= k
	}
}

// brightest returns the brightest of a coarse grid of points in the last
// frame, in the field's own units.
func brightest(f *frame) (float64, float64) {
	bx, by, best := 0.0, 0.0, -1.0
	for j := range 9 {
		for i := range 15 {
			x := (float64(i)/14 - 0.5) * float64(f.fw) / f.radius
			y := (float64(j)/8 - 0.5) * float64(f.fh) * f.aspect / f.radius
			if v := f.envAt(x, y); v > best {
				bx, by, best = x, y, v
			}
		}
	}
	return bx, by
}

// plot draws a point of brightness v at (x, y), shared between the four
// pixels around it so that a moving point moves smoothly.
func (s *splat) plot(f *frame, x, y, v float64) {
	px := x*f.radius + f.cx
	py := y*f.radius/f.aspect + f.cy
	x0, y0 := math.Floor(px), math.Floor(py)
	fx, fy := px-x0, py-y0
	for _, c := range [4][3]float64{
		{x0, y0, (1 - fx) * (1 - fy)}, {x0 + 1, y0, fx * (1 - fy)},
		{x0, y0 + 1, (1 - fx) * fy}, {x0 + 1, y0 + 1, fx * fy},
	} {
		ix, iy := int(c[0]), int(c[1])
		if ix >= 0 && iy >= 0 && ix < f.fw && iy < f.fh {
			i := iy*f.fw + ix
			s.v[i] = math.Max(s.v[i], v*math.Sqrt(c[2]))
		}
	}
}

// line plots points along the segment from (x0, y0) to (x1, y1).
func (s *splat) line(f *frame, x0, y0, x1, y1, v float64) {
	n := int(math.Hypot(x1-x0, y1-y0)*f.radius) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		s.plot(f, x0+(x1-x0)*t, y0+(y1-y0)*t, v)
	}
}

// show draws the splat into buf through the warps.
func (s *splat) show(f *frame, buf []float64) {
	f.fill(buf, func(x, y float64) float64 {
		return bilinear(s.v, f.fw, f.fh, x*f.radius+f.cx, y*f.radius/f.aspect+f.cy)
	})
}

type stars struct {
	splat
	pos [][3]float64
}

func (s *stars) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	s.begin(f, 0.5+0.45*v[2])
	if s.pos == nil {
		for range 160 {
			s.pos = append(s.pos, [3]float64{f.rng.Float64()*2 - 1, f.rng.Float64()*2 - 1, f.rng.Float64()})
		}
	}
	speed := (0.1 + 0.9*v[0]) * f.dt
	for i := range s.pos {
		p := &s.pos[i]
		p[2] -= speed * (1 + 2*f.couple*f.envAt(p[0]/p[2]*0.3, p[1]/p[2]*0.3))
		if p[2] < 0.03 {
			*p = [3]float64{f.rng.Float64()*2 - 1, f.rng.Float64()*2 - 1, 1}
		}
		s.plot(f, p[0]/p[2]*0.3, p[1]/p[2]*0.3, clamp01(1.2-p[2]))
	}
	s.show(f, buf)
}

// orbits traces Lissajous figures, several points chasing round each one.
type orbits struct {
	splat
	a, b float64
}

func (o *orbits) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	o.begin(f, 0.85+0.13*v[2])
	size := 0.4 + 0.6*v[1]
	for k := range 5 {
		off := float64(k) * 0.7
		for j := range 8 {
			t := clock + off + float64(j)*0.01
			o.plot(f, size*math.Sin(o.a*t+off), size*math.Sin(o.b*t), 1)
		}
	}
	o.show(f, buf)
}

// cells draws the edges between regions around points that drift.
type cells struct{ seeds [][4]float64 }

func (c *cells) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	if c.seeds == nil {
		for range 14 {
			c.seeds = append(c.seeds, [4]float64{f.rng.Float64()*3 - 1.5, f.rng.Float64()*2 - 1, f.rng.Float64() - 0.5, f.rng.Float64() - 0.5})
		}
	}
	for i := range c.seeds {
		s := &c.seeds[i]
		s[0] += s[2] * f.dt * v[0]
		s[1] += s[3] * f.dt * v[0]
		if math.Abs(s[0]) > 1.6 {
			s[2] = -s[2]
		}
		if math.Abs(s[1]) > 1.1 {
			s[3] = -s[3]
		}
	}
	edge := 0.02 + 0.12*v[1]
	f.fill(buf, func(x, y float64) float64 {
		d1, d2 := 9.0, 9.0
		for _, s := range c.seeds {
			d := math.Hypot(x-s[0], y-s[1])
			if d < d1 {
				d1, d2 = d, d1
			} else if d < d2 {
				d2 = d
			}
		}
		return clamp01(1 - smooth(0, edge, d2-d1) + v[2]*0.3*(1-d1))
	})
}

type lattice struct{}

func (lattice) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	k := 3 + 9*v[1]
	sharp := 4 + 30*v[2]
	f.fill(buf, func(x, y float64) float64 {
		a := math.Pow(math.Abs(math.Cos((x+clock*0.2)*k)), sharp)
		b := math.Pow(math.Abs(math.Cos(y*k)), sharp)
		return math.Max(a, b)
	})
}

// terrain draws the level history as a skyline scrolling across the frame.
type terrain struct{ mirror bool }

func (t terrain) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	n := len(f.history)
	for py := range f.fh {
		for px := range f.fw {
			buf[py*f.fw+px] = 0
		}
	}
	if n == 0 {
		return
	}
	height := 0.3 + 0.7*v[1]
	for px := range f.fw {
		i := n - 1 - (f.fw-1-px)*n/f.fw
		h := f.history[max(i, 0)] * height * float64(f.fh)
		if t.mirror {
			h /= 2
		}
		for py := range f.fh {
			from := float64(f.fh - py)
			if t.mirror {
				from = float64(f.fh)/2 - math.Abs(float64(py)-float64(f.fh)/2)
			}
			if from <= h {
				buf[py*f.fw+px] = 0.4 + 0.6*from/math.Max(h, 1)
			}
		}
	}
}

type sun struct{ rays float64 }

func (s sun) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	size := 0.1 + 0.4*v[1]
	f.fill(buf, func(x, y float64) float64 {
		r := math.Hypot(x, y)
		core := math.Exp(-r * r / (size * size))
		ray := math.Pow(math.Max(0, math.Cos(s.rays*math.Atan2(y, x)+clock)), 8) * math.Exp(-r*(2-1.5*v[2]))
		return clamp01(core + ray)
	})
}

// flow carries particles along a field of currents that turns slowly, so that
// they draw long, smooth streams.
type flow struct {
	splat
	pts [][3]float64
}

func (w *flow) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	w.begin(f, 0.9+0.08*v[2])
	ex, ey := float64(f.fw)/f.radius/2, float64(f.fh)*f.aspect/f.radius/2
	if w.pts == nil {
		for range 260 {
			w.pts = append(w.pts, [3]float64{(f.rng.Float64()*2 - 1) * ex, (f.rng.Float64()*2 - 1) * ey, f.rng.Float64() * 6})
		}
	}
	k := 1 + 2*v[1]
	speed := (0.15 + 0.6*v[0]) * f.dt
	for i := range w.pts {
		p := &w.pts[i]
		a := math.Sin(p[0]*k+clock*0.3)*math.Cos(p[1]*k*1.3-clock*0.2)*math.Pi*2 +
			math.Sin((p[0]+p[1])*k*0.5+clock*0.1)
		p[0] += math.Cos(a) * speed
		p[1] += math.Sin(a) * speed
		p[2] -= f.dt
		if p[2] < 0 || math.Abs(p[0]) > ex || math.Abs(p[1]) > ey {
			*p = [3]float64{(f.rng.Float64()*2 - 1) * ex, (f.rng.Float64()*2 - 1) * ey, 2 + f.rng.Float64()*6}
		}
		w.plot(f, p[0], p[1], 0.8)
	}
	w.show(f, buf)
}

// beatGrid is a grid of tiles that light in a pattern, a new pattern on each
// beat, each lit tile fading through the beat, blended towards the breath by
// how little the tempo is trusted. While the beat is not trusted a kick moves
// it on.
type beatGrid struct {
	beat    int
	pattern int
	fade    float64
	seed    int
}

// The patterns a beat grid lights its tiles in.
const gridPatterns = 5

func (g *beatGrid) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	next := false
	if f.sig.onTheBeat() {
		if f.sig.Beat != g.beat {
			g.beat, next = f.sig.Beat, true
		}
		g.fade = f.sig.onBeat(math.Pow(1-f.sig.Phase, 2))
	} else {
		g.fade *= math.Exp(-f.dt * 4)
		if f.sig.Kick > 0.6 && g.fade < 0.5 {
			next = true
			g.beat++
		}
	}
	if next {
		g.pattern = f.rng.IntN(gridPatterns)
		g.seed = f.rng.IntN(1 << 16)
		if !f.sig.onTheBeat() {
			g.fade = 1
		}
	}
	cols := 4 + int(12*v[1])
	gap := 0.08 + 0.2*v[2]
	f.fill(buf, func(x, y float64) float64 {
		gx, gy := (x+2)*float64(cols)/4, (y+2)*float64(cols)/4
		cx, cy := int(math.Floor(gx)), int(math.Floor(gy))
		fx, fy := gx-float64(cx), gy-float64(cy)
		if fx < gap || fy < gap {
			return 0
		}
		on := false
		switch g.pattern {
		case 0:
			on = (cx+cy+g.beat)%2 == 0
		case 1:
			on = (cx-cy+cols*4+g.beat)%3 == 0
		case 2:
			on = hash(cx+g.seed, cy) < 0.35
		case 3:
			d := int(math.Hypot(float64(cx)-float64(cols)/2, float64(cy)-float64(cols)/2))
			on = d%2 == g.beat%2
		default:
			on = cx%4 == g.beat%4 || cy%4 == g.beat%4
		}
		if !on {
			return 0.05
		}
		return g.fade
	})
}

// sparks throws bursts of particles from a point that wanders, on each snap,
// and lets them fall and fade.
type sparks struct {
	splat
	pts [][5]float64
}

func (s *sparks) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	s.begin(f, 0.55+0.35*v[2])
	if f.sig.Snap > 0.5 || f.sig.Kick > 0.8 {
		ox, oy := 0.7*math.Sin(clock*0.7), 0.4*math.Sin(clock*1.1)
		if f.rng.Float64() < f.couple {
			ox, oy = brightest(f)
		}
		for range 10 + int(30*v[1]) {
			a := f.rng.Float64() * 2 * math.Pi
			sp := (0.3 + f.rng.Float64()) * (0.6 + 0.8*v[0])
			s.pts = append(s.pts, [5]float64{ox, oy, math.Cos(a) * sp, math.Sin(a) * sp, 1})
		}
	}
	kept := s.pts[:0]
	for _, p := range s.pts {
		p[3] += f.dt * 1.5
		p[0] += p[2] * f.dt
		p[1] += p[3] * f.dt
		p[2] *= math.Exp(-f.dt)
		p[4] -= f.dt * 0.8
		if p[4] > 0 && len(kept) < 600 {
			s.plot(f, p[0], p[1], p[4])
			kept = append(kept, p)
		}
	}
	s.pts = kept
	s.show(f, buf)
}
