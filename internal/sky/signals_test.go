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
	if worst, _ := clickPhase(128, 0, 60); worst > 0.15 {
		t.Errorf("the phase stayed %.2f of a beat from a steady click", worst)
	}
}
