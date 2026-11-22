package sky

import (
	"fmt"
	"math"
)

// linear returns a colour's channels in linear light, from 0 to 1.
func linear(c RGB) (r, g, b float64) { return toLinear(c.R), toLinear(c.G), toLinear(c.B) }

// shade is a colour as it is measured here: its Oklab lightness, chroma and
// hue in degrees.
type shade struct{ l, c, h float64 }

// shadeOf measures a colour.
func shadeOf(col RGB) shade {
	l, a, b := toOklab(linear(col))
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return shade{l, math.Hypot(a, b), h}
}

// apart is the angle between two hues, in degrees, from 0 to 180.
func apart(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// The hues measured here, in degrees of Oklab hue. Pink and magenta run
// from pinkSeenFrom round through 0 to pinkSeenTo; green from greenFrom to greenTo.
const (
	pinkSeenFrom, pinkSeenTo = 315.0, 15.0
	greenFrom, greenTo       = 105.0, 170.0
)

// isPink and isGreen report whether a hue is in the pink and magenta band
// or the green one.
func isPink(h float64) bool  { return h >= pinkSeenFrom || h <= pinkSeenTo }
func isGreen(h float64) bool { return h >= greenFrom && h <= greenTo }

// The chroma from which a colour counts as coloured, as strong and as
// vivid, and the share of the drawn screen from which a hue counts as
// present.
const (
	colouredFrom = 0.04
	strongFrom   = 0.10
	vividFrom    = 0.15
	presentFrom  = 0.10
)

// tinted is the colour of one cell of a picture on a black background.
type tinted struct {
	// ink is how much of the cell is drawn, and fg and bg the palette
	// indexes it is drawn in.
	ink    float64
	fg, bg int
	// pa and pb are the colour coordinates of the cell's palette colour
	// over its lightness, which no change of brightness moves, and sa and sb
	// the colour coordinates of the cell as it is shown.
	pa, pb, sa, sb float64
	// own holds the cell's palette colours as they are: one, or the two of
	// a half block. n is how many there are.
	own [2]shade
	n   int
}

// tintsOf is the colour of each cell of a picture w by h.
func tintsOf(p Picture, w, h int) []tinted {
	type entry struct {
		r, g, b, a, bb float64
		own            shade
	}
	pal := make([]entry, len(p.Palette))
	for i, c := range p.Palette {
		e := entry{own: shadeOf(c)}
		e.r, e.g, e.b = linear(c)
		if l, a, b := toOklab(e.r, e.g, e.b); l > 0 {
			e.a, e.bb = a/l, b/l
		}
		pal[i] = e
	}
	at := func(i int) (entry, bool) {
		if i < 0 || i >= len(pal) {
			return entry{}, false
		}
		return pal[i], true
	}
	out := make([]tinted, w*h)
	for r, line := range p.Lines {
		c := 0
		for _, run := range line {
			f, hasF := at(run.Fg)
			b, hasB := at(run.Bg)
			for _, ch := range run.Text {
				if r < h && c < w {
					t := tinted{fg: run.Fg, bg: run.Bg}
					switch {
					case ch == '▀' && (hasF || hasB):
						t.ink = 1
						t.pa, t.pb = (f.a+b.a)/2, (f.bb+b.bb)/2
						_, t.sa, t.sb = toOklab((f.r+b.r)/2, (f.g+b.g)/2, (f.b+b.b)/2)
						t.own, t.n = [2]shade{f.own, b.own}, 2
					case hasF:
						t.ink = ink(ch)
						t.pa, t.pb = f.a, f.bb
						_, t.sa, t.sb = toOklab(t.ink*f.r, t.ink*f.g, t.ink*f.b)
						t.own, t.n = [2]shade{f.own}, 1
					}
					if t.ink == 0 {
						t.fg, t.bg, t.n = None, None, 0
					}
					out[r*w+c] = t
				}
				c++
			}
		}
	}
	return out
}

// regionTint is the colour of each region of a picture split into cols by
// rows: the mean of its cells' palette colours, each weighed by its ink, so
// that how bright a region is does not move it.
func regionTint(cells []tinted, w, h, cols, rows int) [][2]float64 {
	sum, n := make([][2]float64, cols*rows), make([]float64, cols*rows)
	for r := range h {
		for c := range w {
			k := min(r*rows/h, rows-1)*cols + min(c*cols/w, cols-1)
			t := cells[r*w+c]
			sum[k][0] += t.ink * t.pa
			sum[k][1] += t.ink * t.pb
			n[k] += t.ink
		}
	}
	for k := range sum {
		if n[k] > 0 {
			sum[k][0] /= n[k]
			sum[k][1] /= n[k]
		}
	}
	return sum
}

// The least change in a region's colour that a turn of colour is counted
// from: tintTurn, and twice that. A colour here is its two Oklab colour
// coordinates over its Oklab lightness.
const tintTurn = 0.03

// tintMeter counts flashes of colour in the regions of one grid, at one
// threshold: a flash is a pair of opposing changes of at least the threshold
// in either of a region's colour coordinates.
type tintMeter struct {
	cols, rows int
	least      float64
	meters     []flashMeter
}

func newTintMeter(cols, rows int, least float64) *tintMeter {
	return &tintMeter{cols: cols, rows: rows, least: least, meters: make([]flashMeter, 2*cols*rows)}
}

// add takes the colour of each cell of a frame drawn at the time at, and
// returns how many turns of colour it brought.
func (m *tintMeter) add(cells []tinted, w, h int, at float64) int {
	turns := 0
	for k, c := range regionTint(cells, w, h, m.cols, m.rows) {
		for j := range 2 {
			meter := &m.meters[2*k+j]
			before := len(meter.turns)
			// A flash meter counts changes of 0.1.
			meter.add(c[j]*0.1/m.least, at)
			turns += len(meter.turns) - before
		}
	}
	return turns
}

func (m *tintMeter) String() string {
	secs := 0.0
	for i := range m.meters {
		if n := len(m.meters[i].turns); n > 0 {
			secs = math.Max(secs, m.meters[i].turns[n-1])
		}
	}
	worst, ten, all := flashes(m.meters, secs)
	return fmt.Sprintf("%dx%d at %.2f %.1f %.2f %.3f", m.cols, m.rows, m.least, worst, ten, all)
}

// hueBins is how many equal bands the hue circle is counted in.
const hueBins = 12

// hued is how the colours of one frame differ from the frame before, and
// what colours the frame has.
type hued struct {
	// tint is the mean distance each cell's colour as shown moved, with its
	// lightness left out. hue is the mean distance the palette colour of a
	// cell drawn in both frames moved, and steps the share of those cells
	// drawn in another palette index.
	tint, hue, steps float64
	// chroma is the mean Oklab chroma of the drawn cells' palette colours,
	// weighed by ink, and hues how far their hues are spread, from 0 for
	// one hue to 1 for hues that cancel out.
	chroma, hues float64
	// vivid is the share of the drawn screen at a chroma of vividFrom or
	// more, and pink the share that is pink or magenta at strongFrom or
	// more.
	vivid, pink float64
	// gap is the widest angle between two hues that are each present,
	// opposed whether two strong hues at least 150 degrees apart are both
	// present, and clash whether strong pink and strong green both are.
	gap            float64
	opposed, clash float64
	// families is 1 when the frame shows two families of colour: two hues
	// at least 60 degrees apart that are each present, or greys and a hue
	// that each are.
	families float64
	// roughLight and roughHue are the mean difference between a cell and
	// the cells to its right and below it: in light, and in palette colour
	// with lightness left out between cells that are both drawn.
	roughLight, roughHue float64
}

// huesOf measures the colours of a frame w cells wide, with the light of
// each cell, against those of the frame before.
func huesOf(now, was []tinted, light []float64, w int) hued {
	var out hued
	var both, ink, sumA, sumB, norm float64
	var area, strong [hueBins]float64
	var grey, coloured float64
	for i, t := range now {
		ink += t.ink
		sumA, sumB = sumA+t.ink*t.pa, sumB+t.ink*t.pb
		norm += t.ink * math.Hypot(t.pa, t.pb)
		for _, s := range t.own[:t.n] {
			share := t.ink / float64(t.n)
			out.chroma += share * s.c
			if s.c >= vividFrom {
				out.vivid += share
			}
			if s.c >= strongFrom && isPink(s.h) {
				out.pink += share
			}
			bin := int(s.h/360*hueBins) % hueBins
			if s.c >= colouredFrom {
				area[bin] += share
				coloured += share
			} else if s.l > 0.3 {
				// A dark step has little chroma whatever its palette.
				grey += share
			}
			if s.c >= strongFrom {
				strong[bin] += share
			}
		}
		if was == nil {
			continue
		}
		p := was[i]
		out.tint += math.Hypot(t.sa-p.sa, t.sb-p.sb) / float64(len(now))
		if t.ink > 0 && p.ink > 0 {
			both++
			out.hue += math.Hypot(t.pa-p.pa, t.pb-p.pb)
			if t.fg != p.fg || t.bg != p.bg {
				out.steps++
			}
		}
	}
	if both > 0 {
		out.hue, out.steps = out.hue/both, out.steps/both
	}
	if norm > 0 {
		out.hues = 1 - math.Hypot(sumA, sumB)/norm
	}
	if ink > 0 {
		out.chroma, out.vivid, out.pink = out.chroma/ink, out.vivid/ink, out.pink/ink
		step := 360.0 / hueBins
		var pinks, greens float64
		for a := range hueBins {
			mid := (float64(a) + 0.5) * step
			if isPink(mid) {
				pinks += strong[a] / ink
			}
			if isGreen(mid) {
				greens += strong[a] / ink
			}
			for b := a + 1; b < hueBins; b++ {
				d := apart(mid, (float64(b)+0.5)*step)
				if area[a]/ink >= presentFrom && area[b]/ink >= presentFrom {
					out.gap = math.Max(out.gap, d)
				}
				if d >= 150 && strong[a]/ink >= presentFrom && strong[b]/ink >= presentFrom {
					out.opposed = 1
				}
			}
		}
		if pinks >= presentFrom && greens >= presentFrom {
			out.clash = 1
		}
		if out.gap >= 60 || (grey/ink >= presentFrom && coloured/ink >= presentFrom) {
			out.families = 1
		}
	}
	pairs, hues := 0.0, 0.0
	for i, t := range now {
		for _, j := range []int{i + 1, i + w} {
			if j >= len(now) || (j == i+1 && j%w == 0) {
				continue
			}
			pairs++
			out.roughLight += math.Abs(light[i] - light[j])
			if t.ink > 0 && now[j].ink > 0 {
				hues++
				out.roughHue += math.Hypot(t.pa-now[j].pa, t.pb-now[j].pb)
			}
		}
	}
	if pairs > 0 {
		out.roughLight /= pairs
	}
	if hues > 0 {
		out.roughHue /= hues
	}
	return out
}
