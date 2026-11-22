package sky

import (
	"math"
	"testing"
)

// beaten steps h through secs seconds of a trusted beat at bpm, starting at
// the time from, at 30 frames a second, and returns the steadiness after
// each frame. With kicks set, each beat also carries a kick that dies away.
func beaten(h *steadiness, bpm, from, secs float64, kicks bool) []float64 {
	var out []float64
	dt := 1.0 / 30
	kick := 0.0
	for at := from; at < from+secs; at += dt {
		beats := at * bpm / 60
		sig := Signals{Tempo: bpm, Sure: 1, Beat: int(beats), Phase: beats - math.Floor(beats)}
		kick *= math.Exp(-dt / kickFall)
		if kicks && sig.Phase*60/bpm < dt {
			kick = gridKick
		}
		sig.Kick = kick
		out = append(out, h.step(sig, dt))
	}
	return out
}

// silent steps h through secs seconds with no beat and no kick.
func silent(h *steadiness, secs float64) float64 {
	level := h.level
	for at := 0.0; at < secs; at += 1.0 / 30 {
		level = h.step(Signals{}, 1.0/30)
	}
	return level
}

func TestSteadinessRisesOverSeveralSecondsOfAFastSteadyBeat(t *testing.T) {
	for _, bpm := range []float64{123, 128, 140, 174} {
		var h steadiness
		got := beaten(&h, bpm, 0, 30, false)
		if early := got[2*30]; early > 0.3 {
			t.Errorf("%v BPM: steadiness was %.2f after two seconds, want at most 0.3", bpm, early)
		}
		if late := got[len(got)-1]; late < 0.9 {
			t.Errorf("%v BPM: steadiness was %.2f after thirty seconds, want at least 0.9", bpm, late)
		}
	}
}

func TestSteadinessRisesOnKicksWhereNoBeatIsTrusted(t *testing.T) {
	var h steadiness
	level, dt, kick := 0.0, 1.0/30, 0.0
	for at := 0.0; at < 30; at += dt {
		kick *= math.Exp(-dt / kickFall)
		if math.Mod(at, 60.0/128) < dt {
			kick = gridKick
		}
		level = h.step(Signals{Kick: kick}, dt)
	}
	if level < 0.9 {
		t.Errorf("steadiness was %.2f after thirty seconds of kicks at 128 a minute, want at least 0.9", level)
	}
}

func TestSteadinessStaysAtNothingForABeatFeltAt82(t *testing.T) {
	var h steadiness
	got := beaten(&h, 82, 0, 30, true)
	for i, v := range got {
		if v != 0 {
			t.Fatalf("steadiness was %.3f at %.1f s of a beat at 82 a minute, want 0", v, float64(i)/30)
		}
	}
}

func TestSteadinessIsPartOfTheWayForABeatBetweenTheTwoRates(t *testing.T) {
	var h steadiness
	got := beaten(&h, 105, 0, 40, false)
	if v := got[len(got)-1]; v < 0.3 || v > 0.7 {
		t.Errorf("steadiness settled at %.2f for a beat at 105 a minute, want between 0.3 and 0.7", v)
	}
}

func TestSteadinessFallsInAGapAndTheFirstBeatsAfterItAreAtFullStrength(t *testing.T) {
	var h steadiness
	beaten(&h, 128, 0, 30, true)
	if left := silent(&h, 4); left > 0.1 {
		t.Fatalf("steadiness was %.2f after four seconds without a beat, want at most 0.1", left)
	}
	back := beaten(&h, 128, 34, 8, true)
	if first := back[30]; first > 0.1 {
		t.Errorf("steadiness was %.2f a second after the beat came back, want at most 0.1", first)
	}
	if later := back[len(back)-1]; later < 0.4 {
		t.Errorf("steadiness was %.2f eight seconds after the beat came back, want at least 0.4", later)
	}
}

func TestSteadinessIsLowForEventsThatComeIrregularly(t *testing.T) {
	var h steadiness
	level, dt, kick, next := 0.0, 1.0/30, 0.0, 0.0
	gaps := []float64{0.2, 0.55, 0.3, 0.7, 0.25, 0.45}
	n := 0
	for at := 0.0; at < 30; at += dt {
		kick *= math.Exp(-dt / kickFall)
		if at >= next {
			kick = gridKick
			next += gaps[n%len(gaps)]
			n++
		}
		level = math.Max(level, h.step(Signals{Kick: kick}, dt))
	}
	if level > 0.2 {
		t.Errorf("steadiness reached %.2f for kicks at uneven gaps, want at most 0.2", level)
	}
}

func TestACutStartsTheSteadinessAgain(t *testing.T) {
	p, _, readings, due := floorCase(t, 128)
	if len(due) == 0 {
		t.Fatalf("the plan has no cue to cut to: %v", p.Cues)
	}
	back := due[len(due)-1]
	s := New(3)
	s.Foresee("track", p)
	var before, after, later float64
	last := 0.0
	for i, r := range readings {
		pos := r.Position.Seconds()
		s.Hear(r)
		dt := pos - last
		if i == 0 {
			dt = 1.0 / 30
		}
		s.Step(40, 10, dt, r.At, "track")
		last = pos
		now := s.lis.ear.now
		switch {
		case now > 20 && now < 23:
			before = math.Max(before, s.Signals().Steady)
		case now >= back && now < back+1:
			after = math.Max(after, s.Signals().Steady)
		case now >= back+20:
			later = math.Max(later, s.Signals().Steady)
		}
	}
	if before < 0.7 {
		t.Errorf("steadiness reached %.2f in the seconds before the kicks stopped, want at least 0.7", before)
	}
	if after > 0.1 {
		t.Errorf("steadiness was %.2f in the second after the cut at %.1f s, want at most 0.1", after, back)
	}
	if later < 0.8 {
		t.Errorf("steadiness reached %.2f twenty seconds after the cut, want at least 0.8", later)
	}
}
