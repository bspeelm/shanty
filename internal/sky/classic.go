package sky

import (
	"math"
	"math/rand/v2"
)

// grid is a coarse picture covering the whole field, for the simulations that
// work cell by cell. It is drawn through the warps like everything else.
type grid struct {
	w, h int
	v    []float64
	// shown is what is drawn when the grid is eased: v smoothed over time.
	shown []float64
}

// fit sizes the grid to about cells cells, square on screen, and reports
// whether it had to change, which means the simulation starts again.
func (g *grid) fit(f *frame, cells int) bool {
	ex, ey := float64(f.fw)/f.radius, float64(f.fh)*f.aspect/f.radius
	w := max(8, int(math.Sqrt(float64(cells)*ex/ey)))
	h := max(6, cells/w)
	if w == g.w && h == g.h {
		return false
	}
	g.w, g.h, g.v = w, h, make([]float64, w*h)
	return true
}

// ease draws the grid with each cell gliding towards its value over tau
// seconds, for simulations whose cells flicker from frame to frame.
func (g *grid) ease(f *frame, buf []float64, tau float64) {
	if len(g.shown) != len(g.v) {
		g.shown = append([]float64(nil), g.v...)
	}
	for i, v := range g.v {
		g.shown[i] = glide(g.shown[i], v, f.dt, tau)
	}
	ex, ey := float64(f.fw)/f.radius, float64(f.fh)*f.aspect/f.radius
	f.fill(buf, func(x, y float64) float64 {
		return bilinear(g.shown, g.w, g.h, (x/ex+0.5)*float64(g.w)-0.5, (y/ey+0.5)*float64(g.h)-0.5)
	})
}

func (g *grid) at(x, y int) float64 {
	return g.v[((y+g.h)%g.h)*g.w+(x+g.w)%g.w]
}

// show draws the grid into buf through the warps, blending neighbouring
// cells so that the grid's coarseness does not show.
func (g *grid) show(f *frame, buf []float64) { g.showShifted(f, buf, 0) }

// showShifted draws the grid moved up by shift cells.
func (g *grid) showShifted(f *frame, buf []float64, shift float64) {
	ex, ey := float64(f.fw)/f.radius, float64(f.fh)*f.aspect/f.radius
	f.fill(buf, func(x, y float64) float64 {
		gx := (x/ex+0.5)*float64(g.w) - 0.5
		gy := (y/ey+0.5)*float64(g.h) - 0.5 + shift
		return bilinear(g.v, g.w, g.h, gx, gy)
	})
}

// life is Conway's Game of Life. A cell that dies fades rather than vanishing.
// A kick drops a glider or an R-pentomino somewhere.
type life struct {
	grid
	alive []bool
	carry float64
}

// The shapes a kick drops, as cells relative to a corner.
var (
	glider     = [][2]int{{1, 0}, {2, 1}, {0, 2}, {1, 2}, {2, 2}}
	rPentomino = [][2]int{{1, 0}, {2, 0}, {0, 1}, {1, 1}, {1, 2}}
)

func (l *life) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	if l.fit(f, 3000) {
		l.alive = make([]bool, l.w*l.h)
		for i := range l.alive {
			l.alive[i] = f.rng.IntN(5) == 0
		}
	}
	if f.sig.Kick > 0.6 {
		shape := glider
		if f.rng.IntN(3) == 0 {
			shape = rPentomino
		}
		l.place(shape, f.rng.IntN(l.w), f.rng.IntN(l.h))
	}
	if f.sig.Kick > 0.6 && f.couple > 0 {
		l.seedFrom(f)
	}
	l.carry += f.dt * (4 + 20*v[0])
	for ; l.carry >= 1; l.carry-- {
		l.alive = lifeStep(l.alive, l.w, l.h)
	}
	fade := 0.75 + 0.2*v[2]
	for i, a := range l.alive {
		if a {
			l.v[i] += (1 - l.v[i]) * 0.45
		} else {
			l.v[i] *= fade
		}
	}
	l.show(f, buf)
}

// seedFrom brings cells to life where the last frame was bright, as many as
// the layer is coupled.
func (l *life) seedFrom(f *frame) {
	ex, ey := float64(f.fw)/f.radius, float64(f.fh)*f.aspect/f.radius
	for y := range l.h {
		for x := range l.w {
			e := f.envAt((float64(x)/float64(l.w)-0.5)*ex, (float64(y)/float64(l.h)-0.5)*ey)
			if e > 0.6 && f.rng.Float64() < f.couple*0.3 {
				l.alive[y*l.w+x] = true
			}
		}
	}
}

func (l *life) place(shape [][2]int, x, y int) {
	for _, c := range shape {
		l.alive[((y+c[1])%l.h)*l.w+(x+c[0])%l.w] = true
	}
}

// lifeStep returns the next generation of a w by h board that wraps at its
// edges.
func lifeStep(alive []bool, w, h int) []bool {
	next := make([]bool, len(alive))
	for y := range h {
		for x := range w {
			n := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if (dx != 0 || dy != 0) && alive[((y+dy+h)%h)*w+(x+dx+w)%w] {
						n++
					}
				}
			}
			i := y*w + x
			next[i] = n == 3 || (n == 2 && alive[i])
		}
	}
	return next
}

// julia draws the filled Julia set for a c that circles the edge of the
// Mandelbrot set's main cardioid, pushed in and out by the bass and treble.
type julia struct{ grid }

func (j *julia) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	j.fit(f, 6000)
	a := clock * 0.3
	r := 0.98 + 0.08*(f.sig.Bass-f.sig.Treble)
	cr := r*0.5*math.Cos(a) - 0.25*r*r*math.Cos(2*a)
	ci := r*0.5*math.Sin(a) - 0.25*r*r*math.Sin(2*a)
	zoom := 1.2 + 0.8*v[1]
	for y := range j.h {
		for x := range j.w {
			zr := (float64(x)/float64(j.w) - 0.5) * 3.2 / zoom * float64(j.w) / float64(j.h)
			zi := (float64(y)/float64(j.h) - 0.5) * 3.2 / zoom
			j.v[y*j.w+x] = juliaDepth(zr, zi, cr, ci, 32)
		}
	}
	j.ease(f, buf, 0.06)
}

// juliaDepth iterates z² + c from z and returns how quickly it escapes: 1 for
// a point that never does within limit steps, and less the sooner it escapes.
// The count is made continuous from how far past the escape radius the point
// landed, so that it changes smoothly as c moves.
func juliaDepth(zr, zi, cr, ci float64, limit int) float64 {
	for i := range limit {
		if m := zr*zr + zi*zi; m > 16 {
			smooth := float64(i) + 1 - math.Log2(math.Log2(m)/2)
			return clamp01(smooth / float64(limit))
		}
		zr, zi = zr*zr-zi*zi+cr, 2*zr*zi+ci
	}
	return 1
}

// lorenz traces the Lorenz attractor, turning slowly in front of the viewer.
type lorenz struct {
	splat
	x, y, z float64
}

func (l *lorenz) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	l.begin(f, 0.9+0.08*v[2])
	if l.x == 0 && l.y == 0 && l.z == 0 {
		l.x, l.y, l.z = 0.1, 0, 20
	}
	rho := 24 + 8*f.sig.Level
	turn := clock * 0.2
	for range 40 + int(120*v[0]) {
		h := 0.004
		dx := 10 * (l.y - l.x)
		dy := l.x*(rho-l.z) - l.y
		dz := l.x*l.y - 8.0/3*l.z
		l.x, l.y, l.z = l.x+dx*h, l.y+dy*h, l.z+dz*h
		px, _ := rotate(l.x, l.y, turn)
		l.plot(f, px/25*(0.7+0.5*v[1]), -(l.z-25)/25*(0.7+0.5*v[1]), 1)
	}
	l.show(f, buf)
}

// fire is the classic fire effect: heat is fed into the bottom row and
// carried upward, cooling as it rises.
type fire struct{ grid }

func (g *fire) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	g.fit(f, 4000)
	feed := 0.4 + 0.6*f.sig.Level + 0.5*f.sig.Kick
	ex, ey := float64(f.fw)/f.radius, float64(f.fh)*f.aspect/f.radius
	for x := range g.w {
		e := f.envAt((float64(x)/float64(g.w)-0.5)*ex, 0.4*ey)
		g.v[(g.h-1)*g.w+x] = clamp01((feed + f.couple*e) * f.rng.Float64() * 1.4)
	}
	cool := 0.02 + 0.08*(1-v[1])
	for y := 0; y < g.h-1; y++ {
		for x := range g.w {
			sum := g.at(x-1, y+1) + g.at(x, y+1) + g.at(x+1, y+1) + g.at(x, y+2)
			g.v[y*g.w+x] = math.Max(0, sum/4-cool*f.rng.Float64())
		}
	}
	g.ease(f, buf, 0.05)
}

// automaton is a one-dimensional cellular automaton. Each new row is made from
// the one below by the rule and pushed up from the bottom, one per beat when
// the tempo is known. A snap flips a cell in the new row.
type automaton struct {
	grid
	rule  int
	row   []bool
	carry float64
	beat  int
}

// The rules an automaton is drawn from: each makes a picture worth watching.
var rules = []int{30, 45, 73, 90, 105, 110, 150, 184}

func pickRule(rng *rand.Rand) int { return rules[rng.IntN(len(rules))] }

func (a *automaton) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	if a.fit(f, 5000) {
		a.row = make([]bool, a.w)
		a.row[a.w/2] = true
	}
	steps, part := 0, 0.0
	if f.sig.onTheBeat() {
		if f.sig.Beat != a.beat {
			steps, a.beat = 1, f.sig.Beat
		}
		part = f.sig.Phase
	} else {
		a.carry += f.dt * (4 + 12*v[0])
		steps = int(a.carry)
		a.carry -= float64(steps)
		part = a.carry
	}
	for range steps {
		if f.sig.Snap > 0.5 {
			i := f.rng.IntN(a.w)
			a.row[i] = !a.row[i]
		}
		a.row = ruleStep(a.row, a.rule)
		copy(a.v, a.v[a.w:])
		for x, on := range a.row {
			a.v[(a.h-1)*a.w+x] = 0
			if on {
				a.v[(a.h-1)*a.w+x] = 1
			}
		}
	}
	// The rows slide up between steps, a whole row per step.
	a.showShifted(f, buf, part-1)
}

// ruleStep returns the row after row under an elementary rule, wrapping at
// the ends.
func ruleStep(row []bool, rule int) []bool {
	n := len(row)
	next := make([]bool, n)
	for i := range row {
		k := 0
		if row[(i-1+n)%n] {
			k |= 4
		}
		if row[i] {
			k |= 2
		}
		if row[(i+1)%n] {
			k |= 1
		}
		next[i] = rule>>k&1 == 1
	}
	return next
}

// ants is Langton's ant: on a white cell turn right, on a black one turn left,
// flip the cell and step forward. A kick adds an ant.
type ants struct {
	grid
	black []bool
	ants  [][3]int
}

// The four directions an ant can face, clockwise from up.
var headings = [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

func (a *ants) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	if a.fit(f, 6000) {
		a.black = make([]bool, a.w*a.h)
		a.ants = [][3]int{{a.w / 2, a.h / 2, 0}}
	}
	if f.sig.Kick > 0.7 && len(a.ants) < 12 {
		a.ants = append(a.ants, [3]int{f.rng.IntN(a.w), f.rng.IntN(a.h), f.rng.IntN(4)})
	}
	ex, ey := float64(f.fw)/f.radius, float64(f.fh)*f.aspect/f.radius
	for range 2 + int(30*v[0]) {
		for i := range a.ants {
			a.ants[i] = antStep(a.black, a.w, a.h, a.ants[i])
			ant := &a.ants[i]
			e := f.envAt((float64(ant[0])/float64(a.w)-0.5)*ex, (float64(ant[1])/float64(a.h)-0.5)*ey)
			if e > 0.6 && f.rng.Float64() < f.couple*0.2 {
				ant[2] = (ant[2] + 1) % 4
			}
		}
	}
	for i, b := range a.black {
		if b {
			a.v[i] = math.Max(a.v[i]*0.97, 0.6)
		} else {
			a.v[i] *= 0.9
		}
	}
	for _, ant := range a.ants {
		a.v[ant[1]*a.w+ant[0]] = 1
	}
	a.show(f, buf)
}

// antStep moves one ant, given as x, y and heading, and flips the cell it
// left.
func antStep(black []bool, w, h int, ant [3]int) [3]int {
	i := ant[1]*w + ant[0]
	if black[i] {
		ant[2] = (ant[2] + 3) % 4
	} else {
		ant[2] = (ant[2] + 1) % 4
	}
	black[i] = !black[i]
	ant[0] = (ant[0] + headings[ant[2]][0] + w) % w
	ant[1] = (ant[1] + headings[ant[2]][1] + h) % h
	return ant
}

// reaction is the Gray–Scott model of two chemicals reacting and spreading.
// The bass and the air move its feed and kill rates.
type reaction struct {
	grid
	a, b []float64
	// kicked is set while a kick lasts, so that each kick seeds once.
	kicked bool
}

func (r *reaction) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	if r.fit(f, 2400) {
		r.a, r.b = make([]float64, len(r.v)), make([]float64, len(r.v))
		for i := range r.a {
			r.a[i] = 1
		}
	}
	kick := f.sig.Kick > 0.6
	if (kick && !r.kicked) || clock < f.dt*2 {
		cx, cy := f.rng.IntN(r.w), f.rng.IntN(r.h)
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				r.b[((cy+dy+r.h)%r.h)*r.w+(cx+dx+r.w)%r.w] = 1
			}
		}
	}
	r.kicked = kick
	feed := 0.03 + 0.02*f.sig.Bass
	kill := 0.058 + 0.006*f.sig.Air
	gw, gh := r.w, r.h
	ex, ey := float64(f.fw)/f.radius, float64(f.fh)*f.aspect/f.radius
	extra := make([]float64, gw*gh)
	if f.couple > 0 {
		for y := range gh {
			for x := range gw {
				extra[y*gw+x] = 0.015 * f.couple * f.envAt((float64(x)/float64(gw)-0.5)*ex, (float64(y)/float64(gh)-0.5)*ey)
			}
		}
	}
	for range 2 + int(6*v[0]) {
		na, nb := make([]float64, len(r.a)), make([]float64, len(r.b))
		for y := range gh {
			for x := range gw {
				i := y*gw + x
				lap := func(c []float64) float64 {
					return c[y*gw+(x+1)%gw] + c[y*gw+(x-1+gw)%gw] +
						c[((y+1)%gh)*gw+x] + c[((y-1+gh)%gh)*gw+x] - 4*c[i]
				}
				abb := r.a[i] * r.b[i] * r.b[i]
				fe := feed + extra[i]
				// The two chemicals are kept between 0 and 1, as they are in
				// the model, which keeps each step stable.
				na[i] = clamp01(r.a[i] + 0.2*lap(r.a) - abb + fe*(1-r.a[i]))
				nb[i] = clamp01(r.b[i] + 0.1*lap(r.b) + abb - (fe+kill)*r.b[i])
			}
		}
		r.a, r.b = na, nb
	}
	for i := range r.v {
		r.v[i] = clamp01(r.b[i] * 3)
	}
	r.show(f, buf)
}

// chaos plays the chaos game: a point jumps by one of a few fixed maps chosen
// at random, and where it lands traces a fractal. It draws Barnsley's fern or
// Sierpinski's triangle; the balance tilts which maps are chosen.
type chaos struct {
	splat
	fern bool
	x, y float64
}

func (c *chaos) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	c.begin(f, 0.9+0.09*v[2])
	scale := 0.5 + 0.5*v[1]
	for range 200 + int(1500*f.sig.Level) {
		p := f.rng.Float64()
		if c.fern {
			c.x, c.y = fernStep(c.x, c.y, p)
			c.plot(f, (c.x/5)*scale*1.6, (1-c.y/5)*scale*1.6-0.8*scale, 1)
			continue
		}
		corner := int(p * 3)
		if t := f.sig.Balance; t != 0 && f.rng.Float64() < math.Abs(t)*0.3 {
			corner = 1 + int(math.Copysign(1, t))
			corner = min(max(corner, 0), 2)
		}
		cx, cy := []float64{-1, 0, 1}[corner], []float64{0.8, -0.9, 0.8}[corner]
		c.x, c.y = (c.x+cx)/2, (c.y+cy)/2
		c.plot(f, c.x*scale*1.2, c.y*scale*1.2, 1)
	}
	c.show(f, buf)
}

// fernStep applies one of Barnsley's four maps, chosen by p from 0 to 1.
func fernStep(x, y, p float64) (float64, float64) {
	switch {
	case p < 0.01:
		return 0, 0.16 * y
	case p < 0.86:
		return 0.85*x + 0.04*y, -0.04*x + 0.85*y + 1.6
	case p < 0.93:
		return 0.2*x - 0.26*y, 0.23*x + 0.22*y + 1.6
	default:
		return -0.15*x + 0.28*y, 0.26*x + 0.24*y + 0.44
	}
}

// flock is a flock of boids: each steers towards its neighbours, matches
// their heading and keeps out of their way.
type flock struct {
	splat
	birds [][4]float64
}

func (b *flock) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	b.begin(f, 0.6+0.35*v[2])
	if b.birds == nil {
		for range 90 {
			b.birds = append(b.birds, [4]float64{f.rng.Float64()*2 - 1, f.rng.Float64()*2 - 1, f.rng.Float64() - 0.5, f.rng.Float64() - 0.5})
		}
	}
	pull := 0.2 + 1.5*f.sig.Bass
	speed := 0.3 + 0.8*v[0] + 0.6*f.sig.Treble
	next := make([][4]float64, len(b.birds))
	for i, me := range b.birds {
		var cx, cy, vx, vy, ax, ay float64
		n := 0
		for j, o := range b.birds {
			dx, dy := o[0]-me[0], o[1]-me[1]
			d := math.Hypot(dx, dy)
			if j == i || d > 0.4 {
				continue
			}
			cx, cy, vx, vy = cx+dx, cy+dy, vx+o[2], vy+o[3]
			if d < 0.08 {
				ax, ay = ax-dx/(d+0.01), ay-dy/(d+0.01)
			}
			n++
		}
		nb := me
		if n > 0 {
			k := float64(n)
			nb[2] += (cx/k*pull + (vx/k-me[2])*0.5 + ax*0.02) * f.dt * 4
			nb[3] += (cy/k*pull + (vy/k-me[3])*0.5 + ay*0.02) * f.dt * 4
		}
		nb[2], nb[3] = nb[2]-me[0]*f.dt*0.5, nb[3]-me[1]*f.dt*0.5
		if f.couple > 0 {
			const h = 0.05
			gx := f.envAt(me[0]+h, me[1]) - f.envAt(me[0]-h, me[1])
			gy := f.envAt(me[0], me[1]+h) - f.envAt(me[0], me[1]-h)
			nb[2] += gx / (2 * h) * f.couple * f.dt * 3
			nb[3] += gy / (2 * h) * f.couple * f.dt * 3
		}
		if s := math.Hypot(nb[2], nb[3]); s > 0 {
			nb[2], nb[3] = nb[2]/s*speed, nb[3]/s*speed
		}
		nb[0], nb[1] = nb[0]+nb[2]*f.dt, nb[1]+nb[3]*f.dt
		next[i] = nb
		b.plot(f, nb[0], nb[1], 1)
	}
	b.birds = next
	b.show(f, buf)
}

// epicycles redraws the level history as a closed loop traced by a chain of
// turning circles, the terms of its discrete Fourier transform.
type epicycles struct{ splat }

func (e *epicycles) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	e.begin(f, 0.9)
	n := len(f.history)
	if n < 8 {
		e.show(f, buf)
		return
	}
	pts := make([]complex128, n)
	for i, h := range f.history {
		a := 2 * math.Pi * float64(i) / float64(n)
		r := 0.4 + 0.5*h
		pts[i] = complex(r*math.Cos(a), r*math.Sin(a))
	}
	terms := 3 + int(12*v[1])
	t := math.Mod(clock*0.2, 1)
	var x, y float64
	for k := range terms {
		freq := k/2 + 1
		if k%2 == 1 {
			freq = -freq
		}
		if k == 0 {
			freq = 0
		}
		var c complex128
		for i, p := range pts {
			a := -2 * math.Pi * float64(freq*i) / float64(n)
			c += p * complex(math.Cos(a), math.Sin(a))
		}
		c /= complex(float64(n), 0)
		a := 2 * math.Pi * float64(freq) * t
		nx := x + real(c)*math.Cos(a) - imag(c)*math.Sin(a)
		ny := y + real(c)*math.Sin(a) + imag(c)*math.Cos(a)
		e.line(f, x, y, nx, ny, 0.5)
		x, y = nx, ny
	}
	e.plot(f, x, y, 1)
	e.show(f, buf)
}

// maze carves a maze by recursive backtracking, a step or more at a time, and
// starts a new one a while after it finishes.
type maze struct {
	grid
	open   []bool
	cw, ch int
	seen   []bool
	stack  [][2]int
	done   float64
	carry  float64
}

func (m *maze) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	if m.fit(f, 4000) || m.done > 4 {
		m.cw, m.ch = (m.w-1)/2, (m.h-1)/2
		m.seen = make([]bool, m.cw*m.ch)
		m.open = make([]bool, m.w*m.h)
		m.stack = [][2]int{{0, 0}}
		m.seen[0] = true
		m.done = 0
		clear(m.v)
		m.open[m.w+1] = true
	}
	rate := 20 + 80*v[0]
	if f.sig.Tempo > 0 {
		rate = f.sig.blendBeat(f.sig.Tempo/60*(8+24*v[0]), rate)
	}
	m.carry += f.dt * rate
	for ; m.carry >= 1 && len(m.stack) > 0; m.carry-- {
		m.stack = mazeStep(m.stack, m.seen, m.cw, m.ch, f.rng, func(x, y int) {
			m.open[y*m.w+x] = true
		})
	}
	if len(m.stack) == 0 {
		m.done += f.dt
	}
	for i, o := range m.open {
		if o {
			m.v[i] += (1 - m.v[i]) * 0.3
		}
	}
	m.show(f, buf)
}

// mazeStep takes one step of the recursive backtracker on a cw by ch maze of
// cells, calling carve for every grid square it opens.
func mazeStep(stack [][2]int, seen []bool, cw, ch int, rng *rand.Rand, carve func(x, y int)) [][2]int {
	top := stack[len(stack)-1]
	var open [][2]int
	for _, d := range headings {
		nx, ny := top[0]+d[0], top[1]+d[1]
		if nx >= 0 && ny >= 0 && nx < cw && ny < ch && !seen[ny*cw+nx] {
			open = append(open, [2]int{nx, ny})
		}
	}
	if len(open) == 0 {
		return stack[:len(stack)-1]
	}
	n := open[rng.IntN(len(open))]
	seen[n[1]*cw+n[0]] = true
	carve(top[0]+n[0]+1, top[1]+n[1]+1)
	carve(2*n[0]+1, 2*n[1]+1)
	return append(stack, n)
}

// sorter cuts the level history into bars and bubble-sorts them, a swap at a
// time, then cuts a new set.
type sorter struct {
	bars  []float64
	shown []float64
	i, j  int
	carry float64
	beat  int
	rest  float64
}

func (s *sorter) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	if s.bars == nil || s.rest > 3 {
		s.bars = append([]float64(nil), f.history...)
		for len(s.bars) < 24 {
			s.bars = append(s.bars, f.rng.Float64())
		}
		s.bars = s.bars[len(s.bars)-24:]
		s.shown = append([]float64(nil), s.bars...)
		s.i, s.j, s.rest = 0, 0, 0
	}
	steps := 0
	if f.sig.onTheBeat() && f.sig.Beat != s.beat {
		steps, s.beat = 1+int(7*v[0]), f.sig.Beat
	} else if !f.sig.onTheBeat() {
		s.carry += f.dt * (4 + 20*v[0])
		steps = int(s.carry)
		s.carry -= float64(steps)
	}
	for range steps {
		if !bubbleStep(s.bars, &s.i, &s.j) {
			break
		}
	}
	if s.i >= len(s.bars)-1 {
		s.rest += f.dt
	}
	n := len(s.bars)
	for k, b := range s.bars {
		s.shown[k] = glide(s.shown[k], b, f.dt, 0.08)
	}
	for py := range f.fh {
		for px := range f.fw {
			k := px * n / f.fw
			v := 0.0
			if float64(f.fh-py) <= s.shown[k]*float64(f.fh)*0.9 && px%(f.fw/n+1) != 0 {
				v = 0.5
				if k == s.j || k == s.j+1 {
					v = 1
				}
			}
			buf[py*f.fw+px] = v
		}
	}
}

// bubbleStep makes one comparison of a bubble sort, swapping if needed, and
// reports false once the bars are sorted. i counts passes and j is the
// position within the pass.
func bubbleStep(bars []float64, i, j *int) bool {
	n := len(bars)
	if *i >= n-1 {
		return false
	}
	if bars[*j] > bars[*j+1] {
		bars[*j], bars[*j+1] = bars[*j+1], bars[*j]
	}
	*j++
	if *j >= n-1-*i {
		*i, *j = *i+1, 0
	}
	return true
}

// rarities are the components a rare scene is built around, each on its own.
var rarities = []kind{
	{"game of life", true, true, func(r *rand.Rand) component { return &life{} }},
	{"rule 110", true, true, func(r *rand.Rand) component { return &automaton{rule: 110} }},
	{"langton's ant", false, true, func(r *rand.Rand) component { return &ants{} }},
	{"maze", true, true, func(r *rand.Rand) component { return &maze{} }},
	{"bubble sort", true, true, func(r *rand.Rand) component { return &sorter{} }},
}
