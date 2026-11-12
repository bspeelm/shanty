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
	// cut is set on the frame a cut is due, and cutAt is the time of the
	// cue the last cut was made for.
	cut   bool
	cutAt float64
	// near is set while a cut is less than cutClear away, and cutFade is
	// how long the cut being made takes.
	near    bool
	cutFade float64
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
	// beatReach is how near a beat a kick must fall to be set off on it,
	// and gridKick the strength of a kick set off on a beat.
	beatReach = 0.06
	gridKick  = 0.9
	// playedLeast is the strength of a kick in the quietest part of a track
	// carried by its arrangement, against 1 in its most intense.
	playedLeast = 0.35
	// Kicks fade by the share of their section's beats that carry a kick:
	// none at fadeFrom, and in full at fadeOver above it. Every beat of a
	// section where that share is at least fillFrom is given a kick. A
	// section shorter than sectionLeast is measured over that length.
	fadeFrom     = 0.45
	fadeOver     = 0.15
	fillFrom     = 0.6
	sectionLeast = 8
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
	planShare = 0.85
	// cutFade is how long a cut takes. A section is cut to when it is at
	// least cutStrength strong, or when the share of its beats that carry a
	// kick differs by cutTurn from the share before it.
	cutFade     = 0.35
	cutStrength = 0.5
	cutTurn     = 0.4
	// cutClear is how long before a cut no other change of scene starts.
	cutClear = 8
	// gritBehind is how far grit may trail drive in a character before the
	// music stops counting as led by grit. heavyAhead is how far ahead of
	// the position the plan's level is read for Heavy, and heavyGlide how
	// long Heavy takes to follow.
	gritBehind = 0.1
	heavyAhead = 2
	heavyGlide = 1.5
	// partGlide is how long Heavy and Weight take to reach a new part's,
	// and middling the intensity of a part that hits neither hard nor soft.
	partGlide = 0.15
	middling  = 0.75
	// silentWeight is the Weight of a part with no sound, under which the
	// picture is dark and still.
	silentWeight = -1.25
	// hardFade is how long a cut to a hard boundary takes.
	hardFade = 0.07
	// moodSpan is the stretch a mood is measured over, moodAhead how far
	// ahead of the position the frame's mood is taken, and drummedFrom the
	// share of beats with a kick from which a stretch counts as drummed.
	moodSpan    = 8
	moodAhead   = 0.5
	drummedFrom = 0.5
)

// drummed returns how fully the beats from a to b carry kicks, from 0 to 1:
// the strength of the kicks there for each beat, against the strength a
// kick on every beat would have at the track's usual kick strength. It is 0
// when there are no beats there.
func (p *Plan) drummed(a, b float64) float64 {
	beats := len(between(p.Beats, a, b))
	if beats == 0 {
		return 0
	}
	usual := p.kickStrength()
	if usual == 0 {
		return 0
	}
	sum := 0.0
	for _, h := range p.HitsBetween(a, b) {
		if h.Kind == HitKick {
			sum += math.Min(h.Strength, usual)
		}
	}
	return math.Min(sum/usual/float64(beats), 1)
}

// kickStrength returns the strength of the track's stronger kicks: the
// kick three quarters of the way up its kicks in order of strength.
func (p *Plan) kickStrength() float64 {
	if p.usualKick == 0 {
		var all []float64
		for _, h := range p.Hits {
			if h.Kind == HitKick {
				all = append(all, h.Strength)
			}
		}
		if len(all) == 0 {
			return 0
		}
		slices.Sort(all)
		p.usualKick = all[len(all)*3/4]
	}
	return p.usualKick
}

// mood returns the character of the stretch that starts at t. In a track
// carried by its arrangement it is the mood of the part playing. In any
// other it is the track's own where its beats carry kicks or the track has
// no beats at all, and a soft one, between calm and flowing, where they do
// not.
func (p *Plan) mood(t float64) Character {
	c := p.Character()
	if part, ok := p.partAt(t); ok && p.arranged {
		return part.mood()
	}
	if len(p.Beats) == 0 {
		return c
	}
	// A track led by grit keeps its character where its kicks are few.
	w := math.Max(clamp01(p.drummed(t, t+moodSpan)/drummedFrom), gritLed(c))
	soft := Character{Calm: 0.45, Drive: 0.05, Grit: 0.05, Flow: 0.45}
	a, b := c.axes(), soft.axes()
	mix := func(i int) float64 { return w*a[i] + (1-w)*b[i] }
	return Character{mix(0), mix(1), mix(2), mix(3)}
}

// mood returns the character a part is drawn for: calm at its quietest,
// an even blend as its intensity rises, and led by grit as far as it is
// heavy.
func (part Part) mood() Character {
	calm := Character{Calm: 0.55, Drive: 0.05, Grit: 0.05, Flow: 0.35}
	even := Character{Calm: 0.2, Drive: 0.3, Grit: 0.2, Flow: 0.3}
	heavy := Character{Calm: 0, Drive: 0.3, Grit: 0.6, Flow: 0.1}
	lerp := func(a, b Character, w float64) Character {
		x, y := a.axes(), b.axes()
		mix := func(i int) float64 { return (1-w)*x[i] + w*y[i] }
		return Character{mix(0), mix(1), mix(2), mix(3)}
	}
	return lerp(lerp(calm, even, clamp01((part.Intensity-0.4)/0.4)), heavy, part.Heavy)
}

// gritLed is how far a character is led by grit over drive, from 0 where
// drive is ahead by gritBehind to 1 where grit is ahead by as much.
func gritLed(c Character) float64 {
	return clamp01((c.Grit - c.Drive + gritBehind) / (2 * gritBehind))
}

// heavy follows how far the music is a loud wall of sound with no steady
// kick to carry it. With a trusted plan of a track carried by its
// arrangement it is how heavy the part playing is. Otherwise it is how loud
// the music is, heard and with a trusted plan planned for the seconds
// ahead, by how far its character is led by grit over drive.
func (s *State) heavy(dt float64) float64 {
	level, c := s.lis.meter.slow, s.lis.char
	if p := s.trusted(); p != nil {
		if part, ok := p.partAt(s.lis.ear.now); ok && p.arranged {
			s.weight = glide(s.weight, part.Heavy, dt, partGlide)
			return s.weight
		}
		level = planShare*p.At(s.lis.ear.now+heavyAhead).Level + (1-planShare)*level
		c = p.Character()
	}
	loud := clamp01((level - heavyFrom) / (heavyFull - heavyFrom))
	s.weight = glide(s.weight, loud*gritLed(c), dt, heavyGlide)
	return s.weight
}

// weigh follows how hard the part playing hits, for a trusted plan of a
// track carried by its arrangement: its intensity less middling, held
// between -0.5 and 0.25, or silentWeight for a part with no sound. It is 0
// for any other track.
func (s *State) weigh(dt float64) float64 {
	want := 0.0
	if p := s.trusted(); p != nil && p.arranged {
		if part, ok := p.partAt(s.lis.ear.now); ok {
			want = clamp(part.Intensity-middling, -0.5, 0.25)
			if part.Silent || part.Level <= quietLevel {
				want = silentWeight
			}
		}
	}
	s.bearing = glide(s.bearing, want, dt, partGlide)
	return s.bearing
}

// cutDue reports whether the cue c of a plan measured in bands is one to
// cut to: a drop, a strong section, or, in a track carried by its beat, a
// section where the kicks start or stop.
func (p *Plan) cutDue(c Cue) bool {
	if !p.banded {
		return false
	}
	if c.Kind == CueDrop || c.Strength >= cutStrength {
		return true
	}
	if p.arranged {
		return false
	}
	return math.Abs(p.drummed(c.At, c.At+moodSpan)-p.drummed(c.At-moodSpan, c.At)) >= cutTurn
}

// trusted returns the plan of the track playing when it was measured in
// bands and what is heard agrees with it, and nil otherwise.
func (s *State) trusted() *Plan {
	if p := s.plan(); p != nil && p.banded && !s.foresight.astray {
		return p
	}
	return nil
}

// character returns the character the frame is drawn for: the one heard, or
// with a trusted plan a blend of the mood of the stretch just ahead and the
// one heard. Until enough has been heard, it is the plan's mood alone.
func (s *State) character() Character {
	live := s.lis.char
	p := s.trusted()
	if p == nil {
		return live
	}
	mood := p.mood(s.lis.ear.now + moodAhead)
	if !s.lis.meter.settled() {
		return mood
	}
	a, b := mood.axes(), live.axes()
	mix := func(i int) float64 { return planShare*a[i] + (1-planShare)*b[i] }
	return Character{mix(0), mix(1), mix(2), mix(3)}
}

// onBeat reports whether t is within beatReach of a beat the plan is sure
// of.
func (p *Plan) onBeat(t float64) bool {
	i, _ := slices.BinarySearch(p.Beats, t)
	for _, k := range []int{i - 1, i} {
		if k >= 0 && k < len(p.Beats) && math.Abs(p.Beats[k]-t) <= beatReach {
			if beat, ok := p.BeatAt(p.Beats[k]); ok && beat.Confidence >= kickConfidence {
				return true
			}
		}
	}
	return false
}

// sectionShare returns the share of the beats of the section around t that
// have a kick within beatReach of them. A section runs from one drop or
// section cue to the next, and one shorter than sectionLeast is measured
// with the time either side of it, up to that length.
func (p *Plan) sectionShare(t float64) float64 {
	if p.shares == nil {
		p.edges = []float64{0}
		for _, c := range p.Cues {
			if (c.Kind == CueSection || c.Kind == CueDrop) && c.At > p.edges[len(p.edges)-1] {
				p.edges = append(p.edges, c.At)
			}
		}
		p.edges = append(p.edges, math.Max(p.Length, p.edges[len(p.edges)-1]))
		p.shares = make([]float64, len(p.edges)-1)
		for i := range p.shares {
			a, b := p.edges[i], p.edges[i+1]
			if short := sectionLeast - (b - a); short > 0 {
				a, b = a-short/2, b+short/2
			}
			beats := between(p.Beats, a, b)
			for _, at := range beats {
				if p.kicked(at) {
					p.shares[i] += 1 / float64(len(beats))
				}
			}
		}
	}
	i, _ := slices.BinarySearch(p.edges, t)
	if i >= len(p.edges) || p.edges[i] != t {
		i--
	}
	return p.shares[min(max(i, 0), len(p.shares)-1)]
}

// kicked reports whether a kick falls within beatReach of t.
func (p *Plan) kicked(t float64) bool {
	for _, h := range p.HitsBetween(t-beatReach, t+beatReach) {
		if h.Kind == HitKick {
			return true
		}
	}
	return false
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
	// What falls after the last frame's position and at or before this one's
	// is set off on this frame, the frame on which its sound is first heard.
	lo, hi := math.Nextafter(from, math.Inf(1)), math.Nextafter(pos, math.Inf(1))
	// A kick beside a beat of the grid is set off on the beat, at an even
	// strength, and every beat of a drummed section is given one.
	// In a track carried by its arrangement every kick is set off where it
	// falls, as strong as its part is intense.
	played := 0.0
	if part, ok := p.partAt(pos); ok && p.arranged {
		played = playedLeast + (1-playedLeast)*part.Intensity
	}
	for _, at := range between(p.Beats, lo, hi) {
		beat, ok := p.BeatAt(at)
		if !ok || beat.Confidence < kickConfidence || p.arranged {
			continue
		}
		if share := p.sectionShare(at); share >= fillFrom || p.kicked(at) {
			f.kick = math.Max(f.kick, gridKick*clamp01((share-fadeFrom)/fadeOver))
		}
	}
	// The other kicks fade out in a section that is not drummed.
	drummed := 1.0
	if len(p.Beats) > 0 {
		drummed = clamp01((p.sectionShare(pos) - fadeFrom) / fadeOver)
	}
	for _, h := range p.HitsBetween(lo, hi) {
		v := clamp01(0.35 + 0.65*h.Strength)
		switch h.Kind {
		case HitKick:
			switch {
			case p.arranged:
				f.kick = math.Max(f.kick, played*math.Max(v, gridKick))
			case !p.onBeat(h.At):
				f.kick = math.Max(f.kick, drummed*v)
			}
		case HitSnare:
			f.snap = math.Max(f.snap, v)
		case HitHat:
			f.snap = math.Max(f.snap, hatShare*v)
		}
	}
	f.note *= math.Exp(-dt / noteFall)
	for _, n := range p.NotesBetween(lo, hi) {
		f.note = math.Max(f.note, clamp01(2*n.Strength))
	}
	sig.Kick = f.kick * (0.7 + 0.3*sig.Bass)
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
//   - a plan measured in bands gives the beat, the hits and the busyness,
//     and sets cut on the frame a drop or a marked section is to be cut to.
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
	f.cut, f.near = false, false
	if p != f.last {
		f.last, f.kick, f.snap, f.note, f.busy, f.apart, f.astray = p, 0, 0, 0, sig.Busy, 0, false
		f.cut, f.cutAt = false, 0
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

	// The next drop or section worth a cut is cut to so that the cut is half
	// done as it is heard.
	// A cut is begun on the frame nearest to half its length before its
	// cue, and a cue this frame has stepped past is cut to as well.
	f.near = false
	for _, c := range p.Cues {
		if c.Kind != CueDrop && c.Kind != CueSection || c.At <= from || c.At == f.cutAt || !p.cutDue(c) {
			continue
		}
		fade := cutFade
		if p.hardAt(c.At) {
			fade = hardFade
		}
		switch {
		case c.At-fade/2 <= pos+dt/2:
			f.cut, f.cutAt, f.section, f.cutFade = true, c.At, c.At, fade
		case c.At-pos <= cutClear:
			f.near = true
		}
		if c.At > pos+cutClear {
			break
		}
	}

	lead := math.Max(fadeMin, st.morph/2)
	c, ok := p.Next(pos, CueSection)
	due := ok && c.At-pos <= lead && c.At != f.section && s.age > sectionLead && !p.cutDue(c)
	return sig, c.At, due
}
