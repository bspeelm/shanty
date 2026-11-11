package sky

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

// ready returns a state sized 60 by 16 with one frame drawn, and its frame.
func ready(seed uint64) *State {
	s := New(seed)
	s.Step(60, 16, 1.0/30, time.Time{}, "")
	return s
}

func TestAnEvolvingSceneKeepsSomeOfItsLayersAsTheyWere(t *testing.T) {
	for seed := range uint64(30) {
		s := ready(seed)
		rng := rand.New(rand.NewPCG(seed, 2))
		before := slices.Clone(s.cur.layers)
		s.cur.evolve(rng, styleFor(even), even)
		kept, arrived, leaving := 0, 0, 0
		for _, l := range s.cur.layers {
			switch {
			case slices.Contains(before, l) && l.goal > 0:
				kept++
			case slices.Contains(before, l):
				leaving++
			default:
				arrived++
			}
		}
		if kept < 1 || kept > 3 || arrived < 1 || leaving < 1 {
			t.Fatalf("seed %d: evolving kept %d layers, brought %d and let %d go", seed, kept, arrived, leaving)
		}
	}
}

func TestALayerArrivesAndLeavesGradually(t *testing.T) {
	s := ready(1)
	// The director is held still, so that no further change starts while
	// this one is watched.
	s.life = math.Inf(1)
	s.cur.evolve(rand.New(rand.NewPCG(1, 2)), styleFor(even), even)
	var coming, going *layer
	for _, l := range s.cur.layers {
		if l.goal > 0 && l.presence == 0 {
			coming = l
		}
		if l.goal == 0 {
			going = l
		}
	}
	if coming == nil || going == nil {
		t.Fatal("evolving brought no layer in from nothing, or let none go")
	}
	s.Step(60, 16, 1.0/30, time.Time{}, "")
	if coming.presence <= 0 || coming.presence > 0.2 {
		t.Errorf("one frame in, an arriving layer was %.2f present", coming.presence)
	}
	if going.presence >= 1 || going.presence < 0.8 {
		t.Errorf("one frame in, a leaving layer was %.2f present", going.presence)
	}
	for range 30 * int(s.cur.morph) * 2 {
		s.Step(60, 16, 1.0/30, time.Time{}, "")
	}
	if slices.Contains(s.cur.layers, going) {
		t.Error("a leaving layer was still in the scene long after the change")
	}
	if coming.presence < 0.95 {
		t.Errorf("long after the change an arriving layer was %.2f present", coming.presence)
	}
}

func TestAWarpAtAmountZeroLeavesTheCoordinatesAlone(t *testing.T) {
	s := ready(1)
	w := &warp{kind: warpKaleido, n: 6, amount: 0}
	s.f.warp([]*warp{w}, [][4]float64{{0.5, 0.5, 1}})
	xs := slices.Clone(s.f.xs)
	s.f.warp(nil, nil)
	if !slices.Equal(xs, s.f.xs) {
		t.Error("a kaleidoscope at amount 0 moved the coordinates")
	}
	w.amount = 1
	s.f.warp([]*warp{w}, [][4]float64{{0.5, 0.5, 1}})
	if slices.Equal(xs, s.f.xs) {
		t.Error("a kaleidoscope at amount 1 left the coordinates alone")
	}
}

func TestBoidsAreDrawnTowardsBrightness(t *testing.T) {
	mean := func(couple float64) float64 {
		s := ready(1)
		f := &s.f
		f.env = make([]float64, f.fw*f.fh)
		for py := range f.fh {
			for px := f.fw * 3 / 4; px < f.fw; px++ {
				f.env[py*f.fw+px] = 1
			}
		}
		f.couple, f.rng = couple, rand.New(rand.NewPCG(1, 1))
		f.warp(nil, nil)
		b := &flock{}
		buf := make([]float64, f.fw*f.fh)
		for range 90 {
			b.draw(f, [4]float64{0.5, 0.5, 0.5, 0.5}, 1, buf)
		}
		x := 0.0
		for _, bird := range b.birds {
			x += bird[0]
		}
		return x / float64(len(b.birds))
	}
	if loose, held := mean(0), mean(1); held <= loose {
		t.Errorf("with a bright band on the right the flock's mean x was %.2f coupled and %.2f not", held, loose)
	}
}

func TestADrivingCharacterLocksToTheBeatAndChoosesItsComponents(t *testing.T) {
	st := styleFor(Character{Drive: 1})
	if st.lock < 0.99 {
		t.Errorf("a driving style locked to the beat %.2f", st.lock)
	}
	calm := styleFor(Character{Calm: 1})
	if calm.lock > 0.01 {
		t.Errorf("a calm style locked to the beat %.2f", calm.lock)
	}
	favoured := map[string]bool{"tunnel": true, "lattice": true, "rings": true, "grid": true, "rule": true}
	rng := rand.New(rand.NewPCG(1, 1))
	hits := 0
	for range 1000 {
		if favoured[pickKind(rng, st, &scene{}).name] {
			hits++
		}
	}
	// Five favoured kinds of 23, each four times as likely, come up about
	// 20 of every 38 draws.
	if share := float64(hits) / 1000; share < 0.45 {
		t.Errorf("a driving style chose its favoured components %.2f of the time", share)
	}
}

func TestEachCharacterHasItsOwnColours(t *testing.T) {
	warmth := func(c Character) float64 {
		rng := rand.New(rand.NewPCG(1, 1))
		st := styleFor(c)
		w := 0.0
		for range 50 {
			p := st.palette(rng, c)
			top := p[paletteSize*3/4]
			w += float64(top.R) - float64(top.B)
		}
		return w / 50
	}
	if grit, calm := warmth(Character{Grit: 1}), warmth(Character{Calm: 1}); grit <= calm {
		t.Errorf("gritty palettes were no warmer than calm ones: %.0f against %.0f", grit, calm)
	}
}

func TestTurningAPaletteKeepsItsLightness(t *testing.T) {
	p := []RGB{{200, 40, 40}, {30, 120, 220}}
	turned := turnPalette(p, rand.New(rand.NewPCG(3, 3)))
	for i := range p {
		before := float64(p[i].R) + float64(p[i].G) + float64(p[i].B)
		after := float64(turned[i].R) + float64(turned[i].G) + float64(turned[i].B)
		if math.Abs(before-after) > 6 {
			t.Errorf("turning %v gave %v, changing its lightness", p[i], turned[i])
		}
	}
}
