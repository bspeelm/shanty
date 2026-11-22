package sky

import "math"

// The field is split into weaveCols by weaveRows regions, and a pixel is
// placed by how it stands among the pixels of its region.
const (
	weaveCols, weaveRows = 12, 8
	weaveRegions         = weaveCols * weaveRows
)

// The measures of the weave.
const (
	// A pixel weaveSpan spreads of its region's values below the region's
	// mean is drawn in the first member of the palette, and one as far
	// above it in the second.
	weaveSpan = 1.5
	// weaveLeast is the least spread a region's values count as having, so
	// that a region of nearly one value is one colour.
	weaveLeast = 0.05
	// From the darkest pixels of a region to the brightest the place goes
	// from the first member to the second and back weaveTurns times, so that
	// both are drawn in tones bright enough to show their colour.
	weaveTurns = 1.0
	// weaveTone is how far a pixel's tone moves it along that way, as a
	// share of once there and back for the whole range of tone. Layers of
	// different tones are so drawn in different colours where they cross.
	weaveTone = 0.25
	// weaveHome is how much of the way is given to the first member alone
	// in a palette whose second member is borrowed from another family, so
	// that the palette's own family covers most of the picture.
	weaveHome = 0.35
	// weaveSettle is how long, in seconds, a pixel takes to move to a new
	// place between the members, so that its colour does not turn back and
	// forth from one frame to the next.
	weaveSettle = 0.25
)

// weaver works out which member of the palette each pixel is drawn in, so
// that the colours of a set follow the forms of the picture. A pixel is
// placed by how far its value stands above or below the mean value of its
// region, measured in the spread of the region's values, with the mean and
// the spread blended between the middles of the regions: the darkest and the
// brightest of a form are drawn towards the first member, and what lies
// between towards the second. A region that brightens or dims as a whole on
// a beat, or whose contrast swells, therefore keeps its colours, and they
// move as the forms move.
type weaver struct {
	fw, fh int
	// xcell, ycell, xfrac and yfrac are as in flashGuard, and xreg is the
	// column of regions each column of pixels is in.
	xcell, ycell []int
	xfrac, yfrac []float64
	xreg         []int
}

// resize sets the weaver for a field fw by fh.
func (t *weaver) resize(fw, fh int) {
	t.fw, t.fh = fw, fh
	t.xcell, t.xfrac = spans(fw, weaveCols)
	t.ycell, t.yfrac = spans(fh, weaveRows)
	t.xreg = make([]int, fw)
	for px := range fw {
		t.xreg[px] = px * weaveCols / fw
	}
}

// settle moves place, from 0 for the first member to 1 for the second,
// towards where a frame's values and tones put each pixel, dt seconds after
// the last frame. home is how much of the way is given to the first member
// alone. When snap is set every pixel is put in its place at once.
func (t *weaver) settle(value, tone, place []float64, home, dt float64, snap bool) {
	if len(value) != t.fw*t.fh || len(place) != len(value) || len(tone) != len(value) || t.fw == 0 {
		return
	}
	k := 1.0
	if !snap {
		k = 1 - math.Exp(-dt/weaveSettle)
	}
	var n, mean, spread [weaveRegions]float64
	for py := range t.fh {
		r := py * weaveRows / t.fh * weaveCols
		at := py * t.fw
		for px, c := range t.xreg {
			v := value[at+px]
			mean[r+c] += v
			spread[r+c] += v * v
			n[r+c]++
		}
	}
	for i := range mean {
		k := math.Max(n[i], 1)
		mean[i] /= k
		spread[i] = math.Max(math.Sqrt(math.Max(spread[i]/k-mean[i]*mean[i], 0)), weaveLeast)
	}
	var means, spreads [weaveCols]float64
	for py := range t.fh {
		r0, fy := t.ycell[py], t.yfrac[py]
		r1 := min(r0+1, weaveRows-1)
		for c := range weaveCols {
			means[c] = mean[r0*weaveCols+c]*(1-fy) + mean[r1*weaveCols+c]*fy
			spreads[c] = spread[r0*weaveCols+c]*(1-fy) + spread[r1*weaveCols+c]*fy
		}
		at := py * t.fw
		for px, c0 := range t.xcell {
			fx := t.xfrac[px]
			c1 := min(c0+1, weaveCols-1)
			m := means[c0]*(1-fx) + means[c1]*fx
			s := spreads[c0]*(1-fx) + spreads[c1]*fx
			along := clamp01(0.5+(value[at+px]-m)/(2*weaveSpan*s))*weaveTurns + weaveTone*(clamp01(tone[at+px])-0.4)
			there := 0.5 - 0.5*math.Cos(2*math.Pi*along)
			place[at+px] += (clamp01((there-home)/(1-home)) - place[at+px]) * k
		}
	}
}
