package sky

import (
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// The screen is split into meterCols by meterRows regions, and each is
// watched for flashes on its own.
const meterCols, meterRows = 3, 2

// relLum is the relative luminance of a colour, from 0 for black to 1 for
// white.
func relLum(c RGB) float64 {
	lin := func(u uint8) float64 {
		v := float64(u) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// ink is how much of a cell a character covers, from 0 to 1.
func ink(ch rune) float64 {
	switch {
	case ch >= 0x2800 && ch <= 0x28ff:
		bits := ch - 0x2800
		n := 0
		for ; bits != 0; bits &= bits - 1 {
			n++
		}
		return float64(n) / 8
	case ch == '▀':
		return 1
	case slices.Contains(shadeRamp, ch):
		return float64(slices.Index(shadeRamp, ch)) / float64(len(shadeRamp)-1)
	case slices.Contains(asciiRamp, ch):
		return 0.3 * float64(slices.Index(asciiRamp, ch)) / float64(len(asciiRamp)-1)
	}
	return 0
}

// regionLight is the mean relative luminance of each region of a picture
// drawn on a black background.
func regionLight(p Picture, w, h int) []float64 {
	sum := make([]float64, meterCols*meterRows)
	n := make([]float64, len(sum))
	lum := func(i int) float64 {
		if i < 0 || i >= len(p.Palette) {
			return 0
		}
		return relLum(p.Palette[i])
	}
	for r, line := range p.Lines {
		c := 0
		for _, run := range line {
			for _, ch := range run.Text {
				l := ink(ch) * lum(run.Fg)
				if ch == '▀' {
					l = (lum(run.Fg) + lum(run.Bg)) / 2
				}
				k := min(r*meterRows/h, meterRows-1)*meterCols + min(c*meterCols/w, meterCols-1)
				sum[k] += l
				n[k]++
				c++
			}
		}
	}
	for k := range sum {
		if n[k] > 0 {
			sum[k] /= n[k]
		}
	}
	return sum
}

// flashMeter counts flashes in one region: a flash is a pair of opposing
// changes in luminance of at least 0.1.
type flashMeter struct {
	dir    int
	lo, hi float64
	seen   bool
	turns  []float64
}

func (m *flashMeter) add(l, at float64) {
	if !m.seen {
		m.lo, m.hi, m.seen = l, l, true
		return
	}
	switch {
	case m.dir != 1 && l-m.lo >= 0.1:
		m.dir, m.lo, m.hi = 1, l, l
		m.turns = append(m.turns, at)
	case m.dir != -1 && m.hi-l >= 0.1:
		m.dir, m.lo, m.hi = -1, l, l
		m.turns = append(m.turns, at)
	default:
		m.lo, m.hi = math.Min(m.lo, l), math.Max(m.hi, l)
	}
}

// worst is the most flashes in any one second.
func (m *flashMeter) worst() float64 {
	most := 0
	for i, at := range m.turns {
		j := i
		for j < len(m.turns) && m.turns[j] < at+1 {
			j++
		}
		most = max(most, j-i)
	}
	return float64(most) / 2
}

// synth is a synthetic track: the power of its low end, its high end and its
// middle at a time in seconds.
type synth func(at float64) (bass, hats, mid float64)

// kicks is 120 BPM hard kicks with a hi-hat between them.
func kicks(at float64) (bass, hats, mid float64) {
	beat := math.Mod(at, 0.5)
	bass = 0.001
	if beat < 0.08 {
		bass = 0.3
	}
	if math.Abs(beat-0.25) < 0.04 {
		hats = 0.02
	}
	return bass, hats, 0
}

// strobe is a loud kick and snap together eight times a second.
func strobe(at float64) (bass, hats, mid float64) {
	if math.Mod(at, 0.125) < 0.03 {
		return 0.6, 0.3, 0
	}
	return 0.001, 0.0005, 0
}

// swelling is music with no beat: a pad that swells and fades over seven
// seconds, scattered high transients, and every sixteen seconds a build that
// stops dead and comes back loud.
func swelling(at float64) (bass, hats, mid float64) {
	mid = 0.01 + 0.08*(0.5-0.5*math.Cos(2*math.Pi*at/7))
	cyc := math.Mod(at, 16)
	switch {
	case cyc > 12 && cyc < 15:
		mid += 0.1 * (cyc - 12)
	case cyc >= 15 && cyc < 15.5:
		mid = 0.0005
	case cyc >= 15.5:
		mid = 0.3
	}
	if h := hash(int(at*7), 3); h < 0.3 && math.Mod(at*7, 1) < 0.2 {
		hats = 0.05 * h / 0.3
	}
	return mid * 0.3, hats, mid
}

// watch plays snd into s for secs seconds at 30 frames a second, w by h, and
// returns a flash meter for each region of what was drawn. When favour names
// components, a scene favouring them is put on screen whenever the one
// showing has none of them.
func watch(s *State, snd synth, secs float64, w, h int, favour ...string) []flashMeter {
	var tr track
	meters := make([]flashMeter, meterCols*meterRows)
	dt := 1.0 / 30
	for i := 0; float64(i)*dt < secs; i++ {
		at := float64(i) * dt
		bass, hats, mid := snd(at)
		energy := bass + hats + mid
		diff := bass*band(bassHz, trackRate) + hats*band(trebleHz, trackRate) + mid*band(1000, trackRate)
		s.Hear(tr.block(dt, both(energy), both(diff), 0.05, at-0.27))
		s.Step(w, h, dt, time.Time{}, "track")
		if len(favour) > 0 && s.next == nil && !slices.ContainsFunc(favour, func(n string) bool {
			return strings.Contains(s.cur.combo(), n)
		}) {
			s.cur = favouring(s, favour)
		}
		for k, l := range regionLight(s.Render(true), w, h) {
			meters[k].add(l, at)
		}
	}
	return meters
}

// favouring makes a scene for the music s has heard in which the named
// components are almost always chosen.
func favouring(s *State, names []string) *scene {
	st := styleFor(s.lis.char)
	for _, n := range names {
		st.kinds[n] = 1e6
	}
	s.life = 1e9
	return newScene(rand.New(rand.NewPCG(uint64(s.frames), 7)), st, s.lis.char, Signals{Level: 1, Busy: 1})
}

// worstOf is the most flashes in any one second in any region.
func worstOf(meters []flashMeter) float64 {
	most := 0.0
	for i := range meters {
		most = math.Max(most, meters[i].worst())
	}
	return most
}

// flashLimit is the most flashes a second any region of the screen may show.
const flashLimit = 2

// flashCase is one synthetic track watched for flashes.
type flashCase struct {
	name   string
	snd    synth
	favour []string
}

var flashCases = []flashCase{
	{"hard kicks at 120 BPM", kicks, nil},
	{"a kick and snap eight times a second", strobe, nil},
	{"a kick and snap eight times a second over a grid", strobe, []string{"grid"}},
	{"a kick and snap eight times a second over sparks", strobe, []string{"sparks"}},
	{"a kick and snap eight times a second over a storm", strobe, []string{"storm"}},
	{"a kick and snap eight times a second over rings", strobe, []string{"rings"}},
	{"music with no beat that swells and drops", swelling, nil},
	{"music with no beat over a storm", swelling, []string{"storm"}},
}

// sweepFlashes watches every case for every seed below seeds, for secs
// seconds w by h, and fails a case whose worst region flashes more than
// flashLimit times in a second. Seeds are watched side by side, as many at
// once as there are processors.
func sweepFlashes(t *testing.T, seeds int, secs float64, w, h int) {
	slots := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, fc := range flashCases {
		t.Run(fc.name, func(t *testing.T) {
			t.Parallel()
			worst := make([]float64, seeds)
			var wg sync.WaitGroup
			for seed := range seeds {
				wg.Go(func() {
					slots <- struct{}{}
					worst[seed] = worstOf(watch(New(uint64(seed)), fc.snd, secs, w, h, fc.favour...))
					<-slots
				})
			}
			wg.Wait()
			at := 0
			for seed, n := range worst {
				if n > worst[at] {
					at = seed
				}
			}
			t.Logf("%s: worst %.1f flashes a second, seed %d", fc.name, worst[at], at)
			if worst[at] > flashLimit {
				t.Errorf("seed %d flashed %.1f times a second in one region, want at most %d", at, worst[at], flashLimit)
			}
		})
	}
}

func TestNoRegionFlashesMoreThanTwiceASecond(t *testing.T) {
	seeds, secs := 4, 20.0
	if testing.Short() {
		seeds, secs = 2, 12
	}
	sweepFlashes(t, seeds, secs, 60, 20)
}

// guarded returns a guard for a picture w by h whose value and tone are both
// field, after it has seen the picture still for a second.
func guarded(field []float64, w, h int, half bool) *flashGuard {
	g := &flashGuard{}
	g.resize(w, h)
	still := slices.Clone(field)
	for range 30 {
		copy(field, still)
		g.apply(field, slices.Clone(still), 1.0/30, half)
	}
	return g
}

// fill sets every value of field to v.
func fill(field []float64, v float64) {
	for i := range field {
		field[i] = v
	}
}

func TestAScreenFlashingBlackAndWhiteIsHeldNearItsReference(t *testing.T) {
	const w, h = 90, 40
	for _, half := range []bool{false, true} {
		way := lightDots
		if half {
			way = lightHalf
		}
		field := make([]float64, w*h)
		g := guarded(field, w, h, half)
		for i := range 60 {
			fill(field, float64(i%2))
			tone := slices.Clone(field)
			before := g.ref[way]
			g.apply(field, tone, 1.0/30, half)
			lit, _ := g.measure(field, tone)
			for k, m := range lit[way] {
				if math.Abs(m-g.ref[way][k]) > guardBand+guardSlack+1e-9 {
					t.Fatalf("half blocks %v, frame %d: region %d had light %.3f against a reference of %.3f", half, i, k, m, g.ref[way][k])
				}
				if d := math.Abs(g.ref[way][k] - before[k]); d > guardDrift/30+1e-9 {
					t.Fatalf("half blocks %v, frame %d: region %d's reference moved %.4f in a frame", half, i, k, d)
				}
			}
		}
	}
}

func TestAScreenGoingDarkAtOnceIsHeldUpTowardsWhite(t *testing.T) {
	const w, h = 60, 30
	for _, half := range []bool{false, true} {
		way := lightDots
		if half {
			way = lightHalf
		}
		field := make([]float64, w*h)
		fill(field, 0.8)
		g := guarded(field, w, h, half)
		clear(field)
		tone := make([]float64, len(field))
		g.apply(field, tone, 1.0/30, half)
		lit, _ := g.measure(field, tone)
		for k, m := range lit[way] {
			if m < g.ref[way][k]-guardBand-guardSlack-1e-9 {
				t.Errorf("half blocks %v: region %d fell to light %.3f against a reference of %.3f", half, k, m, g.ref[way][k])
			}
		}
	}
}

func TestATonePushedBrightAtOnceIsHeldBack(t *testing.T) {
	const w, h = 60, 30
	field := make([]float64, w*h)
	fill(field, 0.4)
	g := guarded(field, w, h, true)
	tone := make([]float64, len(field))
	fill(tone, 1)
	g.apply(field, tone, 1.0/30, true)
	lit, _ := g.measure(field, tone)
	for k, m := range lit[lightHalf] {
		if m > g.ref[lightHalf][k]+guardBand+guardSlack+1e-9 {
			t.Errorf("region %d rose to light %.3f against a reference of %.3f", k, m, g.ref[lightHalf][k])
		}
	}
}

func TestASteadyPictureIsLeftAsItIs(t *testing.T) {
	const w, h = 70, 30
	field := make([]float64, w*h)
	for i := range field {
		field[i] = hash(i, 5)
	}
	want := slices.Clone(field)
	g := guarded(field, w, h, false)
	g.apply(field, slices.Clone(want), 1.0/30, false)
	if !slices.Equal(field, want) {
		t.Error("the guard changed a picture that had not changed")
	}
}

func TestAKickWithinTheBandPassesUntouched(t *testing.T) {
	const w, h = 60, 30
	field := make([]float64, w*h)
	fill(field, 0.5)
	g := guarded(field, w, h, false)
	fill(field, 0.53)
	g.apply(field, slices.Clone(field), 1.0/30, false)
	if field[0] != 0.53 {
		t.Errorf("a brightening from 0.5 to 0.53 came out as %.3f", field[0])
	}
}

func TestTheGuardLeavesNoSeamBetweenRegions(t *testing.T) {
	const w, h = 120, 60
	field := make([]float64, w*h)
	fill(field, 0.3)
	g := guarded(field, w, h, false)
	// The whole picture brightens, more to the right and lower down, so that
	// every region is corrected by a different amount.
	for y := range h {
		for x := range w {
			field[y*w+x] = 0.3 + 0.4*float64(x)/w + 0.2*float64(y)/h
		}
	}
	in := 0.0
	for x := 1; x < w; x++ {
		in = math.Max(in, field[x]-field[x-1])
	}
	g.apply(field, slices.Clone(field), 1.0/30, false)
	for y := range h {
		for x := 1; x < w; x++ {
			if d := math.Abs(field[y*w+x] - field[y*w+x-1]); d > 2*in {
				t.Fatalf("at (%d, %d) neighbouring pixels differ by %.4f, where the picture's steepest step is %.4f", x, y, d, in)
			}
		}
	}
	for x := range w {
		for y := 1; y < h; y++ {
			if d := math.Abs(field[y*w+x] - field[(y-1)*w+x]); d > 0.02 {
				t.Fatalf("at (%d, %d) a pixel differs from the one above by %.4f", x, y, d)
			}
		}
	}
}

func TestTowardsWhiteFindsTheLightAskedFor(t *testing.T) {
	const n = 500
	var m moments
	value, tone := make([]float64, n), make([]float64, n)
	for i := range n {
		value[i], tone[i] = 0.4*hash(i, 9), 0.5*hash(i, 4)
		a, b := 1-(value[i]*0.8+tone[i]*0.2), 1-value[i]
		for j, p := range [5]float64{a, a * a, b, a * b, a * a * b} {
			m[j] += p / n
		}
	}
	for _, half := range []bool{false, true} {
		c := towardsWhite(m, half, 0.3)
		got := 0.0
		for i := range n {
			v, tn := 1-(1-value[i])*c, 1-(1-tone[i])*c
			u := v*0.8 + tn*0.2
			if half {
				got += u * u / n
			} else {
				got += u * u * v / n
			}
		}
		if math.Abs(got-0.3) > 1e-4 {
			t.Errorf("half blocks %v: moved towards white by %.3f the light was %.4f, want 0.3", half, c, got)
		}
	}
}

func TestAResizeForgetsTheReferences(t *testing.T) {
	field := make([]float64, 40*20)
	g := guarded(field, 40, 20, false)
	g.resize(50, 20)
	if g.ready {
		t.Error("the guard kept its references across a resize")
	}
}

func TestAKickMovesThePaletteGentlyRatherThanAtOnce(t *testing.T) {
	s := New(3)
	var tr track
	play(s, &tr, 0, 4, 40, 10)
	most, last := 0.0, s.shift
	for i := range 60 {
		play(s, &tr, 4+float64(i)/30, 1.0/30, 40, 10)
		most, last = math.Max(most, math.Abs(s.shift-last)), s.shift
	}
	if most > 0.03 {
		t.Errorf("the palette's shift moved %.3f in one frame", most)
	}
}
