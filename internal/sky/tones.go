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

// family is the hues the palettes of one kind of music may have, in degrees.
type family struct {
	// No step of a palette has a hue below from or above to, its key hue is
	// held between keyFrom and keyTo, and an accent has no hue below
	// accentFrom.
	from, to       float64
	keyFrom, keyTo float64
	accentFrom     float64
	// dark is the hue the dark end of a ramp leans towards.
	dark float64
	// The key hues of the two members of a palette are from mateLeast to
	// mateMost apart.
	mateLeast, mateMost float64
	// gamut is the most chroma a colour may have as a share of what can be
	// shown at its lightness and hue.
	gamut float64
	// crest sets where along a ramp its chroma peaks: half the way raised
	// to the power of one over crest.
	crest float64
}

// The families. None reaches the greens, olives and yellow-greens between
// gold.to and cool.from, or the pinks and magentas between cool.to and
// ember.from.
var (
	// cool runs from teal through blue to violet, its darks leaning to
	// blue-violet. Its accents keep clear of the green-blues.
	cool = family{from: 183, to: 308, keyFrom: 186, keyTo: 300, accentFrom: 200, dark: 265, mateLeast: 57, mateMost: 110, gamut: 0.7, crest: 0.85}
	// ember runs from ember red through orange to amber, its darks leaning
	// to red and its lights to a hot yellow.
	ember = family{from: 25, to: 92, keyFrom: 36, keyTo: 66, accentFrom: 25, dark: 20, mateLeast: 15, mateMost: 30, gamut: 0.95, crest: 1.6}
	// gold runs from a warm brown through amber to gold. Its key hues are
	// too close for two members to differ by hue, so its second member is
	// told apart by its chroma.
	gold = family{from: 40, to: 94, keyFrom: 60, keyTo: 72, accentFrom: 40, dark: 40, mateLeast: 6, mateMost: 12, gamut: 0.9, crest: 1.2}
)

// The measures of a made palette. Hues are in degrees.
const (
	// A cell is drawn in one of setLevels palettes from the first member of
	// a palette to the second.
	setLevels = 7
	// Towards the pinks, from plumFrom up, a cool colour is held to less
	// chroma, down to plumMost at the end of the family.
	plumFrom, plumMost = 290.0, 0.085
	// chromaMost is the most chroma any colour of a made palette has.
	chromaMost = 0.15
	// greyPeak is the peak chroma of a palette of greys, and greyKey its
	// hue: a slight cool tint.
	greyPeak, greyKey = 0.012, 250.0
)

// ceiling is the most chroma a colour of a palette of the family may have
// at the given lightness and hue.
func (f family) ceiling(l, h float64) float64 {
	top := chromaMost
	if h > plumFrom {
		top = plumMost + (chromaMost-plumMost)*clamp01((cool.to-h)/(cool.to-plumFrom))
	}
	return math.Min(top, f.gamut*(lch{l, 0, h}).most())
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
// The zero tones are not a made palette.
type tones struct {
	fam      family
	key, arc float64
	peak     float64
	// contrast is the power the lightness rises by: below 1 the middle of
	// the ramp is brighter, above 1 darker. light is the lightness the rise
	// reaches at the bright end, and top the lightness of the brightest
	// step, which the last few steps rise to.
	contrast   float64
	light, top float64
	// dark is the lightness of the darkest step as a share of light.
	dark float64
	// The top accentSteps steps turn accentTurn degrees beyond the bright
	// end of the arc, at a chroma of accent.
	accentSteps        int
	accentTurn, accent float64
	// A made palette may have a second member, drawn in other places on the
	// screen. In a palette of one family, mate is how far the second
	// member's key hue is from key, in degrees, and a mate of 0 is a palette
	// of one member. A second member borrowed from another family has that
	// family in guest, and mate is its key hue. matePeak is the second
	// member's peak chroma either way.
	mate, matePeak float64
	guest          family
	// stops are the colours of a landscape, dark to bright, and land how far
	// the ramp is made of them.
	stops []RGB
	land  float64
}

// made reports whether the palette is made from numbers.
func (t tones) made() bool { return t.light > 0 }

// rampEnds is the chroma of the ends of a made ramp as a share of its peak.
const rampEnds = 0.3

// leaning returns arc with the sign that takes the dark end of a ramp
// through key towards the hue its family's darks lean to, by the shorter
// way.
func (f family) leaning(key, arc float64) float64 {
	arc = math.Abs(arc)
	if turnTo(key, f.dark) > 0 {
		return -arc
	}
	return arc
}

// step returns the colour at a place along a made ramp, from 0 at its dark
// end to 1 at its bright end. top is set for a step that carries the accent.
func (t tones) step(at float64, top bool) lch {
	c := lch{l: t.light*(t.dark+(1-t.dark)*math.Pow(at, t.contrast)) + (t.top-t.light)*math.Pow(at, 6),
		h: t.key + t.arc*(at-0.5)}
	bell := rampEnds + (1-rampEnds)*math.Sin(math.Pi*math.Pow(at, t.fam.crest))
	c.c = t.peak * bell * (1 - 0.8*math.Pow(at, 6))
	// An accent that would leave the hues an accent may have is left out.
	if h := t.key + t.arc/2 + math.Copysign(t.accentTurn, t.arc); top && t.accentSteps > 0 && h >= t.fam.accentFrom && h <= t.fam.to {
		c.h, c.c = h, t.accent
	}
	c.h = math.Min(math.Max(c.h, t.fam.from), t.fam.to)
	// The lightness so far is that of the grey the step is as bright as.
	// The step is given the lightness at which its own colour is as bright
	// as that grey, which for a strong colour is a little less.
	want := c.l * c.l * c.l
	for range 3 {
		c.c = math.Min(c.c, t.fam.ceiling(c.l, c.h))
		col, _ := c.shown()
		if y := 0.2126*toLinear(col.R) + 0.7152*toLinear(col.G) + 0.0722*toLinear(col.B); y > 0 {
			c.l = clamp01(c.l * math.Cbrt(want/y))
		}
	}
	c.c = math.Min(c.c, t.fam.ceiling(c.l, c.h))
	return c
}

// ramp returns the palette: paletteSize colours from dark to bright. As far
// as the tones are a landscape's, each step is the landscape's own colour
// with landTint of the palette's colour mixed in.
func (t tones) ramp() []RGB {
	p := make([]RGB, paletteSize)
	for i := range p {
		p[i], _ = t.step(float64(i)/float64(paletteSize-1), i >= paletteSize-t.accentSteps).shown()
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
// made. Within one family the key hue goes the short way round, which stays
// inside the family, and every other number goes straight. Between two
// families the hues do not move: the first half of the way the colours are
// a's, fading to grey, and the second half they are b's, rising from it.
// Two palettes of one family whose second members are borrowed from
// different families, or only one of them borrowed, move their first member
// as within a family and their second by way of grey. Lightness, contrast
// and a landscape's share go straight either way.
func mixTones(a, b tones, w float64) tones {
	w = clamp01(w)
	mix := func(x, y float64) float64 { return x + (y-x)*w }
	if a.fam != b.fam || a.guest != b.guest {
		out, left := a, 1-2*w
		if w >= 0.5 {
			out, left = b, 2*w-1
		}
		// Two palettes of one family that differ only in the family their
		// second member is borrowed from keep the hues of their first.
		if a.fam == b.fam {
			out.key, out.arc, out.peak = a.key+turnTo(a.key, b.key)*w, mix(a.arc, b.arc), mix(a.peak, b.peak)
			left = math.Abs(1 - 2*w)
			out.matePeak = out.matePeak * left
			out.contrast, out.light, out.top = mix(a.contrast, b.contrast), mix(a.light, b.light), mix(a.top, b.top)
			out.dark, out.land = mix(a.dark, b.dark), mix(a.land, b.land)
			return out
		}
		out.peak, out.matePeak, out.accent = out.peak*left, out.matePeak*left, out.accent*left
		out.contrast, out.light, out.top = mix(a.contrast, b.contrast), mix(a.light, b.light), mix(a.top, b.top)
		out.dark = mix(a.dark, b.dark)
		out.land = mix(a.land, b.land)
		if len(out.stops) == 0 {
			out.stops = append(a.stops, b.stops...)
		}
		return out
	}
	out := tones{
		fam:      a.fam,
		guest:    a.guest,
		key:      a.key + turnTo(a.key, b.key)*w,
		arc:      mix(a.arc, b.arc),
		peak:     mix(a.peak, b.peak),
		contrast: mix(a.contrast, b.contrast),
		mate:     mix(a.mate, b.mate),
		matePeak: mix(a.matePeak, b.matePeak),
		light:    mix(a.light, b.light),
		top:      mix(a.top, b.top),
		dark:     mix(a.dark, b.dark),
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
// held inside their family, and no longer a landscape's.
func (t tones) turned(by float64) tones {
	t.key = math.Min(math.Max(t.key+by, t.fam.keyFrom), t.fam.keyTo)
	t.arc = t.fam.leaning(t.key, t.arc)
	if !t.borrows() {
		t.mate = t.fam.mated(t.key, t.mate)
	}
	t.land = 0
	return t
}

// borrows reports whether the palette's second member is of another family.
func (t tones) borrows() bool { return t.guest != family{} }

// mated returns how far a second member's key hue is from key, given how
// far it was meant to be: the same distance, on the other side where that
// keeps it between the family's key hues, and otherwise as far as it can
// go.
func (f family) mated(key, mate float64) float64 {
	if mate == 0 {
		return 0
	}
	for _, m := range []float64{mate, -mate} {
		if key+m >= f.keyFrom && key+m <= f.keyTo {
			return m
		}
	}
	if key-f.keyFrom > f.keyTo-key {
		return f.keyFrom - key
	}
	return f.keyTo - key
}

// member returns the palette's first member, or its second, as a palette of
// one member.
func (t tones) member(second bool) tones {
	one := t
	one.mate, one.matePeak = 0, 0
	one.guest = family{}
	switch {
	case second && t.borrows():
		one.fam, one.key, one.peak, one.accentSteps = t.guest, t.mate, t.matePeak, 0
		one.arc = one.fam.leaning(one.key, t.arc)
	case second && t.mate != 0:
		one.key, one.peak, one.accentSteps = t.key+t.mate, t.matePeak, 0
		one.arc = t.fam.leaning(one.key, t.arc)
	}
	return one
}

// table returns the colours cells are drawn in. A made palette of two
// members gives setLevels ramps one after another, from the first member's
// to the second's by way of the palettes between them. Between two members
// of one family those are the palettes of the hues between. Between a member
// and one borrowed from another family no hue is passed through: the first
// member fades to grey over the first half of the ramps and the second rises
// from it over the rest. Any other palette gives its one ramp.
func (t tones) table() []RGB {
	if !t.made() || t.mate == 0 {
		return t.ramp()
	}
	out := make([]RGB, 0, setLevels*paletteSize)
	first, second := t.member(false), t.member(true)
	for l := range setLevels {
		out = append(out, mixTones(first, second, float64(l)/(setLevels-1)).ramp()...)
	}
	return out
}
