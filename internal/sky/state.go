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
	post    post
	label   float64
	frames  int
}

// The shortest crossfade between two scenes, in seconds, and how long a rare
// scene shows its name.
const (
	fadeMin  = 4
	labelFor = 5
	// firstLife is how long the first scene lasts, in seconds: long enough
	// for the character to settle.
	firstLife = settleAfter + 1
	// driftChange is how far the music's character may move from the one
	// the scene on screen was made for before it is replaced.
	driftChange = 0.5
)

// New returns a visualiser whose scenes are drawn from seed.
func New(seed uint64) *State {
	return &State{seed: seed, sims: rand.New(rand.NewPCG(seed, 1))}
}

// Hear takes one reading of the audio.
func (s *State) Hear(r Reading) { s.lis.ear.hear(r) }

// Frames is how many frames Step has drawn.
func (s *State) Frames() int { return s.frames }

// Character is how the music has sounded over the last several seconds.
func (s *State) Character() Character { return s.lis.char }

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
	if changed {
		s.lis.meter.restart()
	}
	char := s.lis.char
	st := styleFor(char)

	if s.cur == nil {
		s.cur = s.make(sig, st, char)
		s.label = 0
		// The first scene is made before the music's character is known, so
		// it gives way soon to one that is made with it.
		s.life = firstLife
	}
	s.age += dt
	s.label += dt
	if s.next == nil && !s.cur.changing() {
		// Music that has turned out unlike what the scene was made for is a
		// reason to start afresh: the first scene, made before anything was
		// heard; a new track; or a track that changes its nature.
		moved := s.age > 8 && s.lis.meter.settled() && s.cur.char.distance(char) > driftChange
		if sig.Section || changed || moved || s.age > s.life {
			s.waiting += dt
			bars := 4
			if st.lock > 0.5 {
				bars = 16
			}
			onBar := sig.Tempo == 0 || (sig.Beat%bars == 0 && sig.Phase < 0.2)
			if onBar || s.waiting > 8 || changed {
				s.change(sig, st, char, changed || moved)
			}
		}
	}
	if sig.Tempo > 0 && sig.Beat%8 == 0 && sig.Beat != s.moved && sig.Kick > 0.7 {
		s.moved = sig.Beat
		s.cur.mutate(s.sims, st)
	}

	s.f.dt, s.f.sig, s.f.history, s.f.rng = dt, sig, s.lis.history, s.sims
	s.cur.step(&s.f)
	if s.next == nil {
		copy(s.value, s.cur.value)
		copy(s.tone, s.cur.tone)
		s.post.apply(&s.f, s.value, sig, s.lis.shape.lift, dt)
		return
	}
	s.next.step(&s.f)
	s.fade += dt / s.fadeLen
	t := smooth(0, 1, s.fade)
	for i := range s.value {
		s.value[i] = s.cur.value[i]*(1-t) + s.next.value[i]*t
		s.tone[i] = s.cur.tone[i]*(1-t) + s.next.tone[i]*t
	}
	s.post.apply(&s.f, s.value, sig, s.lis.shape.lift, dt)
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
	s.post.resize(f)
}

// change moves on from the current scene. Most often the scene grows into
// the next in place, keeping some of its layers. It starts afresh, fading
// across to a new scene, when fresh is set, when the scene was made for a
// character far from the music's now or before the music was heard, for a
// rare scene, after a rare scene, and one time in eight otherwise, so that
// no line of scenes runs for ever.
func (s *State) change(sig Signals, st *style, c Character, fresh bool) {
	rng := s.rng()
	s.age, s.waiting = 0, 0
	s.life = st.life[0] + (st.life[1]-st.life[0])*rng.Float64()
	rare := s.cur.label == "" && rng.IntN(15) == 0
	unlike := s.cur.char == (Character{}) || s.cur.char.distance(c) > freshChange
	if fresh || unlike || rare || s.cur.label != "" || rng.IntN(8) == 0 {
		if rare {
			s.next = rareScene(rng, st, c)
		} else {
			s.next = s.make(sig, st, c)
		}
		s.fade, s.fadeLen = 0, math.Max(fadeMin, st.morph/2)
		return
	}
	s.cur.evolve(rng, st, c)
	s.remember(s.cur.combo())
}

// rng returns a random source for the next scene, drawn from the seed, the
// track and how many scenes have been made.
func (s *State) rng() *rand.Rand {
	s.made++
	h := fnv.New64a()
	_, _ = h.Write([]byte(s.track)) // writing to a hash cannot fail
	return rand.New(rand.NewPCG(s.seed^h.Sum64(), s.made))
}

// make draws a new scene in the given style, avoiding a combination of
// components seen in the last six scenes.
func (s *State) make(mood Signals, st *style, c Character) *scene {
	rng := s.rng()
	s.age, s.life = 0, st.life[0]+(st.life[1]-st.life[0])*rng.Float64()
	var sc *scene
	for range 6 {
		sc = newScene(rng, st, c, mood)
		if !slices.Contains(s.recent, sc.combo()) {
			break
		}
	}
	s.remember(sc.combo())
	return sc
}

func (s *State) remember(combo string) {
	s.recent = append(s.recent, combo)
	if len(s.recent) > 6 {
		s.recent = s.recent[1:]
	}
}

// Render returns the last frame drawn by Step as a picture w by h. Without
// colour every run is drawn in the terminal's own colour.
func (s *State) Render(colour bool) Picture {
	if s.cur == nil || s.w == 0 {
		return Picture{}
	}
	sig := s.lis.sig
	k := look{glyphs: s.cur.glyphs, next: s.cur.toGlyphs, fade: smooth(0, 1, s.cur.glyphsAt),
		colour: colour, shift: 0.15 * (sig.Treble - sig.Bass)}
	palette := s.cur.palette
	if s.next != nil {
		t := smooth(0, 1, s.fade)
		k.glyphs, k.next, k.fade = s.cur.drawnGlyphs(), s.next.drawnGlyphs(), t
		palette = blendPalettes(s.cur.palette, s.next.palette, t)
	}
	lines := render(s.value, s.tone, s.f.fw, s.f.fh, s.w, s.h, k)
	if s.cur.label != "" && s.label < labelFor && s.next == nil {
		lines[s.h-1] = overlay(lines[s.h-1], " "+s.cur.label+" ", s.w)
	}
	return Picture{Lines: lines, Palette: palette}
}

// blendPalettes returns the palette t of the way from a to b.
func blendPalettes(a, b []RGB, t float64) []RGB {
	out := make([]RGB, len(a))
	mix := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	for i := range a {
		out[i] = RGB{mix(a[i].R, b[i].R), mix(a[i].G, b[i].G), mix(a[i].B, b[i].B)}
	}
	return out
}
