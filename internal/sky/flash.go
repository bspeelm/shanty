package sky

import "math"

// The field is split into guardCols by guardRows regions, and the guard
// limits how fast the light of each changes.
const guardCols, guardRows = 3, 2

// guardRegions is how many regions the guard watches.
const guardRegions = guardCols * guardRows

// The guard measures the light of a pixel from its value v and its place on
// the palette u, which is v*0.8 + tone*0.2 as colourAt takes it: u² when
// cells are drawn as half blocks, which fill the cell whatever the value, and
// v·u² when they are drawn as dots or characters, which cover more of the
// cell as the value rises. A region's light is the mean over its pixels.
const (
	// guardDrift is how far a region's reference light may move in a second.
	guardDrift = 0.15
	// guardBand is how far a region's light may stray from its reference on
	// any frame.
	guardBand = 0.03
)

// The two ways the guard measures light: for half blocks, and for dots and
// characters.
const (
	lightHalf = iota
	lightDots
	lightWays
)

// flashGuard keeps the light of every region of the field near a reference
// that moves slowly. A region that would come out brighter than its band is
// scaled towards black, and one that would come out darker is scaled towards
// white, in both value and tone. The corrections are blended between the
// middles of the regions, so that no edge shows between them.
type flashGuard struct {
	fw, fh int
	// ref holds each region's reference light, measured both ways.
	ref   [lightWays][guardRegions]float64
	ready bool
	// mul and add are each region's correction, applied to a value or a tone
	// x as x*mul + add.
	mul, add [guardRegions]float64
	// xcell and ycell are, for each column and row, the region whose middle
	// is at or before it, and xfrac and yfrac how far it is towards the
	// middle of the next.
	xcell, ycell []int
	xfrac, yfrac []float64
	// xreg is the column of regions each column of pixels is in.
	xreg []int
}

// resize sets the guard for a field fw by fh and forgets every reference.
func (g *flashGuard) resize(fw, fh int) {
	g.fw, g.fh, g.ready = fw, fh, false
	g.xcell, g.xfrac = spans(fw, guardCols)
	g.ycell, g.yfrac = spans(fh, guardRows)
	g.xreg = make([]int, fw)
	for px := range fw {
		g.xreg[px] = px * guardCols / fw
	}
}

// spans returns, for each of n pixels split into parts equal regions, the
// region whose middle is at or before it and how far it is towards the
// middle of the next, from 0 to 1.
func spans(n, parts int) ([]int, []float64) {
	cell, frac := make([]int, n), make([]float64, n)
	size := float64(n) / float64(parts)
	for i := range n {
		u := (float64(i)+0.5)/size - 0.5
		c := int(math.Floor(u))
		switch {
		case c < 0:
			c, u = 0, 0
		case c >= parts-1:
			c, u = parts-1, float64(parts-1)
		}
		cell[i], frac[i] = c, u-float64(c)
	}
	return cell, frac
}

// apply moves each region's references towards the region's light by at
// most guardDrift a second, then corrects value and tone in place so that
// each region's light stays within guardBand of its reference. half is set
// when cells are drawn as half blocks. The regions are measured and
// corrected up to guardPasses times, until each is within its band.
func (g *flashGuard) apply(value, tone []float64, dt float64, half bool) {
	if len(value) != g.fw*g.fh || len(tone) != len(value) || g.fw == 0 {
		return
	}
	way, power := lightDots, 3.0
	if half {
		way, power = lightHalf, 2
	}
	for pass := range guardPasses {
		lit, dark := g.measure(value, tone)
		if pass == 0 {
			step := guardDrift * dt
			for w := range lit {
				for k, m := range lit[w] {
					if !g.ready {
						g.ref[w][k] = m
					}
					g.ref[w][k] += math.Max(-step, math.Min(step, m-g.ref[w][k]))
				}
			}
			g.ready = true
		}
		held := false
		for k, m := range lit[way] {
			lo, hi := g.ref[way][k]-guardBand, g.ref[way][k]+guardBand
			g.mul[k], g.add[k] = 1, 0
			switch {
			case m > hi+guardSlack && m > 0:
				g.mul[k], held = math.Pow(math.Max(hi, 0)/m, 1/power), true
			case m < lo-guardSlack:
				c := towardsWhite(dark[k], half, math.Min(lo, 1))
				g.mul[k], g.add[k], held = c, 1-c, true
			}
		}
		if !held {
			return
		}
		g.correct(value, tone)
	}
}

// guardPasses is the most times a frame is measured and corrected, and
// guardSlack how far outside its band a region may be left after a pass.
const (
	guardPasses = 3
	guardSlack  = 0.002
)

// moments holds the means over a region of the powers of a = 1-u and
// b = 1-v that its light is made of when it is moved towards white: a, a²,
// b, ab and a²b.
type moments [5]float64

// measure returns each region's light, measured both ways, and its moments.
func (g *flashGuard) measure(value, tone []float64) (lit [lightWays][guardRegions]float64, dark [guardRegions]moments) {
	var n [guardRegions]float64
	for py := range g.fh {
		r := py * guardRows / g.fh * guardCols
		at := py * g.fw
		for px, c := range g.xreg {
			k := r + c
			v := value[at+px]
			u := v*0.8 + clamp01(tone[at+px])*0.2
			u2 := u * u
			lit[lightHalf][k] += u2
			lit[lightDots][k] += u2 * v
			a, b := 1-u, 1-v
			m := &dark[k]
			m[0] += a
			m[1] += a * a
			m[2] += b
			m[3] += a * b
			m[4] += a * a * b
			n[k]++
		}
	}
	for k := range n {
		inv := 1 / math.Max(n[k], 1)
		for w := range lit {
			lit[w][k] *= inv
		}
		for j := range dark[k] {
			dark[k][j] *= inv
		}
	}
	return lit, dark
}

// correct applies each region's correction to value and tone, blended
// bilinearly between the middles of the regions.
func (g *flashGuard) correct(value, tone []float64) {
	var mul, add [guardCols]float64
	for py := range g.fh {
		r0, fy := g.ycell[py], g.yfrac[py]
		r1 := min(r0+1, guardRows-1)
		for c := range guardCols {
			a, b := r0*guardCols+c, r1*guardCols+c
			mul[c] = g.mul[a]*(1-fy) + g.mul[b]*fy
			add[c] = g.add[a]*(1-fy) + g.add[b]*fy
		}
		at := py * g.fw
		for px, c0 := range g.xcell {
			fx := g.xfrac[px]
			c1 := min(c0+1, guardCols-1)
			m := mul[c0]*(1-fx) + mul[c1]*fx
			d := add[c0]*(1-fx) + add[c1]*fx
			value[at+px] = clamp01(value[at+px]*m + d)
			tone[at+px] = clamp01(tone[at+px]*m + d)
		}
	}
}

// towardsWhite returns the c for which a region moved to 1 - (1-x)*c, in
// value and tone, has a light of target, given its moments m. Moved by c,
// the light of half blocks is the mean of (1-ca)², and that of dots the mean
// of (1-ca)²(1-cb). Both fall from 1 at c = 0 as c rises, so c is found by
// halving the interval it lies in.
func towardsWhite(m moments, half bool, target float64) float64 {
	light := func(c float64) float64 {
		sq := 1 - 2*c*m[0] + c*c*m[1]
		if half {
			return sq
		}
		return sq - c*(m[2]-2*c*m[3]+c*c*m[4])
	}
	lo, hi := 0.0, 1.0
	if light(hi) >= target {
		return 1
	}
	for range 24 {
		mid := (lo + hi) / 2
		if light(mid) > target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}
