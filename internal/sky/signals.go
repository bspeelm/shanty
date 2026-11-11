package sky

import (
	"math"
	"time"
)

// Signals is what the music is doing on one frame. Every field but Tempo,
// Beat, Section and Balance runs from 0 to 1.
type Signals struct {
	// Level, Bass and Treble follow how loud the whole, the low end and the
	// high end are. Air is how noisy or hissing the sound is.
	Level, Bass, Treble, Air float64
	// Balance runs from -1, all left, to 1, all right.
	Balance float64
	// Kick and Snap jump when the low end or the high end rises sharply, and
	// die away over a few frames.
	Kick, Snap float64
	// Busy is how many hits there have been over the last few seconds.
	Busy float64
	// Tempo is the beats per minute, or 0 while it is not known. Phase is how
	// far through the current beat playback is, and Beat counts beats.
	Tempo float64
	Phase float64
	Beat  int
	// Section is set on the frame where the music changes character.
	Section bool
	// Tension rises while the music builds over several seconds. Release
	// jumps when a build lets go or a drop lands, and dies away over a
	// second or two. Hush is how far the level is below the track's running
	// average. Breath rises and falls once a phrase.
	Tension, Release, Hush, Breath float64
}

// The sources a scene can route to a parameter, in the order Signals.source
// returns them.
const (
	srcLevel = iota
	srcBass
	srcTreble
	srcAir
	srcBalance
	srcKick
	srcSnap
	srcBusy
	srcPhase
	srcBar
	srcSlow
	srcSlower
	srcTension
	srcRelease
	srcHush
	sources
)

// source returns one of the signals as a number from 0 to 1, with two slow
// oscillators driven by clock.
func (s Signals) source(i int, clock float64) float64 {
	switch i {
	case srcLevel:
		return s.Level
	case srcBass:
		return s.Bass
	case srcTreble:
		return s.Treble
	case srcAir:
		return s.Air
	case srcBalance:
		return (s.Balance + 1) / 2
	case srcKick:
		return s.Kick
	case srcSnap:
		return s.Snap
	case srcBusy:
		return s.Busy
	case srcPhase:
		return 1 - s.Phase
	case srcBar:
		return (float64(s.Beat%4) + s.Phase) / 4
	case srcSlow:
		return 0.5 + 0.5*math.Sin(clock*2*math.Pi/37)
	case srcSlower:
		return 0.5 + 0.5*math.Sin(clock*2*math.Pi/113)
	case srcTension:
		return s.Tension
	case srcRelease:
		return s.Release
	case srcHush:
		return s.Hush
	}
	return 0
}

// listener follows the slices an ear hands over and keeps the signals.
type listener struct {
	ear      ear
	sig      Signals
	bassAvg  float64
	trebAvg  float64
	hits     []float64
	clock    float64
	short    [3]float64
	long     [3]float64
	quiet    float64
	tempo    tempo
	history  []float64
	historyT float64
	meter    characterMeter
	char     Character
	shape    tensionMeter
}

// historyLen is how many levels the history keeps, one every historyStep
// seconds.
const (
	historyLen  = 128
	historyStep = 0.1
)

// step moves the signals on by dt seconds, for a frame drawn at now.
func (l *listener) step(dt float64, now time.Time) Signals {
	l.clock += dt
	s := &l.sig
	s.Section = false
	in, heard := l.ear.take(dt, now)
	if !heard {
		in = slice{}
	}
	s.Level = follow(s.Level, in.level, 0.6, 0.12)
	s.Bass = follow(s.Bass, in.bass, 0.7, 0.15)
	s.Treble = follow(s.Treble, in.treble, 0.8, 0.25)
	s.Air = follow(s.Air, in.air, 0.3, 0.1)
	s.Balance += (in.balance - s.Balance) * 0.2

	l.bassAvg += (in.bass - l.bassAvg) * 0.05
	l.trebAvg += (in.treble - l.trebAvg) * 0.08
	kick := clamp01((in.bass - l.bassAvg - 0.08) * 4)
	snap := clamp01((in.treble - l.trebAvg - 0.08) * 4)
	s.Kick = math.Max(s.Kick*0.82, kick)
	s.Snap = math.Max(s.Snap*0.75, snap)

	onset := math.Max(kick, snap)
	if onset > 0.3 {
		l.hits = append(l.hits, l.clock)
	}
	for len(l.hits) > 0 && l.hits[0] < l.clock-4 {
		l.hits = l.hits[1:]
	}
	s.Busy = follow(s.Busy, math.Min(float64(len(l.hits))/24, 1), 0.1, 0.05)

	s.Tempo, s.Phase, s.Beat = l.tempo.step(onset, dt)
	l.char = l.meter.step(*s, l.tempo.sure, kick, onset, dt)
	l.shape.step(s, dt)
	l.section(dt)
	l.remember(dt)
	return *s
}

// section sets Section when the last two seconds differ sharply from the last
// twenty. It waits twelve seconds after one before allowing the next.
func (l *listener) section(dt float64) {
	now := [3]float64{l.sig.Level, l.sig.Bass, l.sig.Air}
	dist := 0.0
	for i, v := range now {
		l.short[i] += (v - l.short[i]) * dt / 2
		l.long[i] += (v - l.long[i]) * dt / 20
		dist += math.Abs(l.short[i] - l.long[i])
	}
	l.quiet += dt
	if dist > 0.2 && l.quiet > 12 {
		l.sig.Section, l.quiet = true, 0
	}
}

// remember keeps the level history that terrain, epicycles and the sort draw.
func (l *listener) remember(dt float64) {
	l.historyT += dt
	for l.historyT >= historyStep {
		l.historyT -= historyStep
		l.history = append(l.history, l.sig.Level)
		if len(l.history) > historyLen {
			l.history = l.history[1:]
		}
	}
}

// follow moves v towards target, quickly when it rises and slowly when it
// falls.
func follow(v, target, up, down float64) float64 {
	if target > v {
		return v + (target-v)*up
	}
	return v + (target-v)*down
}
