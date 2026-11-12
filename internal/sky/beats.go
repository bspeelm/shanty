package sky

import (
	"cmp"
	"math"
	"slices"
)

// The bands a block is measured in, in the order of Block.Bands: below
// 150 Hz, 100 to 400 Hz, 1 to 4 kHz and above 6 kHz.
const (
	bandKick = iota
	bandLow
	bandMid
	bandHigh
	bandCount
)

// HitKind is the drum a hit sounds like.
type HitKind int

const (
	// HitKick is a hit in the lowest band.
	HitKick HitKind = iota
	// HitSnare is a hit across the middle and high bands.
	HitSnare
	// HitHat is a hit that adds more power to the high band than to the
	// middle band.
	HitHat
)

func (k HitKind) String() string {
	names := []string{"kick", "snare", "hat"}
	if k < 0 || int(k) >= len(names) {
		return "unknown"
	}
	return names[k]
}

// Hit is one drum hit: when it sounds, in seconds, what it is, and how
// strong it is, from 0 to 1.
type Hit struct {
	At       float64
	Kind     HitKind
	Strength float64
}

// The beat finder's measures. Times are in seconds.
const (
	// beatlessBelow is the Pulse under which a track has no beats.
	beatlessBelow = 0.3
	// Tempos are looked for between slowestBPM and fastestBPM, and
	// preferredBPM is the tempo favoured when two octaves fit about equally.
	slowestBPM   = 55.0
	fastestBPM   = 215.0
	preferredBPM = 115.0
	// The tempo is measured over windows of tempoSpan, one every tempoHop,
	// at tempos tempoRatio apart. A change of up to driftStates of those
	// steps between windows costs driftCost a step, and a larger change costs
	// jumpCost. Each octave squared from preferredBPM costs priorWeight.
	tempoSpan   = 8.0
	tempoHop    = 4.0
	tempoRatio  = 1.01
	driftStates = 2
	driftCost   = 0.05
	jumpCost    = 6
	priorWeight = 0.1
	// octaveNear is how close, in octaves, a window's tempo must be to an
	// octave of the track's median tempo to be moved onto it.
	octaveNear = 0.06
	// pulseFull is the salience above a window's mean that counts as a
	// full Pulse.
	pulseFull = 0.4
	// floorBelow is how far under a band's loud level its quiet is cut off,
	// and silentBelow is the level, in dB, of a band that is silent.
	floorBelow  = 50.0
	silentBelow = -150.0
	// spreadSpan is the time over which an onset's strength is compared
	// with the onsets around it.
	spreadSpan = 4.0
	// tightness is how strongly the beat grid keeps to the local period.
	tightness = 400.0
	// A hit rises by at least hitRise dB, to within hitBelow dB of its
	// band's loud level, and from hitFrom to hitFull deviations above the
	// mean rise within hitSpan, with a deviation of at least hitDeviation
	// dB. Without support for the tempo a hit needs strength lone and must
	// be lonelyGap seconds from any other.
	hitRise      = 3.0
	hitBelow     = 25.0
	hitFrom      = 2.0
	hitFull      = 8.0
	hitSpan      = 2.0
	hitDeviation = 1.0
	lone         = 0.6
	lonelyGap    = 2.5
	// slipCost is the weight of downbeats a bar must lose before the
	// downbeat moves to another beat.
	slipCost = 8.0
	// A beat whose support is under supportedAbove has no evidence of its
	// own; a beat laid between two supported runs has confidence bridged; and
	// a stretch with any band within soundBelow dB of its loud level has
	// sound.
	supportedAbove = 0.2
	bridged        = 0.3
	soundBelow     = 30.0
	// coveredFull is the share of the track that, covered by beats of full
	// confidence, allows a full Pulse.
	coveredFull = 0.6
	// A run of at least rephaseRun beats is moved half a beat when its onsets
	// fall the other way round from the rest of the track, judged by the
	// bands whose onsets are rephaseClear times stronger on the beat than
	// halfway, or the other way, across the track.
	rephaseRun   = 8
	rephaseClear = 1.4
	// A hit counts towards how busy a track is from strength busyAbove, and
	// planBusyFull such hits a second over the whole track is full.
	busyAbove    = 0.3
	planBusyFull = 8.0
	// smoothWithin is how far a beat may be from the line through the beats
	// around it and be moved onto it.
	smoothWithin = 0.025
	// tempoBeats is how many beats either side a beat's tempo is measured
	// over.
	tempoBeats = 4
	// snapWithin is how close a hit must be to a quarter of a beat to be
	// moved onto it.
	snapWithin = 0.03
)

// bandWeights is how much each band counts towards the onsets the beat is
// found from.
var bandWeights = [bandCount]float64{1, 0.7, 0.7, 0.4}

// onsets holds the onset strength of each band at each block.
type onsets struct {
	at    []float64
	hop   float64
	band  [bandCount][]float64
	level [bandCount][]float64
	loud  [bandCount]float64
	rise  [bandCount][]float64
	sum   []float64
}

// bandOnsets returns the onsets of the blocks measured in bands, or nil when
// there are fewer than two tempo windows of them.
func bandOnsets(blocks []Block) *onsets {
	o := &onsets{}
	for _, b := range blocks {
		if len(b.Bands) != bandCount || b.Rate <= 0 || b.Samples <= 0 {
			continue
		}
		o.at = append(o.at, b.At)
		o.hop = math.Max(o.hop, b.Samples/float64(b.Rate))
		for k := range bandCount {
			v := b.Bands[k]
			if math.IsNaN(v) {
				v = math.Inf(-1)
			}
			o.level[k] = append(o.level[k], v)
		}
	}
	n := len(o.at)
	if n == 0 || float64(n)*o.hop < 2*tempoSpan {
		return nil
	}
	o.sum = make([]float64, n)
	for k := range bandCount {
		l := o.level[k]
		o.loud[k] = math.Max(quantile(l, 0.95), silentBelow+floorBelow)
		floor := o.loud[k] - floorBelow
		for i, v := range l {
			if v < floor {
				l[i] = floor
			}
		}
		o.rise[k] = make([]float64, n)
		before := 0.0
		for i, v := range l {
			now := math.Exp(v * math.Ln10 / 10)
			if i > 0 {
				o.rise[k][i] = math.Max(0, now-before)
			}
			before = now
		}
		flux := make([]float64, n)
		for i := 2; i < n; i++ {
			flux[i] = math.Max(0, l[i]-math.Max(l[i-1], l[i-2]))
		}
		local := movingMean(flux, o.blocks(0.25))
		for i := range flux {
			flux[i] = math.Max(0, flux[i]-local[i])
		}
		sq := make([]float64, n)
		for i, v := range flux {
			sq[i] = v * v
		}
		wide, global := movingMean(sq, o.blocks(spreadSpan)), mean(sq)
		for i := range flux {
			flux[i] /= math.Sqrt(math.Max(wide[i], global)) + 1e-9
			o.sum[i] += bandWeights[k] * flux[i]
		}
		o.band[k] = flux
	}
	return o
}

// onsetAt returns when an onset found in block i of band k sounds. The rise
// in power of an onset part of the way through a block is split between that
// block and the next in proportion to how far through it falls, so the time
// is placed within the pair of blocks around i with the larger rise.
func (o *onsets) onsetAt(i, k int) float64 {
	r := o.rise[k]
	a := i
	if i > 0 && (i+1 >= len(r) || r[i-1]+r[i] > r[i]+r[i+1]) {
		a = i - 1
	}
	if a+1 >= len(r) || r[a]+r[a+1] <= 0 {
		return o.at[i]
	}
	return o.at[a] + o.hop*r[a+1]/(r[a]+r[a+1])
}

// strongest returns the band with the largest weighted onset within a block
// of block i.
func (o *onsets) strongest(i int) int {
	best, kind := -1.0, bandKick
	for k := range bandCount {
		for j := max(i-1, 0); j <= min(i+1, len(o.at)-1); j++ {
			if v := bandWeights[k] * o.band[k][j]; v > best {
				best, kind = v, k
			}
		}
	}
	return kind
}

// blocks is the number of blocks in d seconds.
func (o *onsets) blocks(d float64) int { return int(math.Round(d / o.hop)) }

// beats finds the beats, downbeats, hits and character of a track measured in
// bands. A track measured without bands keeps the tempo found from its
// onsets and has no beats.
func (p *Plan) beats(blocks []Block) {
	o := bandOnsets(blocks)
	if o == nil {
		return
	}
	p.banded = true
	period, support, pulse := o.tempo()
	p.Pulse = pulse
	var idx []int
	if pulse >= beatlessBelow {
		idx = o.rephase(o.grid(period))
		p.Beats = make([]float64, len(idx))
		for i, j := range idx {
			p.Beats[i] = o.onsetAt(j, o.strongest(j))
		}
		p.downbeats(o, idx)
		p.confide(o, idx, support)
	}
	p.Tempo, p.Sure = 0, p.Pulse
	if len(p.tempi) > 0 {
		p.Tempo = quantile(p.tempi, 0.5)
	}
	p.hits(o, support)
	p.char = p.character()
}

// joined reports whether beat i is followed by the next beat of the same run,
// with no beats dropped between them.
func (p *Plan) joined(i int) bool { return i+1 < len(p.Beats) && !p.broken[i] }

// rephase moves each run of at least rephaseRun beats on which the bands'
// onsets fall the other way round from the rest of the track onto the
// points halfway between its beats. For each band, the share of onsets on
// the beat rather than halfway is measured over the beats either side, and
// a band whose share across the track is clearly one way or the other
// votes on each beat by how far its share there agrees with the track's.
func (o *onsets) rephase(idx []int) []int {
	n := len(idx)
	if n < 2*rephaseRun {
		return idx
	}
	peak := func(k, i int) float64 {
		v := 0.0
		for j := max(i-1, 0); j <= min(i+1, len(o.at)-1); j++ {
			v = math.Max(v, o.band[k][j])
		}
		return v
	}
	var lean [bandCount][]float64
	for k := range bandCount {
		on, off := make([]float64, n), make([]float64, n)
		for b := 0; b+1 < n; b++ {
			on[b], off[b] = peak(k, idx[b]), peak(k, (idx[b]+idx[b+1])/2)
		}
		on, off = movingMean(on, rephaseRun/2), movingMean(off, rephaseRun/2)
		lean[k] = make([]float64, n)
		for b := range n {
			lean[k][b] = math.Log((on[b] + 0.1) / (off[b] + 0.1))
		}
	}
	vote := make([]float64, n)
	for k := range bandCount {
		typical := quantile(lean[k], 0.5)
		if math.Abs(typical) < math.Log(rephaseClear) {
			continue
		}
		for b := range n {
			vote[b] += lean[k][b] / typical
		}
	}
	out := slices.Clone(idx)
	for b := 0; b < n; {
		e := b
		for e < n-1 && vote[e] < 0 {
			e++
		}
		if e-b >= rephaseRun {
			for c := b; c < e; c++ {
				out[c] = (idx[c] + idx[c+1]) / 2
			}
		}
		b = e + 1
	}
	return out
}

// confide gives each beat a confidence, the support for the tempo where it
// falls, or none when there is no sound within a block of it, and drops the
// beats that have none. A run of beats under
// supportedAbove between two supported runs is laid evenly between them,
// with confidence bridged, when it has sound and the time between the runs
// is within a fifth of a whole number of beats; otherwise it is dropped, as
// are the runs before the first supported beat and after the last. Each
// beat's tempo is then measured over tempoBeats beats of its run either
// side, and a beat with that many either side that is within smoothWithin of
// the line through them is moved onto it.
func (p *Plan) confide(o *onsets, idx []int, support []float64) {
	n := len(idx)
	conf := make([]float64, n)
	for b, i := range idx {
		if o.sounding(max(i-1, 0), min(i+2, len(o.at))) {
			conf[b] = support[i]
		}
	}
	var beats, confs []float64
	var bars []int
	var broken []bool
	keep := func(t, c float64, bar int, gap bool) {
		if gap && len(broken) > 0 {
			broken[len(broken)-1] = true
		}
		beats, confs, bars, broken = append(beats, t), append(confs, c), append(bars, bar), append(broken, false)
	}
	last := -1
	for b := 0; b < n; {
		if conf[b] >= supportedAbove {
			keep(p.Beats[b], conf[b], p.bar[b], last >= 0 && last != b-1)
			last = b
			b++
			continue
		}
		e := b
		for e < n && conf[e] < supportedAbove {
			e++
		}
		if last >= 0 && e < n && o.sounding(idx[last], idx[e]) {
			a, z := p.Beats[last], p.Beats[e]
			beat := (p.Beats[last] - p.Beats[max(last-4, 0)]) / float64(max(last-max(last-4, 0), 1))
			count := math.Round((z - a) / beat)
			if count >= 1 && math.Abs((z-a)/beat-count) < 0.2 {
				for k := 1; k < int(count); k++ {
					keep(a+float64(k)*(z-a)/count, bridged, (p.bar[last]+k)%p.Meter, false)
				}
				b = e
				continue
			}
		}
		b = e
	}
	if len(broken) > 0 {
		broken[len(broken)-1] = true
	}
	p.Beats, p.conf, p.bar, p.broken = beats, confs, bars, broken
	p.tempi = make([]float64, len(p.Beats))
	for b := range p.Beats {
		lo, hi := b, b
		for lo > 0 && b-lo < tempoBeats && p.joined(lo-1) {
			lo--
		}
		for hi < len(p.Beats)-1 && hi-b < tempoBeats && p.joined(hi) {
			hi++
		}
		if hi > lo {
			p.tempi[b] = 60 * float64(hi-lo) / (p.Beats[hi] - p.Beats[lo])
		}
	}
	smooth := slices.Clone(p.Beats)
	for b := range p.Beats {
		lo, hi := b, b
		for lo > 0 && b-lo < tempoBeats && p.joined(lo-1) {
			lo--
		}
		for hi < len(p.Beats)-1 && hi-b < tempoBeats && p.joined(hi) {
			hi++
		}
		if b-lo < tempoBeats || hi-b < tempoBeats {
			continue
		}
		var mx, my, sxy, sxx float64
		for c := lo; c <= hi; c++ {
			mx, my = mx+float64(c), my+p.Beats[c]
		}
		k := float64(hi - lo + 1)
		mx, my = mx/k, my/k
		for c := lo; c <= hi; c++ {
			sxy += (float64(c) - mx) * (p.Beats[c] - my)
			sxx += (float64(c) - mx) * (float64(c) - mx)
		}
		if fit := my + sxy/sxx*(float64(b)-mx); math.Abs(fit-p.Beats[b]) < smoothWithin {
			smooth[b] = fit
		}
	}
	p.Beats = smooth
	p.Downbeats = nil
	for b, bar := range p.bar {
		if bar == 0 {
			p.Downbeats = append(p.Downbeats, p.Beats[b])
		}
	}
	covered := 0.0
	for b := range p.Beats {
		if p.joined(b) {
			covered += (p.Beats[b+1] - p.Beats[b]) * p.conf[b]
		}
	}
	p.Pulse = math.Min(p.Pulse, covered/math.Max(p.Length, 1)/coveredFull)
	if p.Pulse < beatlessBelow {
		p.Beats, p.Downbeats, p.conf, p.bar, p.broken, p.tempi = nil, nil, nil, nil, nil, nil
	}
}

// sounding reports whether any band is within soundBelow dB of its loud
// level somewhere between blocks i and j.
func (o *onsets) sounding(i, j int) bool {
	for k := range bandCount {
		for _, v := range o.level[k][i:j] {
			if v >= o.loud[k]-soundBelow {
				return true
			}
		}
	}
	return false
}

// fold moves each period that is within octaveNear octaves of a whole
// number of octaves from the median period onto the median's octave.
func fold(periods []float64) {
	m := quantile(periods, 0.5)
	for i, p := range periods {
		k := math.Round(math.Log2(p / m))
		if k != 0 && math.Abs(math.Log2(p/m)-k) < octaveNear {
			periods[i] = p / math.Pow(2, k)
		}
	}
}

// salience is how well a beat of lag blocks fits the autocorrelation a: its
// value at the lag, and half and a quarter of its highest value within a
// block of two and four beats.
func salience(a []float64, lag float64) float64 {
	peak := func(x float64) float64 { return max(at(a, x-1), at(a, x), at(a, x+1)) }
	return at(a, lag) + peak(2*lag)/2 + peak(4*lag)/4
}

// tempo returns the period of the beat, in blocks, at each block, and the
// Pulse of the track. Each window of tempoSpan has a salience for each tempo
// from slowestBPM to fastestBPM, a step of tempoRatio apart. The period
// follows the path through the windows with the most salience, less
// priorWeight for each octave squared from preferredBPM, driftCost for each
// step between windows, and at most jumpCost for a change of any size, and
// a window's period within octaveNear of an octave of the median period is
// moved onto the median's octave. The Pulse is the median of how far the path's salience stands above each
// window's mean salience, as a share of pulseFull, times the share of the
// windows that follow on from the one before without a jump.
func (o *onsets) tempo() ([]float64, []float64, float64) {
	span, hop := o.blocks(tempoSpan), o.blocks(tempoHop)
	slow := 60 / slowestBPM / o.hop
	states := int(math.Log(fastestBPM/slowestBPM)/math.Log(tempoRatio)) + 1
	lag, prior := make([]float64, states), make([]float64, states)
	for s := range lag {
		lag[s] = slow / math.Pow(tempoRatio, float64(s))
		prior[s] = -priorWeight * math.Pow(math.Log2(60/(lag[s]*o.hop)/preferredBPM), 2)
	}
	var sal [][]float64
	for start := 0; start+span <= len(o.sum); start += hop {
		x := o.sum[start : start+span]
		a := autocorrelation(x, int(4*slow)+2)
		f := fourier(x, lag)
		top := slices.Max(f) + 1e-9
		row := make([]float64, states)
		for s := range row {
			row[s] = math.Max(salience(a, lag[s]), 0) * f[s] / top
		}
		sal = append(sal, row)
	}
	from := make([][]int, len(sal))
	score := make([]float64, states)
	for s := range score {
		score[s] = sal[0][s] + prior[s]
	}
	for w := 1; w < len(sal); w++ {
		top := 0
		for s := range score {
			if score[s] > score[top] {
				top = s
			}
		}
		next := make([]float64, states)
		from[w] = make([]int, states)
		for s := range next {
			best, arg := score[top]-jumpCost, top
			for d := -driftStates; d <= driftStates; d++ {
				if r := s + d; r >= 0 && r < states {
					if v := score[r] - driftCost*math.Abs(float64(d)); v > best {
						best, arg = v, r
					}
				}
			}
			next[s], from[w][s] = best+sal[w][s]+prior[s], arg
		}
		score = next
	}
	path := make([]int, len(sal))
	for s := range score {
		if score[s] > score[path[len(path)-1]] {
			path[len(path)-1] = s
		}
	}
	for w := len(path) - 1; w > 0; w-- {
		path[w-1] = from[w][path[w]]
	}
	local := make([]float64, len(sal))
	for w, s := range path {
		local[w] = lag[s]
	}
	fold(local)
	strength := make([]float64, len(sal))
	steady := float64(len(sal))
	for w, s := range path {
		strength[w] = sal[w][s] - mean(sal[w])
		if w > 0 && math.Abs(float64(s-path[w-1])) > driftStates {
			steady--
		}
	}
	period := make([]float64, len(o.sum))
	support := make([]float64, len(o.sum))
	for i := range period {
		x := (float64(i) - float64(span)/2) / float64(hop)
		period[i] = at(local, x)
		support[i] = clamp01(at(strength, x) / pulseFull)
	}
	return period, support, clamp01(quantile(strength, 0.5)/pulseFull) * steady / float64(len(sal))
}

// grid returns the blocks of the beats: the sequence of blocks with the most
// onset strength, less a cost for each gap that differs from the local
// period. It runs from the first beat to the last that are among loud
// onsets.
func (o *onsets) grid(period []float64) []int {
	n := len(o.sum)
	sd := math.Sqrt(mean(squares(o.sum)))
	score := make([]float64, n)
	back := make([]int, n)
	logs := make([]float64, int(2*slices.Max(period))+2)
	for g := 1; g < len(logs); g++ {
		logs[g] = math.Log(float64(g))
	}
	for i := range n {
		local := o.sum[i] / (sd + 1e-9)
		p := period[i]
		logP := math.Log(p)
		back[i] = -1
		best := 0.0
		for j := i - int(math.Round(2*p)); j <= i-int(math.Round(p/2)); j++ {
			if j < 0 {
				continue
			}
			d := logs[i-j] - logP
			if s := score[j] - tightness*d*d; back[i] < 0 || s > best {
				best, back[i] = s, j
			}
		}
		score[i] = local + math.Max(best, 0)
		if best <= 0 {
			back[i] = -1
		}
	}
	var maxima []float64
	for i := 1; i < n-1; i++ {
		if score[i] > score[i-1] && score[i] >= score[i+1] {
			maxima = append(maxima, score[i])
		}
	}
	end := n - 1
	if len(maxima) > 0 {
		floor := quantile(maxima, 0.5) / 2
		for i := n - 2; i > 0; i-- {
			if score[i] > score[i-1] && score[i] >= score[i+1] && score[i] > floor {
				end = i
				break
			}
		}
	}
	var beats []int
	for i := end; i >= 0; i = back[i] {
		beats = append(beats, i)
	}
	slices.Reverse(beats)
	return o.trim(beats)
}

// trim drops the beats at the start and end of a grid whose onsets, summed
// over the beat around them, are under a quarter of the median of all beats.
func (o *onsets) trim(beats []int) []int {
	if len(beats) < 3 {
		return beats
	}
	strength := make([]float64, len(beats))
	for b, i := range beats {
		w := (beats[min(b+1, len(beats)-1)] - beats[max(b-1, 0)]) / 4
		for j := max(i-w, 0); j <= min(i+w, len(o.sum)-1); j++ {
			strength[b] += o.sum[j]
		}
	}
	floor := quantile(strength, 0.5) / 4
	lo, hi := 0, len(beats)
	for lo < hi && strength[lo] < floor {
		lo++
	}
	for hi > lo && strength[hi-1] < floor {
		hi--
	}
	return beats[lo:hi]
}

// downbeats marks each beat's place in its bar. A beat's weight as a
// downbeat is the change in each band's mean level from the two beats before
// it to the two beats after it, in dB, and half its low onsets. The meter is
// three when the best place for downbeats in bars of three stands out from
// the rest a quarter more than it does in bars of four, and four otherwise.
// The places follow the path through the beats with the most weight on
// downbeats, each beat moving one place on, less slipCost for each beat that
// moves to any other place.
func (p *Plan) downbeats(o *onsets, idx []int) {
	n := len(idx)
	if n == 0 {
		return
	}
	var sum [bandCount][]float64
	for k := range bandCount {
		sum[k] = make([]float64, len(o.at)+1)
		for i, v := range o.level[k] {
			sum[k][i+1] = sum[k][i] + v
		}
	}
	meanOf := func(k, from, to int) float64 {
		from, to = max(from, 0), min(to, len(o.at))
		if to <= from {
			return 0
		}
		return (sum[k][to] - sum[k][from]) / float64(to-from)
	}
	f := make([]float64, n)
	for b, i := range idx {
		if b >= 2 && b+2 < n {
			for k := range bandCount {
				f[b] += math.Abs(meanOf(k, i, idx[b+2]) - meanOf(k, idx[b-2], i))
			}
		}
		low := 0.0
		for j := max(i-1, 0); j <= min(i+1, len(o.at)-1); j++ {
			low = math.Max(low, o.band[bandKick][j]+o.band[bandLow][j])
		}
		f[b] += low / 2
	}
	m, sd := mean(f), math.Sqrt(mean(squares(f))-mean(f)*mean(f))+1e-9
	for b := range f {
		f[b] = (f[b] - m) / sd
	}
	fit := func(meter int) float64 {
		sums := make([]float64, meter)
		for b, v := range f {
			sums[b%meter] += v
		}
		return slices.Max(sums) / float64(n) * float64(meter)
	}
	p.Meter = 4
	if fit(3) > 1.25*fit(4) && fit(3) > 0 {
		p.Meter = 3
	}
	p.bar = barPlaces(f, p.Meter)
}

// barPlaces returns the place in the bar of each beat, from 0 to meter-1,
// along the path with the most weight w on the beats at place 0, where each
// beat moves one place on from the one before or costs slipCost.
func barPlaces(w []float64, meter int) []int {
	n := len(w)
	score := make([]float64, meter)
	from := make([][]int, n)
	for b := range n {
		next := make([]float64, meter)
		from[b] = make([]int, meter)
		top := 0
		for q := range score {
			if score[q] > score[top] {
				top = q
			}
		}
		for q := range next {
			prev := (q + meter - 1) % meter
			next[q], from[b][q] = score[prev], prev
			if score[top]-slipCost > next[q] {
				next[q], from[b][q] = score[top]-slipCost, top
			}
			if q == 0 {
				next[q] += w[b]
			}
		}
		score = next
	}
	out := make([]int, n)
	for q := range score {
		if score[q] > score[out[n-1]] {
			out[n-1] = q
		}
	}
	for b := n - 1; b > 0; b-- {
		out[b-1] = from[b][out[b]]
	}
	return out
}

// hits finds the hits in each band. A block's rise is how far its band
// rises over the three blocks before it, in dB, and it is measured against
// the rises within hitSpan either side, each held between 0 and twice
// hitRise: their mean and their mean deviation, no less than hitDeviation. A hit is a rise of at least
// hitRise dB, more than any within three blocks either side, to within
// hitBelow dB of the band's loud level, that stands hitFrom deviations above
// the mean; its strength reaches 1 at hitFull deviations. A hit in the
// middle or high band is a snare when it adds at least as much power to the
// middle band as to the high band and the high band rises by half a hit, and
// a hit in the high band that adds more power there is a hat. A kick and a
// snare within a block of each other are one hit, of the stronger kind.
// In a track with no beats, or where the tempo has less support than
// supportedAbove, a hit is kept only
// at strength lone or more and lonely seconds from any stronger hit kept
// there. A hit sounds from the first of its three blocks to carry a quarter
// of the rise, and a hit within snapWithin of a quarter of a beat is moved
// onto it.
func (p *Plan) hits(o *onsets, support []float64) {
	n := len(o.at)
	span := o.blocks(hitSpan)
	var rise, added, score [bandCount][]float64
	for k := range bandCount {
		l := o.level[k]
		rise[k], added[k] = make([]float64, n), make([]float64, n)
		for i := 3; i < n; i++ {
			low := min(l[i-1], l[i-2], l[i-3])
			rise[k][i] = l[i] - low
			added[k][i] = math.Exp(l[i]*math.Ln10/10) - math.Exp(low*math.Ln10/10)
		}
		bounded := make([]float64, n)
		for i, r := range rise[k] {
			bounded[i] = min(max(r, 0), 2*hitRise)
		}
		m := movingMean(bounded, span)
		dev := make([]float64, n)
		for i := range dev {
			dev[i] = math.Abs(bounded[i] - m[i])
		}
		dev = movingMean(dev, span)
		score[k] = make([]float64, n)
		for i := range n {
			score[k][i] = (rise[k][i] - m[i]) / math.Max(dev[i], hitDeviation)
		}
	}
	hit := func(k, i int) bool {
		r := rise[k][i]
		if r < hitRise || score[k][i] < hitFrom || o.level[k][i] < o.loud[k]-hitBelow {
			return false
		}
		for j := max(i-3, 0); j <= min(i+3, n-1); j++ {
			if rise[k][j] > r || (rise[k][j] == r && j < i) {
				return false
			}
		}
		return true
	}
	var found []Hit
	var block []int
	add := func(kind HitKind, k, i int) {
		start := i
		for j := i; j >= max(i-2, 1); j-- {
			if o.level[k][j]-o.level[k][j-1] >= rise[k][i]/4 {
				start = j
			}
		}
		h := Hit{At: o.onsetAt(start, k), Kind: kind, Strength: clamp01((score[k][i] - hitFrom) / (hitFull - hitFrom))}
		if c := len(found) - 1; c >= 0 && i-block[c] <= 1 && (found[c].Kind == HitKick) != (kind == HitKick) {
			if h.Strength > found[c].Strength {
				found[c] = h
			}
			return
		}
		found, block = append(found, h), append(block, i)
	}
	for i := 1; i < n; i++ {
		if hit(bandKick, i) {
			add(HitKick, bandKick, i)
		}
		mid := added[bandMid][i] >= added[bandHigh][i]
		switch {
		case mid && rise[bandHigh][i] >= hitRise/2 && (hit(bandMid, i) || hit(bandHigh, i)):
			add(HitSnare, bandMid, i)
		case !mid && hit(bandHigh, i):
			add(HitHat, bandHigh, i)
		}
	}
	order := make([]int, len(found))
	for h := range order {
		order[h] = h
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(found[b].Strength, found[a].Strength) })
	var lonely []float64
	for _, h := range order {
		if len(p.Beats) > 0 && support[block[h]] >= supportedAbove {
			p.Hits = append(p.Hits, found[h])
			continue
		}
		at := found[h].At
		near := slices.ContainsFunc(lonely, func(t float64) bool { return math.Abs(t-at) < lonelyGap })
		if found[h].Strength >= lone && !near {
			lonely = append(lonely, at)
			p.Hits = append(p.Hits, found[h])
		}
	}
	for h := range p.Hits {
		p.Hits[h].At = p.snap(p.Hits[h].At)
	}
	slices.SortStableFunc(p.Hits, func(a, b Hit) int { return cmp.Compare(a.At, b.At) })
}

// snap returns t moved onto the nearest quarter of a beat when it is within
// snapWithin of one, and t otherwise.
func (p *Plan) snap(t float64) float64 {
	i, _ := slices.BinarySearch(p.Beats, t)
	if i == 0 || i >= len(p.Beats) || !p.joined(i-1) {
		return t
	}
	a, d := p.Beats[i-1], (p.Beats[i]-p.Beats[i-1])/4
	q := a + math.Round((t-a)/d)*d
	if math.Abs(q-t) <= snapWithin {
		return q
	}
	return t
}

// character weighs the whole track's measures into a character, with the
// meaning each measure has in characterMeter. Regular is the share of the
// track covered by beats, weighed by their confidence, and busy counts the
// hits of strength busyAbove or more, at planBusyFull a second for full.
func (p *Plan) character() Character {
	var bright, loud, swell float64
	level := make([]float64, len(p.curve))
	for i, m := range p.curve {
		bright += clamp01(0.5 + (m.Treble+m.Air-m.Bass)*0.6)
		loud += m.Level
		level[i] = m.Level
	}
	n := float64(max(len(p.curve), 1))
	smooth := planBlur(level, 2)
	for i := 1; i < len(smooth); i++ {
		swell += math.Abs(smooth[i]-smooth[i-1]) / planStep
	}
	var strong []float64
	sharp := 0.0
	for _, h := range p.Hits {
		sharp += h.Strength
		if h.Strength >= busyAbove {
			strong = append(strong, h.At)
		}
	}
	var dynamics, busy, windows float64
	for t := 0.0; t+characterWindow <= p.Length; t += characterWindow / 2 {
		lo, hi := steps(t), min(steps(t+characterWindow), len(level))
		dynamics += spread(level[lo:hi])
		busy += float64(len(between(strong, t, t+characterWindow))) / characterWindow
		windows++
	}
	windows = math.Max(windows, 1)
	regular := 0.0
	for b := range p.Beats {
		if p.joined(b) {
			regular += (p.Beats[b+1] - p.Beats[b]) * p.conf[b]
		}
	}
	m := measures{
		steady:   clamp01(p.Pulse * 1.5),
		regular:  clamp01(regular / math.Max(p.Length, 1) / coveredFull),
		busy:     clamp01(busy / windows / planBusyFull),
		bright:   bright / n,
		dynamics: clamp01(dynamics / windows * 5),
		swell:    clamp01(swell / math.Max(n-1, 1) * 6),
		sharp:    clamp01(sharp / math.Max(float64(len(p.Hits)), 1) * 1.2),
		loud:     loud / n,
	}
	return m.character()
}

// between returns the times in a sorted list that are at or after a and
// before b.
func between(times []float64, a, b float64) []float64 {
	lo, _ := slices.BinarySearch(times, a)
	hi, _ := slices.BinarySearch(times, b)
	return times[lo:hi]
}

// Beat is the beat playing at a time.
type Beat struct {
	// Tempo is the beats per minute over the beats around it.
	Tempo float64
	// Phase is how far through the beat the time is, from 0 to 1.
	Phase float64
	// Index counts the beats of the track from 0, and Bar is the beat's
	// place in its bar, counting from 0 on the downbeat.
	Index, Bar int
	// Confidence is how well the music supports the beat, from 0 to 1.
	Confidence float64
}

// BeatAt returns the beat playing at t seconds. It reports false before the
// first beat, from the last beat of a run until the next run starts, after
// the last beat, and for a plan with no beats.
func (p *Plan) BeatAt(t float64) (Beat, bool) {
	if p == nil || len(p.Beats) < 2 || t < p.Beats[0] {
		return Beat{}, false
	}
	i, found := slices.BinarySearch(p.Beats, t)
	if !found {
		i--
	}
	if !p.joined(i) {
		return Beat{}, false
	}
	gap := p.Beats[i+1] - p.Beats[i]
	return Beat{Tempo: p.tempi[i], Phase: (t - p.Beats[i]) / gap, Index: i, Bar: p.bar[i], Confidence: p.conf[i]}, true
}

// HitsBetween returns the hits at or after a seconds and before b, and none
// when b is not after a.
func (p *Plan) HitsBetween(a, b float64) []Hit {
	if p == nil || !(b > a) {
		return nil
	}
	lo, _ := slices.BinarySearchFunc(p.Hits, a, func(h Hit, t float64) int { return cmp.Compare(h.At, t) })
	hi, _ := slices.BinarySearchFunc(p.Hits, b, func(h Hit, t float64) int { return cmp.Compare(h.At, t) })
	return p.Hits[lo:hi]
}

// Character returns the character of the whole track, and an even blend for
// a track measured without bands.
func (p *Plan) Character() Character {
	if p == nil || !p.banded {
		return Character{0.25, 0.25, 0.25, 0.25}
	}
	return p.char
}

// Beatless reports whether the track was measured in bands and has no steady
// beat.
func (p *Plan) Beatless() bool { return p != nil && p.banded && len(p.Beats) == 0 }

// fourier returns the magnitude of the repeats in x, less its mean and
// tapered at both ends, at a period of each of the lags, in samples.
func fourier(x []float64, lags []float64) []float64 {
	m, n := mean(x), float64(len(x))
	d := make([]float64, len(x))
	for i, v := range x {
		d[i] = (v - m) * (0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/n))
	}
	out := make([]float64, len(lags))
	for s, l := range lags {
		c := 2 * math.Cos(2*math.Pi/l)
		var s1, s2 float64
		for _, v := range d {
			s1, s2 = v+c*s1-s2, s1
		}
		out[s] = math.Sqrt(math.Max(s1*s1+s2*s2-c*s1*s2, 0))
	}
	return out
}

// autocorrelation returns the autocorrelation of x less its mean, at lags 0
// to maxLag, divided by its value at lag 0.
func autocorrelation(x []float64, maxLag int) []float64 {
	m := mean(x)
	d := make([]float64, len(x))
	for i, v := range x {
		d[i] = v - m
	}
	out := make([]float64, maxLag+1)
	for l := range out {
		for i := l; i < len(d); i++ {
			out[l] += d[i] * d[i-l]
		}
	}
	if out[0] > 0 {
		zero := out[0]
		for l := range out {
			out[l] /= zero
		}
	}
	return out
}

// at returns v at a fractional index, interpolated between its neighbours,
// and the nearest end outside it.
func at(v []float64, x float64) float64 {
	if len(v) == 0 {
		return 0
	}
	if x <= 0 {
		return v[0]
	}
	i := int(x)
	if i >= len(v)-1 {
		return v[len(v)-1]
	}
	f := x - float64(i)
	return v[i]*(1-f) + v[i+1]*f
}

// movingMean returns the mean of v over the k points either side of each
// point.
func movingMean(v []float64, k int) []float64 {
	sum := make([]float64, len(v)+1)
	for i, x := range v {
		sum[i+1] = sum[i] + x
	}
	out := make([]float64, len(v))
	for i := range v {
		lo, hi := max(i-k, 0), min(i+k+1, len(v))
		out[i] = (sum[hi] - sum[lo]) / float64(hi-lo)
	}
	return out
}

// quantile returns the value below which a fraction q of v lies.
func quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	return s[min(int(q*float64(len(s))), len(s)-1)]
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func squares(v []float64) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x * x
	}
	return out
}
