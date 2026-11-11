package sky

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestAPlanIsUsedOnlyForTheTrackItWasMadeFor(t *testing.T) {
	s := New(1)
	first, second := &Plan{Length: 1}, &Plan{Length: 2}
	s.Foresee("first", first)
	s.Foresee("second", second)
	if s.plan() != nil {
		t.Fatal("a plan is used before any track has played")
	}
	s.Step(20, 10, 0.033, time.Time{}, "first")
	if s.plan() != first {
		t.Errorf("the first track has plan %v", s.plan())
	}
	s.Step(20, 10, 0.033, time.Time{}, "third")
	if s.plan() != nil {
		t.Errorf("a track with no plan has plan %v", s.plan())
	}
	s.Step(20, 10, 0.033, time.Time{}, "second")
	if s.plan() != second {
		t.Errorf("the second track has plan %v", s.plan())
	}
}

func TestANewPlanForATrackReplacesItsOldOne(t *testing.T) {
	s := New(1)
	s.Step(20, 10, 0.033, time.Time{}, "a")
	s.Foresee("a", &Plan{Length: 1})
	newer := &Plan{Length: 2}
	s.Foresee("a", newer)
	if s.plan() != newer || len(s.foresight.plans) != 1 {
		t.Errorf("the track has plan %v among %d", s.plan(), len(s.foresight.plans))
	}
}

func TestOnlyTheLatestPlansAreKept(t *testing.T) {
	s := New(1)
	for i := range foresightKept + 3 {
		s.Foresee(fmt.Sprint(i), &Plan{})
	}
	if len(s.foresight.plans) != foresightKept {
		t.Fatalf("holding %d plans, want %d", len(s.foresight.plans), foresightKept)
	}
	s.Step(20, 10, 0.033, time.Time{}, "0")
	if s.plan() != nil {
		t.Error("the oldest plan was kept")
	}
}

// planned returns a visualiser that has played a steady track for secs
// seconds with p as that track's plan from the start.
func planned(p *Plan, secs float64) (*State, *track) {
	s := New(3)
	s.Foresee("track", p)
	var tr track
	play(s, &tr, 0, secs, 40, 10)
	return s, &tr
}

func TestAPlannedSectionIsReachedJustAsTheChangeForItCompletes(t *testing.T) {
	p := &Plan{Cues: []Cue{{At: 40, Kind: CueSection, Strength: 1}}}
	s, tr := planned(p, 25)
	// The scene has been on screen long enough to change, and nothing but
	// the plan will change it.
	s.life, s.age = math.Inf(1), sectionLead+5
	started := -1.0
	for at := 25.0; at < 45 && started < 0; at += 1.0 / 30 {
		before := s.cur.changing() || s.next != nil
		play(s, tr, at, 1.0/30, 40, 10)
		if !before && (s.cur.changing() || s.next != nil) {
			started = s.lis.ear.now
		}
	}
	if started < 0 {
		t.Fatal("no change began before a planned section")
	}
	lead := math.Max(fadeMin, styleFor(s.lis.char).morph/2)
	if want := 40 - lead; math.Abs(started-want) > 0.1 {
		t.Errorf("the change began at %.2f s of the track, want %.2f s, a change's length before the section", started, want)
	}
}

func TestTensionRisesThroughAPlannedBuildBeforeTheDropIsHeard(t *testing.T) {
	p := &Plan{builds: [][2]float64{{20, 30}}, Cues: []Cue{{At: 30, Kind: CueDrop, Strength: 1}}}
	s, tr := planned(p, 25.27)
	if got := s.Signals().Tension; got < 0.4 {
		t.Errorf("halfway through a planned build tension was %.2f", got)
	}
	play(s, tr, 25.27, 5.3, 40, 10)
	if got := s.Signals().Release; got < 0.6 {
		t.Errorf("as the planned drop was heard release was %.2f", got)
	}
}

func TestTheLightSettlesBeforeAPlannedHush(t *testing.T) {
	p := &Plan{Cues: []Cue{{At: 20, Kind: CueHush, Strength: 1}}}
	s, _ := planned(p, 19.27)
	if got := s.Signals().Hush; got < 0.2 {
		t.Errorf("a second before a planned hush, hush was %.2f", got)
	}
}

func TestWithoutAPlanTheSignalsAreThoseHeard(t *testing.T) {
	with, _ := planned(&Plan{builds: [][2]float64{{0, 100}}}, 10)
	without := New(3)
	var tr track
	play(without, &tr, 0, 10, 40, 10)
	if with.Signals().Tension <= without.Signals().Tension {
		t.Fatal("a plan with a build changed nothing, so this test proves nothing")
	}
	if without.foresight.pulse != 0 || without.plan() != nil {
		t.Error("a visualiser with no plan acted on one")
	}
}
