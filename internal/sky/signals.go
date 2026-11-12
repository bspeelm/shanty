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
	// Kick and Snap jump when the low end or the high end rises sharply for
	// how it has been rising over the last few seconds, and die away over a
	// few frames.
	Kick, Snap float64
	// Busy is how many onsets there have been a second over the last few
	// seconds.
	Busy float64
	// Tempo is the beats per minute, or 0 while it is not known. Phase is how
	// far through the current beat playback is, and Beat counts beats. Sure
	// is the confidence in the tempo: it rises while estimates of the tempo
	// agree and falls while they wander.
	Tempo float64
	Phase float64
	Beat  int
	Sure  float64
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
		return s.onBeat(1 - s.Phase)
	case srcBar:
		return s.onBeat((float64(s.Beat%4) + s.Phase) / 4)
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

// beatWeight is how far what follows the beat follows it rather than the
// breath: the confidence in the tempo while one is reported, and 0 otherwise.
func (s Signals) beatWeight() float64 {
	if s.Tempo == 0 {
		return 0
	}
	return s.Sure
}

// blendBeat blends v, a value that follows the beat, towards other by how
// little the tempo is trusted.
func (s Signals) blendBeat(v, other float64) float64 {
	w := s.beatWeight()
	return w*v + (1-w)*other
}

// onBeat blends v, a value that follows the beat, towards the breath by how
// little the tempo is trusted.
func (s Signals) onBeat(v float64) float64 { return s.blendBeat(v, s.Breath) }

// beatTrust is the beat weight from which what steps on the beat follows it.
const beatTrust = 0.5

// onTheBeat reports whether the beat is trusted enough for what steps on the
// beat to follow it.
func (s Signals) onTheBeat() bool { return s.beatWeight() >= beatTrust }

// listener follows the slices an ear hands over and keeps the signals.
type listener struct {
	ear  ear
	sig  Signals
	low  riseMeter
	high riseMeter
	// hits holds the times of the onsets in the last busySpan seconds, and
	// hit whether the last slice was one.
	hits  []float64
	hit   bool
	clock float64
	short [3]float64
	long  [3]float64
	quiet float64
	tempo tempo
	// restarts is the ear's count of restarts when the tempo last started
	// again.
	restarts int
	history  []float64
	historyT float64
	meter    characterMeter
	char     Character
	shape    tensionMeter
}

// Busy counts the onsets over the last busySpan seconds, or over the time
// listened when that is shorter, and is 1 at busyFull onsets a second.
const (
	busySpan = 4.0
	busyFull = 3.0
)

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

	var kick, snap float64
	for _, f := range l.ear.fresh {
		k, h := l.low.step(f.bass, f.at), l.high.step(f.treble, f.at)
		kick, snap = math.Max(kick, k), math.Max(snap, h)
		hit := math.Max(k, h) > 0.3
		if hit && !l.hit {
			l.hits = append(l.hits, l.clock)
		}
		l.hit = hit
	}
	s.Kick = math.Max(s.Kick*0.82, kick)
	s.Snap = math.Max(s.Snap*0.75, snap)

	onset := math.Max(kick, snap)
	for len(l.hits) > 0 && l.hits[0] < l.clock-busySpan {
		l.hits = l.hits[1:]
	}
	rate := float64(len(l.hits)) / math.Min(math.Max(l.clock, 1), busySpan)
	s.Busy = follow(s.Busy, math.Min(rate/busyFull, 1), 0.1, 0.05)

	if l.ear.restarts != l.restarts {
		l.restarts = l.ear.restarts
		l.tempo.restart()
	}
	s.Tempo, s.Phase, s.Beat = l.tempo.hear(kick, snap, dt)
	s.Sure = l.tempo.conf
	l.char = l.meter.step(*s, s.Sure, kick, onset, dt)
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

// riseMeter finds onsets in one band. A slice's rise is how far the band's
// level climbed over it and the slice before. The meter keeps the running
// mean and mean deviation of the rise over about riseSpan seconds, with each
// rise counted as at most riseKeep deviations above the mean, and scores
// each rise by how many deviations it stands above that mean. An onset is
// also scaled by the size of the rise, from riseLeast to riseLoud, and by
// how sharp it is: the share of the climb over the last riseSlices slices
// that the rise makes up, from riseSoft to riseSharp.
type riseMeter struct {
	last, mean, dev float64
	// ups holds how far the level climbed in each of the last riseSlices
	// slices, the newest at next-1.
	ups  [riseSlices]float64
	next int
	// at is the time of the last slice, in seconds into the track.
	at    float64
	ready bool
}

// The span of the running statistics in seconds, the least deviation a rise
// is measured against, the scores at which an onset starts and reaches full
// strength, and the most deviations above the mean a rise counts as in the
// statistics.
const (
	riseSpan  = 2.0
	riseFloor = 0.015
	riseFrom  = 1.5
	riseFull  = 4.0
	riseKeep  = 1.0
)

// The rises at which an onset starts and reaches full strength, the number
// of slices the sharpness is measured over, and the sharpnesses at which an
// onset starts and reaches full strength.
const (
	riseLeast  = 0.06
	riseLoud   = 0.14
	riseSlices = 6
	riseSoft   = 0.5
	riseSharp  = 0.8
)

// step takes the band's level in a slice placed at seconds into the track,
// and returns the strength of an onset in it, from 0 to 1.
func (r *riseMeter) step(v, at float64) float64 {
	dt := at - r.at
	r.at = at
	if !r.ready || math.Abs(dt) > 1 {
		r.last, r.ups, r.ready = v, [riseSlices]float64{}, true
		return 0
	}
	dt = math.Max(dt, 0)
	r.ups[r.next] = math.Max(v-r.last, 0)
	r.last = v
	rise := r.ups[r.next] + r.ups[(r.next+riseSlices-1)%riseSlices]
	r.next = (r.next + 1) % riseSlices
	climb := 0.0
	for _, u := range r.ups {
		climb += u
	}
	dev := math.Max(r.dev, riseFloor)
	z := (rise - r.mean) / dev
	k := 1 - math.Exp(-dt/riseSpan)
	kept := math.Min(rise, r.mean+riseKeep*dev)
	r.mean += (kept - r.mean) * k
	r.dev += (math.Abs(kept-r.mean) - r.dev) * k
	if rise < riseLeast {
		return 0
	}
	score := clamp01((z - riseFrom) / (riseFull - riseFrom))
	size := clamp01((rise - riseLeast) / (riseLoud - riseLeast))
	sharp := clamp01((rise/climb - riseSoft) / (riseSharp - riseSoft))
	return score * size * sharp
}
