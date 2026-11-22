package sky

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestTheColourConversionAgreesWithKnownValuesAndComesBack(t *testing.T) {
	for _, c := range []struct {
		name    string
		rgb     [3]float64
		l, a, b float64
	}{
		{"white", [3]float64{1, 1, 1}, 1, 0, 0},
		{"red", [3]float64{1, 0, 0}, 0.62796, 0.22486, 0.12585},
		{"green", [3]float64{0, 1, 0}, 0.86644, -0.23389, 0.17950},
		{"blue", [3]float64{0, 0, 1}, 0.45201, -0.03246, -0.31153},
	} {
		l, a, b := toOklab(c.rgb[0], c.rgb[1], c.rgb[2])
		if math.Abs(l-c.l) > 1e-4 || math.Abs(a-c.a) > 1e-4 || math.Abs(b-c.b) > 1e-4 {
			t.Errorf("%s is (%.5f, %.5f, %.5f) in Oklab, want (%.5f, %.5f, %.5f)", c.name, l, a, b, c.l, c.a, c.b)
		}
		r, g, bb := fromOklab(l, a, b)
		if math.Abs(r-c.rgb[0]) > 1e-6 || math.Abs(g-c.rgb[1]) > 1e-6 || math.Abs(bb-c.rgb[2]) > 1e-6 {
			t.Errorf("%s came back from Oklab as (%.6f, %.6f, %.6f)", c.name, r, g, bb)
		}
	}
	for u := range 256 {
		if back := fromLinear(toLinear(uint8(u))); int(back) != u {
			t.Fatalf("channel %d came back from linear light as %d", u, back)
		}
	}
}

// The rules a made palette keeps, as numbers. Hues are in degrees and
// chroma and distances are Oklab's.
const (
	// A step counts as coloured from ruleColoured chroma: below that its hue
	// is not held to anything.
	ruleColoured = 0.03
	// The hues of a palette's coloured steps, the accent apart, lie within
	// ruleSpan of one another, and with the accent within ruleReach. None is
	// below ruleFrom, which keeps out green, olive and yellow-green, or
	// above ruleTo, which keeps out pink and magenta.
	ruleSpan, ruleReach = 62.0, 95.0
	ruleFrom, ruleTo    = 180.0, 312.0
	// No step has more chroma than ruleChroma, and none from rulePlum
	// degrees up has more than rulePlumChroma.
	ruleChroma     = 0.155
	rulePlum       = 300.0
	rulePlumChroma = 0.12
	// Neighbouring steps are no further apart than ruleApart, and each is
	// lighter than the one before by at least ruleRise. The darkest step is
	// left out: no cell is drawn in it.
	ruleApart = 0.08
	// A step of the accent may be ruleAccentApart from the step before.
	ruleAccentApart = 0.10
	ruleRise        = 0.004
	// At most ruleAccent steps, the brightest, lie beyond the arc.
	ruleAccent = 2
)

// broken returns what rules a made palette breaks, or nothing.
func broken(t tones) []string {
	var out []string
	say := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	var hues, body []float64
	p := t.ramp()
	last := lch{}
	for i, col := range p {
		c := t.step(float64(i)/float64(paletteSize-1), i >= paletteSize-t.accentSteps)
		if _, ok := c.shown(); !ok {
			say("step %d cannot be shown: %+v", i, c)
		}
		got := lchOf(col)
		if i > 0 {
			if got.l-last.l < ruleRise {
				say("step %d is %.3f lighter than the step before", i, got.l-last.l)
			}
			sn, cs := math.Sincos(got.h * math.Pi / 180)
			sn0, cs0 := math.Sincos(last.h * math.Pi / 180)
			if d := math.Sqrt(math.Pow(got.l-last.l, 2) + math.Pow(got.c*cs-last.c*cs0, 2) + math.Pow(got.c*sn-last.c*sn0, 2)); i > 1 && d > ruleApart && (i < paletteSize-ruleAccent || d > ruleAccentApart) {
				say("step %d is %.3f from the step before", i, d)
			}
		}
		last = got
		if got.c > ruleChroma || (got.h >= rulePlum && got.c > rulePlumChroma) {
			say("step %d has a chroma of %.3f at hue %.0f", i, got.c, got.h)
		}
		if got.c < ruleColoured {
			continue
		}
		if got.h < ruleFrom || got.h > ruleTo {
			say("step %d has the hue %.0f at a chroma of %.3f", i, got.h, got.c)
		}
		hues = append(hues, got.h)
		if i >= paletteSize-ruleAccent {
			continue
		}
		body = append(body, got.h)
		// A colour is stored in whole steps of red, green and blue, which
		// moves the hue of a faint one by a few degrees.
		lo, hi := t.key-math.Abs(t.arc)/2-6, t.key+math.Abs(t.arc)/2+6
		if got.h < math.Min(lo, bodyFrom) || got.h > math.Max(hi, bodyTo) {
			say("step %d, not among the top %d, has the hue %.0f outside the arc from %.0f to %.0f", i, ruleAccent, got.h, lo, hi)
		}
	}
	if len(body) > 0 {
		if span := slices.Max(body) - slices.Min(body); span > ruleSpan {
			say("the hues of the body span %.0f degrees", span)
		}
		if span := slices.Max(hues) - slices.Min(hues); span > ruleReach {
			say("the hues span %.0f degrees with the accent", span)
		}
	}
	return out
}

// madeFor draws the tones of a character from a seed.
func madeFor(c Character, seed uint64) tones {
	return styleFor(c).tones(rand.New(rand.NewPCG(seed, 9)), c)
}

func TestEveryMadePaletteKeepsTheRulesAndSoDoesEveryTurnAndEveryMorph(t *testing.T) {
	seeds := uint64(400)
	if testing.Short() {
		seeds = 60
	}
	characters := []Character{{Calm: 1}, {Drive: 1}, {Calm: 0.5, Drive: 0.5}, {Drive: 0.7, Flow: 0.3}, {Calm: 0.6, Grit: 0.4}}
	checked, greys, driven := 0, 0, 0
	for _, c := range characters {
		for seed := range seeds {
			a := madeFor(c, seed)
			if !a.made() {
				continue
			}
			if c.Drive == 1 {
				driven++
				if a.peak <= greyPeak {
					greys++
				}
			}
			fail := func(what string, t2 tones) {
				if b := broken(t2); len(b) > 0 {
					t.Errorf("%+v seed %d, %s: %s\n  %+v", c, seed, what, strings.Join(b, "; "), t2)
				}
				checked++
			}
			fail("as drawn", a)
			rng := rand.New(rand.NewPCG(seed, 11))
			turned := a
			for n := range 6 {
				turned = turned.turned((rng.Float64() - 0.5) * 2 * turnMost)
				fail(fmt.Sprintf("after %d turns", n+1), turned)
				fail("half way to a turn", mixTones(a, turned, 0.5))
			}
			b := madeFor(characters[rng.IntN(len(characters))], seed+1000)
			if !b.made() {
				continue
			}
			for _, w := range []float64{0.2, 0.5, 0.8} {
				fail(fmt.Sprintf("%.1f of the way to another palette", w), mixTones(a, b, w))
			}
		}
	}
	if checked < 1000 && !testing.Short() {
		t.Errorf("only %d palettes were checked", checked)
	}
	if share := float64(greys) / float64(max(driven, 1)); math.Abs(share-greyShare) > 0.08 {
		t.Errorf("%.2f of the driving palettes were greys, want about %.2f", share, greyShare)
	}
}

func TestAPaletteOfGreysHasAlmostNoColour(t *testing.T) {
	found := false
	for seed := range uint64(40) {
		a := madeFor(Character{Drive: 1}, seed)
		if a.peak > greyPeak {
			continue
		}
		found = true
		for i, col := range a.ramp() {
			if c := lchOf(col).c; c > 0.02 {
				t.Errorf("seed %d: step %d of a palette of greys has a chroma of %.3f", seed, i, c)
			}
		}
	}
	if !found {
		t.Error("no seed in forty drew a palette of greys")
	}
}

// Grit and Flow palettes are not made from numbers and are held to none of
// the rules above. Grit's are reds, oranges and hot whites and Flow's are
// golds, ambers and soft greens, each a gradient round the colour wheel.
func TestGritAndFlowPalettesAreGradientsRoundTheWheel(t *testing.T) {
	for _, c := range []Character{{Grit: 1}, {Flow: 1}} {
		st := styleFor(c)
		for seed := range uint64(20) {
			got := madeFor(c, seed)
			if got.made() {
				t.Fatalf("%+v seed %d drew a made palette", c, seed)
			}
			rng := rand.New(rand.NewPCG(seed, 9))
			pick(rng, []float64{c.Calm, c.Drive, c.Grit, c.Flow})
			hue, wander := 0.05+0.2*rng.Float64(), 0.25
			if c.Grit == 1 {
				rng = rand.New(rand.NewPCG(seed, 9))
				pick(rng, []float64{c.Calm, c.Drive, c.Grit, c.Flow})
				hue, wander = 0.92+0.15*rng.Float64(), 0.15
			}
			if want := st.wheel(hue, wander); !slices.Equal(got.ramp(), want) {
				t.Errorf("%+v seed %d: the palette is %v, want the wheel's %v", c, seed, got.ramp(), want)
			}
		}
	}
}

func TestAMorphBetweenTwoMadePalettesTakesTheKeyHueTheShortWay(t *testing.T) {
	a := tones{key: 190, arc: 40, peak: 0.1, contrast: 1, light: 0.8, top: 0.9}
	b := tones{key: 300, arc: 40, peak: 0.1, contrast: 1, light: 0.8, top: 0.9}
	last := a.key
	for w := 0.0; w <= 1; w += 0.05 {
		k := mixTones(a, b, w).key
		if k < last-1e-9 || k > 300+1e-9 {
			t.Fatalf("at %.2f the key hue is %.1f after %.1f, want it to rise from 190 to 300", w, k, last)
		}
		last = k
	}
}

// hexRow writes a palette as its colours in hexadecimal, dark to bright.
func hexRow(p []RGB) string {
	var b strings.Builder
	for _, c := range p {
		fmt.Fprintf(&b, "#%02x%02x%02x ", c.R, c.G, c.B)
	}
	return strings.TrimSpace(b.String())
}
