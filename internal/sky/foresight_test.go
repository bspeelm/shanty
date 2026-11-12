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

func TestAPlanArrivingPartWayThroughDoesNotSetOffTheCuesBehindIt(t *testing.T) {
	s := New(3)
	var tr track
	play(s, &tr, 0, 30, 40, 10)
	s.Foresee("track", &Plan{Cues: []Cue{{At: 10, Kind: CueDrop, Strength: 1}, {At: 20, Kind: CueClimax, Strength: 1}}})
	play(s, &tr, 30, 0.5, 40, 10)
	if got := s.foresight.pulse; got != 0 {
		t.Errorf("a plan arriving at 30 s set off a release of %.2f for cues at 10 s and 20 s", got)
	}
}

func TestASeekPastADropDoesNotSetItOff(t *testing.T) {
	p := &Plan{Cues: []Cue{{At: 40, Kind: CueDrop, Strength: 1}}}
	s, tr := planned(p, 10)
	// A seek: the next readings say playback is at 60 s.
	*tr = track{}
	play(s, tr, 60, 0.5, 40, 10)
	if got := s.foresight.pulse; got != 0 {
		t.Errorf("seeking from 10 s to 60 s set off a release of %.2f for the drop at 40 s", got)
	}
}

// scored is a plan of a 128 BPM dance track measured in bands, with no
// curves, so that it is followed whatever is heard.
func scored(seconds float64) (*Plan, groove) {
	g, bass := danceGroove(128, seconds, 0.3)
	p := NewPlan(banded(seconds, g.strikes, bass))
	p.curve = nil
	return p, g
}

func TestWithAPlanMeasuredInBandsTheBeatIsThePlansAndNotTheOneHeard(t *testing.T) {
	p, g := scored(60)
	s := New(3)
	s.Foresee("track", p)
	var tr track
	// What is heard is a 120 BPM click; the plan says 128 BPM.
	play(s, &tr, 0, 30, 40, 10)
	sig := s.Signals()
	if math.Abs(sig.Tempo-128) > 1.5 {
		t.Fatalf("with a plan at 128 BPM the tempo was %.1f", sig.Tempo)
	}
	pos := s.lis.ear.now
	since := pos - g.beats[0]
	want := math.Mod(since/(60.0/128), 1)
	off := sig.Phase - want
	if off -= math.Round(off); math.Abs(off) > 0.08 {
		t.Errorf("at %.2f s the phase was %.2f, want %.2f from the plan's beats", pos, sig.Phase, want)
	}
	if sig.Sure < 0.5 {
		t.Errorf("a plan sure of its beat gave a confidence of %.2f", sig.Sure)
	}
}

func TestAPlannedKickIsSetOffWithinAFrameOfItsTime(t *testing.T) {
	p, _ := scored(40)
	var kicks []float64
	for _, h := range p.Hits {
		if h.Kind == HitKick {
			kicks = append(kicks, h.At)
		}
	}
	s := New(3)
	s.Foresee("track", p)
	var tr track
	play(s, &tr, 0, 10, 40, 10)
	dt := 1.0 / 30
	late, seen := 0.0, 0
	last := s.Signals().Kick
	from := s.lis.ear.now
	for at := 10.0; at < 30; at += dt {
		play(s, &tr, at, dt/2, 40, 10)
		k := s.Signals().Kick
		if k > last+0.15 {
			seen++
			late = math.Max(late, nearest(kicks, s.lis.ear.now))
		}
		last = k
	}
	if want := len(between(kicks, from, s.lis.ear.now)); want < 10 || seen < want-1 {
		t.Fatalf("%d kicks were set off where the plan has %d", seen, want)
	}
	if late > dt {
		t.Errorf("a kick was set off %.0f ms from the nearest planned kick", late*1000)
	}
}

func TestATrackPlannedAsBeatlessHasNoTempoWhateverIsHeard(t *testing.T) {
	p := NewPlan(beatlessPad())
	p.curve = nil
	if !p.Beatless() {
		t.Fatal("the pad's plan has a beat, so this test proves nothing")
	}
	s := New(3)
	var tr track
	play(s, &tr, 0, 25, 40, 10)
	if s.Signals().Tempo == 0 {
		t.Fatal("no tempo was heard in the click, so this test proves nothing")
	}
	s.Foresee("track", p)
	play(s, &tr, 25, 5, 40, 10)
	if sig := s.Signals(); sig.Tempo != 0 || sig.beatWeight() != 0 {
		t.Errorf("with a beatless plan the tempo was %.1f at weight %.2f", sig.Tempo, sig.beatWeight())
	}
}

func TestAPlanWhoseLevelIsFarFromWhatIsHeardIsDroppedAndTakenUpAgain(t *testing.T) {
	p, _ := scored(60)
	// The plan says the track is silent; what is heard is loud.
	p.curve = make([]Moment, int(60/planStep))
	s := New(3)
	s.Foresee("track", p)
	var tr track
	play(s, &tr, 0, 3, 40, 10)
	if s.trusted() == nil {
		t.Fatal("the plan was dropped within three seconds")
	}
	play(s, &tr, 3, 4, 40, 10)
	if s.trusted() != nil {
		t.Fatal("a plan for a silent track was still followed after seven loud seconds")
	}
	if tempo := s.Signals().Tempo; math.Abs(tempo-128) < 2 {
		t.Errorf("the dropped plan still gave its tempo, %.1f", tempo)
	}
	// The plan's level comes to match what is heard.
	heard := s.Signals().Level
	for i := range p.curve {
		p.curve[i].Level = heard
	}
	play(s, &tr, 7, 6, 40, 10)
	if s.trusted() == nil {
		t.Error("the plan was not taken up again once it agreed with what is heard")
	}
}

func TestTheCharacterOfANewTrackIsItsPlansUntilEnoughIsHeard(t *testing.T) {
	p, _ := scored(60)
	if lead := p.Character(); lead.Drive < 0.5 {
		t.Fatalf("the dance plan's character is %+v, so this test proves nothing", lead)
	}
	s := New(3)
	s.Foresee("track", p)
	s.Step(40, 10, 1.0/30, time.Time{}, "track")
	if got := s.character(); got != p.Character() {
		t.Errorf("before anything was heard the character was %+v, want the plan's %+v", got, p.Character())
	}
}

// breakdown is the plan of a 120 BPM track whose kicks stop from 40 s to
// 70 s while its hats and a pad carry on. It has no curves, so that it is
// followed whatever is heard. It returns the time of the cue nearest to the
// kicks coming back.
func breakdown(t *testing.T) (*Plan, float64) {
	t.Helper()
	var g groove
	for at := 0.25; at < 110; at += 0.5 {
		if at < 40 || at > 70 {
			g.play(at, kickVoice, 1, nil)
		}
		g.play(at+0.25, hatVoice, 1, nil)
	}
	pad := func(float64) [bandCount]float64 { return [bandCount]float64{1e-3, 1e-3, 1e-4, 1e-5} }
	p := NewPlan(banded(110, g.strikes, pad))
	p.curve = nil
	back := math.Inf(1)
	for _, c := range p.Cues {
		if (c.Kind == CueSection || c.Kind == CueDrop) && math.Abs(c.At-70) < math.Abs(back-70) {
			back = c.At
		}
	}
	if math.Abs(back-70) > 2 {
		t.Fatalf("the plan has no cue where the kicks come back: %v", p.Cues)
	}
	return p, back
}

func TestAStretchWithoutKicksIsSoftAndTheStretchesAroundItAreNot(t *testing.T) {
	p, _ := breakdown(t)
	if c := p.mood(48); c.Calm+c.Flow <= c.Drive+c.Grit {
		t.Errorf("without kicks the mood was %+v, want calm and flowing to lead", c)
	}
	for _, at := range []float64{10, 80} {
		if c := p.mood(at); c.Drive <= c.Calm || c.Drive <= c.Flow {
			t.Errorf("with kicks at %v s the mood was %+v, want Drive above Calm and Flow", at, c)
		}
	}
}

// cuts plays a planned track from 0 to secs and returns the position of each
// frame on which a cut began. before runs ahead of each frame.
func cuts(s *State, tr *track, secs float64, before func(pos float64)) []float64 {
	var out []float64
	dt := 1.0 / 30
	for at := 0.0; at < secs; at += dt {
		if before != nil {
			before(s.lis.ear.now)
		}
		was := s.next
		play(s, tr, at, dt/2, 40, 10)
		if s.next != nil && s.next != was && s.fadeLen == cutFade {
			out = append(out, s.lis.ear.now)
		}
	}
	return out
}

func TestTheKicksComingBackAreCutToAsTheyAreHeard(t *testing.T) {
	p, back := breakdown(t)
	s := New(3)
	s.Foresee("track", p)
	var tr track
	got := cuts(s, &tr, 90, nil)
	if d := nearest(got, back); d > cutFade {
		t.Fatalf("cuts began at %v; the kicks come back at %.2f s", got, back)
	}
	if c := s.cur.char; c.Drive <= c.Calm {
		t.Errorf("after the kicks came back the scene was made for %+v", c)
	}
}

func TestACutReplacesAChangeOfSceneAlreadyUnderWay(t *testing.T) {
	p, back := breakdown(t)
	s := New(3)
	s.Foresee("track", p)
	var tr track
	started := false
	got := cuts(s, &tr, back+1, func(pos float64) {
		// A slow change is begun two seconds before the kicks come back.
		if !started && pos > back-2 {
			started = true
			s.next = s.make(s.Signals(), styleFor(s.character()), s.character())
			s.fade, s.fadeLen = 0, fadeMin
		}
	})
	if !started {
		t.Fatal("no change was begun, so this test proves nothing")
	}
	if d := nearest(got, back); d > cutFade {
		t.Errorf("with a change under way, cuts began at %v; the kicks come back at %.2f s", got, back)
	}
}
