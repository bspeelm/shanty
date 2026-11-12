//go:build replay

package sky

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// songMap is what is known of a recording from outside the program: its
// parts, the boundaries between them with their rank, and where each part's
// intensity sits. It is the yardstick a run is scored against and is never
// given to the visualiser.
type songMap struct {
	Parts []struct {
		ID, Name, Mode string
		Start, End     float64
		Tempo          any
		Vibe           float64
	}
	Boundaries []struct {
		T, Cue, Arrival float64
		Rank, Kind      string
		Respond         string
	}
}

// column returns the named column of a CSV file as text, one entry a row.
func column(t *testing.T, path, name string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) == 0 {
		t.Fatalf("%s: %v", path, err)
	}
	col := -1
	for i, h := range rows[0] {
		if h == name {
			col = i
		}
	}
	if col < 0 {
		t.Fatalf("%s has no column %q", path, name)
	}
	out := make([]string, 0, len(rows)-1)
	for _, r := range rows[1:] {
		out = append(out, r[col])
	}
	return out
}

// timesOf returns the rows of a CSV file's time column for which keep reports
// true of the row's value in the by column.
func timesOf(t *testing.T, path, timeCol, by string, keep func(string) bool) []float64 {
	at, key := column(t, path, timeCol), column(t, path, by)
	var out []float64
	for i := range at {
		if v, err := strconv.ParseFloat(at[i], 64); err == nil && keep(key[i]) {
			out = append(out, v)
		}
	}
	sort.Float64s(out)
	return out
}

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

// ranks returns the rank of each value, the smallest first.
func ranks(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	out := make([]float64, len(v))
	for r, i := range idx {
		out[i] = float64(r)
	}
	return out
}

// spearman is the rank correlation of a and b.
func spearman(a, b []float64) float64 {
	ra, rb := ranks(a), ranks(b)
	ma, mb := mean(ra), mean(rb)
	var num, da, db float64
	for i := range ra {
		num += (ra[i] - ma) * (rb[i] - mb)
		da += (ra[i] - ma) * (ra[i] - ma)
		db += (rb[i] - mb) * (rb[i] - mb)
	}
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

// within counts the times in a that have a time in b within tol.
func within(a, b []float64, tol float64) int {
	n := 0
	for _, t := range a {
		i := sort.SearchFloat64s(b, t-tol)
		if i < len(b) && b[i] <= t+tol {
			n++
		}
	}
	return n
}

// TestReplayAgainstAMap plays the file SKY_TRACK through the whole chain and
// scores the run against the map SKY_MAP, a JSON file of parts and ranked
// boundaries made from outside the program. Beside the map, files named
// after it with -beats.csv and -drum-events.csv give the measured beats and
// drum hits. It prints, for the whole track, how the cuts fall against the
// scene changes, the tempo, beats and kicks against those measured, and for
// each part the mood it was drawn for and how much the picture moved.
func TestReplayAgainstAMap(t *testing.T) {
	track, mapPath := os.Getenv("SKY_TRACK"), os.Getenv("SKY_MAP")
	if track == "" || mapPath == "" {
		t.Skip("SKY_TRACK and SKY_MAP are not set")
	}
	raw, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	var m songMap
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	stem := strings.TrimSuffix(mapPath, filepath.Ext(mapPath))
	beats := timesOf(t, stem+"-beats.csv", "best_estimate_s", "on_quarter_beat", func(s string) bool { return s == "1" })
	kicks := timesOf(t, stem+"-drum-events.csv", "time_s", "type", func(s string) bool { return s == "kick" })

	cache := os.Getenv("SKY_CACHE")
	if cache == "" {
		cache = t.TempDir()
	}
	p := planOf(t, track)
	const w, h = 100, 30
	s := New(5)
	type frame struct {
		pos, kick, heavy, change, light float64
		char                            Character
		tempo                           float64
	}
	var run []frame
	var cuts []float64
	var prev []float64
	last := 0.0
	for _, r := range liveReadings(measureFile(t, track, cache), 0.27, 1) {
		pos := r.Position.Seconds()
		if pos >= 2 && s.plan() == nil {
			s.Foresee("track", p)
		}
		was := s.next
		s.Hear(r)
		s.Step(w, h, pos-last, r.At, "track")
		last = pos
		sig := s.Signals()
		f := frame{pos: s.lis.ear.now, kick: sig.Kick, heavy: sig.Heavy, char: s.character(), tempo: sig.Tempo}
		cells := cellsOf(s.Render(true), w, h)
		for i, c := range cells {
			f.light += c / float64(len(cells))
			if prev != nil {
				f.change += math.Abs(c-prev[i]) / float64(len(cells))
			}
		}
		prev = cells
		run = append(run, f)
		if s.next != nil && s.next != was && s.fadeLen == cutFade {
			cuts = append(cuts, s.lis.ear.now+cutFade/2)
		}
	}

	fmt.Printf("plan: %d beats, tempo %.1f, pulse %.2f, character %+v\n", len(p.Beats), p.Tempo, p.Pulse, p.Character())

	// Scene changes.
	var majors []float64
	for _, b := range m.Boundaries {
		if b.Rank == "major" {
			majors = append(majors, b.Arrival)
		}
	}
	fmt.Printf("\nscene changes: %d in the map, %d cuts made\n", len(majors), len(cuts))
	used := map[int]bool{}
	hit := 0
	for _, at := range majors {
		best, d := -1, math.Inf(1)
		for i, c := range cuts {
			if e := math.Abs(c - at); e < d {
				best, d = i, e
			}
		}
		verdict := "MISSED"
		if best >= 0 && d <= 1 {
			used[best] = true
			verdict = fmt.Sprintf("cut %+.0f ms from it", 1000*(cuts[best]-at))
			if d <= 0.034 {
				hit++
				verdict += " (on the frame)"
			}
		}
		fmt.Printf("  %7.2f s: %s\n", at, verdict)
	}
	extra := 0
	for i, c := range cuts {
		if !used[i] {
			extra++
			fmt.Printf("  cut at %7.2f s is at no scene change\n", c)
		}
	}
	fmt.Printf("  on the frame: %d of %d; cuts at no scene change: %d\n", hit, len(majors), extra)

	// Beats and kicks.
	if len(beats) > 0 {
		got := within(p.Beats, beats, 0.035)
		found := within(beats, p.Beats, 0.035)
		fmt.Printf("\nbeats: %d measured, %d planned; %.0f%% of measured beats have a planned beat within 35 ms, %.0f%% of planned beats are on a measured one\n",
			len(beats), len(p.Beats), 100*float64(found)/float64(len(beats)), 100*float64(got)/float64(max(len(p.Beats), 1)))
	}
	var pulses []float64
	for i := 1; i < len(run); i++ {
		if run[i].kick > run[i-1].kick+0.1 {
			pulses = append(pulses, run[i].pos)
		}
	}
	if len(kicks) > 0 {
		fmt.Printf("kicks: %d measured, %d pulses; %.0f%% of measured kicks have a pulse within a frame and a half, %.0f%% of pulses are on a measured kick\n",
			len(kicks), len(pulses), 100*float64(within(kicks, pulses, 0.05))/float64(len(kicks)), 100*float64(within(pulses, kicks, 0.05))/float64(max(len(pulses), 1)))
	}

	// Parts.
	fmt.Printf("\n%-4s %-20s %-9s %5s | %6s %6s | %5s %5s %5s %5s %5s | %7s %6s | %6s\n",
		"id", "part", "mode", "vibe", "tempo", "drawn", "calm", "drive", "grit", "flow", "heavy", "change", "light", "kick")
	var vibes, moved, lit []float64
	for _, part := range m.Parts {
		var n, tempo, heavy, change, light, kick float64
		var c [4]float64
		for i, f := range run {
			if f.pos < part.Start || f.pos >= part.End {
				continue
			}
			n++
			tempo, heavy, change, light = tempo+f.tempo, heavy+f.heavy, change+f.change, light+f.light
			for k, v := range f.char.axes() {
				c[k] += v
			}
			if i > 0 && f.kick > run[i-1].kick+0.1 {
				kick += f.kick
			}
		}
		if n == 0 {
			continue
		}
		want, _ := part.Tempo.(float64)
		fmt.Printf("%-4s %-20s %-9s %5.1f | %6.1f %6.1f | %5.2f %5.2f %5.2f %5.2f %5.2f | %7.4f %6.3f | %6.2f\n",
			part.ID, part.Name, part.Mode, part.Vibe, want, tempo/n, c[0]/n, c[1]/n, c[2]/n, c[3]/n, heavy/n, change/n, light/n, kick/math.Max(float64(len(between(pulses, part.Start, part.End))), 1))
		if part.End-part.Start >= 5 {
			vibes, moved, lit = append(vibes, part.Vibe), append(moved, change/n), append(lit, light/n)
		}
	}
	fmt.Printf("\nintensity against the picture, by rank over parts of 5 s or more: movement %.2f, light %.2f\n", spearman(vibes, moved), spearman(vibes, lit))
}
