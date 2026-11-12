package sky

import (
	"math"
	"math/rand/v2"
	"testing"
)

// kickCount counts the kicks a listener reports: the frames where Kick
// rises to 0.5 or more at least 0.1 s after the last.
type kickCount struct {
	last, prev  float64
	heard, near int
}

// see takes a frame heard at a position, its kick, and the distance in
// seconds from that position to the nearest beat. A kick within 60 ms of a
// beat is near it.
func (c *kickCount) see(heard, kick, off float64) {
	if kick >= 0.5 && kick > c.prev && heard-c.last > 0.1 {
		c.heard++
		c.last = heard
		if off <= 0.06 {
			c.near++
		}
	}
	c.prev = kick
}

// offBeat is the distance in seconds from t to the nearest beat of period.
func offBeat(t, period float64) float64 {
	x := math.Mod(t, period)
	return math.Min(x, period-x)
}

// denseMix is a loud, rough mix at 120 BPM: the low end holds at about -7 dB
// and varies by about 0.5 dB from block to block, and a kick on each beat
// lifts it by about 3 dB. The high end is as rough and has no onsets.
func denseMix() func(t float64) (float64, float64) {
	rng := rand.New(rand.NewPCG(5, 5))
	return func(t float64) (float64, float64) {
		rough := 1 + 0.12*(rng.Float64()*2-1)
		return 0.2*rough + 0.2*math.Exp(-math.Mod(t, 0.5)/0.08), 0.01 * rough
	}
}

func TestASmallKickUnderALoudRoughMixIsHeardAndHoldsTheBeat(t *testing.T) {
	var kicks kickCount
	beats, frames, held := 0, 0, 0
	last := 0.0
	hearJittered(denseMix(), 60, func(heard float64, s Signals) {
		if heard < 20 {
			last = heard
			return
		}
		if math.Floor(heard/0.5) != math.Floor(last/0.5) {
			beats++
		}
		last = heard
		kicks.see(heard, s.Kick, offBeat(heard, 0.5))
		frames++
		if math.Abs(s.Tempo/120-1) < 0.02 {
			held++
		}
	})
	if kicks.near < beats*8/10 {
		t.Errorf("%d of %d kicks a few dB above a loud mix were heard", kicks.near, beats)
	}
	if kicks.heard > kicks.near*11/10 {
		t.Errorf("%d kicks were heard where %d fell on the beat", kicks.heard, kicks.near)
	}
	if held < frames*9/10 {
		t.Errorf("the tempo of kicks under a loud mix was held at 120 BPM on %d of %d frames", held, frames)
	}
}

// softDrums is a song at 100 BPM played softly over a held chord that swells
// and fades over eight seconds: a kick on the first and third beats lifts
// the low end by about 4 dB, a snare on the second and fourth lifts the high
// end by about 6 dB and the low end a little, and a hi-hat on every half
// beat lifts the high end by about 2 dB.
func softDrums(t float64) (float64, float64) {
	beat := 0.6
	x := math.Mod(t, beat)
	swell := 0.5 + 0.5*math.Sin(2*math.Pi*t/8)
	bass := 0.02 + 0.01*swell
	hat := 0.002 + 0.001*swell
	if int(math.Floor(t/beat))%2 == 0 {
		bass += 0.045 * math.Exp(-x/0.1)
	} else {
		bass += 0.008 * math.Exp(-x/0.1)
		hat += 0.006 * math.Exp(-x/0.08)
	}
	hat += 0.0012 * math.Exp(-math.Mod(t, beat/2)/0.03)
	return bass, hat
}

func TestSoftDrumsUnderAHeldChordAreFollowed(t *testing.T) {
	var kicks kickCount
	played, frames, held := 0, 0, 0
	last := 0.0
	hearJittered(softDrums, 60, func(heard float64, s Signals) {
		if heard < 20 {
			last = heard
			return
		}
		if math.Floor(heard/1.2) != math.Floor(last/1.2) {
			played++
		}
		last = heard
		kicks.see(heard, s.Kick, offBeat(heard, 1.2))
		frames++
		if math.Abs(s.Tempo/100-1) < 0.02 {
			held++
		}
	})
	if kicks.near < played*7/10 {
		t.Errorf("%d of %d soft kicks were heard", kicks.near, played)
	}
	if held < frames*9/10 {
		t.Errorf("the tempo of soft drums was held at 100 BPM on %d of %d frames", held, frames)
	}
}

// rolling is house at 128 BPM over a rolling bassline: a kick on each beat
// lifts the low end by about 5 dB, a bass note on every quarter of a beat
// but the first lifts it by 2 to 4 dB in a figure two beats long, and a
// hi-hat on each offbeat lifts the high end.
func rolling(t float64) (float64, float64) {
	beat := 60.0 / 128
	x := math.Mod(t, beat)
	q := math.Mod(t, beat/4)
	n := int(math.Floor(t/(beat/4))) % 8
	bass := 0.03 + 0.06*math.Exp(-x/0.07)
	if n%4 != 0 {
		bass += []float64{0, 0.012, 0.025, 0.012, 0, 0.025, 0.04, 0.025}[n] * math.Exp(-q/0.05)
	}
	hat := 0.002
	if n%4 == 2 {
		hat += 0.01 * math.Exp(-q/0.03)
	}
	return bass, hat
}

func TestKicksOverARollingBasslineHoldTheirTempo(t *testing.T) {
	frames, right, half := 0, 0, 0
	hearJittered(rolling, 60, func(heard float64, s Signals) {
		if heard < 15 {
			return
		}
		frames++
		switch {
		case math.Abs(s.Tempo/128-1) < 0.02:
			right++
		case math.Abs(s.Tempo/64-1) < 0.02:
			half++
		}
	})
	if right < frames*9/10 {
		t.Errorf("house at 128 BPM over a rolling bassline was heard at 128 on %d of %d frames, and at 64 on %d", right, frames, half)
	}
}

func TestAFigureTwoBeatsLongWithAKickOnEachBeatIsHeardAtTheBeat(t *testing.T) {
	// The onset strengths on each sixteenth of two beats at 128 BPM: a loud
	// kick with notes after it, then a soft kick.
	figure := []float64{1, 0.7, 0.2, 0, 0.3, 0, 0, 0.1}
	onsets := make([]float64, tempoWindow)
	for e := 0; ; e++ {
		i := int(math.Round(float64(e) * 60 / 128 / 4 / tempoStep))
		if i >= len(onsets) {
			break
		}
		onsets[i] = math.Max(onsets[i], figure[e%len(figure)])
	}
	if bpm, _ := beatOf(blur(nil, onsets, tempoBlur), tempoStep); math.Abs(bpm/128-1) > 0.02 {
		t.Errorf("a two-beat figure with a kick on each beat at 128 BPM was heard at %.1f BPM", bpm)
	}
}

// strummed is a song without drums: a guitar strummed at uneven moments from
// 0.7 to 2.4 seconds apart, each strum lifting the low end by about 5 dB
// and the high end by about 7 dB at once and dying away over a second, over
// a voice that swells and fades over nine seconds. It returns the sound and
// the times of the strums.
func strummed() (func(t float64) (float64, float64), []float64) {
	rng := rand.New(rand.NewPCG(11, 4))
	var strums []float64
	for t := 1.0; t < 120; t += 0.7 + 1.7*rng.Float64() {
		strums = append(strums, t)
	}
	return func(t float64) (float64, float64) {
		swell := 0.5 + 0.5*math.Sin(2*math.Pi*t/9)
		bass, hat := 0.01+0.01*swell, 0.001
		for _, s := range strums {
			if d := t - s; d >= 0 && d < 4 {
				bass += 0.03 * math.Exp(-d/0.8)
				hat += 0.004 * math.Exp(-d/0.3)
			}
		}
		return bass, hat
	}, strums
}

func TestAStrummedSongWithoutDrumsAnswersEachStrumGentlyAndHasNoBeat(t *testing.T) {
	snd, strums := strummed()
	var tempos, flashes, answered, counted int
	var prev Signals
	next := 0
	answer := 0.0
	hearJittered(snd, 120, func(heard float64, s Signals) {
		for next < len(strums) && strums[next] < heard-0.1 {
			if strums[next] > 10 {
				counted++
				if answer >= 0.2 {
					answered++
				}
			}
			answer = 0
			next++
		}
		if next < len(strums) && heard >= strums[next]-0.05 {
			answer = math.Max(answer, math.Max(s.Kick, s.Snap))
		}
		if heard > 10 {
			if s.Tempo > 0 {
				tempos++
			}
			if math.Max(s.Kick, s.Snap) > 0.6 && math.Max(prev.Kick, prev.Snap) <= 0.6 {
				flashes++
			}
		}
		prev = s
	})
	if tempos > 0 {
		t.Errorf("a song without drums was given a tempo on %d frames", tempos)
	}
	if flashes > 0 {
		t.Errorf("a song without drums flashed above 0.6 %d times", flashes)
	}
	if answered < counted*2/3 {
		t.Errorf("%d of %d strums were answered with a hit of 0.2 or more", answered, counted)
	}
}
