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

// rings sends a ring outward from the middle on each kick, or each snap.
type rings struct {
	onSnap bool
	ages   []float64
	since  float64
}

func (g *rings) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	hit := f.sig.Kick
	if g.onSnap {
		hit = f.sig.Snap
	}
	g.since += f.dt
	if hit > 0.55 && g.since > 0.12 {
		g.ages, g.since = append(g.ages, 0), 0
	}
	kept := g.ages[:0]
	for _, a := range g.ages {
		if a+f.dt < 3 {
			kept = append(kept, a+f.dt)
		}
	}
	g.ages = kept
	speed, width := 0.4+1.2*v[0], 0.03+0.15*v[1]
	f.fill(buf, func(x, y float64) float64 {
		r := math.Hypot(x, y)
		sum := 0.0
		for _, a := range g.ages {
			d := (r - a*speed) / width
			sum += math.Exp(-d*d) * math.Exp(-a*1.2)
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
// little each frame, so moving points leave trails.
type splat struct{ v []float64 }

func (s *splat) begin(f *frame, keep float64) {
	if len(s.v) != f.fw*f.fh {
		s.v = make([]float64, f.fw*f.fh)
	}
	for i := range s.v {
		s.v[i] *= keep
	}
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
		p[2] -= speed
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
