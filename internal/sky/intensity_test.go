package sky

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// cellsOf is the light of each cell of a picture: ink times luminance.
func cellsOf(p Picture, w, h int) []float64 {
	out := make([]float64, w*h)
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
				if r < h && c < w {
					out[r*w+c] = l
				}
				c++
			}
		}
	}
	return out
}

// played is what a visualiser did over a whole track: each frame's heard
// position, kick and heaviness, and the position of each cut.
type played struct {
	pos, kick, heavy []float64
	cuts             []float64
}

// onBeats reports how the kick answered the beats from a to b: the share of
// them with a kick of at least 0.6 beginning within 60 ms before or 150 ms
// after, and the mean and spread, in milliseconds, of how long after the
// beat it began.
func (d played) onBeats(beats []float64, a, b float64) (share, lag, spread float64) {
	var lags []float64
	n := 0
	for _, at := range beats {
		if at < a || at >= b {
			continue
		}
		n++
		best, rise, when := 0.0, 0.0, 0.0
		for k := sort.SearchFloat64s(d.pos, at-0.06); k > 0 && k < len(d.pos) && d.pos[k] <= at+0.15; k++ {
			best = math.Max(best, d.kick[k])
			if up := d.kick[k] - d.kick[k-1]; up > rise {
				rise, when = up, d.pos[k]-at
			}
		}
		if rise >= 0.1 && best >= 0.6 {
			lags = append(lags, 1000*when)
		}
	}
	if n == 0 || len(lags) == 0 {
		return 0, 0, 0
	}
	for _, l := range lags {
		lag += l / float64(len(lags))
	}
	for _, l := range lags {
		spread += (l - lag) * (l - lag) / float64(len(lags))
	}
	return float64(len(lags)) / float64(n), lag, math.Sqrt(spread)
}

// gridLight is the mean light of each region of a picture w by h split into
// cols by rows regions.
func gridLight(cells []float64, w, h, cols, rows int) []float64 {
	sum, n := make([]float64, cols*rows), make([]float64, cols*rows)
	for r := range h {
		for c := range w {
			k := min(r*rows/h, rows-1)*cols + min(c*cols/w, cols-1)
			sum[k] += cells[r*w+c]
			n[k]++
		}
	}
	for k := range sum {
		if n[k] > 0 {
			sum[k] /= n[k]
		}
	}
	return sum
}

// The finer grid a picture is watched on beside the guard's own.
const fineCols, fineRows = 6, 4

// seen is what one frame looked like beside the frame before it.
type seen struct {
	// at is the position heard, in seconds.
	at float64
	// change is the mean change in the light of a cell, and churn is one less
	// the correlation of the two frames' cells, which a change of brightness
	// alone leaves at 0.
	change, churn float64
	// drift, punch, turn and shake are how far the picture was moved, as a
	// share of the field's radius: by the feedback's steady zoom and turn, by
	// the kick's push on the zoom, by the warps, and by the shake. throw is
	// their sum.
	drift, punch, turn, shake, throw float64
	// speed is how fast the layers' clocks ran against the wall clock.
	speed float64
	// step is the largest change in the light of one of the guard's regions.
	step float64
	// cut says the frame was drawn during a cut, when the guard is off.
	cut bool
	// kick and snap are the hits the frame was drawn with, beat says it had
	// a beat to follow, and flicks is how many of the finer regions turned
	// from brightening to dimming or back on it.
	kick, snap float64
	beat       bool
	flicks     int
	// combo names the components on screen.
	combo string
	// sure is the confidence in the tempo the frame was drawn with, and
	// steady the steadiness.
	sure, steady float64
}

// A turn of the light in one of the guard's regions is counted under each
// of the things going on when it happened.
const (
	// duringCut and duringFade are a cut and a crossfade between two scenes.
	duringCut = iota
	duringFade
	// duringPalette and duringGlyphs are the scene's palette and its way of
	// drawing cells moving to another.
	duringPalette
	duringGlyphs
	// duringUnseen is a turn in which the region's light as the guard measures
	// it moved by less than half of what makes a flash.
	duringUnseen
	// duringPlain is a turn with none of the others going on.
	duringPlain
	duringCauses
)

var turnNames = [duringCauses]string{"cut", "crossfade", "palette moving", "glyphs moving", "unseen by the guard", "none of these"}

// watcher measures how hard a run of frames is on the eye.
type watcher struct {
	w, h         int
	coarse, fine []flashMeter
	cells, light []float64
	frames       []seen
	// cutTurns counts the turns of light in a guard region on frames drawn
	// during a cut.
	cutTurns int
	// causes counts the turns of light in the guard's regions by what was
	// going on, and guarded holds the light of each region as the guard
	// measures it over the last half second.
	causes  [duringCauses]int
	guarded [][guardRegions]float64
	// drawn holds the light of each region as it is rendered over the same
	// half second, and swungDrawn and swungGuarded add up how far each moved
	// in the half second before every turn.
	drawn                    [][]float64
	swungDrawn, swungGuarded float64
	warps                    map[*warp][2]float64
	layers                   map[*layer]float64
	shake                    [2]float64
}

func newWatcher(w, h int) *watcher {
	return &watcher{w: w, h: h, coarse: make([]flashMeter, meterCols*meterRows),
		fine: make([]flashMeter, fineCols*fineRows), warps: map[*warp][2]float64{}, layers: map[*layer]float64{}}
}

// look measures the frame s has just drawn, dt seconds after the last.
func (m *watcher) look(s *State, dt float64) {
	f := seen{at: s.lis.ear.now, cut: s.cutting && s.next != nil, kick: s.sig.Kick, snap: s.sig.Snap,
		beat: s.sig.onTheBeat(), combo: s.cur.combo(), sure: s.sig.Sure, steady: s.sig.Steady}
	lit, _ := s.guard.measure(s.value, s.tone)
	way := lightDots
	if s.cur.halves() || s.next.halves() {
		way = lightHalf
	}
	m.guarded = append(m.guarded, lit[way])
	if len(m.guarded) > 15 {
		m.guarded = m.guarded[1:]
	}
	cells := cellsOf(s.Render(true), m.w, m.h)
	if m.cells != nil {
		var ma, mb float64
		for i, c := range cells {
			f.change += math.Abs(c-m.cells[i]) / float64(len(cells))
			ma, mb = ma+c/float64(len(cells)), mb+m.cells[i]/float64(len(cells))
		}
		var num, da, db float64
		for i, c := range cells {
			num += (c - ma) * (m.cells[i] - mb)
			da += (c - ma) * (c - ma)
			db += (m.cells[i] - mb) * (m.cells[i] - mb)
		}
		if da > 1e-12 && db > 1e-12 {
			f.churn = 1 - num/math.Sqrt(da*db)
		}
	}
	m.cells = cells

	light := gridLight(cells, m.w, m.h, meterCols, meterRows)
	m.drawn = append(m.drawn, light)
	if len(m.drawn) > 15 {
		m.drawn = m.drawn[1:]
	}
	for k, l := range light {
		before := len(m.coarse[k].turns)
		m.coarse[k].add(l, f.at)
		if f.cut {
			m.cutTurns += len(m.coarse[k].turns) - before
		}
		if len(m.coarse[k].turns) > before {
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, g := range m.guarded {
				lo, hi = math.Min(lo, g[k]), math.Max(hi, g[k])
			}
			dlo, dhi := math.Inf(1), math.Inf(-1)
			for _, d := range m.drawn {
				dlo, dhi = math.Min(dlo, d[k]), math.Max(dhi, d[k])
			}
			m.swungDrawn, m.swungGuarded = m.swungDrawn+dhi-dlo, m.swungGuarded+hi-lo
			plain := true
			count := func(cause int, on bool) {
				if on {
					m.causes[cause]++
					plain = false
				}
			}
			count(duringCut, f.cut)
			count(duringFade, s.next != nil && !f.cut)
			count(duringPalette, s.cur.toPalette != nil)
			count(duringGlyphs, s.cur.glyphs != s.cur.toGlyphs)
			count(duringUnseen, hi-lo < 0.05)
			count(duringPlain, plain)
		}
		if m.light != nil {
			f.step = math.Max(f.step, math.Abs(l-m.light[k]))
		}
	}
	m.light = light
	for k, l := range gridLight(cells, m.w, m.h, fineCols, fineRows) {
		before := len(m.fine[k].turns)
		m.fine[k].add(l, f.at)
		f.flicks += len(m.fine[k].turns) - before
	}

	// A point at two thirds of the radius is the mean point of a disc.
	const reach = 2.0 / 3
	if sc := s.cur; sc != nil && sc.label == "" {
		kick := s.sig.Kick * sc.hits
		breath := 1 + 0.01*(s.sig.Breath-0.5)
		f.drift = reach * (math.Abs((0.96+0.06*sc.fb[1])*breath-1) + math.Abs((sc.fb[2]-0.5)*0.08))
		f.punch = reach * 0.03 * kick * breath
		for _, w := range sc.warps {
			was, ok := m.warps[w]
			m.warps[w] = [2]float64{w.clock, w.v[0]}
			if !ok {
				continue
			}
			dc, dv := math.Abs(w.clock-was[0]), math.Abs(w.v[0]-was[1])
			switch w.kind {
			case warpRotate:
				f.turn += reach * dc * w.amount
			case warpKaleido:
				f.turn += reach * 0.3 * dc * w.amount
			case warpRipple:
				f.turn += reach * 0.25 * w.v[0] * 3 * dc * w.amount
			case warpZoom:
				f.turn += reach * 1.2 * dv / (0.6 + 1.2*w.v[0]) * w.amount
			case warpTwist:
				f.turn += reach * 4 * reach * dv * w.amount
			}
		}
		var rate, weight float64
		for _, l := range sc.layers {
			was, ok := m.layers[l]
			m.layers[l] = l.clock
			if ok && dt > 0 {
				rate += l.presence * (l.clock - was) / dt
				weight += l.presence
			}
		}
		if weight > 0 {
			f.speed = rate / weight
		}
	}
	if s.f.radius > 0 {
		f.shake = math.Hypot(s.f.sx-m.shake[0], s.f.sy-m.shake[1]) / s.f.radius
		m.shake = [2]float64{s.f.sx, s.f.sy}
	}
	f.throw = f.drift + f.punch + f.turn + f.shake
	m.frames = append(m.frames, f)
}

// figures is the mean, the 95th percentile and the largest of some numbers,
// and the largest mean over any ten seconds of them.
type figures struct{ mean, p95, max, ten float64 }

func (s figures) String() string {
	return fmt.Sprintf("%.4f %.4f %.4f %.4f", s.mean, s.p95, s.max, s.ten)
}

// spreadOf summarises pick over the frames keep allows. A nil keep allows
// every frame.
func (m *watcher) spreadOf(pick func(seen) float64, keep func(seen) bool) figures {
	var v, at []float64
	for _, f := range m.frames {
		if keep == nil || keep(f) {
			v, at = append(v, pick(f)), append(at, f.at)
		}
	}
	if len(v) == 0 {
		return figures{}
	}
	out := figures{mean: mean(v)}
	sum, i := 0.0, 0
	for j := range v {
		sum += v[j]
		for at[j]-at[i] > 10 {
			sum -= v[i]
			i++
		}
		if at[j]-at[i] >= 9 {
			out.ten = math.Max(out.ten, sum/float64(j-i+1))
		}
	}
	sorted := slices.Clone(v)
	sort.Float64s(sorted)
	out.p95, out.max = sorted[len(sorted)*95/100], sorted[len(sorted)-1]
	return out
}

// flashes reports the flashing of the region that flashes most: the most
// flashes in one second, the most a second over any ten seconds, and the
// flashes a second over the whole run.
func flashes(meters []flashMeter, secs float64) (worst, ten, all float64) {
	for i := range meters {
		turns := meters[i].turns
		worst = math.Max(worst, meters[i].worst())
		all = math.Max(all, float64(len(turns))/2/math.Max(secs, 1))
		for a, at := range turns {
			b := a
			for b < len(turns) && turns[b] < at+10 {
				b++
			}
			ten = math.Max(ten, float64(b-a)/2/10)
		}
	}
	return worst, ten, all
}

// report is one line of figures for the frames keep allows, under a name.
func (m *watcher) report(name string, keep func(seen) bool) string {
	n, cuts, flicks, beats := 0, 0, 0, 0
	stepIn, stepOut := 0.0, 0.0
	on := map[string]int{}
	for _, f := range m.frames {
		if keep != nil && !keep(f) {
			continue
		}
		n++
		flicks += f.flicks
		if f.beat {
			beats++
		}
		for _, name := range strings.Split(f.combo, "+") {
			on[name]++
		}
		if f.cut {
			cuts++
			stepIn = math.Max(stepIn, f.step)
		} else {
			stepOut = math.Max(stepOut, f.step)
		}
	}
	get := func(pick func(seen) float64) figures { return m.spreadOf(pick, keep) }
	var comps []string
	for name, k := range on {
		if share := 100 * k / max(n, 1); share >= 5 {
			comps = append(comps, fmt.Sprintf("%s %d%%", name, share))
		}
	}
	sort.Strings(comps)
	// Thirty frames a second, and two turns to a flash.
	flick := float64(flicks) / 2 / float64(fineCols*fineRows) / (float64(max(n, 1)) / 30)
	return fmt.Sprintf("%-22s frames %6d cut %4d | flick %.3f | beat %3d%% sure %.2f steady %.2f kick %s snap %s | on screen %s | change %s | churn %s | throw %s | punch %s | turn %s | shake %s | speed %s | step %.3f in cuts %.3f",
		name, n, cuts, flick, 100*beats/max(n, 1), get(func(f seen) float64 { return f.sure }).mean, get(func(f seen) float64 { return f.steady }).mean,
		get(func(f seen) float64 { return f.kick }), get(func(f seen) float64 { return f.snap }), strings.Join(comps, ", "),
		get(func(f seen) float64 { return f.change }), get(func(f seen) float64 { return f.churn }),
		get(func(f seen) float64 { return f.throw }), get(func(f seen) float64 { return f.punch }),
		get(func(f seen) float64 { return f.turn }), get(func(f seen) float64 { return f.shake }),
		get(func(f seen) float64 { return f.speed }), stepOut, stepIn)
}

// flashLine is the flashing on the guard's grid and on the finer one.
func (m *watcher) flashLine() string {
	secs := 0.0
	if len(m.frames) > 0 {
		secs = m.frames[len(m.frames)-1].at - m.frames[0].at
	}
	cw, ct, ca := flashes(m.coarse, secs)
	fw, ft, fa := flashes(m.fine, secs)
	var causes []string
	turns := 0
	for i, n := range m.causes {
		causes = append(causes, fmt.Sprintf("%s %d", turnNames[i], n))
	}
	for k := range m.coarse {
		turns += len(m.coarse[k].turns)
	}
	causes = append(causes, fmt.Sprintf("light moved %.3f as drawn and %.3f as the guard measures it in the half second before a turn",
		m.swungDrawn/math.Max(float64(turns), 1), m.swungGuarded/math.Max(float64(turns), 1)))
	return fmt.Sprintf("flashes a second, worst second / worst ten / whole: %dx%d %.1f %.2f %.3f | %dx%d %.1f %.2f %.3f | turns during cuts %d | turns by what was going on: %s",
		meterCols, meterRows, cw, ct, ca, fineCols, fineRows, fw, ft, fa, m.cutTurns, strings.Join(causes, ", "))
}

// lift reports how much stronger the picture is just after at than once it
// has settled: the mean of pick over the first two bars after at, of four
// beats of beat seconds, against its mean over bars nine to sixteen.
func (m *watcher) lift(pick func(seen) float64, at, beat float64) float64 {
	first := m.spreadOf(pick, func(f seen) bool { return f.at >= at && f.at < at+8*beat }).mean
	later := m.spreadOf(pick, func(f seen) bool { return f.at >= at+32*beat && f.at < at+64*beat }).mean
	if later == 0 {
		return 0
	}
	return first / later
}

// lifts is the lift of the change, the churn and the throw after at.
func (m *watcher) lifts(at, beat float64) string {
	return fmt.Sprintf("after %.1f s: change %.2f churn %.2f throw %.2f", at,
		m.lift(func(f seen) float64 { return f.change }, at, beat),
		m.lift(func(f seen) float64 { return f.churn }, at, beat),
		m.lift(func(f seen) float64 { return f.throw }, at, beat))
}

// A tweak changes a visualiser before each frame, to measure a run without
// one of the things that move it.
type tweak func(*State)

// each applies do to the scene on screen and the one fading in.
func each(s *State, do func(*scene)) {
	for _, sc := range []*scene{s.cur, s.next} {
		if sc != nil {
			do(sc)
		}
	}
}

// unroute zeroes how far the given sources move any parameter of a scene.
func unroute(sc *scene, srcs ...int) {
	cut := func(p *param) {
		for i, src := range p.src {
			if slices.Contains(srcs, src) {
				p.depth[i] = 0
			}
		}
	}
	for _, l := range sc.layers {
		for i := range l.p {
			cut(&l.p[i])
		}
		cut(&l.weight)
	}
	for _, w := range sc.warps {
		cut(&w.p[0])
		cut(&w.p[1])
	}
	for _, p := range []*param{&sc.keep, &sc.zoom, &sc.turn, &sc.bend} {
		cut(p)
	}
}

// without drops the layers of a scene whose component has one of the names.
func without(sc *scene, names ...string) {
	for _, l := range sc.layers {
		if slices.Contains(names, l.name) {
			l.goal, l.presence = 0, 0
		}
	}
}

// tweaks are the runs a track can be measured in, by name.
var tweaks = map[string]tweak{
	"as it is": nil,
	// Kicks and snaps move no parameter and do not push the zoom.
	"no hits": func(s *State) { each(s, func(sc *scene) { sc.hits = 1e-9 }) },
	// Kicks and snaps move no parameter, and still push the zoom.
	"hits unrouted": func(s *State) { each(s, func(sc *scene) { unroute(sc, srcKick, srcSnap) }) },
	// The beat's phase and the bar move no parameter.
	"phase unrouted": func(s *State) { each(s, func(sc *scene) { unroute(sc, srcPhase, srcBar) }) },
	// Turning does not step on the beat.
	"no lock": func(s *State) { each(s, func(sc *scene) { sc.lock = 0 }) },
	"no grid": func(s *State) { each(s, func(sc *scene) { without(sc, "grid") }) },
	"no sparks or rings": func(s *State) {
		each(s, func(sc *scene) { without(sc, "sparks", "rings") })
	},
	"none of them": func(s *State) {
		each(s, func(sc *scene) {
			sc.hits, sc.lock = 1e-9, 0
			unroute(sc, srcKick, srcSnap, srcPhase, srcBar)
			without(sc, "grid", "sparks", "rings")
		})
	},
}

// watchRun plays readings through s, w by h, giving it the plan p once the
// position reaches from, and measures every frame. It returns what the run
// did and what it looked like.
func watchRun(s *State, readings []Reading, p *Plan, from float64, w, h int, tw tweak) (played, *watcher) {
	var out played
	m := newWatcher(w, h)
	last := 0.0
	for i, r := range readings {
		pos := r.Position.Seconds()
		if p != nil && pos >= from && s.plan() == nil {
			s.Foresee("track", p)
		}
		if tw != nil {
			tw(s)
		}
		was := s.next
		s.Hear(r)
		dt := pos - last
		if i == 0 {
			dt = 1.0 / 30
		}
		s.Step(w, h, dt, r.At, "track")
		last = pos
		sig := s.Signals()
		out.pos, out.kick, out.heavy = append(out.pos, s.lis.ear.now), append(out.kick, sig.Kick), append(out.heavy, sig.Heavy)
		if s.next != nil && s.next != was && s.cutting {
			out.cuts = append(out.cuts, s.lis.ear.now)
		}
		m.look(s, dt)
	}
	return out, m
}

// floor is a four-on-the-floor dance track at bpm, secs seconds long: a kick
// on every beat, a clap on the second and fourth and a hat between, with the
// kicks and claps left out from breakFrom to breakTo.
func floor(bpm, secs, breakFrom, breakTo float64) (groove, []Block) {
	var g groove
	beat := 60 / bpm
	for i := 0; 0.3+float64(i)*beat < secs-1; i++ {
		at := 0.3 + float64(i)*beat
		g.beats = append(g.beats, at)
		if at < breakFrom || at >= breakTo {
			g.play(at, kickVoice, 1, &g.kicks)
			if i%2 == 1 {
				g.play(at, clapVoice, 1, &g.snares)
			}
		}
		g.play(at+beat/2, hatVoice, 1, &g.hats)
	}
	pad := func(float64) [bandCount]float64 { return [bandCount]float64{1e-3, 1e-3, 1e-4, 1e-5} }
	return g, banded(secs, g.strikes, pad)
}

// heardFloor is the readings of the same track as it would be heard: a burst
// in the bass on each kick and in the treble on each hat, measured 0.27 s
// ahead of what is heard.
func heardFloor(g groove, secs float64) []Reading {
	var tr track
	var out []Reading
	dt := 1.0 / 30
	for at := 0.0; at < secs; at += dt {
		bass, hats := 0.001, 0.0
		if nearest(g.kicks, at-0.04) <= 0.04 {
			bass = 0.3
		}
		if nearest(g.hats, at-0.02) <= 0.02 {
			hats = 0.02
		}
		diff := bass*band(bassHz, trackRate) + hats*band(trebleHz, trackRate)
		r := tr.block(dt, both(bass+hats), both(diff), 0.05, at-0.27)
		r.At = time.Time{}
		out = append(out, r)
	}
	return out
}

// floorCase is a planned dance track at one tempo, whose kicks stop from 24 s
// to floorBack. The cue where they come back is marked as a hard boundary, so it
// is cut to at once.
func floorCase(t *testing.T, bpm float64) (*Plan, groove, []Reading, []float64) {
	t.Helper()
	g, blocks := floor(bpm, floorSecs, 24, floorBack)
	p := NewPlan(blocks)
	p.curve = nil
	var due []float64
	for _, c := range p.Cues {
		if (c.Kind == CueSection || c.Kind == CueDrop) && p.cutDue(c) {
			due = append(due, c.At)
		}
	}
	if len(due) > 0 {
		p.Bounds = append(p.Bounds, Boundary{At: due[len(due)-1], Strength: 1, Major: true, Hard: true})
	}
	return p, g, heardFloor(g, floorSecs), due
}

// floorSecs is the length of a floorCase track, and floorBack where its
// kicks come back: sixteen bars before the end at the slowest tempo.
const floorSecs, floorBack = 76, 40

// A planned four-on-the-floor track at each dance tempo is played through a
// stop and a return of its kicks, and what its frames look like is logged.
// The figures are a record and carry no limit. The test holds only that the
// run is what it is described as: the plan's kicks were set off on its beats,
// and the return of the kicks was cut to.
func TestAPlannedDanceTrackIsMeasuredAtEachTempo(t *testing.T) {
	if testing.Short() {
		t.Skip("measures three planned tracks")
	}
	for _, bpm := range []float64{128, 140, 174} {
		t.Run(fmt.Sprintf("%.0f BPM", bpm), func(t *testing.T) {
			t.Parallel()
			p, g, readings, due := floorCase(t, bpm)
			if len(due) == 0 {
				t.Fatalf("the plan has no cue to cut to: %v", p.Cues)
			}
			d, m := watchRun(New(3), readings, p, -1, 60, 20, nil)
			share, lag, _ := d.onBeats(between(g.kicks, 4, 22), 4, 22)
			t.Logf("%.0f BPM: plan tempo %.1f, cues to cut to %.1f, cuts at %.1f, kicks answered %.0f%% %+.0f ms",
				bpm, p.Tempo, due, d.cuts, 100*share, lag)
			t.Log(m.flashLine())
			t.Log(m.report("whole", nil))
			t.Log(m.report("kicked", func(f seen) bool { return f.at < 24 || f.at >= floorBack }))
			t.Log(m.report("break", func(f seen) bool { return f.at >= 24 && f.at < floorBack }))
			t.Log("lift " + m.lifts(floorBack, 60/bpm))
			if share < 0.9 {
				t.Errorf("%.0f%% of the kicks from 4 s to 22 s were answered, want at least 90%%", 100*share)
			}
			if len(d.cuts) == 0 {
				t.Errorf("no cut was made; the kicks come back at 40 s and the cues are %.1f", due)
			}
		})
	}
}

// tweakNames returns the names of the tweaks, with the untouched run first.
func tweakNames() []string {
	var out []string
	for name := range tweaks {
		if name != "as it is" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return append([]string{"as it is"}, out...)
}

// tweaksNamed returns the tweaks named in a comma-separated list, or only
// the untouched run for an empty one, and all of them for "all".
func tweaksNamed(t *testing.T, list string) []string {
	t.Helper()
	switch list {
	case "":
		return []string{"as it is"}
	case "all":
		return tweakNames()
	}
	names := strings.Split(list, ",")
	for _, n := range names {
		if _, ok := tweaks[n]; !ok {
			t.Fatalf("%q is not a run; the runs are %q", n, tweakNames())
		}
	}
	return names
}
