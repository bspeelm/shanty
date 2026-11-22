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
	// hits scales how strongly Kick and Snap move parameters.
	hits float64
	// sat and contrast shape the palette: how strong its colours are, and
	// how steeply it rises from dark to bright.
	sat, contrast float64
}

// favour is the weight given to a component a profile favours.
const favour = 8

// profiles holds the style of each axis, in the order of Character.axes.
var profiles = [4]style{
	{ // Calm
		kinds: map[string]float64{"plasma": favour, "reaction": favour, "julia": favour, "flow": favour, "sun": favour, "cells": 2,
			"night lake": favour, "dawn sea": 1.5, "aurora": 1, "storm": 0.3},
		sources: [sources]float64{srcLevel: 3, srcBass: 1, srcAir: 1, srcSlow: 4, srcSlower: 4, srcBalance: 1, srcTension: 1, srcHush: 2},
		warps:   [warpKinds]float64{warpTwist: 4, warpRipple: 4, warpRotate: 2, warpZoom: 1, warpKaleido: 1, warpMirror: 1},
		glyphs:  [4]float64{glyphBraille: 5, glyphHalf: 3, glyphShade: 1},
		glide:   0.4, morph: 20, life: [2]float64{70, 150}, keep: [2]float64{0.6, 1},
		sat: 0.55, contrast: 1.4, hits: 0.4,
	},
	{ // Drive
		kinds: map[string]float64{"tunnel": favour, "lattice": favour, "rings": favour, "grid": favour, "rule": favour, "spiral": 2,
			"aurora": 1, "night lake": 0.15, "dawn sea": 0.15, "storm": 0.03},
		sources: [sources]float64{srcKick: 5, srcPhase: 5, srcBar: 4, srcBass: 2, srcLevel: 1, srcSnap: 1, srcRelease: 2},
		warps:   [warpKinds]float64{warpKaleido: 5, warpRotate: 4, warpZoom: 3, warpMirror: 2, warpPolar: 1},
		glyphs:  [4]float64{glyphHalf: 5, glyphBraille: 2, glyphShade: 2, glyphASCII: 1},
		glide:   0.05, morph: 4, life: [2]float64{30, 70}, keep: [2]float64{0.2, 0.6},
		lock: 1, sat: 1, contrast: 0.7, hits: 1,
	},
	{ // Grit
		kinds: map[string]float64{"fire": favour, "boids": favour, "sparks": favour, "terrain": favour, "stars": favour, "ant": 2,
			"storm": 3, "night lake": 0.3, "dawn sea": 0.3, "aurora": 0.3},
		sources: [sources]float64{srcSnap: 5, srcTreble: 4, srcAir: 4, srcKick: 3, srcLevel: 2, srcBusy: 2, srcRelease: 2},
		warps:   [warpKinds]float64{warpRipple: 4, warpZoom: 4, warpMirror: 2, warpRotate: 1, warpTwist: 1},
		glyphs:  [4]float64{glyphShade: 4, glyphHalf: 4, glyphASCII: 2, glyphBraille: 1},
		glide:   0.08, morph: 6, life: [2]float64{35, 80}, keep: [2]float64{0.3, 0.7},
		lock: 0.3, sat: 0.9, contrast: 0.9, hits: 1,
	},
	{ // Flow
		kinds: map[string]float64{"orbits": favour, "lorenz": favour, "epicycles": favour, "chaos": favour, "spiral": favour, "flow": 2,
			"dawn sea": favour, "night lake": 2, "storm": 0.3},
		sources: [sources]float64{srcLevel: 5, srcBass: 3, srcSlow: 3, srcBalance: 2, srcTreble: 1, srcSlower: 2, srcTension: 2, srcHush: 1},
		warps:   [warpKinds]float64{warpPolar: 4, warpMirror: 4, warpTwist: 2, warpRotate: 2, warpKaleido: 1},
		glyphs:  [4]float64{glyphBraille: 5, glyphHalf: 2, glyphShade: 1},
		glide:   0.25, morph: 14, life: [2]float64{60, 130}, keep: [2]float64{0.5, 0.9},
		sat: 0.45, contrast: 1.1, hits: 0.5,
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
		out.hits += w * p.hits
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

// steadyStorm is the storm's weight in the Grit profile for music whose
// tempo is sure.
const steadyStorm = 0.3

// steady lowers the storm's weight for music with a steady beat. sure is how
// sure the tempo is, from 0 to 1. As far as it is sure, the Grit profile
// weighs the storm at steadyStorm rather than at its own weight.
func (s *style) steady(c Character, sure float64) {
	grit := c.sharpened().Grit
	s.kinds["storm"] -= grit * clamp01(sure) * (profiles[2].kinds["storm"] - steadyStorm)
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

// The measures of the palettes drawn for each kind of music. Hues are in
// degrees.
const (
	// Calm is keyed from blue to blue-violet, and its hue moves calmArc from
	// the dark end of a ramp to the bright end. Its ramps rise to the
	// brightness of a grey of lightness calmLight and end at that of
	// calmTop, and their darkest step has calmDark of the first. These three
	// are set for each kind of music so that a ramp is as bright, step for
	// step, as the ramps drawn before palettes were made.
	calmFrom, calmTo, calmArc    = 200.0, 280.0, 30.0
	calmLight, calmTop, calmDark = 0.86, 0.904, 0.2
	// Drive is keyed anywhere in the cool family, with the widest arc.
	driveArc, driveArcMost = 40.0, 60.0
	driveLight, driveTop   = 0.75, 0.854
	driveDark              = 0.24
	// Grit is keyed anywhere in the ember family. Its two brightest steps
	// are a yellow of hue gritHot, at gritAccent of its peak chroma, and its
	// peak chroma is gritStrength of what its saturation gives.
	gritArc, gritHot, gritAccent = 30.0, 90.0, 0.5
	gritLight, gritTop, gritDark = 0.66, 0.813, 0.29
	gritStrength                 = 1.15
	// Flow is keyed anywhere in the gold family. Its second member has
	// flowMate of the chroma of its first.
	flowArc                      = 40.0
	flowLight, flowTop, flowDark = 0.62, 0.822, 0.27
	flowStrength                 = 1.1
	flowMate                     = 0.45
	// greyShare of the palettes for driving and for heavy music have greys
	// for one member: for driving music, half of those for the first member
	// and half for the second.
	greyShare = 0.25
	// chromaLeast is the peak chroma of a style with no saturation, and
	// chromaSpan how much a style of full saturation adds.
	chromaLeast, chromaSpan = 0.05, 0.08
	// turnMost is how far a scene that grows out of another may turn the key
	// hue of its palette, in degrees either way.
	turnMost = 54.0
)

// tones draws a palette. Its family comes from one axis, drawn by the
// character's weights; its chroma and contrast are the style's.
func (s *style) tones(rng *rand.Rand, c Character) tones {
	axes := c.sharpened().axes()
	t := tones{peak: chromaLeast + chromaSpan*s.sat, contrast: s.contrast}
	// part returns a second number from 0 to 1 drawn out of one already
	// drawn, so that a family takes as many draws as it always has.
	part := func(v float64) float64 { return math.Mod(v*97, 1) }
	// keyed sets the family, the key hue from a number from 0 to 1, and a
	// second member as far from the first as another such number says.
	keyed := func(f family, from, to, v float64) {
		t.fam, t.key = f, from+(to-from)*v
		t.mate, t.matePeak = f.mateLeast+(f.mateMost-f.mateLeast)*part(v*31), t.peak
	}
	switch pick(rng, axes[:]) {
	case 0:
		keyed(cool, calmFrom, calmTo, rng.Float64())
		t.arc, t.light, t.top, t.dark = calmArc, calmLight, calmTop, calmDark
	case 1:
		v, u := rng.Float64(), rng.Float64()
		keyed(cool, cool.keyFrom, cool.keyTo, v)
		t.arc, t.light, t.top, t.dark = driveArc+(driveArcMost-driveArc)*u, driveLight, driveTop, driveDark
		switch grey := part(v); {
		case grey < greyShare/2:
			t.mate, t.key, t.peak = t.key-greyKey, greyKey, greyPeak
		case grey < greyShare:
			t.matePeak = greyPeak
		case part(u) < 0.5:
			t.accentSteps, t.accentTurn, t.accent = 1+int(2*part(u*7)), 20+10*part(u*13), 0.6*t.peak
		}
	case 2:
		v := rng.Float64()
		t.peak *= gritStrength
		keyed(ember, ember.keyFrom, ember.keyTo, v)
		t.arc, t.light, t.top, t.dark = gritArc, gritLight, gritTop, gritDark
		t.accentSteps, t.accentTurn, t.accent = 2, gritHot-(t.key+gritArc/2), gritAccent*t.peak
		if part(v) < greyShare {
			t.matePeak = greyPeak
		}
	default:
		v := rng.Float64()
		t.peak *= flowStrength
		keyed(gold, gold.keyFrom, gold.keyTo, v)
		t.arc, t.light, t.top, t.dark = flowArc, flowLight, flowTop, flowDark
		t.matePeak = flowMate * t.peak
	}
	t.arc = t.fam.leaning(t.key, t.arc)
	if t.peak > greyPeak {
		t.mate = t.fam.mated(t.key, t.mate)
	}
	return t
}

// palette makes the first member of a palette drawn in the style: a
// gradient from dark to bright.
func (s *style) palette(rng *rand.Rand, c Character) []RGB { return s.tones(rng, c).ramp() }
