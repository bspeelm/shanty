package sky

import (
	"math"
	"math/rand/v2"
)

// style is how scenes are made and run for one character of music. The four
// profiles below are blended by the character's weights.
type style struct {
	// kinds weighs how often each component is chosen, by name; a name not
	// listed counts as 1.
	kinds map[string]float64
	// sources weighs how often each signal is routed to a parameter.
	sources [sources]float64
	// warps weighs how often each warp is chosen, and glyphs each way of
	// drawing.
	warps  [warpKinds]float64
	glyphs [4]float64
	// glide is how quickly parameters follow their signals, morph how long a
	// scene takes to grow into the next, and life how long a scene lasts, in
	// seconds.
	glide float64
	morph float64
	life  [2]float64
	// keep is the range the feedback's persistence is drawn from.
	keep [2]float64
	// lock is how strongly turning is stepped on the beat rather than
	// continuous, from 0 to 1.
	lock float64
	// sat and contrast shape the palette: how strong its colours are, and
	// how steeply it rises from dark to bright.
	sat, contrast float64
}

// favour is the weight given to a component a profile favours.
const favour = 8

// profiles holds the style of each axis, in the order of Character.axes.
var profiles = [4]style{
	{ // Calm
		kinds:   map[string]float64{"plasma": favour, "reaction": favour, "julia": favour, "flow": favour, "sun": favour, "cells": 2},
		sources: [sources]float64{srcLevel: 3, srcBass: 1, srcAir: 1, srcSlow: 4, srcSlower: 4, srcBalance: 1},
		warps:   [warpKinds]float64{warpTwist: 4, warpRipple: 4, warpRotate: 2, warpZoom: 1, warpKaleido: 1, warpMirror: 1},
		glyphs:  [4]float64{glyphBraille: 5, glyphHalf: 3, glyphShade: 1},
		glide:   0.4, morph: 20, life: [2]float64{70, 150}, keep: [2]float64{0.6, 1},
		sat: 0.55, contrast: 1.4,
	},
	{ // Drive
		kinds:   map[string]float64{"tunnel": favour, "lattice": favour, "rings": favour, "grid": favour, "rule": favour, "spiral": 2},
		sources: [sources]float64{srcKick: 5, srcPhase: 5, srcBar: 4, srcBass: 2, srcLevel: 1, srcSnap: 1},
		warps:   [warpKinds]float64{warpKaleido: 5, warpRotate: 4, warpZoom: 3, warpMirror: 2, warpPolar: 1},
		glyphs:  [4]float64{glyphHalf: 5, glyphBraille: 2, glyphShade: 2, glyphASCII: 1},
		glide:   0.05, morph: 4, life: [2]float64{30, 70}, keep: [2]float64{0.2, 0.6},
		lock: 1, sat: 1, contrast: 0.7,
	},
	{ // Grit
		kinds:   map[string]float64{"fire": favour, "boids": favour, "sparks": favour, "terrain": favour, "stars": favour, "ant": 2},
		sources: [sources]float64{srcSnap: 5, srcTreble: 4, srcAir: 4, srcKick: 3, srcLevel: 2, srcBusy: 2},
		warps:   [warpKinds]float64{warpRipple: 4, warpZoom: 4, warpMirror: 2, warpRotate: 1, warpTwist: 1},
		glyphs:  [4]float64{glyphShade: 4, glyphHalf: 4, glyphASCII: 2, glyphBraille: 1},
		glide:   0.08, morph: 6, life: [2]float64{35, 80}, keep: [2]float64{0.3, 0.7},
		lock: 0.3, sat: 0.9, contrast: 0.9,
	},
	{ // Flow
		kinds:   map[string]float64{"orbits": favour, "lorenz": favour, "epicycles": favour, "chaos": favour, "spiral": favour, "flow": 2},
		sources: [sources]float64{srcLevel: 5, srcBass: 3, srcSlow: 3, srcBalance: 2, srcTreble: 1, srcSlower: 2},
		warps:   [warpKinds]float64{warpPolar: 4, warpMirror: 4, warpTwist: 2, warpRotate: 2, warpKaleido: 1},
		glyphs:  [4]float64{glyphBraille: 5, glyphHalf: 2, glyphShade: 1},
		glide:   0.25, morph: 14, life: [2]float64{60, 130}, keep: [2]float64{0.5, 0.9},
		sat: 0.45, contrast: 1.1,
	},
}

// styleFor blends the four profiles by a character's weights. A character
// not yet measured, with every weight 0, counts as an even blend.
func styleFor(c Character) *style {
	out := &style{kinds: map[string]float64{}}
	weights := c.sharpened().axes()
	for i, w := range weights {
		p := &profiles[i]
		for name, v := range p.kinds {
			out.kinds[name] += w * (v - 1)
		}
		for j := range out.sources {
			out.sources[j] += w * p.sources[j]
		}
		for j := range out.warps {
			out.warps[j] += w * p.warps[j]
		}
		for j := range out.glyphs {
			out.glyphs[j] += w * p.glyphs[j]
		}
		out.glide += w * p.glide
		out.morph += w * p.morph
		out.life[0] += w * p.life[0]
		out.life[1] += w * p.life[1]
		out.keep[0] += w * p.keep[0]
		out.keep[1] += w * p.keep[1]
		out.lock += w * p.lock
		out.sat += w * p.sat
		out.contrast += w * p.contrast
	}
	for name, v := range out.kinds {
		out.kinds[name] = v + 1
	}
	// Every source keeps some chance, so that routings never settle into one
	// pattern.
	for j := range out.sources {
		out.sources[j] += 0.3
	}
	for j := range out.warps {
		out.warps[j] += 0.2
	}
	return out
}

// kindWeight is how often a component is chosen.
func (s *style) kindWeight(name string) float64 {
	if v, ok := s.kinds[name]; ok {
		return v
	}
	return 1
}

// pick returns an index into weights, chosen with chance in proportion to its
// weight.
func pick(rng *rand.Rand, weights []float64) int {
	total := 0.0
	for _, w := range weights {
		total += w
	}
	r := rng.Float64() * total
	for i, w := range weights {
		if r < w {
			return i
		}
		r -= w
	}
	return len(weights) - 1
}

// param draws a parameter whose sources are chosen by the style.
func (s *style) param(rng *rand.Rand) param {
	p := randomParam(rng)
	p.src = [2]int{pick(rng, s.sources[:]), pick(rng, s.sources[:])}
	return p
}

// palette makes a gradient from dark to bright. Its hue family comes from one
// axis, drawn by the character's weights; its saturation and contrast are the
// style's.
func (s *style) palette(rng *rand.Rand, c Character) []RGB {
	axes := c.sharpened().axes()
	var hue, wander float64
	switch pick(rng, axes[:]) {
	case 0: // deep blues, violets and teals
		hue, wander = 0.45+0.3*rng.Float64(), 0.2
	case 1: // anything, wandering widely
		hue, wander = rng.Float64(), 0.6+0.4*rng.Float64()
	case 2: // reds, oranges and hot whites
		hue, wander = 0.92+0.15*rng.Float64(), 0.15
	default: // golds, ambers and soft greens
		hue, wander = 0.05+0.2*rng.Float64(), 0.25
	}
	p := make([]RGB, paletteSize)
	for i := range p {
		t := float64(i) / float64(paletteSize-1)
		light := 0.12 + 0.88*math.Pow(t, s.contrast)
		h := hue + wander*(t-0.5)
		var c [3]uint8
		for ch := range 3 {
			v := 0.5 + 0.5*math.Cos(2*math.Pi*(h+float64(ch)/3))
			v = s.sat*v + (1-s.sat)*0.6
			// The brightest steps whiten, so that the top of every palette
			// reads as light.
			v += (1 - v) * math.Pow(t, 4) * 0.6
			c[ch] = uint8(255 * clamp01(v*light))
		}
		p[i] = RGB{c[0], c[1], c[2]}
	}
	return p
}

// turnPalette returns p with its hue turned by up to a tenth of the way round,
// for a scene that grows out of another and keeps a likeness of its colours.
// Each colour is rotated about the grey axis, which turns hue and keeps
// lightness.
func turnPalette(p []RGB, rng *rand.Rand) []RGB {
	sn, cs := math.Sincos((rng.Float64() - 0.5) * 0.6 * math.Pi)
	k := sn / math.Sqrt(3)
	out := make([]RGB, len(p))
	for i, col := range p {
		r, g, b := float64(col.R), float64(col.G), float64(col.B)
		m := (r + g + b) / 3
		out[i] = RGB{
			byteOf(m + (r-m)*cs + k*(b-g)),
			byteOf(m + (g-m)*cs + k*(r-b)),
			byteOf(m + (b-m)*cs + k*(g-r)),
		}
	}
	return out
}

func byteOf(v float64) uint8 { return uint8(math.Min(math.Max(v, 0), 255) + 0.5) }
