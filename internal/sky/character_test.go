package sky

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// sound is a synthetic track: the power at the low and high ends and the
// share of samples crossing zero, at time t seconds.
type sound func(t float64) (bass, treble, cross float64)

// hearFor runs a listener on a synthetic track for secs seconds at 30 frames
// a second and returns the character it ends on.
func hearFor(snd sound, secs float64) Character {
	var l listener
	var tr track
	dt := 1.0 / 30
	for at := 0.0; at < secs; at += dt {
		b, h, c := snd(at)
		diff := b*band(bassHz, trackRate) + h*band(trebleHz, trackRate)
		l.ear.hear(tr.block(dt, both(b+h), both(diff), c, at-0.27))
		l.step(dt, time.Time{})
	}
	return l.char
}

// The four kinds of track the character is tested against.
var (
	// fourOnTheFloor is a kick on every beat at 128 BPM with a hi-hat
	// between, steady throughout.
	fourOnTheFloor sound = func(t float64) (float64, float64, float64) {
		beat := math.Mod(t, 60.0/128)
		bass, hat := 0.002, 0.0
		if beat < 0.07 {
			bass = 0.3
		}
		if math.Abs(beat-60.0/256) < 0.03 {
			hat = 0.01
		}
		return bass, hat, 0.05
	}
	// pad is a quiet sustained sound that drifts slowly, with no hits.
	pad sound = func(t float64) (float64, float64, float64) {
		return 0.01 * (1 + 0.15*math.Sin(t/3)), 0.0005, 0.03
	}
	// swells rises and falls widely over several seconds, without hits.
	swells sound = func(t float64) (float64, float64, float64) {
		v := 0.5 + 0.5*math.Sin(2*math.Pi*t/7)
		return 0.0003 + 0.15*v*v, 0.002 * v, 0.04
	}
	// rough is loud, bright and dense, with hits at uneven times.
	rough sound = func() sound {
		rng := rand.New(rand.NewPCG(4, 4))
		var hits []float64
		for t := 0.0; t < 60; t += 0.08 + 0.25*rng.Float64() {
			hits = append(hits, t)
		}
		return func(t float64) (float64, float64, float64) {
			bass, high := 0.03, 0.01
			for _, h := range hits {
				if d := t - h; d >= 0 && d < 0.05 {
					high, bass = 0.08, 0.12
				}
			}
			return bass, high, 0.3
		}
	}()
)

func TestTheCharacterOfEachKindOfTrack(t *testing.T) {
	for _, tc := range []struct {
		name string
		snd  sound
		axis int
	}{
		{"steady kicks at 128 BPM", fourOnTheFloor, 1},
		{"a quiet drifting pad", pad, 0},
		{"wide slow swells", swells, 3},
		{"loud, bright, uneven hits", rough, 2},
	} {
		c := hearFor(tc.snd, 40)
		a := c.axes()
		best := 0
		for i := range a {
			if a[i] > a[best] {
				best = i
			}
		}
		if best != tc.axis {
			t.Errorf("%s was heard as %s", tc.name, describe(c))
		}
	}
}

func describe(c Character) string {
	return fmt.Sprintf("calm %.2f, drive %.2f, grit %.2f, flow %.2f", c.Calm, c.Drive, c.Grit, c.Flow)
}

func TestTheCharacterAlwaysAddsUpToOne(t *testing.T) {
	for _, snd := range []sound{fourOnTheFloor, pad, swells, rough} {
		a := hearFor(snd, 20).axes()
		if sum := a[0] + a[1] + a[2] + a[3]; math.Abs(sum-1) > 0.02 {
			t.Errorf("the character added up to %.3f", sum)
		}
	}
}

func TestEvenGapsAreRegularAndUnevenOnesAreNot(t *testing.T) {
	even := []float64{0, 0.5, 1, 1.5, 2, 2.5}
	uneven := []float64{0, 0.1, 0.9, 1.0, 1.9, 2.5}
	if r := regularity(even); r < 0.99 {
		t.Errorf("evenly spaced times were %.2f regular", r)
	}
	if r := regularity(uneven); r > 0.3 {
		t.Errorf("unevenly spaced times were %.2f regular", r)
	}
}
