package sky

import (
	"math"
	"slices"
)

// Part is one stretch of a track that sounds of a piece: where it starts and
// ends in seconds, and how it sounds.
type Part struct {
	Start, End float64
	// Level is the mean level of the whole sound in dB relative to full
	// scale. Spread is how far the level moves from block to block, as the
	// standard deviation in dB: small for a wall of sound, large for music
	// that pumps with its beat.
	Level, Spread float64
	// Mid is the level of the middle band against the whole sound, in dB.
	Mid float64
	// Intensity is how hard the part hits, from 0 to 1, and Heavy how far
	// it is a loud, level wall of sound in the middle band, from 0 to 1.
	Intensity, Heavy float64
	// Silent is set for what follows the point where the sound stops for
	// good.
	Silent bool
}

// Boundary is where one part ends and the next begins.
type Boundary struct {
	// At is the time of the change in seconds.
	At float64
	// Strength is how unlike the parts either side are, from 0 up.
	Strength float64
	// Major is set for a boundary where the whole character changes, and
	// Hard for one where the level steps at once.
	Major, Hard bool
}

// The part finder's measures. Times are in seconds.
const (
	// partStep is the time between two points of the texture.
	partStep = 0.25
	// The texture either side of a point is compared over partNear and over
	// partFar.
	partNear = 4.0
	partFar  = 12.0
	// partLeast is the shortest part.
	partLeast = 5.0
	// partSnap is how far a boundary is moved to land on a beat.
	partSnap = 0.06
	// The sharpest step of a change is looked for from sharpReach before
	// the earliest place it was found to sharpAfter past the latest, and
	// sharpHold the stretch either side of a step over
	// which the sound is compared.
	sharpReach = 1.5
	sharpAfter = 0.5
	sharpHold  = 1.5
	sharpNear  = 0.85
	// sharpHeading and sharpMost are in units of 6 dB.
	sharpHeading = 0.5
	sharpMost    = 2.0
	// A boundary is major when the partContext either side of it differ by
	// majorFrom.
	majorFrom = 1.5
	// noveltyLeast is the least difference, near and far together, that
	// marks a boundary.
	noveltyLeast = 0.5
	partContext  = 30.0
	// firstBack is how far before a held change its first sound is looked
	// for. A step in level of jumpFull dB over jumpSpan either side of a
	// boundary adds 1 to its strength.
	firstBack = 4.0
	firstHold = 0.3
	jumpSpan  = 0.4
	jumpFull  = 12.0
	// hardJump is the step in level, in dB, from which a boundary is hard.
	hardJump = 4.0
	// stopBelow is how far under the track's usual level, in dB, the sound
	// has stopped.
	stopBelow = 30.0
)

// texture is how a track sounds at each partStep: the level of the whole
// sound in dB, each band against it, and how bright it is.
type texture struct {
	at    []float64
	level []float64
	v     [][]float64
	// block holds the level of every block and when it starts, and blockV
	// its level and width, each in units of 6 dB.
	blockAt, blockLevel []float64
	blockV              [][2]float64
}

// widthOf returns how wide a block's sound is: the level of its side against
// its whole sound in dB, no lower than narrowest.
func widthOf(b Block, level float64) float64 {
	if b.Side <= 0 {
		return narrowest
	}
	return math.Max(20*math.Log10(b.Side)-level, narrowest)
}

// narrowest is the width given to sound that is the same in both channels.
const narrowest = -30.0

// wholeLevel returns the level of a block's whole sound in dB.
func wholeLevel(b Block) float64 {
	e := 0.0
	for _, c := range b.Channels {
		e += power(c.Loudness)
	}
	if e <= 0 || len(b.Channels) == 0 {
		return silentBelow
	}
	return math.Max(10*math.Log10(e/float64(len(b.Channels))), silentBelow)
}

// textureOf measures the texture of the blocks measured in bands.
func textureOf(blocks []Block) *texture {
	x := &texture{}
	type sum struct {
		n, energy float64
		bands     [bandCount]float64
		bright    float64
		side      float64
	}
	var bins []sum
	for _, b := range blocks {
		if len(b.Bands) != bandCount || b.Rate <= 0 || b.Samples <= 0 || len(b.Channels) == 0 {
			continue
		}
		l := wholeLevel(b)
		wide := widthOf(b, l)
		x.blockAt, x.blockLevel = append(x.blockAt, b.At), append(x.blockLevel, l)
		x.blockV = append(x.blockV, [2]float64{l / 6, wide / 6})
		i := int(b.At / partStep)
		for len(bins) <= i {
			bins = append(bins, sum{})
		}
		s := &bins[i]
		s.n++
		s.energy += math.Pow(10, l/10)
		for k, v := range b.Bands {
			if math.IsNaN(v) {
				v = silentBelow
			}
			s.bands[k] += math.Pow(10, math.Max(v, silentBelow)/10)
		}
		d := 0.0
		for _, c := range b.Channels {
			d += c.Difference / float64(len(b.Channels))
		}
		s.bright += d * d
		s.side += b.Side * b.Side
	}
	for i, s := range bins {
		if s.n == 0 {
			continue
		}
		l := math.Max(10*math.Log10(s.energy/s.n+1e-12), silentBelow)
		row := []float64{l / 6}
		for k := range bandCount {
			row = append(row, (math.Max(10*math.Log10(s.bands[k]/s.n+1e-12), silentBelow)-l)/6)
		}
		row = append(row, (math.Max(10*math.Log10(s.bright/s.n+1e-12), silentBelow)-l)/6)
		row = append(row, math.Max(10*math.Log10(s.side/s.n+1e-12)-l, narrowest)/6)
		x.at, x.level, x.v = append(x.at, float64(i)*partStep), append(x.level, l), append(x.v, row)
	}
	return x
}

// apart returns how unlike the texture is over the span before point i and
// over the span after it: the distance between the two means.
func (x *texture) apart(i int, span float64) float64 {
	w := int(span / partStep)
	if i-w < 0 || i+w > len(x.v) {
		return 0
	}
	d := 0.0
	for k := range x.v[i] {
		var a, b float64
		for j := range w {
			a += x.v[i-w+j][k]
			b += x.v[i+j][k]
		}
		d += (a - b) * (a - b) / float64(w*w)
	}
	return math.Sqrt(d)
}

// partOf measures the stretch from a to b.
func (x *texture) partOf(a, b float64) Part {
	p := Part{Start: a, End: b, Level: silentBelow}
	lo, _ := slices.BinarySearch(x.blockAt, a)
	hi, _ := slices.BinarySearch(x.blockAt, b)
	if hi <= lo {
		return p
	}
	l := x.blockLevel[lo:hi]
	p.Level = mean(l)
	for _, v := range l {
		p.Spread += (v - p.Level) * (v - p.Level) / float64(len(l))
	}
	p.Spread = math.Sqrt(p.Spread)
	n := 0.0
	for i, at := range x.at {
		if at >= a && at < b {
			p.Mid += x.v[i][1+bandMid] * 6
			n++
		}
	}
	if n > 0 {
		p.Mid /= n
	}
	return p
}

// between returns how unlike the texture is from a to b and from b to c: the
// distance between the two means, or 0 when either stretch is empty.
func (x *texture) between(a, b, c float64) float64 {
	lo, mid, hi := int(math.Ceil(a/partStep)), int(math.Round(b/partStep)), int(c/partStep)
	lo, hi = max(lo, 0), min(hi, len(x.v))
	if mid <= lo || hi <= mid {
		return 0
	}
	d := 0.0
	for k := range x.v[0] {
		var p, q float64
		for _, row := range x.v[lo:mid] {
			p += row[k]
		}
		for _, row := range x.v[mid:hi] {
			q += row[k]
		}
		p, q = p/float64(mid-lo), q/float64(hi-mid)
		d += (p - q) * (p - q)
	}
	return math.Sqrt(d)
}

// split returns the start of the block within reach of t that best divides
// the blocks around it in two: the one for which the level and width of the
// blocks before it, and of those after it, lie closest to their own means.
func (x *texture) split(t, reach float64) float64 {
	lo, _ := slices.BinarySearch(x.blockAt, t-2*reach)
	hi, _ := slices.BinarySearch(x.blockAt, t+2*reach)
	from, _ := slices.BinarySearch(x.blockAt, t-reach)
	to, _ := slices.BinarySearch(x.blockAt, t+reach)
	if hi-lo < 8 {
		return t
	}
	// Sums of each measure and its square up to each block give the spread
	// either side of every split in one pass.
	n := hi - lo
	var sum, sq [2][]float64
	for k := range 2 {
		sum[k], sq[k] = make([]float64, n+1), make([]float64, n+1)
		for i := range n {
			v := x.blockV[lo+i][k]
			sum[k][i+1], sq[k][i+1] = sum[k][i]+v, sq[k][i]+v*v
		}
	}
	best, at := math.Inf(1), t
	for i := max(from-lo, 4); i < min(to-lo, n-4); i++ {
		cost := 0.0
		for k := range 2 {
			a, b := float64(i), float64(n-i)
			cost += sq[k][i] - sum[k][i]*sum[k][i]/a
			cost += (sq[k][n] - sq[k][i]) - (sum[k][n]-sum[k][i])*(sum[k][n]-sum[k][i])/b
		}
		if cost < best {
			best, at = cost, x.blockAt[lo+i]
		}
	}
	return at
}

// meanOf returns the mean level and width of the blocks from a to b, and
// false when there are none.
func (x *texture) meanOf(a, b float64) ([2]float64, bool) {
	lo, _ := slices.BinarySearch(x.blockAt, a)
	hi, _ := slices.BinarySearch(x.blockAt, b)
	var m [2]float64
	if hi <= lo {
		return m, false
	}
	for _, v := range x.blockV[lo:hi] {
		m[0] += v[0] / float64(hi-lo)
		m[1] += v[1] / float64(hi-lo)
	}
	return m, true
}

// first returns where the sound that holds after t is first heard: the
// start of the earliest block, up to firstBack before t, that lies less
// than half as far from the sound after t as from the sound before that
// stretch, and is followed for firstHold by blocks of which two in three
// lie nearer the sound after. It is t when there is none.
func (x *texture) first(t float64) float64 {
	before, ok1 := x.meanOf(t-2*firstBack, t-firstBack)
	after, ok2 := x.meanOf(t+0.5, t+firstBack)
	if !ok1 || !ok2 {
		return t
	}
	far := func(v, m [2]float64) float64 { return math.Hypot(v[0]-m[0], v[1]-m[1]) }
	if far(before, after) < 0.5 {
		return t
	}
	lo, _ := slices.BinarySearch(x.blockAt, t-firstBack)
	hi, _ := slices.BinarySearch(x.blockAt, t)
	// A block counts only when most of the firstHold from it sounds like
	// what follows too, so that one stray note is not taken for it.
	hold := max(int(firstHold/x.hop()), 1)
	for i := lo; i < hi; i++ {
		if far(x.blockV[i], after) >= far(x.blockV[i], before)/2 {
			continue
		}
		like := 0
		for _, v := range x.blockV[i:min(i+hold, len(x.blockV))] {
			if far(v, after) < far(v, before) {
				like++
			}
		}
		if 3*like >= 2*hold {
			return x.blockAt[i]
		}
	}
	return t
}

// hop returns the time from one block to the next.
func (x *texture) hop() float64 {
	if len(x.blockAt) < 2 {
		return partStep
	}
	return (x.blockAt[len(x.blockAt)-1] - x.blockAt[0]) / float64(len(x.blockAt)-1)
}

// sharpest returns the start of the block between start and end that best
// marks a change. A block's mark is the step in level across it, three blocks
// either side, added to half the step in width, six blocks either side,
// and to the difference in level and width between the sharpHold before it
// and the sharpHold after it, each counted up to sharpMost. Where the level over those stretches moves by sharpHeading or
// more, only a step the same way counts. The block returned is the earliest
// whose mark is within sharpNear of the highest.
func (x *texture) sharpest(start, end float64) float64 {
	t := (start + end) / 2
	lo, _ := slices.BinarySearch(x.blockAt, start)
	hi, _ := slices.BinarySearch(x.blockAt, end)
	const k = 3
	from, to := max(lo, k), min(hi, len(x.blockV)-k)
	if to <= from {
		return t
	}
	score := make([]float64, to-from)
	for i := from; i < to; i++ {
		var a, b float64
		for j := range k {
			a += x.blockV[i-k+j][0] / k
			b += x.blockV[i+j][0] / k
		}
		before, ok1 := x.meanOf(x.blockAt[i]-sharpHold, x.blockAt[i])
		after, ok2 := x.meanOf(x.blockAt[i], x.blockAt[i]+sharpHold)
		if !ok1 || !ok2 {
			continue
		}
		// A step counts when it goes the way the level is heading, or
		// either way when the level is heading nowhere.
		step, heading := b-a, after[0]-before[0]
		switch {
		case math.Abs(heading) < sharpHeading:
			step = math.Abs(step)
		case heading < 0:
			step = -step
		}
		// The width steps too; it is taken over twice as many blocks and
		// counts for half, since it wavers from block to block.
		wide := 0.0
		if i >= 2*k && i+2*k <= len(x.blockV) {
			for j := range 2 * k {
				wide += (x.blockV[i+j][1] - x.blockV[i-2*k+j][1]) / (2 * k)
			}
		}
		score[i-from] = clamp(step, 0, sharpMost) + math.Min(math.Abs(wide)/2, sharpMost) +
			math.Min(math.Hypot(before[0]-after[0], before[1]-after[1]), sharpMost)
	}
	best := slices.Max(score)
	for i, v := range score {
		if v >= sharpNear*best {
			return x.blockAt[from+i]
		}
	}
	return t
}

// jump returns how far the level steps at t, in dB: the mean of the
// jumpSpan after it less the mean of the jumpSpan before it.
func (x *texture) jump(t float64) float64 {
	a, ok1 := x.meanOf(t-jumpSpan, t)
	b, ok2 := x.meanOf(t, t+jumpSpan)
	if !ok1 || !ok2 {
		return 0
	}
	return 6 * (b[0] - a[0])
}

// stop returns when the sound ends for good: the start of the first block
// after which every block is stopBelow dB under the track's usual level. It
// is the end of the track when the sound runs to it.
func (x *texture) stop() float64 {
	n := len(x.blockAt)
	if n == 0 {
		return 0
	}
	floor := quantile(x.blockLevel, 0.5) - stopBelow
	i := n
	for i > 0 && x.blockLevel[i-1] < floor {
		i--
	}
	if i >= n {
		return x.at[len(x.at)-1] + partStep
	}
	return x.blockAt[i]
}

// findParts splits a track into parts. A point is a boundary where the
// texture before it and after it differ more than anywhere within partNear
// of it, by at least noveltyLeast, near and far taken together. Each
// boundary is moved onto the block that best divides the sound around it,
// and where the sound that follows is first heard is found, and the
// boundary is put on the sharpest step from sharpReach before the earliest
// of those to sharpAfter past the latest, and then onto a beat when one is
// within partSnap. Where the sound stops
// for good before the end of the track is a boundary too. A boundary's
// strength is how unlike the partContext before it and after it are, less
// for a boundary with a shorter stretch of sound on either side, and more
// for a step in level at the boundary itself.
func findParts(blocks []Block, beats []float64) ([]Part, []Boundary) {
	x := textureOf(blocks)
	n := len(x.v)
	if n == 0 {
		return nil, nil
	}
	length := x.at[n-1] + partStep
	end := x.stop()
	novelty := make([]float64, n)
	for i := range novelty {
		novelty[i] = x.apart(i, partNear) + x.apart(i, partFar)
	}
	w := int(partNear / partStep)
	var cuts []float64
	for i := w; i < n-w; i++ {
		peak := novelty[i] >= noveltyLeast
		for j := i - w; j <= i+w && peak; j++ {
			peak = j == i || novelty[j] < novelty[i] || (j > i && novelty[j] == novelty[i])
		}
		if !peak {
			continue
		}
		// The change, the block that best divides the sound around it and
		// the first sound of what follows bound where its exact time is
		// looked for.
		split := x.split(x.at[i], partNear/2)
		first := x.first(split)
		at := x.sharpest(min(x.at[i], split, first)-sharpReach, max(x.at[i], split, first)+sharpAfter)
		j, _ := slices.BinarySearch(beats, at)
		for _, k := range []int{j - 1, j} {
			if k >= 0 && k < len(beats) && math.Abs(beats[k]-at) <= partSnap {
				at = beats[k]
				break
			}
		}
		if at >= partLeast && at <= end-partLeast && (len(cuts) == 0 || at-cuts[len(cuts)-1] >= partLeast) {
			cuts = append(cuts, at)
		}
	}
	if end < length-partStep {
		cuts = append(cuts, x.sharpest(end-sharpReach, end+sharpAfter))
	}
	edges := append(append([]float64{0}, cuts...), length)
	parts := make([]Part, len(edges)-1)
	for i := range parts {
		parts[i] = x.partOf(edges[i], edges[i+1])
	}
	if end < length-partStep {
		parts[len(parts)-1].Silent = true
	}
	bounds := make([]Boundary, len(cuts))
	for i, at := range cuts {
		before, after := math.Min(at, partContext), math.Min(end-at, partContext)
		s := x.between(at-before, at, at+after) * math.Min(1, math.Min(before, after)/(partContext/2))
		if i == len(cuts)-1 && end < length-partStep {
			s = x.between(at-before, at, length)
		}
		jump := math.Abs(x.jump(at))
		s += jump / jumpFull
		bounds[i] = Boundary{At: at, Strength: s, Major: s >= majorFrom, Hard: jump >= hardJump}
	}
	return parts, bounds
}

// clamp returns v held between lo and hi.
func clamp(v, lo, hi float64) float64 { return math.Min(math.Max(v, lo), hi) }

// The measures of a track carried by its arrangement. Levels are in dB.
const (
	// A track with beats whose kicks usually lift the peak of the band
	// where a kick starts by less than kickWeighty dB is carried by its
	// arrangement and not its beat.
	kickWeighty = 6.5
	// A part's intensity falls to 0 at intensityRange under the level of
	// the track's loudest parts, and at quietLevel whatever the track.
	intensityRange = 14.0
	quietLevel     = -34.0
	loudLevel      = -14.0
	// A part is heavy when it is within heavyUnder of the loudest parts and
	// above heavyLevel, its level spreads by less than heavySpread, and its
	// middle band is no more than heavyMid under the whole sound. Each
	// counts in full from its first figure and not at all at its second.
	heavyUnder  = 2.0
	heavyNone   = 4.5
	heavyLevel  = -10.5
	heavyQuiet  = -12.5
	heavySpread = 3.0
	heavyWaver  = 5.0
	heavyMid    = -9.0
	heavyThin   = -11.5
	// developFrom is the strength from which a boundary that is not major
	// still changes the scene by degrees.
	developFrom = 1.0
)

// lightKick reports whether the track has beats and its kicks are light: the
// peak of the band where a kick starts, over the block a kicked beat falls
// in and the two after it, stands less than kickWeighty dB above its mean
// over the two blocks before, for half the kicked beats or more. It is
// false for a track measured without that peak.
func (p *Plan) lightKick(blocks []Block) bool {
	var at, peak []float64
	for _, b := range blocks {
		if len(b.Bands) == bandCount && b.Punch > 0 {
			at, peak = append(at, b.At), append(peak, math.Max(20*math.Log10(b.Punch), silentBelow))
		}
	}
	var punch []float64
	for _, beat := range p.Beats {
		if i, _ := slices.BinarySearch(at, beat); i >= 3 && i+2 <= len(peak) && p.kicked(beat) {
			punch = append(punch, slices.Max(peak[i-1:i+2])-mean(peak[i-3:i-1]))
		}
	}
	return len(punch) > 0 && quantile(punch, 0.5) < kickWeighty
}

// arrange finds the parts of a track measured in bands and, when the track
// is carried by its arrangement, makes its boundaries the cues the scenes
// change on: a major boundary as a drop, one of strength developFrom or
// more as a section, and the sections and drops found from the level alone
// set aside.
func (p *Plan) arrange(blocks []Block) {
	if !p.banded {
		return
	}
	x := textureOf(blocks)
	if len(x.blockAt) == 0 {
		return
	}
	p.Parts, p.Bounds = findParts(blocks, p.Beats)
	if len(p.Parts) == 0 {
		return
	}
	p.arranged = p.lightKick(blocks)
	var levels []float64
	for _, part := range p.Parts {
		for range int(part.End-part.Start) + 1 {
			levels = append(levels, part.Level)
		}
	}
	top := quantile(levels, 0.9)
	for i := range p.Parts {
		part := &p.Parts[i]
		near := clamp01(1 - (top-part.Level)/intensityRange)
		part.Intensity = near * clamp01((part.Level-quietLevel)/(loudLevel-quietLevel))
		if part.Silent {
			part.Intensity = 0
			continue
		}
		part.Heavy = clamp01((part.Level-(top-heavyNone))/(heavyNone-heavyUnder)) *
			clamp01((part.Level-heavyQuiet)/(heavyLevel-heavyQuiet)) *
			clamp01((heavyWaver-part.Spread)/(heavyWaver-heavySpread)) *
			clamp01((part.Mid-heavyThin)/(heavyMid-heavyThin))
	}
	if !p.arranged {
		return
	}
	cues := p.Cues[:0:0]
	for _, c := range p.Cues {
		if c.Kind != CueSection && c.Kind != CueDrop {
			cues = append(cues, c)
		}
	}
	for _, b := range p.Bounds {
		switch {
		case b.Major:
			cues = append(cues, Cue{At: b.At, Kind: CueDrop, Strength: 1})
		case b.Strength >= developFrom:
			cues = append(cues, Cue{At: b.At, Kind: CueSection, Strength: clamp01(b.Strength / (2 * majorFrom))})
		}
	}
	p.Cues = cues
}

// partAt returns the part playing at t, and false when the track has none.
func (p *Plan) partAt(t float64) (Part, bool) {
	for _, part := range p.Parts {
		if t < part.End {
			return part, true
		}
	}
	if n := len(p.Parts); n > 0 {
		return p.Parts[n-1], true
	}
	return Part{}, false
}

// hardAt reports whether the boundary at t is one where the level steps at
// once.
func (p *Plan) hardAt(t float64) bool {
	for _, b := range p.Bounds {
		if b.At == t {
			return b.Hard
		}
	}
	return false
}
