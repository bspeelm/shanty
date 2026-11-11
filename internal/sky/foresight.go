package sky

import "math"

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

// foresee shapes a frame's signals by the plan of the track playing, when
// there is one, and reports whether a scene change is due so that it lands
// on the next section. With a plan:
//   - tension follows the plan's builds, rising before the drop arrives;
//   - a drop or the climax sets off a release as it is heard;
//   - the light begins to settle two seconds before a hush.
//
// A change is due when the next section is as far ahead as a change takes
// in the style, so that the change completes on it. The section's time is
// returned with it, for the caller to mark once the change is made.
func (s *State) foresee(sig Signals, st *style, dt float64) (Signals, float64, bool) {
	f := &s.foresight
	p := s.plan()
	if p == nil || !s.lis.ear.have {
		f.heard, f.pulse, f.following = 0, 0, false
		return sig, 0, false
	}
	pos := s.lis.ear.now
	// Cues are acted on as playback passes them. A plan that arrives part of
	// the way through a track, or a jump in position, starts from where
	// playback is, so the cues behind it are not all set off at once.
	from := f.heard
	if !f.following || pos < from || pos-from > 1 {
		from = pos
	}
	f.heard, f.following = pos, true

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
