package tui

import (
	"math"
	"time"
)

// Heard is one reading of the audio that mpv's measuring filter took. The
// filter keeps running totals, so two readings in a row describe the audio
// between them.
type Heard struct {
	// Samples is how many samples per channel the filter has measured since
	// the track started or was last seeked.
	Samples float64
	// Loudness is the RMS level of all of those samples, in dB relative to
	// full scale.
	Loudness float64
	// Rate is the sample rate, in samples per second.
	Rate int
	// Position is the playback position, the point being heard, when the
	// reading was taken.
	Position time.Duration
}

// earLead is how far ahead of the speakers the filter is assumed to measure,
// in seconds, until a reading from the start of a track says otherwise. It is
// mpv's default 200 ms audio buffer plus the output latency measured on
// PipeWire.
const earLead = 0.27

// earKeep is how long a point is kept after it has been heard, in seconds.
const earKeep = 1.0

// A point is how loud the audio is at one point in the track: level runs from
// 0 for silence to 1 for full scale, and at is in seconds from the start.
type point struct {
	at, level float64
}

// ear turns readings into marks placed at the point in the track they
// describe, and says which one is being heard.
type ear struct {
	last Heard
	have bool
	// lead is how far the filter measures ahead of what is heard, in seconds.
	lead float64
	// now is the playback position of the latest reading, and taken is the
	// position take last reached, both in seconds.
	now, taken float64
	marks      []point
}

// hear takes one reading and returns the ear with the audio since the
// previous reading marked.
//
// The sample count gives the point in the track the filter has reached, when
// counting started at the beginning of the track. When it started at a seek,
// the count is smaller than the position, and the point is the position plus
// the lead learned earlier instead.
func (e ear) hear(h Heard) ear {
	if e.lead == 0 {
		e.lead = earLead
	}
	e.now = h.Position.Seconds()
	prev, had := e.last, e.have
	e.last, e.have = h, true
	if h.Rate <= 0 {
		e.have = false
		return e
	}
	if !had || h.Samples < prev.Samples || h.Rate != prev.Rate {
		e.marks, e.taken = nil, 0
		return e
	}
	if h.Samples == prev.Samples {
		return e
	}
	rate := float64(h.Rate)
	end := h.Samples / rate
	half := (h.Samples - prev.Samples) / rate / 2
	at := end - half
	if lead := end - e.now; lead >= 0 && lead <= 2 {
		e.lead = lead
	} else {
		at = e.now + e.lead - half
	}
	energy := (h.Samples*power(h.Loudness) - prev.Samples*power(prev.Loudness)) /
		(h.Samples - prev.Samples)

	kept := e.marks[:0:0]
	for _, m := range e.marks {
		if m.at >= e.now-earKeep {
			kept = append(kept, m)
		}
	}
	e.marks = append(kept, point{at: at, level: levelOf(energy)})
	return e
}

// take moves the point being heard on by step seconds, the length of a frame,
// and returns the loudest mark that falls within half a frame of it and has
// not been returned before, so that a short peak between two frames is still
// drawn and is drawn on the frame nearest to it. When there is no such mark it
// returns the newest one already returned, and false when there is none at
// all. The next reading sets the point being heard to the position mpv
// reports.
func (e ear) take(step float64) (ear, float64, bool) {
	if e.have {
		e.now += step
	}
	until := e.now + step/2
	level, found := 0.0, false
	for _, m := range e.marks {
		if m.at > e.taken && m.at <= until {
			level, found = math.Max(level, m.level), true
		}
	}
	if found {
		e.taken = until
		return e, level, true
	}
	for i := len(e.marks) - 1; i >= 0; i-- {
		if e.marks[i].at <= until {
			return e, e.marks[i].level, true
		}
	}
	return e, 0, false
}

// power converts a level in dB to mean power. Negative infinity, which is
// silence, is zero.
func power(db float64) float64 {
	if math.IsInf(db, -1) || math.IsNaN(db) {
		return 0
	}
	return math.Pow(10, db/10)
}

// levelOf maps a mean power to 0 at -50 dB and below, rising evenly to 1 at
// -5 dB and above.
func levelOf(energy float64) float64 {
	if energy <= 0 {
		return 0
	}
	db := 10 * math.Log10(energy)
	return math.Min(math.Max((db+50)/45, 0), 1)
}

// sound is what the visualiser draws from. loud follows the level closely,
// pulse jumps when the level rises sharply above its recent average and dies
// away over a few frames, and average is that recent average. The zero sound
// draws the pattern the clock alone would draw.
type sound struct {
	loud, pulse, average float64
}

// follow moves the sound one frame on towards a level that was heard.
func (s sound) follow(level float64) sound {
	if level > s.loud {
		s.loud += (level - s.loud) * 0.6
	} else {
		s.loud += (level - s.loud) * 0.15
	}
	s.average += (level - s.average) * 0.04
	s.pulse *= 0.8
	if rise := (level - s.average - 0.08) * 4; rise > s.pulse {
		s.pulse = math.Min(rise, 1)
	}
	return s
}

// fade moves the sound one frame on with nothing heard, so that it dies away
// rather than holding the last level.
func (s sound) fade() sound {
	s.loud *= 0.85
	s.pulse *= 0.8
	s.average *= 0.96
	return s
}
