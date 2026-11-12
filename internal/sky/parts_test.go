package sky

import (
	"math"
	"testing"
	"time"
)

// stretch is a run of sound for a made-up track: how long it lasts, its
// level in dB, how wide it is as the level of its side against the whole,
// the level of its middle band against the whole, and the peak a kick lifts
// the band where it starts to, against a floor of 0.02, on every half
// second.
type stretch struct {
	secs, level, width, mid, kick float64
}

// stretches returns the blocks of a track made of the given runs of sound,
// with a hat on every quarter second so that it has beats.
func stretches(runs ...stretch) []Block {
	hop := float64(synthBlock) / synthRate
	var out []Block
	at := 0.0
	for _, r := range runs {
		for end := at + r.secs; at < end; at += hop {
			onKick := math.Mod(at, 0.5) < hop
			onHat := math.Mod(at+0.25, 0.5) < hop
			level := r.level
			if onKick && r.kick > 0 {
				level += 1
			}
			high := level - 30
			if onHat {
				high = level - 6
			}
			kick := level - 12
			if onKick {
				kick = level - 3
			}
			punch := 0.02
			if onKick {
				punch = r.kick
			}
			c := Channel{Loudness: level, Difference: 0.05, Crossings: 0.05 * synthBlock}
			out = append(out, Block{At: at, Samples: synthBlock, Channels: []Channel{c, c},
				Bands: []float64{kick, level - 8, level + r.mid, high, kick},
				Side:  math.Pow(10, (level+r.width)/20), Punch: punch, Rate: synthRate, Scale: 1})
		}
	}
	return out
}

// song is a track with a light kick that goes from a quiet, narrow part to
// a loud, wide wall of sound, back to a quiet one, and stops two seconds
// before its end.
func song() []Block {
	return stretches(
		stretch{40, -18, -25, -12, 0.04},
		stretch{40, -9, -5, -6, 0.04},
		stretch{40, -18, -25, -12, 0.04},
		stretch{2, -80, -30, -12, 0},
	)
}

func TestAChangeOfTextureIsABoundaryPlacedOnTheBlockItHappensIn(t *testing.T) {
	_, bounds := findParts(song(), nil)
	hop := float64(synthBlock) / synthRate
	for _, want := range []float64{40, 80, 120} {
		found := false
		for _, b := range bounds {
			if math.Abs(b.At-want) <= 2*hop {
				found = true
				if !b.Major || !b.Hard {
					t.Errorf("the change at %v s was major %v and hard %v", want, b.Major, b.Hard)
				}
			}
		}
		if !found {
			t.Errorf("no boundary within two blocks of the change at %v s: %+v", want, bounds)
		}
	}
	for _, b := range bounds {
		if b.Major && math.Abs(b.At-40) > 1 && math.Abs(b.At-80) > 1 && math.Abs(b.At-120) > 1 {
			t.Errorf("a major boundary at %.2f s, where nothing changes", b.At)
		}
	}
}

func TestWhatFollowsTheStopIsSilentAndTheLoudLevelWallIsHeavy(t *testing.T) {
	p := NewPlan(song())
	if !p.arranged {
		t.Fatal("a track with a light kick was not taken as carried by its arrangement")
	}
	quiet, _ := p.partAt(20)
	loud, _ := p.partAt(60)
	after, _ := p.partAt(121.5)
	if loud.Heavy < 0.9 || quiet.Heavy > 0.1 {
		t.Errorf("the wall of sound was heavy %.2f and the quiet part %.2f", loud.Heavy, quiet.Heavy)
	}
	if loud.Intensity <= quiet.Intensity+0.3 {
		t.Errorf("the loud part's intensity was %.2f and the quiet part's %.2f", loud.Intensity, quiet.Intensity)
	}
	if !after.Silent {
		t.Errorf("the part after the stop was %+v, want silent", after)
	}
	if c := quiet.mood(); c.Calm <= c.Grit {
		t.Errorf("the quiet part's mood was %+v, want calm above grit", c)
	}
	if c := loud.mood(); c.Grit <= c.Calm || c.Grit <= c.Flow {
		t.Errorf("the heavy part's mood was %+v, want grit to lead", c)
	}
}

func TestATrackWhoseKickCarriesItKeepsTheCuesFoundFromItsLevel(t *testing.T) {
	heavyKick := stretches(stretch{60, -12, -10, -10, 0.5}, stretch{60, -12, -10, -10, 0.5})
	if p := NewPlan(heavyKick); p.arranged {
		t.Error("a track whose kicks lift their band by 28 dB was taken as carried by its arrangement")
	}
	lightKick := stretches(stretch{60, -12, -10, -10, 0.035}, stretch{60, -12, -10, -10, 0.035})
	if p := NewPlan(lightKick); !p.arranged {
		t.Error("a track whose kicks lift their band by 5 dB was not taken as carried by its arrangement")
	}
}

func TestAHardBoundaryIsCutToEvenWhenAFrameStepsOverIt(t *testing.T) {
	p := NewPlan(song())
	p.curve = nil
	var hard float64
	for _, b := range p.Bounds {
		if b.Major && b.Hard && math.Abs(b.At-40) < 1 {
			hard = b.At
		}
	}
	if hard == 0 {
		t.Fatalf("the plan has no hard boundary at 40 s: %+v", p.Bounds)
	}
	// Frames are drawn 50 ms apart, further than a hard cut is long, at
	// every offset from the boundary.
	for offset := 0.0; offset < 0.05; offset += 0.005 {
		s := New(3)
		s.Foresee("track", p)
		var tr track
		cut := false
		for at := hard - 30 + offset; at < hard+1; at += 0.05 {
			was := s.next
			s.Hear(tr.block(0.05, both(0.02), both(0.0001), 0.05, at-0.27))
			s.Step(40, 10, 0.05, time.Time{}, "track")
			if s.next != nil && s.next != was && s.cutting {
				cut = true
				if d := math.Abs(s.lis.ear.now - hard); d > 0.06 {
					t.Errorf("at offset %.3f the cut began %.0f ms from the boundary", offset, 1000*d)
				}
			}
		}
		if !cut {
			t.Errorf("at offset %.3f no cut was made at the hard boundary", offset)
		}
	}
}

func TestAfterTheStopThePictureIsDarkAndStill(t *testing.T) {
	if e := exposureFor(Signals{Weight: silentWeight}); e != 0 {
		t.Errorf("with no sound the exposure was %.2f", e)
	}
	if lit, dark := exposureFor(Signals{Weight: 0.25}), exposureFor(Signals{Weight: -0.3}); lit <= dark {
		t.Errorf("an intense part was lit to %.2f and a soft one to %.2f", lit, dark)
	}
}
