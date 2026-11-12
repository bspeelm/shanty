//go:build corpus

package main

import (
	"cmp"
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/sky"
)

// The corpus evaluation measures every audio file in SHANTY_CORPUS with the
// real analyser, one at a time, builds its plan, and compares the plan's beats with the
// beats in SHANTY_CORPUS_REF/<name>.beats, one time in seconds per line, when
// that file exists. SHANTY_CORPUS_CACHE, when set, is a directory where the
// measured blocks are kept, so that a second run builds plans without
// measuring again. It runs with
//
//	go test -tags=corpus -run Corpus -v ./cmd/shanty
const (
	corpusEnv      = "SHANTY_CORPUS"
	corpusRefEnv   = "SHANTY_CORPUS_REF"
	corpusCacheEnv = "SHANTY_CORPUS_CACHE"
	// corpusOutEnv, when set, is a directory where each plan's beats and
	// downbeats are written, one time per line, as <name>.beats and
	// <name>.downbeats, and its hits as <name>.hits.
	corpusOutEnv = "SHANTY_CORPUS_OUT"
	// corpusWholeEnv, when set, is a directory of blocks measured by the
	// analyser before the bands, written by TestCorpusWholeSound in
	// internal/mpv; the cues of the two analyses are compared.
	corpusWholeEnv = "SHANTY_CORPUS_WHOLE"
	// matchWithin is how close an estimated beat must be to a reference beat
	// to count as the same beat.
	matchWithin = 0.07
)

func TestCorpus(t *testing.T) {
	dir := os.Getenv(corpusEnv)
	if dir == "" {
		t.Skip(corpusEnv + " is not set")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.flac"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no tracks in %s: %v", dir, err)
	}
	slices.Sort(files)
	measures := make([][]mpv.Block, len(files))
	failures := make([]error, len(files))
	for i, file := range files {
		measures[i], failures[i] = measured(t, file, trackName(file))
	}
	for i, file := range files {
		if failures[i] != nil {
			t.Errorf("%s: %v", trackName(file), failures[i])
			continue
		}
		t.Log(evaluate(t, file, measures[i]))
	}
}

func trackName(file string) string {
	return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

// evaluate builds the plan of one measured track, timing the build on its
// own, and describes the plan and how its beats compare with the reference.
func evaluate(t *testing.T, file string, blocks []mpv.Block) string {
	name := trackName(file)
	in := planBlocks(blocks)
	var plan *sky.Plan
	var took []time.Duration
	for range 5 {
		start := time.Now()
		plan = sky.NewPlan(in)
		took = append(took, time.Since(start))
	}
	slices.Sort(took)
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %.0f s, plan built in %s\n", name, plan.Length, took[len(took)/2].Round(time.Millisecond))
	fmt.Fprintf(&b, "  tempo %.2f BPM, %d beats, pulse %.2f, beatless %v, meter %d, %d downbeats\n",
		plan.Tempo, len(plan.Beats), plan.Pulse, plan.Beatless(), plan.Meter, len(plan.Downbeats))
	counts := map[sky.HitKind]int{}
	for _, h := range plan.Hits {
		counts[h.Kind]++
	}
	c := plan.Character()
	fmt.Fprintf(&b, "  hits: %d kick, %d snare, %d hat; character calm %.2f drive %.2f grit %.2f flow %.2f\n",
		counts[sky.HitKick], counts[sky.HitSnare], counts[sky.HitHat], c.Calm, c.Drive, c.Grit, c.Flow)
	kicks := kickOnsets(in)
	covered, confidence := coverage(plan)
	strong, most := strongHits(plan.Hits, 5)
	fmt.Fprintf(&b, "  beats cover %.0f%% of the track at mean confidence %.2f; hits %.2f a second, strong %.2f a second, at most %d strong in 5 s; notes %.2f a second\n",
		100*covered, confidence, float64(len(plan.Hits))/plan.Length, strong/plan.Length, most, float64(len(plan.Notes))/plan.Length)
	fmt.Fprintf(&b, "  kick band onsets in the blocks %.2f a second; hits %.2f kicks and %.2f snares a second\n",
		float64(len(kicks))/plan.Length, float64(counts[sky.HitKick])/plan.Length, float64(counts[sky.HitSnare])/plan.Length)
	fmt.Fprintf(&b, "  kick band against our beats: %s\n", alignment(plan.Beats, kicks))
	fmt.Fprintf(&b, "  downbeats: %s\n", downbeatSanity(plan, in))
	if out := os.Getenv(corpusOutEnv); out != "" {
		for ext, times := range map[string][]float64{".beats": plan.Beats, ".downbeats": plan.Downbeats} {
			var lines strings.Builder
			for _, t := range times {
				fmt.Fprintf(&lines, "%.4f\n", t)
			}
			if err := os.WriteFile(filepath.Join(out, name+ext), []byte(lines.String()), 0o600); err != nil {
				t.Error(err)
			}
		}
		var lines strings.Builder
		for _, h := range plan.Hits {
			fmt.Fprintf(&lines, "%.4f %s %.2f\n", h.At, h.Kind, h.Strength)
		}
		if err := os.WriteFile(filepath.Join(out, name+".hits"), []byte(lines.String()), 0o600); err != nil {
			t.Error(err)
		}
	}
	if whole := os.Getenv(corpusWholeEnv); whole != "" {
		fmt.Fprintf(&b, "  cues against the analyser before the bands: %s\n", cueDiff(filepath.Join(whole, name+".gob"), plan))
	}
	ref, err := readBeats(filepath.Join(refDir(dir(file)), name+".beats"))
	if err != nil {
		return b.String()
	}
	f, run, offset := score(plan.Beats, ref)
	double, half := doubled(ref), halved(ref)
	fd, _, _ := score(plan.Beats, double)
	fh0, _, _ := score(plan.Beats, half[0])
	fh1, _, _ := score(plan.Beats, half[1])
	refTempo := 60 * float64(len(ref)-1) / (ref[len(ref)-1] - ref[0])
	fmt.Fprintf(&b, "  reference %.2f BPM, %d beats; kick band against its beats: %s\n", refTempo, len(ref), alignment(ref, kicks))
	shifted := make([]float64, len(plan.Beats))
	for i, t := range plan.Beats {
		shifted[i] = t - offset
	}
	fs, runs, _ := score(shifted, ref)
	fmt.Fprintf(&b, "  F %.3f, longest run %.0f%% of the reference, median offset %+.1f ms; with the offset removed F %.3f, longest run %.0f%%; F at double %.3f, at half %.3f/%.3f",
		f, 100*run, 1000*offset, fs, 100*runs, fd, fh0, fh1)
	return b.String()
}

func dir(file string) string { return filepath.Dir(file) }

// refDir is where the reference beats are.
func refDir(audio string) string {
	if d := os.Getenv(corpusRefEnv); d != "" {
		return d
	}
	return audio
}

// measured returns the blocks of a track, from the cache when it holds them.
func measured(t *testing.T, file, name string) ([]mpv.Block, error) {
	cache := os.Getenv(corpusCacheEnv)
	path := filepath.Join(cache, name+".gob")
	if cache != "" {
		if f, err := os.Open(path); err == nil {
			defer f.Close() // a read-only file has nothing to flush
			var blocks []mpv.Block
			if err := gob.NewDecoder(f).Decode(&blocks); err == nil {
				return blocks, nil
			}
		}
	}
	sock, err := os.MkdirTemp("/tmp", "sh")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(sock) // a temporary directory left behind is harmless
	start := time.Now()
	blocks, err := mpv.Analyse(t.Context(), mpv.Options{Socket: filepath.Join(sock, "mpv.sock")}, file)
	if err != nil {
		return nil, err
	}
	t.Logf("%s: measured %d blocks in %s", name, len(blocks), time.Since(start).Round(time.Millisecond))
	if cache != "" {
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer f.Close() // the encoder's error is the one that matters
		if err := gob.NewEncoder(f).Encode(blocks); err != nil {
			return nil, err
		}
	}
	return blocks, nil
}

func readBeats(path string) ([]float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []float64
	for _, line := range strings.Fields(string(b)) {
		if v, err := strconv.ParseFloat(line, 64); err == nil {
			out = append(out, v)
		}
	}
	return out, nil
}

// score returns the F-measure of the estimated beats against the reference
// within matchWithin, the longest run of consecutive estimated beats matched
// to consecutive reference beats as a share of the reference, and the median
// signed time from a reference beat to the estimated beat matched to it.
func score(est, ref []float64) (float64, float64, float64) {
	if len(est) == 0 || len(ref) == 0 {
		return 0, 0, 0
	}
	matched := make([]int, len(est))
	used := make([]bool, len(ref))
	var offsets []float64
	hits := 0
	for i, e := range est {
		matched[i] = -1
		j, _ := slices.BinarySearch(ref, e)
		best := -1
		for _, k := range []int{j - 1, j} {
			if k >= 0 && k < len(ref) && !used[k] && math.Abs(ref[k]-e) <= matchWithin &&
				(best < 0 || math.Abs(ref[k]-e) < math.Abs(ref[best]-e)) {
				best = k
			}
		}
		if best >= 0 {
			used[best], matched[i] = true, best
			offsets = append(offsets, e-ref[best])
			hits++
		}
	}
	p, r := float64(hits)/float64(len(est)), float64(hits)/float64(len(ref))
	f := 0.0
	if p+r > 0 {
		f = 2 * p * r / (p + r)
	}
	longest, run := 0, 0
	for i := range est {
		switch {
		case matched[i] < 0:
			run = 0
		case run > 0 && matched[i-1] == matched[i]-1:
			run++
		default:
			run = 1
		}
		longest = max(longest, run)
	}
	return f, float64(longest) / float64(len(ref)), median(offsets)
}

// doubled returns the reference with a beat added halfway between each two.
func doubled(ref []float64) []float64 {
	var out []float64
	for i, r := range ref {
		out = append(out, r)
		if i+1 < len(ref) {
			out = append(out, (r+ref[i+1])/2)
		}
	}
	return out
}

// halved returns the reference's even beats and its odd beats.
func halved(ref []float64) [2][]float64 {
	var out [2][]float64
	for i, r := range ref {
		out[i%2] = append(out[i%2], r)
	}
	return out
}

// kickOnsets returns the kick onsets found from the measured blocks alone:
// for each block whose lowest band rises by at least 6 dB over the three
// blocks before it, more than any block within three either side, the start
// of the block among it and the two before it with the steepest rise.
func kickOnsets(blocks []sky.Block) []float64 {
	var at, l []float64
	for _, b := range blocks {
		if len(b.Bands) > 0 {
			at, l = append(at, b.At), append(l, math.Max(b.Bands[0], -150))
		}
	}
	rise := make([]float64, len(l))
	for i := 3; i < len(l); i++ {
		rise[i] = l[i] - min(l[i-1], l[i-2], l[i-3])
	}
	var out []float64
	for i := range rise {
		peak := rise[i] >= 6
		for j := max(i-3, 0); j <= min(i+3, len(rise)-1) && peak; j++ {
			peak = j == i || rise[j] < rise[i] || (rise[j] == rise[i] && j > i)
		}
		if peak {
			steep := i
			for j := max(i-2, 1); j <= i; j++ {
				if l[j]-l[j-1] > l[steep]-l[steep-1] {
					steep = j
				}
			}
			out = append(out, at[steep])
		}
	}
	return out
}

// alignment describes where the kick onsets fall against a list of beats:
// the share of beats with one within 70 ms, the median time from a beat to
// the nearest such onset and the median deviation from it, and the share of beats with an onset within 70 ms
// of the point halfway to the next beat.
func alignment(beats, kicks []float64) string {
	if len(beats) < 2 || len(kicks) == 0 {
		return "no beats"
	}
	nearest := func(t float64) (float64, bool) {
		j, _ := slices.BinarySearch(kicks, t)
		best, ok := 0.0, false
		for _, k := range []int{j - 1, j} {
			if k >= 0 && k < len(kicks) && math.Abs(kicks[k]-t) <= 0.07 && (!ok || math.Abs(kicks[k]-t) < math.Abs(best)) {
				best, ok = kicks[k]-t, true
			}
		}
		return best, ok
	}
	var offsets []float64
	half := 0
	for i, t := range beats {
		if d, ok := nearest(t); ok {
			offsets = append(offsets, d)
		}
		if i+1 < len(beats) {
			if _, ok := nearest((t + beats[i+1]) / 2); ok {
				half++
			}
		}
	}
	m := median(offsets)
	spread := make([]float64, len(offsets))
	for i, d := range offsets {
		spread[i] = math.Abs(d - m)
	}
	return fmt.Sprintf("%.0f%% of beats have a kick onset within 70 ms, median offset %+.1f ms, median deviation %.1f ms; %.0f%% halfway between beats",
		100*float64(len(offsets))/float64(len(beats)), 1000*m, 1000*median(spread), 100*float64(half)/float64(len(beats)-1))
}

// coverage is the share of the track between two beats of one run, and the
// mean confidence of the beats.
func coverage(p *sky.Plan) (float64, float64) {
	var span, conf float64
	for i := 0; i+1 < len(p.Beats); i++ {
		if beat, ok := p.BeatAt(p.Beats[i]); ok {
			span += p.Beats[i+1] - p.Beats[i]
			conf += beat.Confidence
		}
	}
	if len(p.Beats) == 0 {
		return 0, 0
	}
	return span / p.Length, conf / float64(len(p.Beats))
}

// strongHits counts the hits of strength 0.3 or more, and the most of them
// in any window of the given seconds.
func strongHits(hits []sky.Hit, window float64) (float64, int) {
	var strong []float64
	for _, h := range hits {
		if h.Strength >= 0.3 {
			strong = append(strong, h.At)
		}
	}
	most := 0
	for i, t := range strong {
		j, _ := slices.BinarySearch(strong, t+window)
		most = max(most, j-i)
	}
	return float64(len(strong)), most
}

// downbeatSanity describes where the downbeats fall. Entries are the beats
// where the level of the whole sound over the beat after them rises most
// over the beat before them: the top tenth of beats, and at least 3 dB. It
// reports the share of entries that are downbeats, against the share by
// chance, and the mean kick strength on each place in the bar.
func downbeatSanity(p *sky.Plan, blocks []sky.Block) string {
	if len(p.Beats) < 3 {
		return "no beats"
	}
	at := make([]float64, len(blocks))
	db := make([]float64, len(blocks))
	for i, b := range blocks {
		at[i] = b.At
		db[i] = -100.0
		if len(b.Channels) > 0 && !math.IsInf(b.Channels[0].Loudness, -1) {
			db[i] = math.Max(b.Channels[0].Loudness, -100)
		}
	}
	level := func(from, to float64) float64 {
		lo, _ := slices.BinarySearch(at, from)
		hi, _ := slices.BinarySearch(at, to)
		if hi <= lo {
			return -100
		}
		sum := 0.0
		for _, v := range db[lo:hi] {
			sum += v
		}
		return sum / float64(hi-lo)
	}
	type jump struct {
		rise float64
		bar  int
	}
	var jumps []jump
	for i := 1; i+1 < len(p.Beats); i++ {
		beat, ok := p.BeatAt(p.Beats[i])
		if ok {
			jumps = append(jumps, jump{level(p.Beats[i], p.Beats[i+1]) - level(p.Beats[i-1], p.Beats[i]), beat.Bar})
		}
	}
	slices.SortFunc(jumps, func(a, b jump) int { return cmp.Compare(b.rise, a.rise) })
	on, all := 0, 0
	places := make([]int, p.Meter)
	for _, j := range jumps[:len(jumps)/10] {
		if j.rise < 3 {
			break
		}
		all++
		places[j.bar]++
		if j.bar == 0 {
			on++
		}
	}
	kick := make([]float64, p.Meter)
	count := make([]float64, p.Meter)
	for _, h := range p.Hits {
		if beat, ok := p.BeatAt(h.At); ok && h.Kind == sky.HitKick && (beat.Phase < 0.1 || beat.Phase > 0.9) {
			phase, bar := beat.Phase, beat.Bar
			if phase > 0.9 {
				bar = (bar + 1) % p.Meter
			}
			kick[bar] += h.Strength
		}
	}
	for b := range p.Beats {
		if beat, ok := p.BeatAt(p.Beats[b]); ok {
			count[beat.Bar]++
		}
	}
	parts := make([]string, p.Meter)
	for i := range kick {
		parts[i] = fmt.Sprintf("%.2f", kick[i]/math.Max(count[i], 1))
	}
	return fmt.Sprintf("%d of %d entries on a downbeat (%.0f%%, chance %.0f%%), by place in bar %v; kick by place in bar %s",
		on, all, 100*float64(on)/math.Max(float64(all), 1), 100/float64(p.Meter), places, strings.Join(parts, " "))
}

// cueDiff compares a plan's cues with those of the plan built from the
// blocks in path: how many match a cue of the same kind within half a
// second, those that do not, and the largest difference in Tension.
func cueDiff(path string, plan *sky.Plan) string {
	f, err := os.Open(path)
	if err != nil {
		return err.Error()
	}
	defer f.Close() // a read-only file has nothing to flush
	var blocks []mpv.Block
	if err := gob.NewDecoder(f).Decode(&blocks); err != nil {
		return err.Error()
	}
	old := sky.NewPlan(planBlocks(blocks))
	unmatched := func(a, b []sky.Cue) []string {
		var out []string
		for _, c := range a {
			found := false
			for _, d := range b {
				found = found || (d.Kind == c.Kind && math.Abs(d.At-c.At) <= 0.5)
			}
			if !found {
				out = append(out, fmt.Sprintf("%s %.1f", c.Kind, c.At))
			}
		}
		return out
	}
	gone, added := unmatched(old.Cues, plan.Cues), unmatched(plan.Cues, old.Cues)
	tension := 0.0
	for t := 0.0; t < plan.Length; t += 0.5 {
		tension = math.Max(tension, math.Abs(plan.Tension(t)-old.Tension(t)))
	}
	return fmt.Sprintf("%d of %d the same; only before: [%s]; only now: [%s]; Tension differs by at most %.2f",
		len(old.Cues)-len(gone), len(old.Cues), strings.Join(gone, ", "), strings.Join(added, ", "), tension)
}

func gaps(v []float64) []float64 {
	var out []float64
	for i := 1; i < len(v); i++ {
		out = append(out, v[i]-v[i-1])
	}
	return out
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	return s[len(s)/2]
}
