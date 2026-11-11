package sky

import (
	"math"
	"math/rand/v2"
)

// maxPixels caps the size of the field a scene draws into. The renderers
// sample it, so a large terminal costs no more than this.
const maxPixels = 36000

// frame is what a component needs to draw one frame.
type frame struct {
	// fw and fh are the field's size in pixels, and aspect is the height of a
	// pixel divided by its width.
	fw, fh int
	aspect float64
	// cx, cy and radius place the unit circle in the middle of the field.
	cx, cy, radius float64
	dt             float64
	sig            Signals
	// history is the level over the last few seconds, oldest first.
	history []float64
	rng     *rand.Rand
	// xs and ys are every pixel's coordinates after the scene's warps.
	xs, ys []float64
}

// warp works out xs and ys for the given warps and their values. A pixel's
// coordinates are in the field's own units, where the unit circle fills the
// shorter side.
func (f *frame) warp(warps []warp, values [][4]float64) {
	n := f.fw * f.fh
	if len(f.xs) != n {
		f.xs, f.ys = make([]float64, n), make([]float64, n)
	}
	for py := range f.fh {
		for px := range f.fw {
			x := (float64(px) - f.cx) / f.radius
			y := (float64(py) - f.cy) * f.aspect / f.radius
			for i, w := range warps {
				x, y = w.apply(x, y, values[i])
			}
			f.xs[py*f.fw+px], f.ys[py*f.fw+px] = x, y
		}
	}
}

// fill sets every pixel of buf from fn, given the warped coordinates.
func (f *frame) fill(buf []float64, fn func(x, y float64) float64) {
	for i := range buf {
		buf[i] = fn(f.xs[i], f.ys[i])
	}
}

// param is one modulated number. Its value is base plus each source's signal
// times its depth, clamped to 0 and 1.
type param struct {
	base  float64
	src   [2]int
	depth [2]float64
}

func (p param) value(s Signals, clock float64) float64 {
	return clamp01(p.base + p.depth[0]*s.source(p.src[0], clock) + p.depth[1]*s.source(p.src[1], clock))
}

// randomParam draws a param with one or two sources. Depths are larger for
// the sources that spend most of their time near zero.
func randomParam(rng *rand.Rand) param {
	p := param{base: 0.15 + 0.6*rng.Float64(), src: [2]int{rng.IntN(sources), rng.IntN(sources)}}
	for i := range 2 {
		d := 0.2 + 0.5*rng.Float64()
		if i == 1 && rng.IntN(2) == 0 {
			d = 0
		}
		if rng.IntN(4) == 0 {
			d = -d
		}
		p.depth[i] = d
	}
	return p
}

// warpKind is a way of bending the coordinates before a scene's layers see
// them.
type warpKind int

const (
	warpRotate warpKind = iota
	warpZoom
	warpKaleido
	warpTwist
	warpRipple
	warpMirror
	warpPolar
	warpKinds
)

// warp bends coordinates. n is a whole-number setting fixed when the scene is
// made, such as the number of folds of a kaleidoscope.
type warp struct {
	kind  warpKind
	n     int
	p     [2]param
	clock float64
}

// apply bends (x, y) given the warp's two parameter values and its clock,
// which is stored in v[2].
func (w warp) apply(x, y float64, v [4]float64) (float64, float64) {
	switch w.kind {
	case warpRotate:
		return rotate(x, y, v[2])
	case warpZoom:
		z := 0.6 + 1.2*v[0]
		return x / z, y / z
	case warpKaleido:
		r, a := math.Hypot(x, y), math.Atan2(y, x)+v[2]*0.3
		seg := 2 * math.Pi / float64(w.n)
		a = math.Mod(math.Abs(a), seg)
		if a > seg/2 {
			a = seg - a
		}
		return r * math.Cos(a), r * math.Sin(a)
	case warpTwist:
		r := math.Hypot(x, y)
		return rotate(x, y, (v[0]-0.5)*4*r)
	case warpRipple:
		r := math.Hypot(x, y)
		k := 1 + 0.25*v[0]*math.Sin(r*(4+10*v[1])-v[2]*3)
		return x * k, y * k
	case warpMirror:
		if w.n == 0 {
			return math.Abs(x), y
		}
		return math.Abs(x), math.Abs(y)
	case warpPolar:
		return math.Atan2(y, x) / math.Pi, math.Hypot(x, y)*2 - 1
	}
	return x, y
}

func rotate(x, y, a float64) (float64, float64) {
	s, c := math.Sincos(a)
	return x*c - y*s, x*s + y*c
}

// blend is how a layer is combined with the layers under it.
type blend int

const (
	blendAdd blend = iota
	blendMax
	blendScreen
	blendDifference
	blends
)

func (b blend) mix(under, over float64) float64 {
	switch b {
	case blendMax:
		return math.Max(under, over)
	case blendScreen:
		return 1 - (1-under)*(1-over)
	case blendDifference:
		return math.Abs(under - over)
	}
	return math.Min(under+over, 1)
}

// glide moves v towards target as a smoothing filter with time constant tau
// seconds would over dt seconds.
func glide(v, target, dt, tau float64) float64 {
	return v + (target-v)*(1-math.Exp(-dt/tau))
}

// bilinear samples a w by h picture at (sx, sy) in cells, blending the four
// cells around it. Outside the picture it is 0.
func bilinear(v []float64, w, h int, sx, sy float64) float64 {
	if sx < 0 || sy < 0 || sx > float64(w-1) || sy > float64(h-1) {
		return 0
	}
	x0, y0 := int(sx), int(sy)
	x1, y1 := min(x0+1, w-1), min(y0+1, h-1)
	fx, fy := sx-float64(x0), sy-float64(y0)
	top := v[y0*w+x0]*(1-fx) + v[y0*w+x1]*fx
	bot := v[y1*w+x0]*(1-fx) + v[y1*w+x1]*fx
	return top*(1-fy) + bot*fy
}

// smooth is 0 below a, 1 above b, and a smooth step between.
func smooth(a, b, v float64) float64 {
	t := clamp01((v - a) / (b - a))
	return t * t * (3 - 2*t)
}

// hash returns a number from 0 to 1 that depends only on its inputs.
func hash(a, b int) float64 {
	h := uint32(a)*73856093 ^ uint32(b)*19349663
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return float64(h%10007) / 10007
}
