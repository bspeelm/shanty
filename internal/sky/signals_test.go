package sky

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// onsetTrain is n frames at dt seconds of onset strength with a spike every
// period seconds.
func onsetTrain(n int, dt, period float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		if math.Mod(float64(i)*dt, period) < dt {
			out[i] = 1
		}
	}
	return out
}

func TestTheTempoOfAClickTrackIsFound(t *testing.T) {
	for _, bpm := range []float64{90, 120, 140} {
		got, sure := periodOf(onsetTrain(tempoWindow, 1.0/30, 60/bpm), 1.0/30)
		if math.Abs(got-bpm) > 3 || sure < 0.3 {
			t.Errorf("a click at %v BPM was heard as %.1f BPM, %.2f sure", bpm, got, sure)
		}
	}
}

func TestBeatsAreCountedOnceTheTempoIsKnown(t *testing.T) {
	var tp tempo
	beats := 0
	for _, o := range onsetTrain(600, 1.0/30, 0.5) {
		_, _, beats = tp.step(o, 1.0/30)
	}
	// Twenty seconds at 120 BPM is 40 beats, less the few seconds it takes to
	// find the tempo.
	if beats < 25 || beats > 41 {
		t.Errorf("twenty seconds at 120 BPM counted %d beats", beats)
	}
}

func TestSilenceHasNoTempo(t *testing.T) {
	var tp tempo
	var bpm float64
	for range 300 {
		bpm, _, _ = tp.step(0, 1.0/30)
	}
	if bpm != 0 {
		t.Errorf("silence was given a tempo of %.1f", bpm)
	}
}

// listen runs a listener on a synthetic track for secs seconds at 30 frames a
// second. bass and treble give each block's power at the two ends.
func listen(l *listener, tr *track, from, secs float64, bass, treble func(t float64) float64) []Signals {
	var out []Signals
	dt := 1.0 / 30
	for at := from; at < from+secs; at += dt {
		b, h := bass(at), treble(at)
		diff := b*band(bassHz, trackRate) + h*band(trebleHz, trackRate)
		l.ear.hear(tr.block(dt, both(b+h), both(diff), 0, at-0.27))
		out = append(out, l.step(dt, time.Time{}))
	}
	return out
}

func steady(v float64) func(float64) float64 { return func(float64) float64 { return v } }

func TestAKickPulsesAndThePulseDiesAway(t *testing.T) {
	var l listener
	var tr track
	listen(&l, &tr, 0, 3, steady(0.001), steady(0))
	// The jump is heard 0.27 s after it is measured, so the listening runs on
	// past it.
	sig := listen(&l, &tr, 3, 0.1, steady(0.3), steady(0))
	sig = append(sig, listen(&l, &tr, 3.1, 0.4, steady(0.001), steady(0))...)
	struck := 0.0
	for _, s := range sig {
		struck = math.Max(struck, s.Kick)
	}
	if struck < 0.5 {
		t.Fatalf("a jump in the bass kicked only %.2f", struck)
	}
	sig = listen(&l, &tr, 3.5, 0.5, steady(0.001), steady(0))
	if k := sig[len(sig)-1].Kick; k > struck/4 {
		t.Errorf("half a second later the kick was still %.2f, from %.2f", k, struck)
	}
}

func TestASteadySoundDoesNotKick(t *testing.T) {
	var l listener
	var tr track
	sig := listen(&l, &tr, 0, 5, steady(0.1), steady(0.001))
	if k := sig[len(sig)-1].Kick; k > 0.05 {
		t.Errorf("five seconds of a steady sound kicked %.2f", k)
	}
}

func TestAChangeOfCharacterIsASection(t *testing.T) {
	var l listener
	var tr track
	quiet := listen(&l, &tr, 0, 25, steady(0.0001), steady(0))
	for _, s := range quiet[len(quiet)/2:] {
		if s.Section {
			t.Fatal("a steady quiet passage was heard as a new section")
		}
	}
	loud := listen(&l, &tr, 25, 4, steady(0.3), steady(0.01))
	found := false
	for _, s := range loud {
		found = found || s.Section
	}
	if !found {
		t.Error("a quiet passage turning loud was not heard as a new section")
	}
}

// clickPhase plays a click track at bpm into a tempo for secs seconds, with
// frames of 1/30 s varied by up to jitter seconds, and returns the largest
// distance in beats between the tempo's phase and the clicks over the last
// half of the run, and the tempo found at the end.
func clickPhase(bpm, jitter, secs float64) (float64, float64) {
	var tp tempo
	rng := rand.New(rand.NewPCG(1, 2))
	period := 60 / bpm
	now, worst, found := 0.0, 0.0, 0.0
	for now < secs {
		dt := 1.0/30 + (rng.Float64()*2-1)*jitter
		before := now
		now += dt
		onset := 0.0
		if math.Floor(now/period) != math.Floor(before/period) {
			onset = 1
		}
		var phase float64
		found, phase, _ = tp.step(onset, dt)
		if now > secs/2 {
			off := phase - math.Mod(now/period, 1)
			worst = math.Max(worst, math.Abs(off-math.Round(off)))
		}
	}
	return worst, found
}

func TestFramesOfUnevenLengthDoNotPullTheTempoAway(t *testing.T) {
	for _, bpm := range []float64{97, 120, 140} {
		worst, found := clickPhase(bpm, 0.01, 120)
		if math.Abs(found-bpm) > 3 {
			t.Errorf("a click at %v BPM with uneven frames was heard as %.1f BPM", bpm, found)
		}
		if worst > 0.15 {
			t.Errorf("a click at %v BPM with uneven frames drifted %.2f of a beat from the phase", bpm, worst)
		}
	}
}

func TestAPhaseFoundHalfABeatOutMovesOntoTheClicks(t *testing.T) {
	// At 128 BPM the phase starts about half a beat from the clicks when the
	// tempo is first found.
	if worst, _ := clickPhase(128, 0, 60); worst > 0.1 {
		t.Errorf("the phase stayed %.2f of a beat from a steady click", worst)
	}
}

func TestThePhaseGlidesOntoTheBeatAndNeverJumps(t *testing.T) {
	// The clicks move half a beat once the tempo is held, as when the beat
	// clock has found the offbeat.
	var tp tempo
	period, dt := 60.0/128, 1.0/30
	worst, apart := 0.0, 0.0
	var last float64
	var lastBeat int
	held := false
	for now := 0.0; now < 60; now += dt {
		shift := 0.0
		if now > 20 {
			shift = period / 2
		}
		onset := 0.0
		if math.Floor((now-shift)/period) != math.Floor((now-shift-dt)/period) {
			onset = 1
		}
		bpm, phase, beat := tp.step(onset, dt)
		if bpm > 0 && held {
			moved := phase - last + float64(beat-lastBeat) - dt*bpm/60
			worst = math.Max(worst, math.Abs(moved))
		}
		if now > 28 {
			off := phase - math.Mod((now-shift)/period, 1)
			apart = math.Max(apart, math.Abs(off-math.Round(off)))
		}
		held, last, lastBeat = bpm > 0, phase, beat
	}
	if worst > tempoGlide+1e-9 {
		t.Errorf("the phase jumped %.2f of a beat in one frame", worst)
	}
	if apart > 0.1 {
		t.Errorf("eight seconds after the clicks moved, the phase was still %.2f of a beat from them", apart)
	}
}

// sidechained is 125 BPM techno: a kick on each beat over a sustained
// bassline that ducks under the kick and swells back, and a hi-hat on each
// offbeat. It returns the power at the low and high ends at t seconds.
func sidechained(t float64) (float64, float64) {
	x := math.Mod(t, 60.0/125)
	bass := 0.05*(1-0.8*math.Exp(-x/0.15)) + 0.12*math.Exp(-x/0.04)
	hat := 0.0
	if math.Abs(x-60.0/250) < 0.03 {
		hat = 0.02
	}
	return bass, hat
}

// swaying has no beat: the low end swells and fades over seven seconds, and
// the high end flickers at random moments.
func swaying() func(t float64) (float64, float64) {
	rng := rand.New(rand.NewPCG(7, 7))
	var flickers []float64
	for t := 0.0; t < 300; t += 0.1 + 0.6*rng.Float64() {
		flickers = append(flickers, t)
	}
	return func(t float64) (float64, float64) {
		v := 0.5 + 0.5*math.Sin(2*math.Pi*t/7)
		hat := 0.0005
		for _, f := range flickers {
			if d := t - f; d >= 0 && d < 0.04 {
				hat = 0.02
			}
		}
		return 0.002 + 0.05*v*v, hat
	}
}

// hearJittered runs a listener on a synthetic track for secs seconds, with
// frames of 1/30 s varied by up to 10 ms, and calls each with the position
// heard and the signals.
func hearJittered(snd func(float64) (float64, float64), secs float64, each func(heard float64, s Signals)) {
	var l listener
	var tr track
	rng := rand.New(rand.NewPCG(3, 9))
	for at := 0.0; at < secs; {
		dt := 1.0/30 + (rng.Float64()*2-1)*0.01
		at += dt
		b, h := snd(at)
		diff := b*band(bassHz, trackRate) + h*band(trebleHz, trackRate)
		l.ear.hear(tr.block(dt, both(b+h), both(diff), 0, at-0.27))
		each(l.ear.now, l.step(dt, time.Time{}))
	}
}

func TestKicksUnderASidechainedBasslineHoldTheBeatAndOffbeatHatsDoNot(t *testing.T) {
	period := 60.0 / 125
	worst, unlocked := 0.0, 0
	hearJittered(sidechained, 140, func(heard float64, s Signals) {
		if heard < 20 {
			return
		}
		if s.Tempo == 0 {
			unlocked++
			return
		}
		off := s.Phase - math.Mod(heard/period, 1)
		worst = math.Max(worst, math.Abs(off-math.Round(off)))
	})
	if unlocked > 0 {
		t.Errorf("the tempo was lost on %d frames of two minutes of steady techno", unlocked)
	}
	if worst > 0.1 {
		t.Errorf("the phase strayed %.2f of a beat from the kicks", worst)
	}
}

func TestMusicWithoutABeatDoesNotPulse(t *testing.T) {
	var tempos, frames int
	var sure float64
	// Each beat-driven source notes when it pulses: a rise above 0.7 after a
	// fall below 0.3.
	pulses := map[int][]float64{}
	low := map[int]bool{}
	hearJittered(swaying(), 180, func(heard float64, s Signals) {
		if heard < 20 {
			return
		}
		frames++
		if s.Tempo > 0 {
			tempos++
		}
		sure = math.Max(sure, s.Sure)
		for _, src := range []int{srcPhase, srcBar} {
			v := s.source(src, heard)
			if v < 0.3 {
				low[src] = true
			}
			if v > 0.7 && low[src] {
				pulses[src] = append(pulses[src], heard)
				low[src] = false
			}
		}
	})
	if tempos > 0 {
		t.Errorf("music without a beat was given a tempo on %d of %d frames, %.2f sure", tempos, frames, sure)
	}
	for _, src := range []int{srcPhase, srcBar} {
		// The most pulses within any ten seconds.
		most := 0
		for i, at := range pulses[src] {
			n := 0
			for _, later := range pulses[src][i:] {
				if later < at+10 {
					n++
				}
			}
			most = max(most, n)
		}
		if hz := float64(most) / 10; hz > 0.5 {
			t.Errorf("source %d pulsed %.1f times a second on music without a beat", src, hz)
		}
	}
}
