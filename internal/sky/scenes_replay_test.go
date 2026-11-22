//go:build replay

package sky

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestReplayScenes plays each file named in SKY_CORPUS once for each seed in
// SKY_SEEDS and prints what the director did with it: every time the
// components on screen change, the time, whether it was a cut, the
// components, and the character the scene was made for; and for every ten
// seconds the mean of the character heard, the plan's mood, the character
// drawn for, how fully the beats carry kicks, and the hush, tension and
// release, with the share of frames on which a landscape was on screen and
// the share on which the scene's character was led by calm and flow.
func TestReplayScenes(t *testing.T) {
	files := os.Getenv("SKY_CORPUS")
	if files == "" {
		t.Skip("SKY_CORPUS is not set")
	}
	seeds := []uint64{1}
	if list := os.Getenv("SKY_SEEDS"); list != "" {
		seeds = nil
		for _, f := range strings.Split(list, ",") {
			n, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			seeds = append(seeds, n)
		}
	}
	short := func(c Character) string {
		return fmt.Sprintf("calm %.2f drive %.2f grit %.2f flow %.2f", c.Calm, c.Drive, c.Grit, c.Flow)
	}
	for _, path := range strings.Split(files, ",") {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		p := planOf(t, path)
		m := measureFile(t, path, os.Getenv("SKY_CACHE"))
		fmt.Printf("\n== %s: character of the plan %s\n", name, short(p.Character()))
		for _, seed := range seeds {
			fmt.Printf("-- %s seed %d\n", name, seed)
			s := New(seed)
			last, combo := 0.0, ""
			type sum struct {
				n, land, soft               float64
				live, mood, drawn           [4]float64
				drummed, hush, tense, letGo float64
			}
			var bins []sum
			add := func(to *[4]float64, c Character) {
				for i, v := range c.axes() {
					to[i] += v
				}
			}
			for _, r := range liveReadings(m, 0.27, seed) {
				pos := r.Position.Seconds()
				if pos >= 2 && s.plan() == nil {
					s.Foresee("track", p)
				}
				s.Hear(r)
				s.Step(40, 12, pos-last, r.At, "track")
				last = pos
				now := s.lis.ear.now
				if c := s.cur.combo(); c != combo {
					how := "grew into"
					if s.cutting {
						how = "cut to"
					}
					fmt.Printf("   %6.1f s %-9s %-40s made for %s\n", now, how, c, short(s.cur.char))
					combo = c
				}
				k := int(now / 10)
				if k < 0 {
					continue
				}
				for len(bins) <= k {
					bins = append(bins, sum{})
				}
				b := &bins[k]
				b.n++
				add(&b.live, s.lis.char)
				add(&b.mood, p.mood(now+moodAhead))
				add(&b.drawn, s.character())
				b.drummed += p.drummed(now, now+moodSpan)
				b.hush, b.tense, b.letGo = b.hush+s.sig.Hush, b.tense+s.sig.Tension, b.letGo+s.sig.Release
				for _, l := range s.cur.layers {
					if isLandscape(l.name) && l.goal > 0 {
						b.land++
						break
					}
				}
				if c := s.cur.char; c.Calm+c.Flow > c.Drive+c.Grit {
					b.soft++
				}
			}
			for k, b := range bins {
				if b.n == 0 {
					continue
				}
				ch := func(v [4]float64) string {
					return fmt.Sprintf("%.2f %.2f %.2f %.2f", v[0]/b.n, v[1]/b.n, v[2]/b.n, v[3]/b.n)
				}
				fmt.Printf("   %3d-%3d s: heard %s | mood %s | drawn for %s | drummed %.2f share kicked %.2f | hush %.2f tension %.2f release %.2f | landscape %3.0f%% soft scene %3.0f%%\n",
					10*k, 10*k+10, ch(b.live), ch(b.mood), ch(b.drawn), b.drummed/b.n, p.sectionShare(float64(10*k+5)),
					b.hush/b.n, b.tense/b.n, b.letGo/b.n, 100*b.land/b.n, 100*b.soft/b.n)
			}
		}
	}
}
