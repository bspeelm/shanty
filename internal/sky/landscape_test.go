package sky

import (
	"math"
	"math/rand/v2"
	"testing"
)

// field returns a frame sized as a visualiser w cells by h would size it,
// with no warps.
func field(w, h int) *frame {
	s := New(1)
	s.resize(w, h)
	f := &s.f
	f.warp(nil, nil)
	f.dt, f.rng = 1.0/30, rand.New(rand.NewPCG(1, 1))
	return f
}

// still returns a landscape of a setting with water, a moon, two ridges, no
// aurora and no weather, so that what it draws can be measured.
func still(name string, seed uint64) *landscape {
	l := newLandscape(rand.New(rand.NewPCG(seed, seed)), settings[name])
	l.water, l.aurora, l.weather = true, false, weatherNone
	l.body, l.bodyX, l.bodyY = bodyMoon, 0.5, 0.6
	l.horizon = 0.6
	l.ridges = l.ridges[:0]
	for _, d := range []int{0, 2} {
		l.ridges = append(l.ridges, ridge{depth: depths[d], profile: ridgeProfile(int(seed) + d)})
	}
	return l
}

// drawFor draws l for frames frames with the given signals and returns the
// last frame.
func drawFor(l *landscape, f *frame, sig Signals, frames int) []float64 {
	buf := make([]float64, f.fw*f.fh)
	f.sig = sig
	for range frames {
		l.draw(f, [4]float64{0.5, 0.5, 0.5, 0.5}, 1, buf)
	}
	return buf
}

// horizonRow is the row a landscape's horizon falls on in f.
func horizonRow(l *landscape, f *frame) int {
	return min(max(int(l.horizon*float64(f.fh)+0.5), 1), f.fh-1)
}

// mirrorError is the mean difference between the water and the reflection
// of the rows above it.
func mirrorError(l *landscape, f *frame, buf []float64) float64 {
	hy := horizonRow(l, f)
	sum, n := 0.0, 0
	for d := 0; d < min(hy, f.fh-hy); d++ {
		for x := range f.fw {
			above := buf[(hy-1-d)*f.fw+x]
			below := buf[(hy+d)*f.fw+x]
			sum += math.Abs(below - above*reflectivity)
			n++
		}
	}
	return sum / float64(n)
}

func TestEveryLandscapeDrawsWithinRangeAtEverySize(t *testing.T) {
	moods := []Signals{
		{},
		{Tension: 1, Bass: 1, Kick: 1, Snap: 1},
		{Release: 1, Tempo: 120, Phase: 0.1},
		{Hush: 1},
	}
	for name := range settings {
		for _, size := range [][2]int{{1, 1}, {7, 3}, {30, 8}, {100, 30}, {203, 61}} {
			for seed := range uint64(4) {
				f := field(size[0], size[1])
				f.couple, f.env = 1, make([]float64, f.fw*f.fh)
				for i := range f.env {
					f.env[i] = hash(i, 3)
				}
				l := newLandscape(rand.New(rand.NewPCG(seed, 5)), settings[name])
				buf := make([]float64, f.fw*f.fh)
				for i := range 120 {
					f.sig = moods[i/30]
					l.draw(f, [4]float64{0.5, 0.5, 0.5, 0.5}, 1, buf)
					for j, v := range buf {
						tone := l.tones()[j]
						if math.IsNaN(v) || v < 0 || v > 1 || math.IsNaN(tone) || tone < 0 || tone > 1 {
							t.Fatalf("%s at %dx%d drew value %v and tone %v at pixel %d", name, size[0], size[1], v, tone, j)
						}
					}
				}
			}
		}
	}
}

func TestStillWaterReflectsTheSkyAboveIt(t *testing.T) {
	for name := range settings {
		f := field(100, 30)
		l := still(name, 2)
		buf := drawFor(l, f, Signals{}, 60)
		if e := mirrorError(l, f, buf); e > 0.01 {
			t.Errorf("%s: with no bass the water differed from the reflected sky by %.3f on average", name, e)
		}
	}
}

func TestBassMovesTheWaterAndAHushStillsIt(t *testing.T) {
	f := field(100, 30)
	l := still("night lake", 3)
	moving := mirrorError(l, f, drawFor(l, f, Signals{Bass: 0.8}, 90))
	l = still("night lake", 3)
	hushed := mirrorError(l, f, drawFor(l, f, Signals{Bass: 0.8, Hush: 1}, 90))
	if moving < 0.005 {
		t.Errorf("with bass the water differed from the reflected sky by only %.4f", moving)
	}
	if hushed > moving/3 {
		t.Errorf("in a hush the ripples left the water %.4f from the reflected sky, against %.4f without", hushed, moving)
	}
}

// skyLight is the mean brightness of the rows above the horizon.
func skyLight(l *landscape, f *frame, buf []float64) float64 {
	hy := horizonRow(l, f)
	sum := 0.0
	for _, v := range buf[:hy*f.fw] {
		sum += v
	}
	return sum / float64(hy*f.fw)
}

func TestTensionDimsTheLightAndGathersCloudAndAReleaseBreaksItThrough(t *testing.T) {
	for _, name := range []string{"dawn sea", "storm"} {
		light := func(sig Signals) (float64, float64) {
			f := field(100, 30)
			l := still(name, 4)
			buf := drawFor(l, f, sig, 300)
			return skyLight(l, f, buf), l.cover
		}
		rest, restCloud := light(Signals{})
		tense, tenseCloud := light(Signals{Tension: 1})
		released, releasedCloud := light(Signals{Release: 1})
		if tense > rest*0.85 {
			t.Errorf("%s: under tension the sky was %.3f bright against %.3f at rest", name, tense, rest)
		}
		if tenseCloud < restCloud+0.3 {
			t.Errorf("%s: under tension the cloud cover was %.2f against %.2f at rest", name, tenseCloud, restCloud)
		}
		if released < rest*1.15 {
			t.Errorf("%s: on a release the sky was %.3f bright against %.3f at rest", name, released, rest)
		}
		if releasedCloud > restCloud {
			t.Errorf("%s: on a release the cloud cover was %.2f against %.2f at rest", name, releasedCloud, restCloud)
		}
	}
}

func TestSkyLandAndWaterHaveTheirOwnTones(t *testing.T) {
	f := field(100, 30)
	l := still("night lake", 5)
	l.body = bodyNone
	drawFor(l, f, Signals{}, 30)
	hy := horizonRow(l, f)
	tone := l.tones()
	sky := tone[f.fw/2]
	water := tone[(f.fh-1)*f.fw+f.fw/2]
	// The near ridge stands at least its base above the horizon, so the row
	// just above the horizon is land in every column.
	land := tone[(hy-1)*f.fw+f.fw/2]
	if math.Abs(sky-land) < 0.2 || math.Abs(sky-water) < 0.2 || math.Abs(land-water) < 0.2 {
		t.Errorf("the sky's tone was %.2f, the land's %.2f and the water's %.2f", sky, land, water)
	}
}

func TestASnapInAStormFlashesLightning(t *testing.T) {
	f := field(100, 30)
	l := still("storm", 6)
	buf := drawFor(l, f, Signals{}, 60)
	before := skyLight(l, f, buf)
	buf = drawFor(l, f, Signals{Snap: 1}, 1)
	if after := skyLight(l, f, buf); after < before+0.2 {
		t.Errorf("a snap in a storm lit the sky from %.3f to only %.3f", before, after)
	}
}

func TestReleaseSendsFirefliesUp(t *testing.T) {
	height := func(sig Signals) float64 {
		f := field(100, 30)
		l := still("night lake", 7)
		l.weather = weatherFireflies
		drawFor(l, f, sig, 60)
		y := 0.0
		for _, m := range l.rise {
			y += m.y
		}
		return y / float64(len(l.rise))
	}
	if rest, released := height(Signals{}), height(Signals{Release: 1}); released > rest-0.02 {
		t.Errorf("on a release the fireflies' mean height was %.3f from the top against %.3f at rest", released, rest)
	}
}

// landShare is the share of n scenes made in a character that have a
// landscape in them.
func landShare(c Character, n int) float64 {
	rng := rand.New(rand.NewPCG(8, 8))
	st := styleFor(c)
	got := 0
	for range n {
		if newScene(rng, st, c, Signals{}).landed() {
			got++
		}
	}
	return float64(got) / float64(n)
}

func TestCalmMusicOftenShowsALandscapeAndDrivingMusicRarely(t *testing.T) {
	if share := landShare(Character{Calm: 1}, 1000); share < 0.25 {
		t.Errorf("%.2f of calm scenes had a landscape, want a quarter or more", share)
	}
	if share := landShare(Character{Flow: 1}, 1000); share < 0.25 {
		t.Errorf("%.2f of flowing scenes had a landscape, want a quarter or more", share)
	}
	if share := landShare(Character{Drive: 1}, 1000); share > 0.12 {
		t.Errorf("%.2f of driving scenes had a landscape, want few", share)
	}
	if share := landShare(even, 1000); share < 0.15 || share > 0.4 {
		t.Errorf("%.2f of scenes in an even character had a landscape, want about a quarter", share)
	}
}

func TestALandscapeKeepsItsWeightThroughMutations(t *testing.T) {
	rng := rand.New(rand.NewPCG(10, 10))
	st := styleFor(Character{Calm: 1})
	sc := newScene(rng, st, Character{Calm: 1}, Signals{})
	for !sc.landed() {
		sc = newScene(rng, st, Character{Calm: 1}, Signals{})
	}
	for range 200 {
		sc.mutate(rng, st)
	}
	if w := sc.layers[0].weight; w != (param{base: landWeight}) {
		t.Errorf("after 200 mutations the landscape's weight was %+v", w)
	}
}

func TestALandscapeIsAlwaysTheFirstLayer(t *testing.T) {
	rng := rand.New(rand.NewPCG(9, 9))
	st := styleFor(Character{Calm: 1})
	seen := 0
	for range 300 {
		sc := newScene(rng, st, Character{Calm: 1}, Signals{})
		for range 3 {
			sc.evolve(rng, st, Character{Calm: 1})
			for _, l := range sc.layers {
				l.presence = l.goal
			}
			sc.advance(0)
		}
		lands := 0
		for i, l := range sc.layers {
			if isLandscape(l.name) {
				lands++
				seen++
				if i != 0 {
					t.Fatalf("a landscape was layer %d of %v", i, sc.combo())
				}
			}
		}
		if lands > 1 {
			t.Fatalf("a scene held %d landscapes", lands)
		}
	}
	if seen == 0 {
		t.Fatal("no scene had a landscape")
	}
}

func TestAScenePicksUpAgainOnceItsLandscapeHasGone(t *testing.T) {
	st := styleFor(Character{Calm: 1})
	for seed := uint64(1); seed <= 200; seed++ {
		rng := rand.New(rand.NewPCG(seed, 8))
		s := newScene(rng, st, Character{Calm: 1}, Signals{})
		if !s.landed() {
			continue
		}
		var over *layer
		for _, l := range s.layers[1:] {
			over = l
		}
		if over == nil {
			continue
		}
		quietened := over.weight.base
		// The landscape leaves, and the next change finds none.
		s.layers[0].goal = 0
		s.evolve(rng, st, Character{Calm: 1})
		if s.landed() {
			continue
		}
		if s.grounded {
			t.Fatal("a scene whose landscape had gone was still arranged around one")
		}
		if over.weight.base <= quietened {
			t.Errorf("a layer kept its lowered weight %.2f after the landscape went", over.weight.base)
		}
		if s.zoom.depth == ([2]float64{}) && s.turn.depth == ([2]float64{}) {
			t.Error("the feedback still neither zoomed nor turned after the landscape went")
		}
		return
	}
	t.Fatal("no seed made a scene with a landscape and a layer over it whose landscape then went")
}
