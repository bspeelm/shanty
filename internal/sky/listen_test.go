package sky

import (
	"math"
	"testing"
	"time"
)

// track builds the readings mpv would give for a synthetic track, a block at
// a time, at 44.1 kHz on two channels.
type track struct {
	n     float64
	tot   [2]struct{ energy, diff, cross float64 }
	epoch float64
}

const trackRate = 44100

// block adds secs seconds of samples whose channels have mean power energy[c]
// and mean squared difference diff[c], crossing zero cross times a sample, and
// returns the reading at position pos seconds.
func (t *track) block(secs float64, energy, diff [2]float64, cross, pos float64) Reading {
	dn := secs * trackRate
	if t.epoch > 0 && t.n+dn > t.epoch {
		t.n = 0
		t.tot = [2]struct{ energy, diff, cross float64 }{}
	}
	r := Reading{Rate: trackRate, Scale: 1, Epoch: t.epoch, Position: time.Duration(pos * float64(time.Second))}
	if r.Epoch == 0 {
		r.Epoch = 1e12
	}
	for c := range 2 {
		t.tot[c].energy += energy[c] * dn
		t.tot[c].diff += diff[c] * dn
		t.tot[c].cross += cross * dn
	}
	t.n += dn
	r.Samples = t.n
	for c := range 2 {
		ch := Channel{Loudness: math.Inf(-1), Difference: math.Sqrt(t.tot[c].diff / t.n), Crossings: t.tot[c].cross}
		if t.tot[c].energy > 0 {
			ch.Loudness = 10 * math.Log10(t.tot[c].energy/t.n)
		}
		r.Channels = append(r.Channels, ch)
	}
	return r
}

func both(v float64) [2]float64 { return [2]float64{v, v} }

func TestALowSoundIsHeardAsBassAndAHighOneAsTreble(t *testing.T) {
	for _, tc := range []struct {
		name         string
		hz           float64
		bass, treble bool
	}{
		{"a 40 Hz tone, like a kick drum", 40, true, false},
		{"an 8 kHz tone, like a hi-hat", 8000, false, true},
	} {
		var tr track
		var e ear
		power := 0.1
		diff := power * band(tc.hz, trackRate)
		e.hear(tr.block(0.1, both(0), both(0), 0, 0))
		e.hear(tr.block(0.1, both(power), both(diff), 0, 0))
		s := e.slices[0]
		if (s.bass > 0.5) != tc.bass || (s.treble > 0.5) != tc.treble {
			t.Errorf("%s was heard as bass %.2f, treble %.2f", tc.name, s.bass, s.treble)
		}
	}
}

func TestBalanceFollowsTheLouderChannel(t *testing.T) {
	var tr track
	var e ear
	e.hear(tr.block(0.1, both(0), both(0), 0, 0))
	e.hear(tr.block(0.1, [2]float64{0.001, 0.1}, both(0), 0, 0))
	if b := e.slices[0].balance; b < 0.9 {
		t.Errorf("a sound almost all on the right was heard at balance %.2f", b)
	}
}

func TestAirIsTheShareOfSamplesThatCrossZero(t *testing.T) {
	var tr track
	var e ear
	e.hear(tr.block(0.1, both(0.1), both(0), 0, 0))
	e.hear(tr.block(0.1, both(0.1), both(0), 0.15, 0))
	if a := e.slices[0].air; math.Abs(a-0.5) > 1e-9 {
		t.Errorf("crossing zero on 15%% of samples was heard as air %.2f, want 0.50", a)
	}
}

func TestASliceIsPlacedWhereTheMeasuredAudioIsInTheTrack(t *testing.T) {
	// The filter has measured 0.5 s while 0.23 s is heard, so the stretch
	// between the readings is 0.4 s to 0.5 s, and the lead is 0.27 s.
	var tr track
	var e ear
	e.hear(tr.block(0.4, both(0.1), both(0), 0, 0.13))
	e.hear(tr.block(0.1, both(0.1), both(0), 0, 0.23))
	if got := e.slices[0].at; math.Abs(got-0.45) > 1e-9 {
		t.Errorf("the slice was placed at %.3f s, want 0.450 s", got)
	}
	if math.Abs(e.lead-0.27) > 1e-9 {
		t.Errorf("the lead was learned as %.3f s, want 0.270 s", e.lead)
	}
}

func TestTheCountCarriesOnAcrossTheEndOfAnEpoch(t *testing.T) {
	tr := track{epoch: trackRate}
	var e ear
	pos := 0.0
	for range 12 {
		pos += 0.1
		e.hear(tr.block(0.1, both(0.1), both(0), 0, pos-0.27))
	}
	if e.epochs != 1 {
		t.Fatalf("1.2 s in epochs of 1 s counted %d epochs, want 1", e.epochs)
	}
	last := e.slices[len(e.slices)-1]
	if math.Abs(last.at-1.15) > 1e-9 {
		t.Errorf("after the epoch ended the slice was placed at %.3f s, want 1.150 s", last.at)
	}
}

func TestAfterASeekTheSliceIsPlacedByTheLeadLearnedBefore(t *testing.T) {
	var tr track
	var e ear
	e.hear(tr.block(0.4, both(0.1), both(0), 0, 0.13))
	e.hear(tr.block(0.1, both(0.1), both(0), 0, 0.23))
	// A seek to 60 s starts the count again.
	tr = track{}
	e.hear(tr.block(0.1, both(0.1), both(0), 0, 60))
	if len(e.slices) != 0 {
		t.Fatalf("slices from before the seek were kept: %v", e.slices)
	}
	e.hear(tr.block(0.1, both(0.1), both(0), 0, 60.1))
	if got, want := e.slices[0].at, 60.1+0.27-0.05; math.Abs(got-want) > 1e-9 {
		t.Errorf("the slice was placed at %.3f s, want %.3f s", got, want)
	}
}

func TestSlicesHeardWhileNoFrameIsDrawnAreNotKeptForOnsets(t *testing.T) {
	var tr track
	var e ear
	pos := 0.0
	for range 600 {
		pos += 0.1
		e.hear(tr.block(0.1, both(0.1), both(0), 0, pos-0.27))
	}
	if len(e.pending) > 15 {
		t.Errorf("a minute heard with no frame drawn left %d slices waiting", len(e.pending))
	}
}

func TestEverySliceReachesTheOnsetsOnceAndInOrder(t *testing.T) {
	var tr track
	var e ear
	pos := 0.0
	var seen []float64
	for range 50 {
		// Two readings arrive for each frame.
		for range 2 {
			pos += 0.02
			e.hear(tr.block(0.02, both(0.1), both(0), 0, pos-0.27))
		}
		e.take(0.04, time.Time{})
		for _, s := range e.fresh {
			seen = append(seen, s.at)
		}
	}
	for range 30 {
		e.take(0.04, time.Time{})
		for _, s := range e.fresh {
			seen = append(seen, s.at)
		}
	}
	if len(seen) != 99 {
		t.Errorf("%d slices reached the onsets, want 99", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] <= seen[i-1] {
			t.Fatalf("slice %d at %.3f s came after one at %.3f s", i, seen[i], seen[i-1])
		}
	}
}

func TestNothingIsHeardBeforeThePositionReachesIt(t *testing.T) {
	var tr track
	var e ear
	e.hear(tr.block(0.4, both(0), both(0), 0, 0.13))
	e.hear(tr.block(0.1, both(0.1), both(0), 0, 0.23))
	if _, heard := e.take(0, time.Time{}); heard {
		t.Fatal("audio 0.2 s ahead of the speakers was drawn before it was heard")
	}
	e.now = 0.46
	if s, heard := e.take(0, time.Time{}); !heard || s.level == 0 {
		t.Errorf("once it was reached, take gave %+v, %v", s, heard)
	}
}

func TestAShortPeakBetweenTwoFramesIsStillDrawn(t *testing.T) {
	e := ear{now: 1, slices: []slice{{at: 0.90, level: 0.2}, {at: 0.93, level: 0.9}, {at: 0.96, level: 0.2}}}
	s, _ := e.take(0, time.Time{})
	if s.level != 0.9 {
		t.Errorf("take gave %.2f, want the peak of 0.90", s.level)
	}
	if s, _ := e.take(0, time.Time{}); s.level != 0.2 {
		t.Errorf("the next frame gave %.2f, want the newest level 0.20", s.level)
	}
}

func TestThePositionMovesOnWithTheFramesBetweenReadings(t *testing.T) {
	var tr track
	var e ear
	e.hear(tr.block(0.4, both(0), both(0), 0, 0.13))
	e.hear(tr.block(0.1, both(0.1), both(0), 0, 0.23)) // heard at 0.45 s
	frames := 0
	for heard := false; !heard && frames < 10; frames++ {
		_, heard = e.take(0.033, time.Time{})
	}
	// Frame 7 reaches 0.23 + 7 × 0.033 + half a frame = 0.478 s, the first
	// within half a frame of 0.45 s.
	if frames != 7 {
		t.Errorf("the slice at 0.45 s was drawn on frame %d, want 7", frames)
	}
}

func TestSilenceIsTheBottomOfTheScale(t *testing.T) {
	for _, tc := range []struct{ db, want float64 }{
		{math.Inf(-1), 0}, {-80, 0}, {-50, 0}, {-27.5, 0.5}, {-5, 1}, {0, 1},
	} {
		if got := levelOf(power(tc.db), -50, -5); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%v dB is level %.3f, want %.3f", tc.db, got, tc.want)
		}
	}
}

func TestAReadingHandledLateIsStillDrawnWhereItIsHeard(t *testing.T) {
	var tr track
	var e ear
	clock := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	r := tr.block(0.4, both(0), both(0), 0, 0.13)
	r.At = clock
	e.hear(r)
	r = tr.block(0.1, both(0.1), both(0), 0, 0.23) // heard at 0.45 s
	r.At = clock.Add(100 * time.Millisecond)
	// However long the reading waited before it was handled, a frame drawn
	// 220 ms after mpv gave the position draws 0.23 s + 0.22 s = 0.45 s.
	e.hear(r)
	if _, heard := e.take(0.033, r.At.Add(220*time.Millisecond)); !heard {
		t.Fatalf("at %.3f s the slice at 0.45 s was not drawn", e.now)
	}
	if math.Abs(e.now-0.45) > 1e-9 {
		t.Errorf("the position heard was %.3f s, want 0.450 s", e.now)
	}
}
