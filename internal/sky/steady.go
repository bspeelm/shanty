package sky

import (
	"math"
	"slices"
)

// The measures of steadiness. Times are in seconds.
const (
	// steadyGaps is how many of the latest gaps between events are kept, and
	// steadyLeast how many there must be before the events count as steady.
	steadyGaps  = 8
	steadyLeast = 4
	// An event closer than steadyNear to the last one is the same event.
	steadyNear = 0.12
	// Events count as frequent from steadyFrom a second, and in full at
	// steadyFull a second.
	steadyFrom = 1.5
	steadyFull = 2.0
	// The gaps may differ from their median by a mean share of steadyTight
	// and still count as regular in full, which is what whole frames add to
	// an even beat, and by steadyLoose before they stop counting as regular.
	steadyTight = 0.1
	steadyLoose = 0.25
	// steadyLate is how many median gaps may pass without an event before
	// the run of events is over.
	steadyLate = 1.75
	// steadyRise and steadyFall are how long the steadiness takes to follow
	// the events up and down.
	steadyRise = 4.0
	steadyFall = 1.0
	// steadyKick is the least kick that counts as an event, and steadyEdge
	// how far it must rise in one frame.
	steadyKick = 0.5
	steadyEdge = 0.2
	// steadyFresh is the rise of the release in one frame that starts the
	// steadiness again.
	steadyFresh = 0.3
	// steadyPhase is the share of the beat's hold on a parameter that is
	// given up, to the breath, for music that is steady in full.
	steadyPhase = 0.8
	// pulseAttack is how many frames a pulse on the beat takes to rise for
	// music that is steady in full, and pulseFrame the length of a frame.
	pulseAttack = 3
	pulseFrame  = 1.0 / 30
	// steadyTurn is the share of the turn a warp makes on each beat that is
	// given up for music that is steady in full. turnSettle is how long a
	// step takes to settle, in seconds, and turnSlack how much longer it
	// takes for music that is steady in full, as a share.
	steadyTurn = 0.6
	turnSettle = 0.08
	turnSlack  = 1.0
)

// run is a run of events of one kind: whether it has begun, the time since
// its last event, and the times between its latest events.
type run struct {
	going bool
	since float64
	gaps  []float64
}

// step moves the run on by dt seconds, with an event on this frame or not,
// and returns how regular and frequent its events are, from 0 to 1.
func (r *run) step(event bool, dt float64) float64 {
	r.since += dt
	if event && (r.since >= steadyNear || !r.going) {
		if r.going {
			r.gaps = append(r.gaps, r.since)
		}
		if len(r.gaps) > steadyGaps {
			r.gaps = r.gaps[1:]
		}
		r.going, r.since = true, 0
	}
	if len(r.gaps) == 0 {
		return 0
	}
	sorted := slices.Clone(r.gaps)
	slices.Sort(sorted)
	median := sorted[len(sorted)/2]
	if r.since > steadyLate*median {
		r.gaps, r.going = r.gaps[:0], false
		return 0
	}
	if len(r.gaps) < steadyLeast {
		return 0
	}
	apart := 0.0
	for _, g := range r.gaps {
		apart += math.Abs(g-median) / median / float64(len(r.gaps))
	}
	often := clamp01((1/median - steadyFrom) / (steadyFull - steadyFrom))
	return often * clamp01((steadyLoose-apart)/(steadyLoose-steadyTight))
}

// steadiness follows how regularly and how often the picture has been moved
// on the beat. It follows two runs of events: the frames on which a trusted
// beat begins, and the frames on which a kick rises.
type steadiness struct {
	beat         int
	kick         float64
	beats, kicks run
	level        float64
}

// restart forgets the events so far and sets the steadiness to 0.
func (h *steadiness) restart() {
	h.beats.gaps, h.kicks.gaps, h.level = h.beats.gaps[:0], h.kicks.gaps[:0], 0
}

// step takes the signals of a frame dt seconds after the last and returns
// the steadiness, from 0 to 1. It rises over steadyRise seconds while the
// beats or the kicks arrive regularly at steadyFrom a second or more, and
// falls over steadyFall seconds when they are slower, irregular or have
// stopped.
func (h *steadiness) step(sig Signals, dt float64) float64 {
	onBeat := sig.onTheBeat() && sig.Beat != h.beat
	onKick := sig.Kick >= steadyKick && sig.Kick > h.kick+steadyEdge
	h.beat, h.kick = sig.Beat, sig.Kick
	target := math.Max(h.beats.step(onBeat, dt), h.kicks.step(onKick, dt))
	tau := steadyRise
	if target < h.level {
		tau = steadyFall
	}
	h.level = glide(h.level, target, dt, tau)
	return h.level
}
