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
	// more than ruleEdge outside its family, which allows for a colour being
	// stored in whole steps of red, green and blue.
	ruleSpan, ruleReach = 62.0, 95.0
	ruleEdge            = 4.0
	// No coloured step of any family is green, olive or yellow-green, from
	// ruleGreenFrom to ruleGreenTo, or pink or magenta, from rulePinkFrom
	// round through 0 to rulePinkTo.
	ruleGreenFrom, ruleGreenTo = 97.0, 179.0
	rulePinkFrom, rulePinkTo   = 313.0, 20.0
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
		if got.h < t.fam.from-ruleEdge || got.h > t.fam.to+ruleEdge {
			say("step %d has the hue %.0f at a chroma of %.3f, outside its family", i, got.h, got.c)
		}
		if (got.h > ruleGreenFrom && got.h < ruleGreenTo) || got.h > rulePinkFrom || got.h < rulePinkTo {
			say("step %d has the hue %.0f at a chroma of %.3f, a green or a pink", i, got.h, got.c)
		}
		hues = append(hues, got.h)
		if i >= paletteSize-ruleAccent {
			continue
		}
		body = append(body, got.h)
		// A colour is stored in whole steps of red, green and blue, which
		// moves the hue of a faint one by a few degrees.
		lo, hi := t.key-math.Abs(t.arc)/2-6, t.key+math.Abs(t.arc)/2+6
		if got.h < math.Min(lo, t.fam.from) || got.h > math.Max(hi, t.fam.to) {
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
	characters := []Character{{Calm: 1}, {Drive: 1}, {Grit: 1}, {Flow: 1}, {Calm: 0.5, Drive: 0.5}, {Drive: 0.7, Flow: 0.3}, {Calm: 0.6, Grit: 0.4}}
	checked, greys, driven := 0, 0, 0
	for _, c := range characters {
		for seed := range seeds {
			a := madeFor(c, seed)
			if c.Drive == 1 {
				driven++
				if a.peak <= greyPeak || a.matePeak <= greyPeak {
					greys++
				}
			}
			// Every palette between the first member and the second is held
			// to the rules, and so every colour a cell may be drawn in.
			fail := func(what string, t2 tones) {
				for l := range setLevels {
					level := mixTones(t2.member(false), t2.member(true), float64(l)/(setLevels-1))
					if b := broken(level); len(b) > 0 {
						t.Errorf("%+v seed %d, %s, level %d: %s\n  %+v", c, seed, what, l, strings.Join(b, "; "), t2)
					}
					checked++
				}
				if got := len(t2.table()); got != setLevels*paletteSize && t2.mate != 0 {
					t.Errorf("%+v seed %d, %s: the table has %d colours, want %d", c, seed, what, got, setLevels*paletteSize)
				}
			}
			// The members of a palette as drawn or turned are as far apart
			// as their family sets, never opposed, and both keyed inside it.
			// Greys have no hue to be apart by.
			apart := func(what string, t2 tones) {
				second := t2.key + t2.mate
				coloured := t2.peak > greyPeak && t2.matePeak > greyPeak
				f := t2.fam
				if d := math.Abs(t2.mate); coloured && (d < f.mateLeast-0.01 || d > f.mateMost+0.01) {
					t.Errorf("%+v seed %d, %s: the members are %.0f degrees apart, want %v to %v", c, seed, what, d, f.mateLeast, f.mateMost)
				}
				if coloured && (second < f.keyFrom-0.01 || second > f.keyTo+0.01) {
					t.Errorf("%+v seed %d, %s: the second member is keyed at %.0f", c, seed, what, second)
				}
				if t2.mate == 0 {
					t.Errorf("%+v seed %d, %s: the palette has one member", c, seed, what)
				}
			}
			apart("as drawn", a)
			fail("as drawn", a)
			rng := rand.New(rand.NewPCG(seed, 11))
			turned := a
			for n := range 6 {
				turned = turned.turned((rng.Float64() - 0.5) * 2 * turnMost)
				fail(fmt.Sprintf("after %d turns", n+1), turned)
				apart(fmt.Sprintf("after %d turns", n+1), turned)
				fail("half way to a turn", mixTones(a, turned, 0.5))
			}
			b := madeFor(characters[rng.IntN(len(characters))], seed+1000)
			for _, w := range []float64{0.2, 0.5, 0.8} {
				fail(fmt.Sprintf("%.1f of the way to another palette", w), mixTones(a, b, w))
			}
		}
	}
	if checked < 5000 && !testing.Short() {
		t.Errorf("only %d palettes were checked", checked)
	}
	if share := float64(greys) / float64(max(driven, 1)); math.Abs(share-greyShare) > 0.08 {
		t.Errorf("%.2f of the driving palettes had greys for one member, want about %.2f", share, greyShare)
	}
}

func TestAMemberOfGreysHasAlmostNoColour(t *testing.T) {
	found := false
	for seed := range uint64(40) {
		a := madeFor(Character{Drive: 1}, seed)
		if a.peak > greyPeak {
			continue
		}
		found = true
		for i, col := range a.member(false).ramp() {
			if c := lchOf(col).c; c > 0.02 {
				t.Errorf("seed %d: step %d of a member of greys has a chroma of %.3f", seed, i, c)
			}
		}
	}
	if !found {
		t.Error("no seed in forty drew a palette whose first member is of greys")
	}
}

// Heavy music keeps its reds, oranges and ambers and ends in a hot
// yellow-white; flowing music runs from a warm brown to gold.
func TestHeavyMusicEndsInAHotYellowWhiteAndFlowingMusicRunsFromBrownToGold(t *testing.T) {
	for seed := range uint64(60) {
		grit := madeFor(Character{Grit: 1}, seed)
		if grit.fam != ember {
			t.Fatalf("seed %d: heavy music drew %+v, not an ember palette", seed, grit.fam)
		}
		p := grit.table()
		for at := 0; at < len(p); at += paletteSize {
			top := lchOf(p[at+paletteSize-1])
			if top.l < 0.8 || (top.c >= ruleColoured && (top.h < 60 || top.h > 100)) {
				t.Errorf("seed %d: the top of a heavy ramp is %+v, want a light yellow or a white", seed, top)
			}
			mid := lchOf(p[at+paletteSize/2])
			if grit.peak > greyPeak && at == 0 && (mid.h < 25 || mid.h > 80 || mid.c < 0.08) {
				t.Errorf("seed %d: the middle of a heavy ramp is %+v, want a strong red, orange or amber", seed, mid)
			}
		}
		flow := madeFor(Character{Flow: 1}, seed)
		if flow.fam != gold {
			t.Fatalf("seed %d: flowing music drew %+v, not a gold palette", seed, flow.fam)
		}
		for i, col := range flow.ramp() {
			// A dark, weak yellow reads as olive.
			if c := lchOf(col); c.l < 0.5 && c.c >= ruleColoured && c.h > 80 {
				t.Errorf("seed %d: dark step %d of a flowing ramp is %+v, a dark yellow", seed, i, c)
			}
		}
	}
}

func TestAMorphBetweenTwoFamiliesGoesByWayOfGreyAndMovesNoHue(t *testing.T) {
	a, b := madeFor(Character{Drive: 1}, 3), madeFor(Character{Grit: 1}, 3)
	for w := 0.0; w <= 1.0001; w += 0.05 {
		got := mixTones(a, b, w)
		if got.key != a.key && got.key != b.key {
			t.Fatalf("at %.2f the key hue is %.1f, neither %.1f nor %.1f", w, got.key, a.key, b.key)
		}
		side := a
		if got.fam == b.fam {
			side = b
		}
		if want := side.peak * math.Abs(1-2*w); math.Abs(got.peak-want) > 1e-9 {
			t.Fatalf("at %.2f the peak chroma is %.4f, want %.4f", w, got.peak, want)
		}
	}
	for i, col := range mixTones(a, b, 0.5).ramp() {
		if c := lchOf(col).c; c > 0.01 {
			t.Errorf("half way, step %d has a chroma of %.3f, want a grey", i, c)
		}
	}
}

func TestAMorphBetweenTwoMadePalettesTakesTheKeyHueTheShortWay(t *testing.T) {
	a := tones{fam: cool, key: 190, arc: 40, peak: 0.1, contrast: 1, light: 0.8, top: 0.9}
	b := tones{fam: cool, key: 300, arc: 40, peak: 0.1, contrast: 1, light: 0.8, top: 0.9}
	last := a.key
	for w := 0.0; w <= 1; w += 0.05 {
		k := mixTones(a, b, w).key
		if k < last-1e-9 || k > 300+1e-9 {
			t.Fatalf("at %.2f the key hue is %.1f after %.1f, want it to rise from 190 to 300", w, k, last)
		}
		last = k
	}
}

// The aurora is drawn in teal, blue and violet: lighter step by step, and
// with no coloured step outside the cool family.
func TestTheAuroraIsDrawnInTealBlueAndViolet(t *testing.T) {
	last := -1.0
	for i, col := range settings["aurora"].paletteOf(nil) {
		c := lchOf(col)
		if c.l <= last {
			t.Errorf("step %d of the aurora is no lighter than the step before", i)
		}
		last = c.l
		if c.c >= ruleColoured && (c.h < cool.from-ruleEdge || c.h > cool.to+ruleEdge) {
			t.Errorf("step %d of the aurora has the hue %.0f at a chroma of %.3f", i, c.h, c.c)
		}
		if c.c > ruleChroma {
			t.Errorf("step %d of the aurora has a chroma of %.3f", i, c.c)
		}
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
