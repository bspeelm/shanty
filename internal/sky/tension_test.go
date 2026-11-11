package sky

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// levelPower is the power of a sound heard at a level from 0 to 1.
func levelPower(level float64) float64 { return math.Pow(10, (-50+45*level)/10) }

// shape runs a listener on a synthetic track for secs seconds at 30 frames a
// second and returns the signals of every frame.
func shape(snd sound, secs float64) []Signals {
	var l listener
	var tr track
	var out []Signals
	dt := 1.0 / 30
	for at := 0.0; at < secs; at += dt {
		b, h, c := snd(at)
		diff := b*band(bassHz, trackRate) + h*band(trebleHz, trackRate)
		l.ear.hear(tr.block(dt, both(b+h), both(diff), c, at-0.27))
		out = append(out, l.step(dt, time.Time{}))
	}
	return out
}

// crescendo is quiet for ten seconds, climbs from quiet to loud and from dull
// to bright over eight, and falls back to quiet at 18 seconds.
var crescendo sound = func(t float64) (float64, float64, float64) {
	climb := clamp01((t - 10) / 8)
	if t >= 18 {
		climb = 0
	}
	p := levelPower(0.2 + 0.7*climb)
	return p * (1 - 0.3*climb), p * 0.3 * climb, 0.05
}

func TestTensionBuildsThroughACrescendoAndReleasesAfterItsPeak(t *testing.T) {
	sig := shape(crescendo, 24)
	at := func(sec float64) Signals { return sig[int(sec*30)] }
	if v := at(9.5).Tension; v > 0.1 {
		t.Errorf("before the crescendo the tension was %.2f", v)
	}
	if v := at(17.5).Tension; v < 0.5 {
		t.Errorf("near the top of the crescendo the tension was only %.2f", v)
	}
	building := 0.0
	for _, s := range sig[10*30 : 17*30] {
		building = math.Max(building, s.Release)
	}
	if building > 0.2 {
		t.Errorf("while the crescendo was still building the release reached %.2f", building)
	}
	released := 0.0
	for _, s := range sig[18*30 : 22*30] {
		released = math.Max(released, s.Release)
	}
	if released < 0.6 {
		t.Errorf("after the crescendo's peak the release reached only %.2f", released)
	}
	if v := at(23.9).Release; v > released/3 {
		t.Errorf("two seconds after it fired the release was still %.2f, from %.2f", v, released)
	}
}

func TestADropAfterABuildIsARelease(t *testing.T) {
	sig := shape(func(t float64) (float64, float64, float64) {
		level := 0.2 + 0.3*clamp01((t-10)/6)
		if t >= 16 {
			level = 0.9
		}
		p := levelPower(level)
		return p * 0.8, p * 0.2, 0.05
	}, 20)
	released := 0.0
	for _, s := range sig[16*30 : 17*30] {
		released = math.Max(released, s.Release)
	}
	if released < 0.5 {
		t.Errorf("a loud drop after a build released only %.2f", released)
	}
}

func TestAQuietPassageAfterLoudMusicIsAHush(t *testing.T) {
	sig := shape(func(t float64) (float64, float64, float64) {
		if t < 30 {
			return levelPower(0.8), levelPower(0.6), 0.05
		}
		return levelPower(0.25), 0, 0.05
	}, 36)
	if v := sig[29*30].Hush; v > 0.1 {
		t.Errorf("thirty seconds of steady loud music read as a hush of %.2f", v)
	}
	if v := sig[len(sig)-1].Hush; v < 0.6 {
		t.Errorf("a quiet passage after loud music read as a hush of only %.2f", v)
	}
}

func TestASteadyTrackHasNoTensionReleaseOrHush(t *testing.T) {
	for name, snd := range map[string]sound{"steady kicks": fourOnTheFloor, "a quiet pad": pad} {
		var most Signals
		for _, s := range shape(snd, 40)[10*30:] {
			most.Tension = math.Max(most.Tension, s.Tension)
			most.Release = math.Max(most.Release, s.Release)
			most.Hush = math.Max(most.Hush, s.Hush)
		}
		if most.Tension > 0.2 || most.Release > 0.2 || most.Hush > 0.2 {
			t.Errorf("%s reached tension %.2f, release %.2f and hush %.2f",
				name, most.Tension, most.Release, most.Hush)
		}
	}
}

// breathPeriod runs a tension meter for a minute at a tempo, 0 for none, and
// returns the mean time between the breath rising through one half.
func breathPeriod(bpm float64) float64 {
	var m tensionMeter
	var s Signals
	dt := 1.0 / 30
	var rises []float64
	for at := 0.0; at < 60; at += dt {
		s.Tempo = bpm
		if bpm > 0 {
			beats := at * bpm / 60
			s.Beat, s.Phase = int(beats), beats-math.Floor(beats)
		}
		before := s.Breath
		m.breathe(&s, dt)
		if at > 5 && before < 0.5 && s.Breath >= 0.5 {
			rises = append(rises, at)
		}
	}
	if len(rises) < 2 {
		return 0
	}
	return (rises[len(rises)-1] - rises[0]) / float64(len(rises)-1)
}

func TestABreathLastsFourBarsAtAKnownTempoAndSixSecondsWithout(t *testing.T) {
	for _, tc := range []struct {
		name string
		bpm  float64
		want float64
	}{
		{"four bars at 120 BPM", 120, 8},
		{"eight bars at 180 BPM, where a bar is short", 180, 32 * 60.0 / 180},
		{"six seconds with no tempo", 0, 6},
	} {
		if got := breathPeriod(tc.bpm); math.Abs(got-tc.want) > 0.1 {
			t.Errorf("%s: a breath lasted %.2f s, want %.2f", tc.name, got, tc.want)
		}
	}
}

func TestExposureDimsInAHushAndLiftsOnARelease(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  Signals
		want float64
	}{
		{"steady music", Signals{Breath: 0.5}, 0.4},
		{"a hush", Signals{Hush: 1, Breath: 0.5}, 0.2},
		{"a release", Signals{Release: 1, Breath: 0.5}, 0.45},
	} {
		if got := exposureFor(tc.sig); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("in %s the exposure was %.3f, want %.3f", tc.name, got, tc.want)
		}
	}
}

func TestTheVignetteClosesInUnderTension(t *testing.T) {
	s := New(1)
	s.resize(60, 16)
	lit := func(sig Signals) float64 {
		p := post{}
		p.resize(&s.f)
		value := make([]float64, s.f.fw*s.f.fh)
		for range 60 {
			for i := range value {
				value[i] = 0.5
			}
			p.apply(&s.f, value, sig, 0, 1.0/30)
		}
		sum := 0.0
		for _, v := range value {
			sum += v
		}
		return sum / float64(len(value))
	}
	rest, tense := lit(Signals{}), lit(Signals{Tension: 1})
	if rest < 0.45 {
		t.Errorf("at rest the vignette darkened a mid-grey frame to %.3f", rest)
	}
	if tense > rest-0.02 {
		t.Errorf("under tension the frame was %.3f against %.3f at rest", tense, rest)
	}
}

func TestACalmStyleMutesKicks(t *testing.T) {
	moved := func(c Character) float64 {
		s := New(1)
		s.Step(40, 10, 1.0/30, zeroTime, "")
		st := styleFor(c)
		l := newLayer(rand.New(rand.NewPCG(1, 1)), kinds[2], st)
		l.weight = param{base: 0, src: [2]int{srcKick, srcKick}, depth: [2]float64{1, 0}}
		sc := &scene{palette: s.cur.palette, layers: []*layer{l}, hits: st.hits}
		s.f.dt = 1.0 / 30
		s.f.sig = Signals{}
		sc.step(&s.f)
		s.f.sig = Signals{Kick: 1}
		sc.step(&s.f)
		return l.w
	}
	calm, drive := moved(Character{Calm: 1}), moved(Character{Drive: 1})
	if calm > drive*0.6 {
		t.Errorf("a kick moved a parameter %.2f in a calm style and %.2f in a driving one", calm, drive)
	}
}

func TestBloomLightsAroundBrightPixelsAndLeavesDarknessDark(t *testing.T) {
	const w, h = 21, 21
	value, bright, tmp := make([]float64, w*h), make([]float64, w*h), make([]float64, w*h)
	bloom(value, bright, tmp, w, h, 3, 2, 1)
	for i, v := range value {
		if v != 0 {
			t.Fatalf("bloom lit pixel %d of a dark frame to %.3f", i, v)
		}
	}
	value[10*w+10] = 1
	bloom(value, bright, tmp, w, h, 3, 2, 1)
	if v := value[10*w+12]; v <= 0 {
		t.Error("bloom left the pixel two to the right of a bright one dark")
	}
	if v := value[12*w+10]; v <= 0 {
		t.Error("bloom left the pixel two below a bright one dark")
	}
	if v := value[0]; v != 0 {
		t.Errorf("bloom lit a far corner to %.3f", v)
	}
}
