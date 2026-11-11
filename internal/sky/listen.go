// Package sky draws the visualiser. It turns readings of the audio into
// signals, runs a scene of components driven by those signals, and renders the
// result as lines of text. It does no I/O, and everything it draws follows
// from a seed and the readings it is given.
package sky

import (
	"math"
	"time"
)

// Reading is one reading of mpv's measuring filter. The filter keeps running
// totals that restart every Epoch samples, when a track starts and after a
// seek.
type Reading struct {
	// Samples is how many samples per channel the totals cover.
	Samples float64
	// Channels holds the totals for each channel.
	Channels []Channel
	// Rate is the sample rate, and Scale is the value of a full-scale sample
	// in the units of Channel.Difference.
	Rate  int
	Scale float64
	// Epoch is the number of samples after which the totals restart.
	Epoch float64
	// Position is the playback position when the reading was taken, and At is
	// the time on the clock then.
	Position time.Duration
	At       time.Time
}

// Channel is the running totals for one channel.
type Channel struct {
	// Loudness is the RMS level in dB relative to full scale.
	Loudness float64
	// Difference is the RMS of the difference between neighbouring samples.
	Difference float64
	// Crossings is the number of zero crossings.
	Crossings float64
}

// slice is what the audio held between two readings, placed at the point in
// the track it describes. Every field but balance runs from 0 to 1; balance
// runs from -1, all left, to 1, all right.
type slice struct {
	at                                float64
	level, bass, treble, air, balance float64
}

// earLead is the assumed distance in seconds between what the filter measures
// and what is heard, until a reading from the start of a track measures it.
const earLead = 0.27

// The two frequencies the two-band split is built on, in hertz.
const (
	bassHz   = 80
	trebleHz = 6000
)

// ear turns readings into slices and says which of them are being heard.
type ear struct {
	last   Reading
	have   bool
	epochs int
	lead   float64
	// now is the position being heard and taken is how far take has reached,
	// both in seconds into the track. pos and at are the position and clock
	// time of the last reading.
	now, taken float64
	pos        float64
	at         time.Time
	slices     []slice
}

// hear takes one reading. A fall in the sample count is an epoch ending when
// the count was close to the epoch and playback moved on normally; any other
// fall is a new track or a seek, and starts again.
func (e *ear) hear(r Reading) {
	if e.lead == 0 {
		e.lead = earLead
	}
	prev, had := e.last, e.have
	e.last, e.have = r, r.Rate > 0 && len(r.Channels) > 0
	e.now = r.Position.Seconds()
	e.pos, e.at = e.now, r.At
	if !e.have {
		return
	}
	if !had || r.Rate != prev.Rate || len(r.Channels) != len(prev.Channels) {
		e.restart()
		return
	}
	if r.Samples < prev.Samples {
		moved := (r.Position - prev.Position).Seconds()
		if prev.Samples > r.Epoch-float64(r.Rate)/2 && moved >= 0 && moved < 0.5 {
			e.epochs++
		} else {
			e.restart()
		}
		return
	}
	if r.Samples == prev.Samples {
		return
	}
	e.add(prev, r)
}

func (e *ear) restart() {
	e.epochs, e.slices, e.taken = 0, nil, 0
}

// add works out the slice between two readings in the same epoch.
func (e *ear) add(prev, r Reading) {
	n1, n2 := prev.Samples, r.Samples
	dn := n2 - n1
	rate := float64(r.Rate)
	scale := r.Scale
	if scale <= 0 {
		scale = 1
	}
	var energy, diff, cross float64
	powers := make([]float64, len(r.Channels))
	for i, c := range r.Channels {
		p := prev.Channels[i]
		powers[i] = math.Max(0, (n2*power(c.Loudness)-n1*power(p.Loudness))/dn)
		d2, d1 := c.Difference/scale, p.Difference/scale
		energy += powers[i]
		diff += math.Max(0, (n2*d2*d2-n1*d1*d1)/dn)
		cross += math.Max(0, (c.Crossings-p.Crossings)/dn)
	}
	k := float64(len(r.Channels))
	energy, diff, cross = energy/k, diff/k, cross/k

	s := measure(energy, diff, cross, rate)
	if len(powers) > 1 && powers[0]+powers[1] > 0 {
		s.balance = (powers[1] - powers[0]) / (powers[0] + powers[1])
	}

	end := (float64(e.epochs)*r.Epoch + n2) / rate
	half := dn / rate / 2
	if lead := end - e.now; lead >= 0 && lead <= 2 {
		e.lead, s.at = lead, end-half
	} else {
		s.at = e.now + e.lead - half
	}
	kept := e.slices[:0:0]
	for _, old := range e.slices {
		if old.at >= e.now-1 {
			kept = append(kept, old)
		}
	}
	e.slices = append(kept, s)
}

// measure returns the slice for audio with the given mean power, mean squared
// difference between neighbouring samples relative to full scale, and zero
// crossings per sample, at the given sample rate. The two-band split divides
// the power between the low end and the high end.
func measure(energy, diff, cross, rate float64) slice {
	gb, gt := band(bassHz, rate), band(trebleHz, rate)
	treble := math.Min(energy, math.Max(0, (diff-energy*gb)/(gt-gb)))
	return slice{
		level:  levelOf(energy, -50, -5),
		bass:   levelOf(energy-treble, -50, -5),
		treble: levelOf(treble, -45, -12),
		air:    math.Min(cross/0.3, 1),
	}
}

// take sets the position being heard for a frame drawn at the time now, and
// returns the slices within half a step of it that have not been returned
// before: the loudest level and bands among them, and their mean air and
// balance. With none new it returns the newest slice already heard, and false
// when there is none at all.
//
// The position is the last reading's plus the time since it was taken, so a
// reading or a frame that waited in line is not drawn late. Without clock
// times the position moves on by step instead.
func (e *ear) take(step float64, now time.Time) (slice, bool) {
	switch {
	case !e.have:
	case !now.IsZero() && !e.at.IsZero():
		e.now = math.Max(e.now, e.pos+now.Sub(e.at).Seconds())
	default:
		e.now += step
	}
	until := e.now + step/2
	var out slice
	n := 0
	for _, s := range e.slices {
		if s.at > e.taken && s.at <= until {
			out.level = math.Max(out.level, s.level)
			out.bass = math.Max(out.bass, s.bass)
			out.treble = math.Max(out.treble, s.treble)
			out.air += s.air
			out.balance += s.balance
			n++
		}
	}
	if n > 0 {
		out.air /= float64(n)
		out.balance /= float64(n)
		e.taken = until
		return out, true
	}
	for i := len(e.slices) - 1; i >= 0; i-- {
		if e.slices[i].at <= until {
			return e.slices[i], true
		}
	}
	return slice{}, false
}

// band is the ratio of difference energy to energy for a sine at f hertz.
func band(f, rate float64) float64 {
	s := 2 * math.Sin(math.Pi*f/rate)
	return s * s
}

// power converts a level in dB to mean power. Silence is zero.
func power(db float64) float64 {
	if math.IsInf(db, -1) || math.IsNaN(db) {
		return 0
	}
	return math.Pow(10, db/10)
}

// levelOf maps a mean power to 0 at floor dB and below, rising evenly to 1 at
// ceiling dB and above.
func levelOf(energy, floor, ceiling float64) float64 {
	if energy <= 0 {
		return 0
	}
	db := 10 * math.Log10(energy)
	return clamp01((db - floor) / (ceiling - floor))
}

func clamp01(v float64) float64 { return math.Min(math.Max(v, 0), 1) }
