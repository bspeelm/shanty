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

func TestTheHitsOfAFourOnTheFloorTrackAreKicksClapsAndHatsWithAKickUnderAClapCountedOnce(t *testing.T) {
	g, bass := danceGroove(124, 60, 0.137)
	p := NewPlan(banded(60, g.strikes, bass))
	found := map[HitKind][]float64{}
	var all []float64
	for _, h := range p.Hits {
		found[h.Kind] = append(found[h.Kind], h.At)
		all = append(all, h.At)
		if h.Strength <= 0 || h.Strength > 1 {
			t.Errorf("a %s at %.3fs has strength %v", h.Kind, h.At, h.Strength)
		}
	}
	var alone []float64
	for _, k := range g.kicks {
		if nearest(g.snares, k) > 0.01 {
			alone = append(alone, k)
		}
	}
	drums := slices.Concat(found[HitKick], found[HitSnare])
	slices.Sort(drums)
	for _, c := range []struct {
		name         string
		truth, found []float64
	}{{"lone kick", alone, found[HitKick]}, {"kick under a clap", g.snares, drums}, {"hat", g.hats, found[HitHat]}} {
		heard := 0
		for _, at := range c.truth {
			if nearest(c.found, at) <= 0.03 {
				heard++
			}
		}
		if float64(heard) < 0.9*float64(len(c.truth)) {
			t.Errorf("%d of %d %ss were found", heard, len(c.truth), c.name)
		}
	}
	for _, at := range g.snares {
		n := 0
		for _, h := range all {
			if math.Abs(h-at) <= 0.03 {
				n++
			}
		}
		if n > 1 {
			t.Errorf("the kick and clap at %.3f are %d hits", at, n)
		}
	}
	truth := slices.Concat(g.kicks, g.hats)
	slices.Sort(truth)
	false := 0
	for _, at := range all {
		if nearest(truth, at) > 0.03 {
			false++
		}
	}
	if float64(false) > 0.05*float64(len(all)) {
		t.Errorf("%d of %d hits are not a drum", false, len(all))
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

// beatlessPad is three minutes of slow swells, random flickers in the high
// band, and random low thumps.
func beatlessPad() []Block {
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
	return banded(180, g.strikes, swell)
}

func TestABeatlessTrackHasAtMostHalfAStrongHitASecondInAnyFiveSeconds(t *testing.T) {
	p := NewPlan(beatlessPad())
	var strong []float64
	for _, h := range p.Hits {
		if h.Strength >= 0.3 {
			strong = append(strong, h.At)
		}
	}
	for i, at := range strong {
		j, _ := slices.BinarySearch(strong, at+5)
		if j-i > 2 {
			t.Errorf("a beatless track has %d strong hits in the 5 s from %.1f s", j-i, at)
		}
	}
	for i := 1; i < len(p.Hits); i++ {
		if p.Hits[i].At-p.Hits[i-1].At < 0.03 {
			t.Errorf("hits at %.3f and %.3f are one sound counted twice", p.Hits[i-1].At, p.Hits[i].At)
		}
	}
}

func TestABeatlessPadWithSwellsAndFlickersHasNoGrid(t *testing.T) {
	p := NewPlan(beatlessPad())
	if !p.Beatless() || len(p.Beats) != 0 || p.Pulse >= beatlessBelow {
		t.Errorf("a beatless pad has Pulse %.2f and %d beats at %.1f BPM", p.Pulse, len(p.Beats), p.Tempo)
	}
	if _, ok := p.BeatAt(60); ok {
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
	if c.Drive <= max(c.Calm, c.Flow, c.Grit) {
		t.Errorf("a four on the floor's character is %+v, not led by drive", c)
	}
	if sum := c.Calm + c.Drive + c.Grit + c.Flow; math.Abs(sum-1) > 1e-9 {
		t.Errorf("the character adds up to %v", sum)
	}
}

func TestAFourOnTheFloorKickOverARollingBasslineIsFoundAndLedByDrive(t *testing.T) {
	var g groove
	beat := 60 / 126.0
	roll := voice{[bandCount]float64{0.5, 0.4, 0.002, 0}, 0.06}
	for i := 0; float64(i)*beat < 89; i++ {
		at := 0.1 + float64(i)*beat
		g.beats = append(g.beats, at)
		g.play(at, kickVoice, 1, &g.kicks)
		for q := 1; q < 4; q++ {
			g.play(at+float64(q)*beat/4, roll, 0.8, nil)
		}
		g.play(at+beat/2, hatVoice, 1, nil)
	}
	steady := func(float64) [bandCount]float64 { return [bandCount]float64{0.1, 0.1, 1e-4, 1e-5} }
	p := NewPlan(banded(90, g.strikes, steady))
	tracked(t, p, g.beats, 2, 87)
	var kicks []float64
	for _, h := range p.Hits {
		if h.Kind == HitKick {
			kicks = append(kicks, h.At)
		}
	}
	heard := 0
	for _, k := range g.kicks {
		if nearest(kicks, k) <= 0.03 {
			heard++
		}
	}
	if float64(heard) < 0.9*float64(len(g.kicks)) {
		t.Errorf("%d of %d kicks over the bassline were found", heard, len(g.kicks))
	}
	if c := p.Character(); c.Drive <= max(c.Calm, c.Flow, c.Grit) {
		t.Errorf("a four on the floor over a rolling bassline has character %+v, not led by drive", c)
	}
}

func TestASilentGapInAGrooveHasNoBeats(t *testing.T) {
	var g groove
	beat := 0.5
	for at := 0.25; at < 100; at += beat {
		if at > 30 && at < 70 {
			continue
		}
		g.beats = append(g.beats, at)
		g.play(at, kickVoice, 1, nil)
		g.play(at+beat/2, hatVoice, 1, nil)
	}
	silence := func(float64) [bandCount]float64 { return [bandCount]float64{} }
	p := NewPlan(banded(100, g.strikes, silence))
	for _, b := range p.Beats {
		if b > 31 && b < 70 {
			t.Errorf("a beat at %.2f s falls in the silence", b)
		}
	}
	if beat, ok := p.BeatAt(50); ok {
		t.Errorf("BeatAt(50) in the silence is %+v", beat)
	}
	tracked(t, p, g.beats, 2, 29)
	tracked(t, p, g.beats, 71, 97)
}

func TestTheTempoOfAnExactGrooveHoldsFromBeatToBeat(t *testing.T) {
	g, bass := danceGroove(120, 60, 0.137)
	p := NewPlan(banded(60, g.strikes, bass))
	for b := 4; b+5 < len(p.Beats); b++ {
		mid := (p.Beats[b] + p.Beats[b+1]) / 2
		if beat, ok := p.BeatAt(mid); !ok || math.Abs(beat.Tempo-120) > 0.5 {
			t.Errorf("at beat %d the tempo is %+v, want 120 within 0.5", b, beat)
		}
	}
}

func TestWhenTheKicksStopTheGridDoesNotMoveOntoTheOffbeatHats(t *testing.T) {
	var g groove
	beat := 60 / 125.0
	var hats []float64
	for i := 0; float64(i)*beat < 99; i++ {
		at := 0.2 + float64(i)*beat
		g.beats = append(g.beats, at)
		if at < 40 || at > 64 {
			g.play(at, kickVoice, 1, nil)
			if i%2 == 1 {
				g.play(at, clapVoice, 1, nil)
			}
		} else {
			hats = append(hats, at+beat/2)
		}
		g.play(at+beat/2, hatVoice, 1, nil)
	}
	p := NewPlan(banded(100, g.strikes, quietBed))
	for _, b := range p.Beats {
		if b > 40 && b < 64 && nearest(hats, b) < 0.06 {
			t.Errorf("the beat at %.3f s is on an offbeat hat", b)
		}
	}
	tracked(t, p, g.beats, 2, 39)
	tracked(t, p, g.beats, 65, 97)
}

func TestBeatAtGivesTheTempoPhaseBeatAndPlaceInTheBar(t *testing.T) {
	g, bass := danceGroove(120, 60, 0.137)
	p := NewPlan(banded(60, g.strikes, bass))
	if len(p.Beats) < 10 {
		t.Fatalf("the plan has %d beats", len(p.Beats))
	}
	b := 10
	mid := (p.Beats[b] + p.Beats[b+1]) / 2
	got, ok := p.BeatAt(mid)
	if !ok || got.Index != b || math.Abs(got.Phase-0.5) > 1e-9 || math.Abs(got.Tempo-120) > 1 || got.Bar != p.bar[b] || got.Confidence <= 0.5 {
		t.Errorf("BeatAt(%.3f) = %+v, %v; want about 120 BPM, phase 0.5, beat %d, bar %d, confident", mid, got, ok, b, p.bar[b])
	}
	if got, ok := p.BeatAt(p.Beats[b]); !ok || got.Index != b || got.Phase != 0 {
		t.Errorf("on beat %d BeatAt says %+v", b, got)
	}
	for _, at := range []float64{p.Beats[0] - 0.01, p.Beats[len(p.Beats)-1]} {
		if _, ok := p.BeatAt(at); ok {
			t.Errorf("BeatAt(%.3f) is outside the beats and reports one", at)
		}
	}
	for i, d := range p.Downbeats {
		if got, _ := p.BeatAt(d); got.Bar != 0 {
			t.Errorf("downbeat %d at %.3f is at place %d in its bar", i, d, got.Bar)
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
	if _, ok := p.BeatAt(10); ok {
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

func TestHitsBetweenAnEndBeforeItsStartIsEmpty(t *testing.T) {
	g, bass := danceGroove(120, 30, 0.137)
	p := NewPlan(banded(30, g.strikes, bass))
	if got := p.HitsBetween(12.5, 11.5); len(got) != 0 {
		t.Errorf("HitsBetween(12.5, 11.5) returned %d hits", len(got))
	}
}

func TestBlocksOfNaNLeaveNoNaNInThePlan(t *testing.T) {
	nan := math.NaN()
	var blocks []Block
	for i := range 3000 {
		c := Channel{Loudness: nan, Difference: nan, Crossings: nan}
		blocks = append(blocks, Block{At: float64(i) * synthBlock / synthRate, Samples: synthBlock, Rate: synthRate, Scale: 1,
			Channels: []Channel{c, c}, Bands: []float64{nan, nan, nan, nan}})
	}
	p := NewPlan(blocks)
	c := p.Character()
	for name, v := range map[string]float64{"Pulse": p.Pulse, "Tempo": p.Tempo, "Sure": p.Sure, "Calm": c.Calm, "Drive": c.Drive,
		"Grit": c.Grit, "Flow": c.Flow, "Level": p.At(10).Level, "Tension": p.Tension(10)} {
		if math.IsNaN(v) {
			t.Errorf("%s is NaN", name)
		}
	}
	if len(p.Hits) != 0 || len(p.Beats) != 0 {
		t.Errorf("blocks of NaN gave %d hits and %d beats", len(p.Hits), len(p.Beats))
	}
	for _, cue := range p.Cues {
		if math.IsNaN(cue.At) || math.IsNaN(cue.Strength) {
			t.Errorf("a cue is %+v", cue)
		}
	}
}
