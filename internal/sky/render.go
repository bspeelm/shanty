package sky

import (
	"math"
	"math/rand/v2"
)

// RGB is a colour, each channel from 0 to 255.
type RGB struct{ R, G, B uint8 }

// Run is a stretch of a line drawn in one colour. Fg and Bg index the
// picture's palette; None leaves the terminal's own colour, and Label marks
// the name of a rare scene.
type Run struct {
	Text   string
	Fg, Bg int
}

// The colour indexes that are not in a palette.
const (
	None  = -1
	Label = -2
)

// Picture is a rendered frame: lines of runs, and the palette they index.
type Picture struct {
	Lines   [][]Run
	Palette []RGB
}

// glyphs is how a scene turns the field into characters.
type glyphs int

const (
	glyphShade glyphs = iota
	glyphASCII
	glyphBraille
	glyphHalf
)

func pickGlyphs(rng *rand.Rand) glyphs {
	switch r := rng.IntN(9); {
	case r < 3:
		return glyphBraille
	case r < 6:
		return glyphHalf
	case r < 8:
		return glyphShade
	}
	return glyphASCII
}

// The character ramps for shade and ASCII, from empty to full.
var (
	shadeRamp = []rune(" ░▒▓█")
	asciiRamp = []rune(" .:-=+*#%@")
)

// paletteSize is how many steps a palette has. A field value is rounded to
// one of them, so that neighbouring cells share a colour.
const paletteSize = 16

// newPalette makes a gradient from dark to bright whose hue wanders along a
// cosine curve set by rng.
func newPalette(rng *rand.Rand) []RGB {
	var freq, phase [3]float64
	for i := range 3 {
		freq[i] = 0.4 + 0.8*rng.Float64()
		phase[i] = rng.Float64()
	}
	p := make([]RGB, paletteSize)
	for i := range p {
		t := float64(i) / float64(paletteSize-1)
		light := 0.15 + 0.85*math.Pow(t, 0.8)
		var c [3]uint8
		for ch := range 3 {
			v := 0.5 + 0.5*math.Cos(2*math.Pi*(freq[ch]*t+phase[ch]))
			c[ch] = uint8(255 * clamp01((0.25+0.75*v)*light))
		}
		p[i] = RGB{c[0], c[1], c[2]}
	}
	return p
}

// colourAt returns the palette index for a field value and tone, or None for
// a value too faint to draw.
func colourAt(v, tone float64) int {
	if v < 0.04 {
		return None
	}
	i := int(clamp01(v*0.8+tone*0.2) * (paletteSize - 1))
	return min(max(i, 1), paletteSize-1)
}

// render draws value and tone, both fw by fh, as h lines w cells wide. When
// colour is false every run is None and half blocks are drawn as shades.
func render(value, tone []float64, fw, fh, w, h int, g glyphs, colour bool) [][]Run {
	if g == glyphHalf && !colour {
		g = glyphShade
	}
	sample := func(sx, sy float64) (float64, float64) {
		px := min(int(sx*float64(fw)), fw-1)
		py := min(int(sy*float64(fh)), fh-1)
		return value[py*fw+px], tone[py*fw+px]
	}
	lines := make([][]Run, h)
	for r := range h {
		var line runLine
		for c := range w {
			x0, y0 := float64(c)/float64(w), float64(r)/float64(h)
			cw, ch := 1/float64(w), 1/float64(h)
			switch g {
			case glyphBraille:
				bits, peak, ptone := 0, 0.0, 0.0
				for dy := range 4 {
					for dx := range 2 {
						v, t := sample(x0+(float64(dx)+0.5)*cw/2, y0+(float64(dy)+0.5)*ch/4)
						if v > bayer[dy][dx] {
							bits |= brailleBit[dy][dx]
						}
						if v > peak {
							peak, ptone = v, t
						}
					}
				}
				ch := ' '
				if bits != 0 {
					ch = rune(0x2800 + bits)
				}
				line.add(ch, fg(colour, peak, ptone), None)
			case glyphHalf:
				tv, tt := sample(x0+cw/2, y0+ch/4)
				bv, bt := sample(x0+cw/2, y0+ch*3/4)
				top, bot := colourAt(tv, tt), colourAt(bv, bt)
				if top == None && bot == None {
					line.add(' ', None, None)
				} else {
					line.add('▀', top, bot)
				}
			default:
				ramp := shadeRamp
				if g == glyphASCII {
					ramp = asciiRamp
				}
				v1, t1 := sample(x0+cw/2, y0+ch/4)
				v2, _ := sample(x0+cw/2, y0+ch*3/4)
				v := (v1 + v2) / 2
				i := min(int(v*float64(len(ramp))), len(ramp)-1)
				line.add(ramp[i], fg(colour, v, t1), None)
			}
		}
		lines[r] = line.runs
	}
	return lines
}

// The ordered-dither threshold for each dot of a braille cell, and the bit
// that dot sets in the character.
var (
	bayer      = [4][2]float64{{0.06, 0.56}, {0.81, 0.31}, {0.19, 0.69}, {0.94, 0.44}}
	brailleBit = [4][2]int{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
)

func fg(colour bool, v, tone float64) int {
	if !colour {
		return None
	}
	return colourAt(v, tone)
}

// runLine builds a line, joining neighbouring characters of the same colours
// into one run.
type runLine struct{ runs []Run }

func (l *runLine) add(ch rune, fg, bg int) {
	if ch == ' ' && bg == None {
		fg = None
	}
	if n := len(l.runs); n > 0 && l.runs[n-1].Fg == fg && l.runs[n-1].Bg == bg {
		l.runs[n-1].Text += string(ch)
		return
	}
	l.runs = append(l.runs, Run{Text: string(ch), Fg: fg, Bg: bg})
}

// overlay writes text over the right-hand end of a line as a Label run.
func overlay(line []Run, text string, w int) []Run {
	n := len([]rune(text))
	if n >= w {
		return line
	}
	var out []Run
	kept := 0
	for _, r := range line {
		rs := []rune(r.Text)
		if kept+len(rs) > w-n {
			rs = rs[:w-n-kept]
		}
		if len(rs) > 0 {
			out = append(out, Run{Text: string(rs), Fg: r.Fg, Bg: r.Bg})
		}
		kept += len(rs)
		if kept >= w-n {
			break
		}
	}
	return append(out, Run{Text: text, Fg: Label, Bg: None})
}
