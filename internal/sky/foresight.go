package sky

import (
	"math"
	"slices"
)

// foresightState holds the plans the visualiser has been given, the most
// recent last, keyed by the name of the track as Step is given it. heard is
// how far into the track the last frame was, pulse the release a cue last
// set off, and section the time of the last section a change was made for.
type foresightState struct {
	plans   []foreseen
	heard   float64
	pulse   float64
	section float64
	// following is set while the last frame was drawn to a plan.
	following bool
	// last is the plan the last frame was drawn to. kick, snap and note are
	// the planned hits and notes dying away, and busy the planned hits a
	// second around the position. apart is how long the level heard has differed from the
	// plan's, and astray is set while the plan is not followed for it.
	last       *Plan
	kick, snap float64
	note       float64
	busy       float64
	apart      float64
	astray     bool
}

type foreseen struct {
	track string
	plan  *Plan
}

// foresightKept is how many plans the visualiser holds at once.
const foresightKept = 4

// Foresee gives the visualiser the plan of a track, named as Step names it.
// A plan for a track it already holds replaces the one it had.
func (s *State) Foresee(track string, p *Plan) {
	f := &s.foresight
	kept := f.plans[:0]
	for _, old := range f.plans {
		if old.track != track {
			kept = append(kept, old)
		}
	}
	f.plans = append(kept, foreseen{track: track, plan: p})
	if len(f.plans) > foresightKept {
		f.plans = f.plans[1:]
	}
}

// plan returns the plan of the track playing, or nil when there is none.
func (s *State) plan() *Plan {
	for _, f := range s.foresight.plans {
		if f.track == s.track {
			return f.plan
		}
	}
	return nil
}

// sectionLead is the least time, in seconds, a scene must have been on screen
// before a planned section changes it.
const sectionLead = 15

// The measures of following a plan. Times are in seconds.
const (
	// kickFall and snapFall are how long a planned kick and a planned snare
	// or hat take to fall to 1/e.
	kickFall = 0.17
	snapFall = 0.12
	// hatShare is how strongly a hat counts beside a snare.
	hatShare = 0.6
	// noteFall is how long a planned note takes to fall to 1/e, and
	// noteShare the most a note lifts the snap.
	noteFall  = 0.3
	noteShare = 0.35
	// The plan stops being followed once the level heard has been more than
	// apartLevel from the plan's for apartFor seconds.
	apartLevel = 0.3
	apartFor   = 4
	// planShare is the weight of the plan's character beside the character
	// heard, once enough has been heard to have one.
	planShare = 0.6
)

// trusted returns the plan of the track playing when it was measured in
// bands and what is heard agrees with it, and nil otherwise.
func (s *State) trusted() *Plan {
	if p := s.plan(); p != nil && p.banded && !s.foresight.astray {
		return p
	}
	return nil
}

// character returns the character the frame is drawn for: the one heard, or
// with a trusted plan a blend of the track's and the one heard. Until enough
// has been heard, it is the track's alone.
func (s *State) character() Character {
	live := s.lis.char
	p := s.trusted()
	if p == nil {
		return live
	}
	if !s.lis.meter.settled() {
		return p.Character()
	}
	a, b := p.Character().axes(), live.axes()
	mix := func(i int) float64 { return planShare*a[i] + (1-planShare)*b[i] }
	return Character{mix(0), mix(1), mix(2), mix(3)}
}

// score replaces the beat, the hits and the busyness heard with the plan's,
// for the frame at pos that follows the one at from. Each planned kick, and
// each planned snare or hat, is set off as playback reaches it and dies
// away, scaled by how loud its band is heard. A planned note lifts the snap
// a little and dies away more slowly. Where the plan has no beat the frame
// has no tempo.
func (s *State) score(sig Signals, p *Plan, from, pos, dt float64) Signals {
	f := &s.foresight
	f.kick *= math.Exp(-dt / kickFall)
	f.snap *= math.Exp(-dt / snapFall)
	// A hit is set off on the frame nearest to it.
	for _, h := range p.HitsBetween(from+dt/2, pos+dt/2) {
		v := clamp01(0.35 + 0.65*h.Strength)
		switch h.Kind {
		case HitKick:
			f.kick = math.Max(f.kick, v)
		case HitSnare:
			f.snap = math.Max(f.snap, v)
		case HitHat:
			f.snap = math.Max(f.snap, hatShare*v)
		}
	}
	f.note *= math.Exp(-dt / noteFall)
	for _, n := range p.NotesBetween(from+dt/2, pos+dt/2) {
		f.note = math.Max(f.note, clamp01(2*n.Strength))
	}
	sig.Kick = f.kick * (0.6 + 0.4*sig.Bass)
	sig.Snap = math.Max(f.snap*(0.6+0.4*sig.Treble), noteShare*f.note)
	near := float64(len(p.HitsBetween(pos-busySpan/2, pos+busySpan/2)))
	f.busy = glide(f.busy, math.Min(near/busySpan/busyFull, 1), dt, 0.5)
	sig.Busy = f.busy

	b, ok := p.BeatAt(pos)
	if !ok {
		sig.Tempo, sig.Phase, sig.Sure = 0, 0, 0
		return sig
	}
	meter := max(p.Meter, 1)
	bars, _ := slices.BinarySearch(p.Downbeats, pos)
	sig.Tempo, sig.Phase, sig.Sure = b.Tempo, b.Phase, b.Confidence
	sig.Beat = meter*bars + b.Bar
	return sig
}

// agrees follows how far the level heard is from the plan's, and reports
// whether the plan is still followed. A plan is dropped once the two have
// been apart for apartFor seconds, and taken up again once they have agreed
// for as long.
func (f *foresightState) agrees(heard, planned, dt float64) bool {
	if math.Abs(heard-planned) > apartLevel {
		f.apart = math.Min(f.apart+dt, apartFor)
	} else {
		f.apart = math.Max(f.apart-dt, 0)
	}
	if f.apart >= apartFor {
		f.astray = true
	} else if f.apart == 0 {
		f.astray = false
	}
	return !f.astray
}

// foresee shapes a frame's signals by the plan of the track playing, when
// there is one, and reports whether a scene change is due so that it lands
// on the next section. With a plan:
//   - tension follows the plan's builds, rising before the drop arrives;
//   - a drop or the climax sets off a release as it is heard;
//   - the light begins to settle two seconds before a hush;
//   - a plan measured in bands gives the beat, the hits and the busyness.
//
// A plan whose level stays far from the level heard is not followed until
// the two agree again. A plan with no curves is always followed.
//
// A change is due when the next section is as far ahead as a change takes
// in the style, so that the change completes on it. The section's time is
// returned with it, for the caller to mark once the change is made.
func (s *State) foresee(sig Signals, st *style, dt float64) (Signals, float64, bool) {
	f := &s.foresight
	p := s.plan()
	if p != f.last {
		f.last, f.kick, f.snap, f.note, f.busy, f.apart, f.astray = p, 0, 0, 0, sig.Busy, 0, false
	}
	if p == nil || !s.lis.ear.have {
		f.heard, f.pulse, f.following = 0, 0, false
		return sig, 0, false
	}
	pos := s.lis.ear.now
	if len(p.curve) > 0 && !f.agrees(sig.Level, p.At(pos).Level, dt) {
		f.heard, f.pulse, f.following = 0, 0, false
		return sig, 0, false
	}
	// Cues are acted on as playback passes them. A plan that arrives part of
	// the way through a track, or a jump in position, starts from where
	// playback is, so the cues behind it are not all set off at once.
	from := f.heard
	if !f.following || pos < from || pos-from > 1 {
		from = pos
	}
	f.heard, f.following = pos, true
	if p.banded {
		sig = s.score(sig, p, from, pos, dt)
	}

	sig.Tension = math.Max(sig.Tension, p.Tension(pos))
	f.pulse *= math.Exp(-dt / 0.7)
	for _, kind := range []CueKind{CueDrop, CueClimax} {
		if c, ok := p.Next(from, kind); ok && c.At > from && c.At <= pos {
			f.pulse = math.Max(f.pulse, clamp01(0.6+0.4*c.Strength))
		}
	}
	sig.Release = math.Max(sig.Release, f.pulse)
	if c, ok := p.Next(pos, CueHush); ok && c.At-pos < 2 {
		sig.Hush = math.Max(sig.Hush, 0.6*(1-(c.At-pos)/2))
	}

	lead := math.Max(fadeMin, st.morph/2)
	c, ok := p.Next(pos, CueSection)
	due := ok && c.At-pos <= lead && c.At != f.section && s.age > sectionLead
	return sig, c.At, due
}
