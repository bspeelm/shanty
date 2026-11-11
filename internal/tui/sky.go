package tui

import "math"

// skyShades are the five brightness levels sky draws, from empty to full.
const skyShades = " ░▒▓█"

// skyAspect is the height of a character cell divided by its width. sky
// multiplies vertical distances by it so a circle in the frame looks round.
const skyAspect = 2.10

// sky renders one frame of the visualiser: a three-armed logarithmic spiral
// turning in a fixed star field, with a ripple running outward and a finer
// structure drifting the other way. The frame depends only on the size given,
// the elapsed time t in seconds and the sound s. A louder sound spreads the
// spiral further out, and a pulse strengthens the ripple and brightens the
// whole frame. The zero sound draws the pattern from t alone. The result is h
// lines, each w columns wide, and is nil when w or h is not positive.
func sky(w, h int, t float64, s sound) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	shades := []rune(skyShades)
	cx := float64(w-1) / 2
	cy := float64(h-1) / 2
	radius := math.Min(float64(w)/2, float64(h)*skyAspect/2)
	lines := make([]string, h)
	row := make([]rune, w)
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			dx := float64(c) - cx
			dy := (float64(r) - cy) * skyAspect
			rad := math.Hypot(dx, dy)
			th := math.Atan2(dy, dx)

			v := math.Sin(3*th+2.20*math.Log(rad+1)-1.10*t) +
				(0.60+1.20*s.pulse)*math.Sin(rad*0.45-1.70*t) +
				0.40*math.Sin(5*th+0.60*t)

			v = v/2 + 0.5
			amp := 1 / (1 + rad/(radius*(0.62+0.40*s.loud)))
			lvl := math.Min(math.Max(v*amp*(1.80+0.60*s.pulse), 0), 0.999)
			i := int(lvl * 5)
			switch {
			case i > 0:
				row[c] = shades[i]
			case skyStar(c, r):
				row[c] = '·'
			default:
				row[c] = ' '
			}
		}
		lines[r] = string(row)
	}
	return lines
}

// skyStar reports whether the cell at column c and row r holds a star. It
// hashes the coordinates, so the star field is the same on every machine and
// covers about 2% of the cells.
func skyStar(c, r int) bool {
	h := uint32(c)*73856093 ^ uint32(r)*19349663
	h ^= h >> 13
	return h%1000 < 22
}
