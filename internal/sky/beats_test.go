package sky

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

// voice is a drum: its peak power in each band when it is struck, and the
// time in seconds its power takes to fall by a factor of e.
type voice struct {
	power [bandCount]float64
	decay float64
}

var (
	kickVoice  = voice{[bandCount]float64{1, 0.15, 0.003, 0.0005}, 0.12}
	clapVoice  = voice{[bandCount]float64{0.003, 0.03, 0.3, 0.15}, 0.08}
	hatVoice   = voice{[bandCount]float64{0, 0, 0.004, 0.12}, 0.03}
	rideVoice  = voice{[bandCount]float64{0, 0, 0.01, 0.1}, 0.2}
	thumpVoice = voice{[bandCount]float64{0.3, 0.1, 0.001, 0}, 0.25}
)

// strike is a voice struck at a time with a gain.
type strike struct {
	at   float64
	v    voice
	gain float64
}

// bed is the power in each band of the sound under the strikes at a time.
type bed func(t float64) [bandCount]float64

// quietBed is a faint, steady hiss.
func quietBed(float64) [bandCount]float64 { return [bandCount]float64{1e-5, 1e-5, 1e-5, 1e-5} }

const synthRate, synthBlock = 48000, 1024

// banded returns the blocks of a track of the given length: each block's
// power in each band is the mean, over the block, of the bed and every
// strike so far dying away.
func banded(seconds float64, strikes []strike, under bed) []Block {
	slices.SortFunc(strikes, func(a, b strike) int {
		if a.at < b.at {
			return -1
		}
		if a.at > b.at {
			return 1
		}
		return 0
	})
	hop := float64(synthBlock) / synthRate
	const sub = 16
	var out []Block
	first := 0
	for i := 0; float64(i)*hop < seconds; i++ {
		at := float64(i) * hop
		var power [bandCount]float64
		for s := range sub {
			t := at + (float64(s)+0.5)*hop/sub
			b := under(t)
			for first < len(strikes) && strikes[first].at < t-2 {
				first++
			}
			for _, h := range strikes[first:] {
				if h.at > t {
					break
				}
				for k := range bandCount {
					b[k] += h.gain * h.v.power[k] * math.Exp(-(t-h.at)/h.v.decay)
				}
			}
			for k := range bandCount {
				power[k] += b[k] / sub
			}
		}
		bands := make([]float64, bandCount)
		total := 0.0
		for k, p := range power {
			bands[k] = 10 * math.Log10(p)
			total += p
		}
		c := Channel{Loudness: 10 * math.Log10(total), Difference: math.Sqrt(power[bandMid]+power[bandHigh]) / 4, Crossings: 0.05 * synthBlock}
		out = append(out, Block{At: at, Samples: synthBlock, Channels: []Channel{c}, Bands: bands, Rate: synthRate, Scale: 1})
	}
	return out
}

// groove is a track's beats in seconds and the strikes that play it.
type groove struct {
	beats, downbeats    []float64
	kicks, snares, hats []float64
	strikes             []strike
}

// play adds a strike to a groove, keeping its time in the list of its kind.
func (g *groove) play(at float64, v voice, gain float64, kind *[]float64) {
	g.strikes = append(g.strikes, strike{at, v, gain})
	if kind != nil {
		*kind = append(*kind, at)
	}
}

// danceGroove is a dance track at a steady tempo with a phase offset:
// a kick on every beat, a clap on the second and fourth, a hat on every
// off-beat, and a bassline whose note changes loudness at each bar.
func danceGroove(bpm, seconds, offset float64) (groove, bed) {
	var g groove
	beat := 60 / bpm
	for i := 0; offset+float64(i)*beat < seconds-1; i++ {
		t := offset + float64(i)*beat
		g.beats = append(g.beats, t)
		if i%4 == 0 {
			g.downbeats = append(g.downbeats, t)
		}
		g.play(t, kickVoice, 1, &g.kicks)
		if i%2 == 1 {
			g.play(t, clapVoice, 1, &g.snares)
		}
		g.play(t+beat/2, hatVoice, 1, &g.hats)
	}
	notes := []float64{0.05, 0.02, 0.08, 0.03}
	bass := func(t float64) [bandCount]float64 {
		bar := int(math.Floor((t - offset) / (4 * beat)))
		v := notes[((bar%4)+4)%4]
		return [bandCount]float64{v, v, 1e-4, 1e-5}
	}
	return g, bass
}

// nearest returns the distance from t to the nearest time in a sorted list.
func nearest(times []float64, t float64) float64 {
	i, _ := slices.BinarySearch(times, t)
	d := math.Inf(1)
	for _, j := range []int{i - 1, i} {
		if j >= 0 && j < len(times) {
			d = math.Min(d, math.Abs(times[j]-t))
		}
	}
	return d
}

// tracked reports how many of the true beats, between from and to seconds,
// have a planned beat within 20 ms, and how many there are.
func tracked(t *testing.T, p *Plan, truth []float64, from, to float64) {
	t.Helper()
	hit, all := 0, 0
	var worst float64
	for _, b := range truth {
		if b < from || b > to {
			continue
		}
		all++
		d := nearest(p.Beats, b)
		if d <= 0.02 {
			hit++
		}
		worst = math.Max(worst, d)
	}
	if all == 0 || hit < all {
		t.Errorf("%d of %d beats from %vs to %vs have a planned beat within 20 ms; the worst is %.0f ms off; the plan has %d beats at %.2f BPM",
			hit, all, from, to, 1000*worst, len(p.Beats), p.Tempo)
	}
}

func TestTheGridOfAFourOnTheFloorTrackIsWithin20msOfEveryBeat(t *testing.T) {
	g, bass := danceGroove(124, 90, 0.137)
	p := NewPlan(banded(90, g.strikes, bass))
	if math.Abs(p.Tempo-124) > 0.5 {
		t.Errorf("the tempo is %.2f, want 124", p.Tempo)
	}
	tracked(t, p, g.beats, 2, 87)
	if p.Beatless() || p.Pulse < 0.8 {
		t.Errorf("a steady four on the floor has Pulse %.2f", p.Pulse)
	}
}

func TestTheDownbeatsOfAFourOnTheFloorTrackFallOnItsBars(t *testing.T) {
	g, bass := danceGroove(124, 90, 0.137)
	p := NewPlan(banded(90, g.strikes, bass))
	if p.Meter != 4 {
		t.Fatalf("the meter is %d, want 4", p.Meter)
	}
	on := 0
	for _, d := range p.Downbeats {
		if nearest(g.downbeats, d) <= 0.02 {
			on++
		}
	}
	if len(p.Downbeats) == 0 || float64(on) < 0.9*float64(len(p.Downbeats)) {
		t.Errorf("%d of %d downbeats fall on a bar", on, len(p.Downbeats))
	}
}

func TestTheHitsOfAFourOnTheFloorTrackAreKicksClapsAndHats(t *testing.T) {
	g, bass := danceGroove(124, 60, 0.137)
	p := NewPlan(banded(60, g.strikes, bass))
	found := map[HitKind][]float64{}
	for _, h := range p.Hits {
		found[h.Kind] = append(found[h.Kind], h.At)
		if h.Strength <= 0 || h.Strength > 1 {
			t.Errorf("a %s at %.3fs has strength %v", h.Kind, h.At, h.Strength)
		}
	}
	for _, c := range []struct {
		kind  HitKind
		truth []float64
	}{{HitKick, g.kicks}, {HitSnare, g.snares}, {HitHat, g.hats}} {
		heard := 0
		for _, at := range c.truth {
			if nearest(found[c.kind], at) <= 0.03 {
				heard++
			}
		}
		false := 0
		for _, at := range found[c.kind] {
			if nearest(c.truth, at) > 0.03 {
				false++
			}
		}
		if float64(heard) < 0.9*float64(len(c.truth)) || float64(false) > 0.1*float64(len(found[c.kind])) {
			t.Errorf("%d of %d %ss were found, and %d of %d found are not one", heard, len(c.truth), c.kind, false, len(found[c.kind]))
		}
	}
}

func TestAHalfTimeGrooveKeepsTheBeatOfItsHats(t *testing.T) {
	var g groove
	beat := 60 / 140.0
	for i := 0; float64(i)*beat < 89; i++ {
		at := 0.211 + float64(i)*beat
		g.beats = append(g.beats, at)
		g.play(at, hatVoice, 1, nil)
		switch i % 4 {
		case 0:
			g.play(at, kickVoice, 1, nil)
		case 2:
			g.play(at, clapVoice, 1, nil)
		}
	}
	p := NewPlan(banded(90, g.strikes, quietBed))
	if math.Abs(p.Tempo-140) > 0.5 {
		t.Errorf("the tempo is %.2f, want 140", p.Tempo)
	}
	tracked(t, p, g.beats, 2, 87)
}

func TestASwungRideIsTrackedOnItsBeatsAndNotItsSkips(t *testing.T) {
	var g groove
	beat := 60 / 132.0
	for i := 0; float64(i)*beat < 89; i++ {
		at := 0.05 + float64(i)*beat
		g.beats = append(g.beats, at)
		g.play(at, rideVoice, 1, nil)
		g.play(at, thumpVoice, 0.5, nil)
		if i%2 == 1 {
			g.play(at+2*beat/3, rideVoice, 0.6, nil)
		}
	}
	p := NewPlan(banded(90, g.strikes, quietBed))
	if math.Abs(p.Tempo-132) > 0.5 {
		t.Errorf("the tempo is %.2f, want 132", p.Tempo)
	}
	tracked(t, p, g.beats, 2, 87)
}

func TestTheGridFollowsATempoThatRisesFrom120To128(t *testing.T) {
	var g groove
	const seconds = 120.0
	at := 0.3
	for at < seconds-1 {
		g.beats = append(g.beats, at)
		g.play(at, kickVoice, 1, nil)
		bpm := 120 + 8*at/seconds
		g.play(at+30/bpm, hatVoice, 1, nil)
		at += 60 / bpm
	}
	p := NewPlan(banded(seconds, g.strikes, quietBed))
	tracked(t, p, g.beats, 2, seconds-3)
}

func TestABeatlessPadWithSwellsAndFlickersHasNoGrid(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var g groove
	for at := 0.0; at < 180; at += r.ExpFloat64() / 1.5 {
		g.play(at, hatVoice, 0.2+0.8*r.Float64(), nil)
	}
	for at := 0.0; at < 180; at += r.ExpFloat64() / 0.3 {
		g.play(at, thumpVoice, 0.3+0.7*r.Float64(), nil)
	}
	swell := func(t float64) [bandCount]float64 {
		a := 0.02 * (1.2 + math.Sin(2*math.Pi*t/7.3)) * (1.2 + math.Sin(2*math.Pi*t/12.9+1))
		return [bandCount]float64{a / 2, a, a / 3, a / 20}
	}
	p := NewPlan(banded(180, g.strikes, swell))
	if !p.Beatless() || len(p.Beats) != 0 || p.Pulse >= beatlessBelow {
		t.Errorf("a beatless pad has Pulse %.2f and %d beats at %.1f BPM", p.Pulse, len(p.Beats), p.Tempo)
	}
	if _, _, _, _, ok := p.BeatAt(60); ok {
		t.Error("a beatless pad has a beat at 60 s")
	}
	c := p.Character()
	if c.Drive >= math.Max(c.Calm, c.Flow) {
		t.Errorf("a beatless pad's character is %+v, led by drive", c)
	}
}

func TestAFourOnTheFloorTrackIsLedByDrive(t *testing.T) {
	g, bass := danceGroove(124, 90, 0.137)
	c := NewPlan(banded(90, g.strikes, bass)).Character()
	if c.Drive <= math.Max(c.Calm, c.Flow) {
		t.Errorf("a four on the floor's character is %+v, not led by drive over calm and flow", c)
	}
	if sum := c.Calm + c.Drive + c.Grit + c.Flow; math.Abs(sum-1) > 1e-9 {
		t.Errorf("the character adds up to %v", sum)
	}
}

func TestBeatAtGivesTheTempoPhaseBeatAndPlaceInTheBar(t *testing.T) {
	g, bass := danceGroove(120, 60, 0.137)
	p := NewPlan(banded(60, g.strikes, bass))
	if len(p.Beats) < 10 {
		t.Fatalf("the plan has %d beats", len(p.Beats))
	}
	b := 10
	mid := (p.Beats[b] + p.Beats[b+1]) / 2
	tempo, phase, beat, bar, ok := p.BeatAt(mid)
	if !ok || beat != b || math.Abs(phase-0.5) > 1e-9 || math.Abs(tempo-120) > 3 || bar != p.bar[b] {
		t.Errorf("BeatAt(%.3f) = %.1f, %.2f, %d, %d, %v; want about 120, 0.5, %d, %d, true", mid, tempo, phase, beat, bar, ok, b, p.bar[b])
	}
	if _, phase, beat, _, ok := p.BeatAt(p.Beats[b]); !ok || beat != b || phase != 0 {
		t.Errorf("on beat %d BeatAt says beat %d at phase %v", b, beat, phase)
	}
	for _, at := range []float64{p.Beats[0] - 0.01, p.Beats[len(p.Beats)-1]} {
		if _, _, _, _, ok := p.BeatAt(at); ok {
			t.Errorf("BeatAt(%.3f) is outside the beats and reports one", at)
		}
	}
	for i, d := range p.Downbeats {
		if _, _, _, bar, _ := p.BeatAt(d); bar != 0 {
			t.Errorf("downbeat %d at %.3f is at place %d in its bar", i, d, bar)
		}
	}
}

func TestHitsBetweenReturnsTheHitsInOrderWithinTheSpan(t *testing.T) {
	g, bass := danceGroove(120, 30, 0.137)
	p := NewPlan(banded(30, g.strikes, bass))
	got := p.HitsBetween(10, 12)
	if len(got) == 0 {
		t.Fatal("no hits between 10 and 12 s")
	}
	for i, h := range got {
		if h.At < 10 || h.At >= 12 || (i > 0 && h.At < got[i-1].At) {
			t.Errorf("hit %d at %.3f is out of order or outside 10 to 12 s", i, h.At)
		}
	}
	if (*Plan)(nil).HitsBetween(0, 1) != nil {
		t.Error("a nil plan has hits")
	}
}

func TestATrackMeasuredWithoutBandsHasNoBeatsAndIsNotBeatless(t *testing.T) {
	p := NewPlan(timeline(60, func(float64) float64 { return 0.5 }))
	if len(p.Beats) != 0 || len(p.Hits) != 0 || p.Beatless() {
		t.Errorf("a plan without bands has %d beats, %d hits, and beatless %v", len(p.Beats), len(p.Hits), p.Beatless())
	}
	if c := p.Character(); c != (Character{0.25, 0.25, 0.25, 0.25}) {
		t.Errorf("a plan without bands has character %+v", c)
	}
	if _, _, _, _, ok := p.BeatAt(10); ok {
		t.Error("a plan without bands has a beat")
	}
}

func TestASilentTrackMeasuredInBandsHasNoBeatsAndNoHits(t *testing.T) {
	silence := func(float64) [bandCount]float64 { return [bandCount]float64{} }
	p := NewPlan(banded(60, nil, silence))
	if !p.Beatless() || len(p.Hits) != 0 || math.IsNaN(p.Pulse) {
		t.Errorf("a silent track has Pulse %v, %d beats and %d hits", p.Pulse, len(p.Beats), len(p.Hits))
	}
}

func TestHitKindsHaveNames(t *testing.T) {
	for k, want := range map[HitKind]string{HitKick: "kick", HitSnare: "snare", HitHat: "hat", HitKind(7): "unknown"} {
		if k.String() != want {
			t.Errorf("%d is named %q, want %q", k, k.String(), want)
		}
	}
}
