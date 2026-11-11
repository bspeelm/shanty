package sky

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

var zeroTime time.Time

func TestGlideFollowsATimeConstant(t *testing.T) {
	v := 0.0
	for range 10 {
		v = glide(v, 1, 0.01, 0.1)
	}
	// After one time constant a smoothing filter has covered 1 - 1/e.
	if want := 1 - math.Exp(-1); math.Abs(v-want) > 1e-9 {
		t.Errorf("after one time constant the value was %.4f, want %.4f", v, want)
	}
}

func TestBilinearBlendsTheFourCellsAround(t *testing.T) {
	v := []float64{0, 1, 2, 3}
	for _, tc := range []struct{ x, y, want float64 }{
		{0, 0, 0}, {1, 0, 1}, {0.5, 0, 0.5}, {0.5, 0.5, 1.5}, {1, 1, 3}, {-0.1, 0, 0}, {1.1, 0, 0},
	} {
		if got := bilinear(v, 2, 2, tc.x, tc.y); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("at %v,%v got %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestAPointMovingByLessThanAPixelChangesThePictureALittle(t *testing.T) {
	s := New(1)
	s.Step(40, 10, 1.0/30, zeroTime, "")
	var sp splat
	sp.begin(&s.f, 0)
	sp.plot(&s.f, 0, 0, 1)
	a := append([]float64(nil), sp.v...)
	sp.begin(&s.f, 0)
	sp.plot(&s.f, 0.2/s.f.radius, 0, 1)
	moved := 0.0
	for i := range a {
		moved += math.Abs(a[i] - sp.v[i])
	}
	if moved == 0 || moved > 1 {
		t.Errorf("moving a point a fifth of a pixel changed the picture by %.2f", moved)
	}
}

func TestTheJuliaDepthChangesSmoothlyWithC(t *testing.T) {
	// A point outside the set, which escapes for every c tried. On the edge of
	// the set the depth jumps from escaping late to never escaping, which is
	// the set's own edge and not a step in the count.
	prev := juliaDepth(0.9, 0.6, -0.8, 0.156, 32)
	if prev == 1 {
		t.Fatal("the point chosen is inside the set")
	}
	for i := 1; i <= 100; i++ {
		d := juliaDepth(0.9, 0.6, -0.8+float64(i)*0.0005, 0.156, 32)
		if math.Abs(d-prev) > 0.05 {
			t.Fatalf("a step of 0.0005 in c moved the depth from %.3f to %.3f", prev, d)
		}
		prev = d
	}
}

func TestALayersWeightRisesQuicklyAndFallsSlowly(t *testing.T) {
	s := New(1)
	s.Step(40, 10, 1.0/30, zeroTime, "")
	rng := rand.New(rand.NewPCG(1, 1))
	l := newLayer(rng, kinds[2], styleFor(Character{Calm: 1}))
	l.weight = param{base: 0, src: [2]int{srcKick, srcKick}, depth: [2]float64{1, 0}}
	sc := &scene{palette: newPalette(rng), layers: []*layer{l}}
	frame := func(kick float64) float64 {
		s.f.sig = Signals{Kick: kick}
		s.f.dt = 1.0 / 30
		sc.step(&s.f)
		return l.w
	}
	frame(0)
	up := frame(1)
	if up < 0.6 {
		t.Errorf("one frame after a kick the weight was %.2f", up)
	}
	if fell := up - frame(0); fell > up/2 {
		t.Errorf("in the frame after the kick the weight fell %.2f, having risen %.2f", fell, up)
	}
}

func TestAFadeDissolvesFromOneWayOfDrawingToTheNext(t *testing.T) {
	share := func(fade float64) float64 {
		k := look{glyphs: glyphShade, next: glyphBraille, fade: fade}
		n := 0
		for r := range 40 {
			for c := range 100 {
				if k.glyphsAt(c, r) == glyphBraille {
					n++
				}
			}
		}
		return float64(n) / 4000
	}
	if share(0) != 0 || share(1) != 1 {
		t.Errorf("a fade drew %.2f of cells new at its start and %.2f at its end", share(0), share(1))
	}
	if s := share(0.5); s < 0.4 || s > 0.6 {
		t.Errorf("halfway through a fade %.2f of cells were drawn the new way", s)
	}
}

func TestPalettesBlendAcrossAFade(t *testing.T) {
	a, b := []RGB{{0, 0, 0}, {200, 100, 0}}, []RGB{{100, 100, 100}, {0, 100, 200}}
	if got := blendPalettes(a, b, 0); got[1] != a[1] {
		t.Errorf("at the start of a fade the palette was %v", got)
	}
	if got := blendPalettes(a, b, 0.5); got[0] != (RGB{50, 50, 50}) || got[1] != (RGB{100, 100, 100}) {
		t.Errorf("halfway through a fade the palette was %v", got)
	}
}
