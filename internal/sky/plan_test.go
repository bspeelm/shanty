package sky

import (
	"math"
	"math/rand/v2"
	"testing"
)

// timeline returns the blocks of a track of the given length whose level at
// each time is shape(t), on the 0 to 1 scale of Moment.Level.
func timeline(seconds float64, shape func(t float64) float64) []Block {
	const rate, n = 44100, 1024
	var out []Block
	for i := 0; float64(i*n)/rate < seconds; i++ {
		at := float64(i*n) / rate
		db := -50 + 45*shape(at)
		c := Channel{Loudness: db, Crossings: 0.05 * n}
		out = append(out, Block{At: at, Samples: n, Channels: []Channel{c, c}, Rate: rate, Scale: 1 << 15})
	}
	return out
}

// structured is a track with a quiet intro, a build, a drop into a loud
// section, a quiet bridge, a climax and a quiet outro.
func structured(t float64) float64 {
	switch {
	case t < 10:
		return 0.3
	case t < 20:
		return 0.3 + 0.03*(t-10)
	case t < 40:
		return 0.8
	case t < 55:
		return 0.35
	case t < 75:
		return 0.95
	}
	return 0.3
}

// near reports whether the plan has a cue of a kind within a second of at.
func near(p *Plan, kind CueKind, at float64) bool {
	for _, c := range p.Cues {
		if c.Kind == kind && math.Abs(c.At-at) <= 1 {
			return true
		}
	}
	return false
}

func count(p *Plan, kind CueKind) int {
	n := 0
	for _, c := range p.Cues {
		if c.Kind == kind {
			n++
		}
	}
	return n
}

func TestThePlannerFindsEachPartOfATrackWithinASecond(t *testing.T) {
	p := NewPlan(timeline(90, structured))
	want := []struct {
		kind CueKind
		at   float64
	}{
		{CueBuild, 10},
		{CueDrop, 20},
		{CueSection, 20},
		{CueSection, 40},
		{CueHush, 40},
		{CueSection, 55},
		{CueClimax, 65},
		{CueSection, 75},
		{CueHush, 75},
	}
	for _, w := range want {
		if !near(p, w.kind, w.at) {
			t.Errorf("no %s within a second of %vs; the cues are %v", w.kind, w.at, p.Cues)
		}
	}
	for kind, n := range map[CueKind]int{CueBuild: 1, CueDrop: 1, CueClimax: 1, CueHush: 2} {
		if got := count(p, kind); got != n {
			t.Errorf("found %d of %s, want %d; the cues are %v", got, kind, n, p.Cues)
		}
	}
	for i := 1; i < len(p.Cues); i++ {
		if p.Cues[i].At < p.Cues[i-1].At {
			t.Fatalf("the cues are out of order: %v", p.Cues)
		}
	}
}

func TestANoisyStepUpWithoutARiseBeforeItIsNotABuild(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	p := NewPlan(timeline(40, func(t float64) float64 {
		jitter := 0.03 * (rng.Float64() - 0.5)
		if t < 20 {
			return 0.3 + jitter
		}
		return 0.9 + jitter
	}))
	if n := count(p, CueBuild) + count(p, CueDrop); n != 0 {
		t.Errorf("a plain step up was read as a build or a drop: %v", p.Cues)
	}
	if !near(p, CueSection, 20) {
		t.Errorf("the step is not a section: %v", p.Cues)
	}
}

func TestTensionRisesThroughABuildAndFallsAfterItsDrop(t *testing.T) {
	p := NewPlan(timeline(90, structured))
	if p.Tension(5) != 0 {
		t.Errorf("tension in the intro is %v, want 0", p.Tension(5))
	}
	last := 0.0
	for at := 11.0; at < 20; at++ {
		v := p.Tension(at)
		if v <= last {
			t.Fatalf("tension fell from %v to %v at %vs in the build", last, v, at)
		}
		last = v
	}
	if last < 0.8 {
		t.Errorf("tension just before the drop is %v, want at least 0.8", last)
	}
	if v := p.Tension(22); v >= last || v <= 0 {
		t.Errorf("tension two seconds after the drop is %v, want between 0 and %v", v, last)
	}
	if v := p.Tension(30); v != 0 {
		t.Errorf("tension in the loud section is %v, want 0", v)
	}
}

func TestAFlatTrackHasNoBuildsDropsOrHushes(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	p := NewPlan(timeline(120, func(float64) float64 { return 0.6 + 0.02*(rng.Float64()-0.5) }))
	for _, kind := range []CueKind{CueBuild, CueDrop, CueHush, CueSection} {
		if n := count(p, kind); n != 0 {
			t.Errorf("a flat track has %d of %s: %v", n, kind, p.Cues)
		}
	}
	if p.Tension(60) != 0 {
		t.Errorf("a flat track has tension %v", p.Tension(60))
	}
}

func TestTheTempoComesFromTheOnsetsOfTheWholeTrack(t *testing.T) {
	p := NewPlan(timeline(60, func(t float64) float64 {
		if math.Mod(t, 0.5) < 0.03 {
			return 0.9
		}
		return 0.3
	}))
	if math.Abs(p.Tempo-120) > 2 {
		t.Errorf("the tempo of a beat every half second is %v, want 120", p.Tempo)
	}
}

func TestNextFindsTheFirstCueOfAKindAtOrAfterATime(t *testing.T) {
	p := &Plan{Cues: []Cue{{At: 5, Kind: CueSection}, {At: 10, Kind: CueDrop}, {At: 20, Kind: CueSection}}}
	if c, ok := p.Next(5, CueSection); !ok || c.At != 5 {
		t.Errorf("Next(5) = %v, %v; want the cue at 5", c, ok)
	}
	if c, ok := p.Next(6, CueSection); !ok || c.At != 20 {
		t.Errorf("Next(6) = %v, %v; want the cue at 20", c, ok)
	}
	if _, ok := p.Next(11, CueDrop); ok {
		t.Error("Next found a drop after the last one")
	}
	var none *Plan
	if _, ok := none.Next(0, CueSection); ok || none.Tension(0) != 0 || none.At(0) != (Moment{}) {
		t.Error("a missing plan has cues")
	}
}

func TestAtInterpolatesTheCurves(t *testing.T) {
	p := NewPlan(timeline(90, structured))
	if got := p.At(5).Level; math.Abs(got-0.3) > 0.01 {
		t.Errorf("the level in the intro is %v, want 0.3", got)
	}
	if got := p.At(15).Level; math.Abs(got-0.45) > 0.02 {
		t.Errorf("the level halfway through the build is %v, want 0.45", got)
	}
	if got := p.At(65).Level; math.Abs(got-0.95) > 0.01 {
		t.Errorf("the level in the climax is %v, want 0.95", got)
	}
	if got := p.At(-1); got != p.At(0) {
		t.Errorf("before the start the curves read %v, want the first point %v", got, p.At(0))
	}
}

func TestAnEmptyTrackHasAnEmptyPlan(t *testing.T) {
	p := NewPlan(nil)
	if len(p.Cues) != 0 || p.Tempo != 0 || p.At(3) != (Moment{}) {
		t.Errorf("a plan of nothing is %+v", p)
	}
}
