package tui

import (
	"math"
	"strings"
	"testing"
	"time"
)

// at is a reading taken at position pos seconds, with the filter having
// measured n samples at 1000 a second whose RMS level is db.
func at(pos, n, db float64) Heard {
	return Heard{Samples: n, Loudness: db, Rate: 1000, Position: time.Duration(pos * float64(time.Second))}
}

// cumulative returns the running RMS level, in dB, of a first stretch of n1
// samples at db1 followed by n2 samples at db2.
func cumulative(n1, db1, n2, db2 float64) float64 {
	return 10 * math.Log10((n1*power(db1)+n2*power(db2))/(n1+n2))
}

func TestTheLevelBetweenTwoReadingsIsWorkedOutFromTheRunningTotals(t *testing.T) {
	// 300 quiet samples, then 100 loud ones. The running level after both is
	// mostly the quiet part; the stretch between the readings is the loud one.
	e := ear{}.hear(at(0.03, 300, -45))
	e = e.hear(at(0.13, 400, cumulative(300, -45, 100, -10)))
	if len(e.marks) != 1 {
		t.Fatalf("got %d points, want 1", len(e.marks))
	}
	if got, want := e.marks[0].level, levelOf(power(-10)); math.Abs(got-want) > 1e-9 {
		t.Errorf("the stretch between the readings was read as %.3f, want %.3f", got, want)
	}
}

func TestAPointIsPlacedWhereTheMeasuredAudioIsInTheTrack(t *testing.T) {
	// The filter has measured 0.5 s of the track while 0.23 s is being heard,
	// so it is 0.27 s ahead, and the stretch between the readings is
	// 0.4 s to 0.5 s of the track.
	e := ear{}.hear(at(0.13, 400, -20))
	e = e.hear(at(0.23, 500, -20))
	if got := e.marks[0].at; math.Abs(got-0.45) > 1e-9 {
		t.Errorf("the point was placed at %.3f s, want 0.450 s", got)
	}
	if math.Abs(e.lead-0.27) > 1e-9 {
		t.Errorf("the lead was learned as %.3f s, want 0.270 s", e.lead)
	}
}

func TestAfterASeekThePointIsPlacedByTheLeadLearnedBefore(t *testing.T) {
	e := ear{}.hear(at(0.13, 400, -20))
	e = e.hear(at(0.23, 500, -20)) // learns a lead of 0.27 s
	// A seek to 60 s starts the count again, so it no longer says where in the
	// track the filter is.
	e = e.hear(at(60.0, 100, -20))
	if len(e.marks) != 0 {
		t.Fatalf("points from before the seek were kept: %v", e.marks)
	}
	e = e.hear(at(60.1, 200, -20))
	if got, want := e.marks[0].at, 60.1+0.27-0.05; math.Abs(got-want) > 1e-9 {
		t.Errorf("the point was placed at %.3f s, want %.3f s", got, want)
	}
}

func TestNothingIsHeardBeforeThePlaybackPositionReachesIt(t *testing.T) {
	e := ear{}.hear(at(0.13, 400, -20))
	e = e.hear(at(0.23, 500, cumulative(400, -20, 100, -10))) // 0.4 s to 0.5 s of the track
	if _, _, heard := e.take(0); heard {
		t.Fatal("audio 0.2 s ahead of the speakers was drawn before it was heard")
	}
	e.now = 0.46
	if _, level, heard := e.take(0); !heard || math.Abs(level-levelOf(power(-10))) > 1e-9 {
		t.Errorf("once it was reached, take gave %.3f, %v", level, heard)
	}
}

func TestAShortPeakBetweenTwoFramesIsStillDrawn(t *testing.T) {
	e := ear{now: 1, marks: []point{{at: 0.90, level: 0.2}, {at: 0.93, level: 0.9}, {at: 0.96, level: 0.2}}}
	e, level, _ := e.take(0)
	if level != 0.9 {
		t.Errorf("take gave %.2f, want the peak of 0.90", level)
	}
	// The next frame, with nothing new heard, holds the newest level rather
	// than repeating the peak.
	if _, level, _ := e.take(0); level != 0.2 {
		t.Errorf("the next frame gave %.2f, want the newest level 0.20", level)
	}
}

func TestANewTrackStartsWithNothingHeard(t *testing.T) {
	e := ear{}.hear(at(10.0, 10_270, -20))
	e = e.hear(at(10.1, 10_370, -20))
	e = e.hear(at(0.0, 230, -20)) // the count starts again with the next track
	if len(e.marks) != 0 {
		t.Errorf("points from the last track were carried into the next: %v", e.marks)
	}
}

func TestSilenceIsTheBottomOfTheScale(t *testing.T) {
	for _, tc := range []struct {
		db, want float64
	}{
		{math.Inf(-1), 0},
		{-80, 0},
		{-50, 0},
		{-27.5, 0.5},
		{-5, 1},
		{0, 1},
	} {
		if got := levelOf(power(tc.db)); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%v dB is level %.3f, want %.3f", tc.db, got, tc.want)
		}
	}
}

func TestABeatPulsesAndThePulseDiesAway(t *testing.T) {
	var s sound
	for range 50 {
		s = s.follow(0.3)
	}
	if s.pulse > 0.01 {
		t.Fatalf("a steady level pulsed: %.3f", s.pulse)
	}
	s = s.follow(0.9)
	if s.pulse < 0.5 {
		t.Fatalf("a jump from 0.3 to 0.9 pulsed only %.3f", s.pulse)
	}
	struck := s.pulse
	for range 10 {
		s = s.follow(0.3)
	}
	if s.pulse > struck/4 {
		t.Errorf("the pulse was still %.3f ten frames after the beat, from %.3f", s.pulse, struck)
	}
}

func TestWithNothingHeardTheSoundDiesAway(t *testing.T) {
	s := sound{loud: 0.8, pulse: 0.9, average: 0.5}
	for range 40 {
		s = s.fade()
	}
	if s.loud > 0.01 || s.pulse > 0.01 {
		t.Errorf("forty frames of nothing left %+v", s)
	}
}

func TestALoudFrameDrawsMoreThanAQuietOne(t *testing.T) {
	filled := func(lines []string) int {
		n := 0
		for _, line := range lines {
			n += strings.Count(line, "▓") + strings.Count(line, "█")
		}
		return n
	}
	quiet := filled(sky(80, 24, 2.0, sound{}))
	loud := filled(sky(80, 24, 2.0, sound{loud: 1, pulse: 1}))
	if loud <= quiet {
		t.Errorf("a loud beat filled %d bright cells and silence %d", loud, quiet)
	}
}

func TestReadingsOnlyMoveThePatternWhileItIsShowing(t *testing.T) {
	m := loaded(t)
	next, cmd := m.Update(ShowVisualiser{})
	m = next.(Model)
	m = send(t, m, at(0.13, 400, -20), at(0.23, 500, -6))
	m.ear.now = 0.5 // the loud stretch is now being heard
	m, _ = turn(t, m, cmd)
	if m.sound.loud == 0 {
		t.Error("a loud reading being heard did not reach the pattern")
	}
	reopened := send(t, m, ShowVisualiser{})
	if reopened.sound != (sound{}) || len(reopened.ear.marks) != 0 {
		t.Error("opening the visualiser again kept what was heard last time")
	}
}

func TestThePointBeingHeardMovesOnWithTheFramesBetweenReadings(t *testing.T) {
	e := ear{}.hear(at(0.13, 400, -20))
	e = e.hear(at(0.23, 500, cumulative(400, -20, 100, -10))) // heard at 0.45 s
	// Seven frames of 33 ms take the point being heard from 0.23 s to 0.461 s.
	// That is the first frame within half a frame of the mark; the sixth, at
	// 0.428 s, is not.
	var heard bool
	frames := 0
	for !heard && frames < 10 {
		e, _, heard = e.take(0.033)
		frames++
	}
	if frames != 7 {
		t.Errorf("the mark at 0.45 s was drawn on frame %d, want 7", frames)
	}
}
