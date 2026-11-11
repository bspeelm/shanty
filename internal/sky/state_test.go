package sky

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// play runs s for secs seconds of a synthetic 120 BPM track at 30 frames a
// second, w by h, from position from.
func play(s *State, tr *track, from, secs float64, w, h int) {
	dt := 1.0 / 30
	for at := from; at < from+secs; at += dt {
		beat := math.Mod(at, 0.5)
		bass, hats := 0.001, 0.0
		if beat < 0.08 {
			bass = 0.3
		}
		if math.Abs(beat-0.25) < 0.04 {
			hats = 0.02
		}
		diff := bass*band(bassHz, trackRate) + hats*band(trebleHz, trackRate)
		s.Hear(tr.block(dt, both(bass+hats), both(diff), 0.05, at-0.27))
		s.Step(w, h, dt, "track")
	}
}

// text joins a picture's runs into plain lines.
func text(p Picture) []string {
	var out []string
	for _, l := range p.Lines {
		var b strings.Builder
		for _, r := range l {
			b.WriteString(r.Text)
		}
		out = append(out, b.String())
	}
	return out
}

func TestTheSameSeedDrawsTheSameFrames(t *testing.T) {
	a, b := New(7), New(7)
	var ta, tb track
	play(a, &ta, 0, 3, 60, 16)
	play(b, &tb, 0, 3, 60, 16)
	if !slices.Equal(text(a.Render(true)), text(b.Render(true))) {
		t.Error("two visualisers from the same seed drew different frames")
	}
}

func TestDifferentSeedsMakeDifferentScenes(t *testing.T) {
	combos := map[string]bool{}
	for seed := range uint64(20) {
		s := New(seed)
		s.Step(40, 10, 1.0/30, "")
		combos[s.cur.combo()+s.cur.label] = true
	}
	if len(combos) < 10 {
		t.Errorf("twenty seeds made only %d different scenes", len(combos))
	}
}

func TestAboutOneSceneInFifteenIsRare(t *testing.T) {
	s := New(3)
	s.Step(40, 10, 1.0/30, "")
	rare := 0
	const n = 3000
	for range n {
		s.cur = s.make(Signals{})
		if s.cur.label != "" {
			rare++
		}
	}
	if share := float64(rare) / n; share < 1.0/25 || share > 1.0/10 {
		t.Errorf("%d of %d scenes were rare, want about 1 in 15", rare, n)
	}
}

func TestARecentCombinationIsNotDrawnAgain(t *testing.T) {
	s := New(5)
	s.Step(40, 10, 1.0/30, "")
	repeats := 0
	var last []string
	for range 200 {
		sc := s.make(Signals{})
		if sc.label == "" && slices.Contains(last, sc.combo()) {
			repeats++
		}
		if sc.label == "" {
			last = append(last, sc.combo())
			if len(last) > 3 {
				last = last[1:]
			}
		}
	}
	if repeats > 5 {
		t.Errorf("%d of 200 scenes repeated one of the three before", repeats)
	}
}

func TestANewSectionBringsANewScene(t *testing.T) {
	s := New(11)
	var tr track
	play(s, &tr, 0, 2, 40, 10)
	first := s.cur
	s.lis.sig.Section = true
	s.lis.quiet = 20
	s.lis.short = [3]float64{1, 1, 1}
	play(s, &tr, 2, 12, 40, 10)
	if s.cur == first {
		t.Error("a change of section left the same scene on screen")
	}
}

func TestASteadyTrackKeepsItsSceneForAWhile(t *testing.T) {
	s := New(13)
	var tr track
	play(s, &tr, 0, 2, 40, 10)
	first := s.cur
	play(s, &tr, 2, 20, 40, 10)
	if s.cur != first {
		t.Error("the scene changed within twenty seconds of steady music")
	}
}

func TestEveryWayOfDrawingFillsTheSpaceExactly(t *testing.T) {
	for _, g := range []glyphs{glyphShade, glyphASCII, glyphBraille, glyphHalf} {
		for _, size := range [][2]int{{1, 1}, {7, 3}, {80, 24}, {203, 61}} {
			w, h := size[0], size[1]
			s := New(1)
			s.Step(w, h, 1.0/30, "")
			s.cur.glyphs = g
			for _, colour := range []bool{true, false} {
				lines := text(s.Render(colour))
				if len(lines) != h {
					t.Fatalf("glyphs %d at %dx%d drew %d lines", g, w, h, len(lines))
				}
				for i, l := range lines {
					if n := utf8.RuneCountInString(l); n != w {
						t.Fatalf("glyphs %d at %dx%d drew line %d %d wide", g, w, h, i, n)
					}
				}
			}
		}
	}
}

func TestWithoutColourNothingIsColoured(t *testing.T) {
	s := New(2)
	var tr track
	play(s, &tr, 0, 2, 50, 12)
	for _, g := range []glyphs{glyphShade, glyphASCII, glyphBraille, glyphHalf} {
		s.cur.glyphs = g
		for _, line := range s.Render(false).Lines {
			for _, r := range line {
				if r.Fg != None || r.Bg != None {
					t.Fatalf("glyphs %d drew %q in colour %d on %d with colour off", g, r.Text, r.Fg, r.Bg)
				}
			}
		}
	}
}

func TestARareSceneShowsItsNameAndThenHidesIt(t *testing.T) {
	s := New(4)
	s.Step(60, 12, 1.0/30, "")
	s.cur = rareScene(rand.New(rand.NewPCG(1, 1)))
	s.label = 0
	last := s.Render(false).Lines[11]
	if got := last[len(last)-1]; got.Fg != Label || !strings.Contains(got.Text, s.cur.label) {
		t.Fatalf("the last run of the last line is %+v, want the scene's name", got)
	}
	s.label = labelFor + 1
	for _, r := range s.Render(false).Lines[11] {
		if r.Fg == Label {
			t.Fatal("the name was still showing after it should have gone")
		}
	}
}

func TestEveryComponentDrawsWithinRange(t *testing.T) {
	all := append(append([]kind(nil), kinds...), rarities...)
	for _, k := range all {
		s := New(1)
		var tr track
		play(s, &tr, 0, 0.5, 30, 8)
		rng := rand.New(rand.NewPCG(9, 9))
		c := k.make(rng)
		buf := make([]float64, s.f.fw*s.f.fh)
		s.f.warp(nil, nil)
		for range 40 {
			c.draw(&s.f, [4]float64{0.5, 0.5, 0.5, 0.5}, 1, buf)
		}
		for i, v := range buf {
			if math.IsNaN(v) || v < 0 || v > 1.0001 {
				t.Fatalf("%s drew %v at pixel %d", k.name, v, i)
			}
		}
	}
}

func BenchmarkFrame(b *testing.B) {
	s := New(1)
	var tr track
	play(s, &tr, 0, 1, 200, 60)
	b.ResetTimer()
	for b.Loop() {
		s.Step(200, 60, 1.0/30, "track")
		s.Render(true)
	}
}
