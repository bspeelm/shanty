package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/bspeelm/shanty/internal/sky"
)

// Heard is one reading of the audio, passed to the visualiser.
type Heard sky.Reading

// Foresight is the plan of a track, measured ahead of playback, for the
// visualiser. Track is the title the track is shown under while it plays.
type Foresight struct {
	Track string
	Plan  *sky.Plan
}

// paintSky turns a picture of the visualiser into w by h lines of text, with
// its colours when the picture has them.
func paintSky(p sky.Picture, w, h int) []string {
	lines := make([]string, h)
	for r := range h {
		if r >= len(p.Lines) {
			lines[r] = strings.Repeat(" ", w)
			continue
		}
		var b strings.Builder
		for _, run := range p.Lines[r] {
			switch {
			case run.Fg == sky.Label:
				b.WriteString(faint().Render(run.Text))
			case run.Fg == sky.None && run.Bg == sky.None:
				b.WriteString(run.Text)
			default:
				pre, post := skyColours(p.Palette, run.Fg, run.Bg)
				b.WriteString(pre + run.Text + post)
			}
		}
		lines[r] = b.String()
	}
	return lines
}

// skyEscapes holds the sequences that start and end each pair of colours,
// taken from lipgloss so that they suit the terminal.
var skyEscapes = struct {
	sync.Mutex
	m map[[2]sky.RGB][2]string
}{m: map[[2]sky.RGB][2]string{}}

// skyColours returns the sequences to write before and after text drawn in
// palette colours fg and bg, either of which may be sky.None.
func skyColours(palette []sky.RGB, fg, bg int) (string, string) {
	var key [2]sky.RGB
	style := lipgloss.NewStyle()
	if fg >= 0 && fg < len(palette) {
		key[0] = palette[fg]
		style = style.Foreground(hex(palette[fg]))
	}
	if bg >= 0 && bg < len(palette) {
		key[1] = palette[bg]
		style = style.Background(hex(palette[bg]))
	}
	skyEscapes.Lock()
	defer skyEscapes.Unlock()
	if e, ok := skyEscapes.m[key]; ok {
		return e[0], e[1]
	}
	pre, post, _ := strings.Cut(style.Render("x"), "x")
	if len(skyEscapes.m) >= 4096 {
		clear(skyEscapes.m)
	}
	skyEscapes.m[key] = [2]string{pre, post}
	return pre, post
}

func hex(c sky.RGB) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}
