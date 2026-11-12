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
	"sync"
	"testing"
	"time"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/sky"
)

// The corpus evaluation measures every audio file in SHANTY_CORPUS with the
// real analyser, builds its plan, and compares the plan's beats with the
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
	var wg sync.WaitGroup
	for i, file := range files {
		wg.Add(1)
		go func() {
			defer wg.Done()
			measures[i], failures[i] = measured(t, file, trackName(file))
		}()
	}
	wg.Wait()
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
	fmt.Fprintf(&b, "  kicks on our beats %.2f; downbeats: %s\n", onBeats(plan.Beats, plan.Hits), downbeatSanity(plan, in))
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
	fmt.Fprintf(&b, "  reference %.2f BPM, %d beats, kicks within 70 ms of its beats %.2f, of ours %.2f\n",
		refTempo, len(ref), within(ref, plan.Hits, 0.07), within(plan.Beats, plan.Hits, 0.07))
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

// onBeats is the share of beats with a kick within 30 ms of them.
func onBeats(beats []float64, hits []sky.Hit) float64 { return within(beats, hits, 0.03) }

// within is the share of beats with a kick within d seconds of them.
func within(beats []float64, hits []sky.Hit, d float64) float64 {
	var kicks []float64
	for _, h := range hits {
		if h.Kind == sky.HitKick {
			kicks = append(kicks, h.At)
		}
	}
	if len(beats) == 0 || len(kicks) == 0 {
		return 0
	}
	n := 0
	for _, b := range beats {
		j, _ := slices.BinarySearch(kicks, b-d)
		if j < len(kicks) && kicks[j] <= b+d {
			n++
		}
	}
	return float64(n) / float64(len(beats))
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
		_, _, _, bar, ok := p.BeatAt(p.Beats[i])
		if ok {
			jumps = append(jumps, jump{level(p.Beats[i], p.Beats[i+1]) - level(p.Beats[i-1], p.Beats[i]), bar})
		}
	}
	slices.SortFunc(jumps, func(a, b jump) int { return cmp.Compare(b.rise, a.rise) })
	on, all := 0, 0
	for _, j := range jumps[:len(jumps)/10] {
		if j.rise < 3 {
			break
		}
		all++
		if j.bar == 0 {
			on++
		}
	}
	kick := make([]float64, p.Meter)
	count := make([]float64, p.Meter)
	for _, h := range p.Hits {
		if _, phase, _, bar, ok := p.BeatAt(h.At); ok && h.Kind == sky.HitKick && (phase < 0.1 || phase > 0.9) {
			if phase > 0.9 {
				bar = (bar + 1) % p.Meter
			}
			kick[bar] += h.Strength
		}
	}
	for b := range p.Beats {
		if _, _, _, bar, ok := p.BeatAt(p.Beats[b]); ok {
			count[bar]++
		}
	}
	parts := make([]string, p.Meter)
	for i := range kick {
		parts[i] = fmt.Sprintf("%.2f", kick[i]/math.Max(count[i], 1))
	}
	return fmt.Sprintf("%d of %d entries on a downbeat (%.0f%%, chance %.0f%%); kick by place in bar %s",
		on, all, 100*float64(on)/math.Max(float64(all), 1), 100/float64(p.Meter), strings.Join(parts, " "))
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
