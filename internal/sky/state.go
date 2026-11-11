package sky

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"time"
)

// State is the visualiser between frames: what it has heard, the scene on
// screen and the one fading in, and the random source that made them. Hear
// and Step change it in place.
type State struct {
	seed    uint64
	made    uint64
	sims    *rand.Rand
	lis     listener
	cur     *scene
	next    *scene
	fade    float64
	fadeLen float64
	waiting float64
	age     float64
	life    float64
	recent  []string
	track   string
	moved   int
	w, h    int
	f       frame
	value   []float64
	tone    []float64
	label   float64
	frames  int
}

// The bounds on how long a scene stays before the director moves on, and how
// long a crossfade takes, in seconds.
const (
	sceneMin = 45
	sceneMax = 120
	fadeMin  = 4
	fadeMax  = 8
	// labelFor is how long a rare scene shows its name.
	labelFor = 5
)

// New returns a visualiser whose scenes are drawn from seed.
func New(seed uint64) *State {
	return &State{seed: seed, sims: rand.New(rand.NewPCG(seed, 1))}
}

// Hear takes one reading of the audio.
func (s *State) Hear(r Reading) { s.lis.ear.hear(r) }

// Frames is how many frames Step has drawn.
func (s *State) Frames() int { return s.frames }

// Signals is what the music was doing on the last frame.
func (s *State) Signals() Signals { return s.lis.sig }

// Label is the name of the rare scene on screen, or empty.
func (s *State) Label() string {
	if s.cur == nil {
		return ""
	}
	return s.cur.label
}

// Step moves the visualiser on by dt seconds and draws the next frame, due at
// the time now, for a space w cells wide and h tall. track names what is
// playing; a change of track brings a change of scene. A zero now moves what
// is heard on by dt instead of by the clock.
func (s *State) Step(w, h int, dt float64, now time.Time, track string) {
	if w <= 0 || h <= 0 {
		return
	}
	if w != s.w || h != s.h {
		s.resize(w, h)
	}
	s.frames++
	sig := s.lis.step(dt, now)
	changed := track != s.track && s.track != ""
	s.track = track

	if s.cur == nil {
		s.cur = s.make(sig)
		s.label = 0
	}
	s.age += dt
	s.label += dt
	if s.next == nil {
		if sig.Section || changed || s.age > s.life {
			s.waiting += dt
			if sig.Tempo == 0 || (sig.Beat%4 == 0 && sig.Phase < 0.2) || s.waiting > 3 || changed {
				s.next = s.make(sig)
				s.fade, s.fadeLen = 0, fadeMin+(fadeMax-fadeMin)*s.sims.Float64()
				if changed {
					s.fadeLen = fadeMin
				}
			}
		}
	}
	if sig.Tempo > 0 && sig.Beat%8 == 0 && sig.Beat != s.moved && sig.Kick > 0.7 {
		s.moved = sig.Beat
		s.cur.mutate(s.sims)
	}

	s.f.dt, s.f.sig, s.f.history, s.f.rng = dt, sig, s.lis.history, s.sims
	s.cur.step(&s.f)
	if s.next == nil {
		copy(s.value, s.cur.value)
		copy(s.tone, s.cur.tone)
		return
	}
	s.next.step(&s.f)
	s.fade += dt / s.fadeLen
	t := smooth(0, 1, s.fade)
	for i := range s.value {
		s.value[i] = s.cur.value[i]*(1-t) + s.next.value[i]*t
		s.tone[i] = s.cur.tone[i]*(1-t) + s.next.tone[i]*t
	}
	if s.fade >= 1 {
		s.cur, s.next = s.next, nil
		s.age, s.waiting, s.label = 0, 0, 0
	}
}

// resize sets the field for a new size: the braille grid of the space, scaled
// down to at most maxPixels.
func (s *State) resize(w, h int) {
	s.w, s.h = w, h
	fw, fh := float64(w*2), float64(h*4)
	if k := math.Sqrt(maxPixels / (fw * fh)); k < 1 {
		fw, fh = fw*k, fh*k
	}
	f := &s.f
	f.fw, f.fh = max(int(fw), 2), max(int(fh), 2)
	// A terminal cell is about 2.1 times as tall as it is wide.
	f.aspect = 2.1 * float64(h) / float64(w) * float64(f.fw) / float64(f.fh)
	f.cx, f.cy = float64(f.fw-1)/2, float64(f.fh-1)/2
	f.radius = math.Min(float64(f.fw)/2, float64(f.fh)*f.aspect/2)
	s.value, s.tone = make([]float64, f.fw*f.fh), make([]float64, f.fw*f.fh)
}

// make draws a new scene. One in fifteen is a rare scene, never two in a row,
// and a combination of components seen in the last six scenes is drawn again.
func (s *State) make(mood Signals) *scene {
	s.made++
	h := fnv.New64a()
	_, _ = h.Write([]byte(s.track)) // writing to a hash cannot fail
	rng := rand.New(rand.NewPCG(s.seed^h.Sum64(), s.made))
	s.age, s.life = 0, sceneMin+(sceneMax-sceneMin)*rng.Float64()
	if (s.cur == nil || s.cur.label == "") && rng.IntN(15) == 0 {
		return rareScene(rng)
	}
	var sc *scene
	for range 6 {
		sc = newScene(rng, mood)
		if !slices.Contains(s.recent, sc.combo()) {
			break
		}
	}
	s.recent = append(s.recent, sc.combo())
	if len(s.recent) > 6 {
		s.recent = s.recent[1:]
	}
	return sc
}

// Render returns the last frame drawn by Step as a picture w by h. Without
// colour every run is drawn in the terminal's own colour.
func (s *State) Render(colour bool) Picture {
	if s.cur == nil || s.w == 0 {
		return Picture{}
	}
	sc := s.cur
	if s.next != nil && s.fade > 0.5 {
		sc = s.next
	}
	lines := render(s.value, s.tone, s.f.fw, s.f.fh, s.w, s.h, sc.glyphs, colour)
	if sc.label != "" && s.label < labelFor && s.next == nil {
		lines[s.h-1] = overlay(lines[s.h-1], " "+sc.label+" ", s.w)
	}
	return Picture{Lines: lines, Palette: sc.palette}
}
