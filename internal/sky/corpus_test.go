//go:build replay

package sky

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// block is one block of the measuring filter with its totals restarting on
// every block: the samples it covers and each channel's measures.
type block struct {
	n        float64
	channels [2]Channel
}

// measured is what mpv's filter measures in a file: the blocks the live
// filter sees, and the level of the band below 150 Hz in short steps.
type measured struct {
	rate   int
	blocks []block
	low    []float64
	lowDt  float64
}

// mpvMetadata runs mpv over a file with the given filter chain and returns
// the metadata it prints, one map per frame, with the frame's sample count.
func mpvMetadata(path, chain string) ([]map[string]string, error) {
	cmd := exec.Command("nice", "-n", "19", "mpv", "--no-config", "--no-video", "--ao=null",
		"--ao-null-untimed=yes", "--msg-level=all=no",
		"--af=lavfi=["+chain+",ametadata=mode=print:file=-]", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var frames []map[string]string
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "frame:") {
			frames = append(frames, map[string]string{"frame": line})
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && len(frames) > 0 {
			frames[len(frames)-1][k] = v
		}
	}
	return frames, nil
}

// measureFile returns the file's measures, read from the cache directory
// when it holds them and written to it when it does not.
func measureFile(t *testing.T, path, cache string) measured {
	key := filepath.Join(cache, strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "_")+".measured")
	if m, ok := readMeasured(key); ok {
		return m
	}
	var m measured
	frames, err := mpvMetadata(path, "aformat=sample_fmts=s32,asetnsamples=n=1024:p=0,astats=metadata=1:reset=1:measure_perchannel=RMS_level+RMS_difference+Zero_crossings:measure_overall=Number_of_samples")
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range frames {
		if i == 1 {
			fs := strings.Fields(f["frame"])
			pts, _ := strconv.ParseFloat(strings.TrimPrefix(fs[1], "pts:"), 64)
			sec, _ := strconv.ParseFloat(strings.TrimPrefix(fs[2], "pts_time:"), 64)
			m.rate = int(math.Round(pts / sec))
		}
		var b block
		b.n, _ = strconv.ParseFloat(f["lavfi.astats.Overall.Number_of_samples"], 64)
		for c := range 2 {
			p := "lavfi.astats." + strconv.Itoa(c+1) + "."
			ch := Channel{Loudness: math.Inf(-1)}
			if v, err := strconv.ParseFloat(f[p+"RMS_level"], 64); err == nil {
				ch.Loudness = v
			}
			// A field that does not parse leaves the measure at zero.
			ch.Difference, _ = strconv.ParseFloat(f[p+"RMS_difference"], 64)
			ch.Crossings, _ = strconv.ParseFloat(f[p+"Zero_crossings"], 64)
			b.channels[c] = ch
		}
		m.blocks = append(m.blocks, b)
	}
	n := m.rate / 200
	m.lowDt = float64(n) / float64(m.rate)
	frames, err = mpvMetadata(path, fmt.Sprintf("pan=mono|c0=0.5*c0+0.5*c1,lowpass=f=150,lowpass=f=150,asetnsamples=n=%d:p=0,astats=metadata=1:reset=1:measure_perchannel=RMS_level:measure_overall=none", n))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frames {
		v, err := strconv.ParseFloat(f["lavfi.astats.1.RMS_level"], 64)
		if err != nil {
			v = -120
		}
		m.low = append(m.low, math.Max(v, -120))
	}
	writeMeasured(key, m)
	return m
}

func writeMeasured(path string, m measured) {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %g %d %d\n", m.rate, m.lowDt, len(m.blocks), len(m.low))
	for _, bl := range m.blocks {
		fmt.Fprintf(&b, "%g", bl.n)
		for _, c := range bl.channels {
			fmt.Fprintf(&b, " %g %g %g", c.Loudness, c.Difference, c.Crossings)
		}
		b.WriteByte('\n')
	}
	for _, v := range m.low {
		fmt.Fprintf(&b, "%g\n", v)
	}
	// The cache only saves decoding again; a failure to write it loses nothing.
	_ = os.WriteFile(path, []byte(b.String()), 0o600)
}

func readMeasured(path string) (measured, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return measured{}, false
	}
	var m measured
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var nb, nl int
	if _, err := fmt.Sscan(lines[0], &m.rate, &m.lowDt, &nb, &nl); err != nil {
		return measured{}, false
	}
	for _, line := range lines[1 : 1+nb] {
		f := strings.Fields(line)
		v := make([]float64, len(f))
		for i, s := range f {
			v[i], _ = strconv.ParseFloat(s, 64)
		}
		bl := block{n: v[0]}
		for c := range 2 {
			bl.channels[c] = Channel{Loudness: v[1+3*c], Difference: v[2+3*c], Crossings: v[3+3*c]}
		}
		m.blocks = append(m.blocks, bl)
	}
	for _, line := range lines[1+nb:] {
		v, _ := strconv.ParseFloat(line, 64)
		m.low = append(m.low, v)
	}
	return m, true
}

// liveReadings returns the readings the live filter would give for the
// blocks: totals over epochs of 64 blocks, taken at heard positions spaced
// a thirtieth of a second apart with up to 10 ms of jitter, measured lead
// seconds ahead of what is heard.
func liveReadings(m measured, lead float64, seed uint64) []Reading {
	rng := rand.New(rand.NewPCG(seed, 1))
	rate := float64(m.rate)
	ends := make([]float64, len(m.blocks))
	sum := 0.0
	for i, b := range m.blocks {
		sum += b.n
		ends[i] = sum / rate
	}
	start := time.Unix(1e9, 0)
	var out []Reading
	j := -1
	for pos := 0.0; pos < sum/rate-lead; pos += 1.0/30 + (rng.Float64()*2-1)*0.01 {
		for j+1 < len(ends) && ends[j+1] <= pos+lead {
			j++
		}
		if j < 0 {
			continue
		}
		from := j - j%epochBlocks
		r := Reading{Rate: m.rate, Scale: 1 << 31, Epoch: recordedEpoch,
			Position: time.Duration(pos * float64(time.Second)),
			At:       start.Add(time.Duration(pos * float64(time.Second)))}
		var n float64
		var e, d2, z [2]float64
		for _, b := range m.blocks[from : j+1] {
			n += b.n
			for c, ch := range b.channels {
				e[c] += b.n * power(ch.Loudness)
				d2[c] += b.n * ch.Difference * ch.Difference
				z[c] += ch.Crossings
			}
		}
		r.Samples = n
		for c := range 2 {
			ch := Channel{Loudness: math.Inf(-1), Difference: math.Sqrt(d2[c] / n), Crossings: z[c]}
			if e[c] > 0 {
				ch.Loudness = 10 * math.Log10(e[c]/n)
			}
			r.Channels = append(r.Channels, ch)
		}
		out = append(out, r)
	}
	return out
}

// epochBlocks is the number of blocks in an epoch.
const epochBlocks = recordedEpoch / 1024

// truthKicks returns the onsets in the band below 150 Hz. The level is the
// mean power over the last 20 ms, and its rise at a step is how far it
// climbed over the four steps before. An onset is a step whose rise is the
// largest within 60 ms either side, at least truthLeast dB, and more than
// the median rise over the three seconds around it plus truthDevs median
// deviations, no closer than 120 ms to the last. It is placed where the
// 20 ms began.
func truthKicks(low []float64, dt float64) []float64 {
	n := len(low)
	env := make([]float64, n)
	for i := 3; i < n; i++ {
		p := 0.0
		for _, v := range low[i-3 : i+1] {
			p += power(v) / 4
		}
		env[i] = 10 * math.Log10(p+1e-12)
	}
	flux := make([]float64, n)
	for i := 7; i < n; i++ {
		flux[i] = math.Max(0, env[i]-env[i-4])
	}
	half := int(1.5 / dt)
	near := int(0.06 / dt)
	var out []float64
	window := make([]float64, 0, 2*half+1)
	for i := range n {
		if flux[i] < truthLeast {
			continue
		}
		peak := true
		for k := max(0, i-near); k <= min(n-1, i+near); k++ {
			if flux[k] > flux[i] || (flux[k] == flux[i] && k < i) {
				peak = false
				break
			}
		}
		if !peak {
			continue
		}
		window = append(window[:0], flux[max(0, i-half):min(n, i+half+1)]...)
		sort.Float64s(window)
		med := window[len(window)/2]
		for k := range window {
			window[k] = math.Abs(window[k] - med)
		}
		sort.Float64s(window)
		mad := window[len(window)/2]
		if flux[i] < med+truthDevs*mad {
			continue
		}
		at := float64(i-3) * dt
		if len(out) > 0 && at-out[len(out)-1] < 0.12 {
			continue
		}
		out = append(out, at)
	}
	return out
}

// The least rise of an onset in dB, and how many median deviations above
// the median rise it must be.
const (
	truthLeast = 6
	truthDevs  = 4
)

// readBeats reads one time in seconds per line.
func readBeats(path string) []float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []float64
	for _, f := range strings.Fields(string(data)) {
		if v, err := strconv.ParseFloat(f, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// matched counts the times in a that have a time in b within tol.
func matched(a, b []float64, tol float64) int {
	n := 0
	for _, x := range a {
		i := sort.SearchFloat64s(b, x-tol)
		if i < len(b) && b[i] <= x+tol {
			n++
		}
	}
	return n
}

// refTempo returns the tempo of the median gap between reference beats, or 0
// when there are too few.
func refTempo(beats []float64) float64 {
	if len(beats) < 8 {
		return 0
	}
	gaps := make([]float64, 0, len(beats))
	for i := 1; i < len(beats); i++ {
		gaps = append(gaps, beats[i]-beats[i-1])
	}
	slices.Sort(gaps)
	return 60 / gaps[len(gaps)/2]
}

// heardTrack is what a listener did over one track of the corpus.
type heardTrack struct {
	secs float64
	// kicks holds the times of the kicks: frames where Kick reaches 0.5 or
	// more and rises, at least 0.1 s after the last.
	kicks []float64
	// phases holds the phase at each frame with a tempo.
	phases                                []struct{ at, phase float64 }
	tempos                                map[int]int
	frames, withTempo, right, snaps, soft int
	flashes                               int
	busy, moved                           float64
	lockAt                                float64
	last                                  Character
}

// hearTrack plays a file's readings through a listener and counts what it
// heard, with a tempo within 3% of ref counted as right.
func hearTrack(m measured, ref float64) heardTrack {
	readings := liveReadings(m, earLead, 5)
	h := heardTrack{tempos: map[int]int{}, lockAt: -1}
	var l listener
	var prev Signals
	for i, r := range readings {
		dt := 1.0 / 30
		if i > 0 {
			dt = r.At.Sub(readings[i-1].At).Seconds()
		}
		l.ear.hear(r)
		s := l.step(dt, r.At)
		heard := r.Position.Seconds()
		h.frames++
		h.busy += s.Busy
		h.moved += math.Abs(s.Level - prev.Level)
		if s.Kick >= 0.5 && s.Kick > prev.Kick && (len(h.kicks) == 0 || heard-h.kicks[len(h.kicks)-1] > 0.1) {
			h.kicks = append(h.kicks, heard)
		}
		if math.Max(s.Kick, s.Snap) > 0.25 && math.Max(prev.Kick, prev.Snap) <= 0.25 {
			h.soft++
		}
		if s.Snap > 0.5 && prev.Snap <= 0.5 {
			h.snaps++
		}
		if s.Tempo == 0 && s.Kick > 0.6 && prev.Kick <= 0.6 {
			h.flashes++
		}
		if s.Tempo > 0 {
			h.withTempo++
			h.tempos[int(math.Round(s.Tempo))]++
			h.phases = append(h.phases, struct{ at, phase float64 }{heard, s.Phase})
			if ref > 0 && math.Abs(s.Tempo/ref-1) < 0.03 {
				h.right++
				if h.lockAt < 0 {
					h.lockAt = heard
				}
			}
		}
		prev = s
	}
	h.secs = readings[len(readings)-1].Position.Seconds()
	h.last = l.char
	return h
}

// TestReplayACorpus plays the audio files in SKY_CORPUS, a list separated by
// commas, through a listener as the live filter would measure them, and
// prints for each: the onsets below 150 Hz and the kicks heard, the share of
// those onsets with a kick within 50 ms and of the kicks with an onset, and
// the median time from onset to kick; the hits above 0.25 and the snaps a
// minute; the share of frames with a tempo and with the tempo of the
// reference beats in SKY_REFS, and when that tempo was first reported; the
// mean phase at the onsets and how closely it gathers; the kicks above 0.6
// without a tempo a minute; the mean Busy, how far the level moves a second,
// the character, and the tempos reported most. Measures are cached in
// SKY_CACHE.
func TestReplayACorpus(t *testing.T) {
	files := os.Getenv("SKY_CORPUS")
	if files == "" {
		t.Skip("SKY_CORPUS is not set")
	}
	cache := os.Getenv("SKY_CACHE")
	if cache == "" {
		cache = t.TempDir()
	}
	refs := os.Getenv("SKY_REFS")
	fmt.Printf("%-10s %4s %5s %5s %5s %5s %4s %5s %5s %5s %5s %5s %5s %6s %5s %5s %4s %4s  %-23s %s\n",
		"track", "secs", "truth", "kicks", "recal", "prec", "lag", "hit/m", "snp/m", "tmp%", "right", "ref", "lock", "phase", "gath", "fl/m", "busy", "move", "character", "tempos")
	for _, path := range strings.Split(files, ",") {
		m := measureFile(t, path, cache)
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		truth := truthKicks(m.low, m.lowDt)
		ref := 0.0
		if refs != "" {
			ref = refTempo(readBeats(filepath.Join(refs, name+".beats")))
		}
		h := hearTrack(m, ref)
		var lags []float64
		for _, k := range h.kicks {
			i := sort.SearchFloat64s(truth, k-0.05)
			if i < len(truth) && truth[i] <= k+0.05 {
				lags = append(lags, k-truth[i])
			}
		}
		lag := 0.0
		if len(lags) > 0 {
			slices.Sort(lags)
			lag = lags[len(lags)/2]
		}
		var x, y float64
		n := 0
		for _, at := range truth {
			i := sort.Search(len(h.phases), func(i int) bool { return h.phases[i].at >= at })
			if i < len(h.phases) && h.phases[i].at-at < 0.04 {
				sn, cs := math.Sincos(2 * math.Pi * h.phases[i].phase)
				x, y = x+cs, y+sn
				n++
			}
		}
		gather := 0.0
		if n > 0 {
			gather = math.Hypot(x, y) / float64(n)
		}
		var tempos []int
		for k := range h.tempos {
			tempos = append(tempos, k)
		}
		sort.Slice(tempos, func(i, j int) bool { return h.tempos[tempos[i]] > h.tempos[tempos[j]] })
		var common []string
		for _, k := range tempos[:min(3, len(tempos))] {
			common = append(common, fmt.Sprintf("%d:%.0f%%", k, 100*frac(h.tempos[k], h.frames)))
		}
		mins := h.secs / 60
		c := h.last
		fmt.Printf("%-10s %4.0f %5d %5d %5.2f %5.2f %4.0f %5.0f %5.0f %5.0f %5.0f %5.1f %5.1f %+6.2f %5.2f %5.1f %4.2f %4.2f  c%.2f d%.2f g%.2f f%.2f  %s\n",
			name, h.secs, len(truth), len(h.kicks), frac(matched(truth, h.kicks, 0.05), len(truth)),
			frac(matched(h.kicks, truth, 0.05), len(h.kicks)), lag*1000, float64(h.soft)/mins,
			float64(h.snaps)/mins, 100*frac(h.withTempo, h.frames), 100*frac(h.right, h.frames), ref, h.lockAt,
			math.Atan2(y, x)/(2*math.Pi), gather, float64(h.flashes)/mins, h.busy/float64(h.frames),
			h.moved/h.secs, c.Calm, c.Drive, c.Grit, c.Flow, strings.Join(common, " "))
	}
}

func frac(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
