package sky

import "math"

// toLinear and fromLinear convert one channel of a colour between its stored
// form and linear light, from 0 to 1.
func toLinear(u uint8) float64 {
	v := float64(u) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func fromLinear(v float64) uint8 {
	v = clamp01(v)
	if v <= 0.0031308 {
		return uint8(255*12.92*v + 0.5)
	}
	return uint8(255*(1.055*math.Pow(v, 1/2.4)-0.055) + 0.5)
}

// toOklab returns the lightness and the two colour coordinates, in the Oklab
// space, of a colour given in linear light. Equal distances in that space
// look about equally different.
func toOklab(r, g, b float64) (l, a, bb float64) {
	x := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	y := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	z := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return 0.2104542553*x + 0.7936177850*y - 0.0040720468*z,
		1.9779984951*x - 2.4285922050*y + 0.4505937099*z,
		0.0259040371*x + 0.7827717662*y - 0.8086757660*z
}

// fromOklab returns the colour, in linear light, of a lightness and two
// colour coordinates in the Oklab space. A channel outside 0 to 1 means the
// colour cannot be shown.
func fromOklab(l, a, b float64) (r, g, bb float64) {
	x := l + 0.3963377774*a + 0.2158037573*b
	y := l - 0.1055613458*a - 0.0638541728*b
	z := l - 0.0894841775*a - 1.2914855480*b
	x, y, z = x*x*x, y*y*y, z*z*z
	return 4.0767416621*x - 3.3077115913*y + 0.2309699292*z,
		-1.2684380046*x + 2.6097574011*y - 0.3413193965*z,
		-0.0041960863*x - 0.7034186147*y + 1.7076147010*z
}

// lch is a colour by its Oklab lightness, from 0 to 1, its chroma, which is
// its distance from grey, and its hue in degrees.
type lch struct{ l, c, h float64 }

// lchOf measures a colour.
func lchOf(col RGB) lch {
	l, a, b := toOklab(toLinear(col.R), toLinear(col.G), toLinear(col.B))
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return lch{l, math.Hypot(a, b), h}
}

// shown returns the colour, and reports whether it can be shown.
func (c lch) shown() (RGB, bool) {
	sn, cs := math.Sincos(c.h * math.Pi / 180)
	r, g, b := fromOklab(c.l, c.c*cs, c.c*sn)
	const slack = 1e-4
	ok := r >= -slack && g >= -slack && b >= -slack && r <= 1+slack && g <= 1+slack && b <= 1+slack
	return RGB{fromLinear(r), fromLinear(g), fromLinear(b)}, ok
}

// most returns the largest chroma that can be shown at the colour's
// lightness and hue, up to 0.4.
func (c lch) most() float64 {
	lo, hi := 0.0, 0.4
	for range 14 {
		mid := (lo + hi) / 2
		if _, ok := (lch{c.l, mid, c.h}).shown(); ok {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// The measures of a made palette. Hues are in degrees.
const (
	// A made palette's hues run from teal through blue to violet: no step
	// has a hue below bodyFrom or above bodyTo.
	bodyFrom, bodyTo = 183.0, 308.0
	// The key hue of a made palette is held between keyFrom and keyTo.
	keyFrom, keyTo = 186.0, 300.0
	// Towards the pinks, from plumFrom up, a colour is held to less chroma,
	// down to plumMost at bodyTo.
	plumFrom, plumMost = 290.0, 0.085
	// chromaMost is the most chroma any colour of a made palette has, and
	// gamutShare the most as a share of what can be shown at the colour's
	// lightness and hue.
	chromaMost = 0.15
	gamutShare = 0.7
	// greyPeak is the peak chroma of a palette of greys, and greyKey its
	// hue: a slight cool tint.
	greyPeak, greyKey = 0.012, 250.0
)

// ceiling is the most chroma a colour of a made palette may have at the
// given lightness and hue.
func ceiling(l, h float64) float64 {
	top := chromaMost
	if h > plumFrom {
		top = plumMost + (chromaMost-plumMost)*clamp01((bodyTo-h)/(bodyTo-plumFrom))
	}
	return math.Min(top, gamutShare*(lch{l, 0, h}).most())
}

// turnTo returns the angle to add to the hue a to reach the hue b the short
// way round, from -180 to 180 degrees.
func turnTo(a, b float64) float64 {
	return math.Mod(b-a+540, 360) - 180
}

// tones is a palette as what it is made from. Its ramp runs from dark to
// bright.
//
// A made palette is a few numbers. Lightness rises along its ramp; the hue
// moves along it by arc degrees, through key at its middle; and chroma is
// low at the dark end, reaches peak in the middle and falls towards a
// near-white top.
//
// A palette that is not made holds its colours in fixed.
type tones struct {
	key, arc float64
	peak     float64
	// contrast is the power the lightness rises by: below 1 the middle of
	// the ramp is brighter, above 1 darker.
	contrast float64
	// The top accentSteps steps turn accentTurn degrees beyond the bright
	// end of the arc, at a chroma of accent.
	accentSteps        int
	accentTurn, accent float64
	fixed              []RGB
	// stops are the colours of a landscape, dark to bright, and land how far
	// the ramp is made of them.
	stops []RGB
	land  float64
}

// made reports whether the palette is made from numbers.
func (t tones) made() bool { return t.fixed == nil }

// The lightness of the darkest and the brightest step of a made ramp, the
// chroma of its ends as a share of its peak, and the hue its dark end leans
// towards.
const (
	rampDark, rampBright = 0.2, 0.97
	rampEnds             = 0.3
	coolHue              = 265.0
)

// leaning returns arc with the sign that takes the dark end of a ramp
// through key towards the cool hue, by the shorter way.
func leaning(key, arc float64) float64 {
	arc = math.Abs(arc)
	if turnTo(key, coolHue) > 0 {
		return -arc
	}
	return arc
}

// step returns the colour at a place along a made ramp, from 0 at its dark
// end to 1 at its bright end. top is set for a step that carries the accent.
func (t tones) step(at float64, top bool) lch {
	c := lch{l: rampDark + (rampBright-rampDark)*math.Pow(at, t.contrast), h: t.key + t.arc*(at-0.5)}
	bell := rampEnds + (1-rampEnds)*math.Sin(math.Pi*math.Pow(at, 0.85))
	c.c = t.peak * bell * (1 - 0.8*math.Pow(at, 6))
	// An accent that would leave the hues a made palette may have is left
	// out.
	if h := t.key + t.arc/2 + math.Copysign(t.accentTurn, t.arc); top && t.accentSteps > 0 && h >= bodyFrom && h <= bodyTo {
		c.h, c.c = h, t.accent
	}
	c.h = math.Min(math.Max(c.h, bodyFrom), bodyTo)
	c.c = math.Min(c.c, ceiling(c.l, c.h))
	return c
}

// ramp returns the palette: paletteSize colours from dark to bright. As far
// as the tones are a landscape's, each step is the landscape's own colour
// with landTint of the palette's colour mixed in.
func (t tones) ramp() []RGB {
	p := t.fixed
	if p == nil {
		p = make([]RGB, paletteSize)
		for i := range p {
			p[i], _ = t.step(float64(i)/float64(paletteSize-1), i >= paletteSize-t.accentSteps).shown()
		}
	}
	if t.land <= 0 || len(t.stops) < 2 {
		return p
	}
	return blendPalettes(p, stopped(t.stops, p), clamp01(t.land))
}

// stopped returns a landscape's colours spread evenly from the first step
// of a palette to the last, with landTint of tint mixed into each step.
func stopped(colours, tint []RGB) []RGB {
	p := make([]RGB, paletteSize)
	stops := len(colours) - 1
	for i := range p {
		u := float64(i) / float64(paletteSize-1) * float64(stops)
		k := min(int(u), stops-1)
		c := blendPalettes(colours[k:k+1], colours[k+1:k+2], u-float64(k))
		if i < len(tint) {
			c = blendPalettes(c, tint[i:i+1], landTint)
		}
		p[i] = c[0]
	}
	return p
}

// mixTones returns the made tones w of the way from a to b, both of them
// made. The key hue goes the short way round, which for two made palettes
// stays between teal and violet, and every other number goes straight.
func mixTones(a, b tones, w float64) tones {
	w = clamp01(w)
	mix := func(x, y float64) float64 { return x + (y-x)*w }
	out := tones{
		key:      a.key + turnTo(a.key, b.key)*w,
		arc:      mix(a.arc, b.arc),
		peak:     mix(a.peak, b.peak),
		contrast: mix(a.contrast, b.contrast),
		land:     mix(a.land, b.land),
	}
	// Tones without an accent count as having one that is not turned and
	// has the chroma the top of their ramp has.
	accent := func(t tones) (turn, chroma float64) {
		if t.accentSteps > 0 {
			return t.accentTurn, t.accent
		}
		return 0, t.peak * rampEnds * 0.2
	}
	turnA, chromaA := accent(a)
	turnB, chromaB := accent(b)
	out.accentSteps, out.accentTurn, out.accent = max(a.accentSteps, b.accentSteps), mix(turnA, turnB), mix(chromaA, chromaB)
	near, far := a, b
	if w >= 0.5 {
		near, far = b, a
	}
	switch {
	case len(a.stops) == len(b.stops) && len(a.stops) > 0:
		out.stops = blendPalettes(a.stops, b.stops, w)
	case len(near.stops) > 0:
		out.stops = near.stops
	default:
		out.stops = far.stops
	}
	return out
}

// turned returns made tones with their key hue turned by the given degrees,
// held between teal and violet, and no longer a landscape's.
func (t tones) turned(by float64) tones {
	t.key = math.Min(math.Max(t.key+by, keyFrom), keyTo)
	t.arc = leaning(t.key, t.arc)
	t.land = 0
	return t
}
