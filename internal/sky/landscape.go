package sky

import (
	"math"
	"math/rand/v2"
)

// toner is a component that gives each pixel a tone of its own. tones returns
// the tone of every pixel of the last frame drawn, from 0 to 1, or nil before
// the first.
type toner interface {
	tones() []float64
}

// weather is what falls, rises or drifts across a landscape.
type weather int

const (
	weatherNone weather = iota
	weatherSnow
	weatherRain
	weatherEmbers
	weatherFireflies
	weatherCloud
	weathers
)

// setting is a kind of landscape: the chances of the elements one is made
// with.
type setting struct {
	// tod is the range the time of day is drawn from, where 0 is night, 0.3
	// dawn or dusk, 0.6 golden hour and 1 full day.
	tod [2]float64
	// water, body and aurora are the chances of water below the horizon, of
	// a sun or a moon, and of aurora curtains.
	water, body, aurora float64
	// ridges is the fewest and the most ridge lines.
	ridges [2]int
	// weather weighs each kind of weather.
	weather [weathers]float64
	// cloud is the cloud cover at rest, from 0 to 1.
	cloud float64
	// sea draws ridges only where they rise high, as headlands and islands.
	// storm flashes lightning on a snap. pulse brightens the aurora on the
	// beat.
	sea, storm, pulse bool
}

// settings holds each kind of landscape by name. Calm favours the night lake,
// Flow the dawn sea, Grit the storm and Drive the aurora.
var settings = map[string]setting{
	"night lake": {
		tod: [2]float64{0, 0.08}, water: 0.9, body: 0.9, aurora: 0.1, ridges: [2]int{1, 3},
		weather: [weathers]float64{weatherNone: 3, weatherSnow: 2, weatherFireflies: 2, weatherCloud: 1},
		cloud:   0.1,
	},
	"dawn sea": {
		tod: [2]float64{0.25, 0.65}, water: 1, body: 0.9, ridges: [2]int{0, 2},
		weather: [weathers]float64{weatherNone: 3, weatherCloud: 3, weatherFireflies: 1},
		cloud:   0.2, sea: true,
	},
	"storm": {
		tod: [2]float64{0.3, 0.6}, water: 0.5, body: 0.4, ridges: [2]int{1, 3},
		weather: [weathers]float64{weatherRain: 5, weatherSnow: 1, weatherEmbers: 1, weatherCloud: 1},
		cloud:   0.6, storm: true,
	},
	"aurora": {
		tod: [2]float64{0, 0.05}, water: 0.5, body: 0.3, aurora: 1, ridges: [2]int{1, 2},
		weather: [weathers]float64{weatherNone: 3, weatherSnow: 2},
		pulse:   true,
	},
}

// isLandscape reports whether the component of the given name is a
// landscape.
func isLandscape(name string) bool {
	_, ok := settings[name]
	return ok
}

// Whether a landscape has a sun, a moon or neither.
const (
	bodyNone = iota
	bodySun
	bodyMoon
)

// The tones of a landscape's regions.
const (
	toneSky   = 0.85
	toneCloud = 0.65
	toneWater = 0.55
	toneBody  = 1
)

// reflectivity is how bright water's reflection is beside what it reflects.
const reflectivity = 0.62

// ridgeLen is how many heights a ridge profile holds. A profile wraps round,
// so that a drifting ridge never ends.
const ridgeLen = 1024

// The size of the cloud texture, which wraps round across the sky.
const (
	cloudW = 64
	cloudH = 16
)

// depth is how a ridge at one distance is drawn: how high it stands above
// the horizon and how much it rises and falls, as shares of the field's
// height; how much of its profile spans the width; how fast it drifts at full
// wind, in profile steps a second; how much of the sky's light it takes on;
// and its tone.
type depth struct {
	base, rise, span, speed, haze, tone float64
}

// depths are the far, middle and near ridges.
var depths = [3]depth{
	{0.05, 0.10, 0.9, 3, 0.65, 0.35},
	{0.02, 0.14, 0.6, 7, 0.35, 0.22},
	{-0.03, 0.2, 0.4, 15, 0.08, 0.1},
}

// ridge is one line of hills.
type ridge struct {
	depth
	profile []float64
	off     float64
}

// at returns the ridge's height at profile position u, from 0 to 1.
func (r *ridge) at(u float64) float64 {
	u -= math.Floor(u/ridgeLen) * ridgeLen
	i := int(u)
	if i < 0 || i >= ridgeLen {
		return 0
	}
	a, b := r.profile[i], r.profile[(i+1)%ridgeLen]
	return a + (b-a)*(u-float64(i))
}

// mote is one flake, drop, ember or firefly: its position as shares of the
// field's width and height, the density below which it shows, a phase and a
// speed, both from 0 to 1.
type mote struct{ x, y, u, ph, sp float64 }

// landscape is a composed scene of sky, land and water, drawn in depth order:
// sky, sun or moon, aurora and stars, cloud, ridges from far to near, water
// reflecting all of them, and weather over everything. It draws in the
// field's own pixels and is not bent by the scene's warps. Its elements are
// chosen when it is made; the music moves its light and weather.
type landscape struct {
	set setting
	rng *rand.Rand
	// water, aurora and weather are the elements chosen when it was made.
	// body is the sun, the moon or neither, at bodyX across the width and
	// bodyY of the way up from the horizon to the top.
	water   bool
	aurora  bool
	weather weather
	body    int
	bodyX   float64
	bodyY   float64
	// horizon is the horizon's height from the top, as a share of the
	// field's height, and tod0 the time of day the landscape rests at.
	horizon float64
	tod0    float64
	ridges  []ridge
	clouds  []float64
	// stars holds each star's position as shares of the width and of the
	// sky's height, its brightness and its phase.
	stars [][4]float64
	fall  []mote
	rise  []mote
	bolt  [][2]float64

	ready bool
	t     float64
	// tension, release and hush follow the signals of the same names. tod
	// is the time of day, lit how strongly the light is lit, cover the
	// cloud cover, wind how fast cloud and ridges drift, ripple how far the
	// water moves in pixels, and pulse the aurora's beat.
	tension, release, hush float64
	tod, lit, cover, wind  float64
	ripple, pulse          float64
	drift                  float64
	flash, wait            float64
	// aim is where fireflies gather across the width, and aimAt how long
	// until it is looked for again.
	aim, aimAt float64
	// mx and r are the sun or moon's place across the field and its
	// radius, in pixels, and path how bright its light lies on moving water.
	mx, r, path float64

	// tone is the tone of each pixel, shade the cloud over each pixel of
	// the sky, rows the sky's brightness in each row, and aur the aurora's
	// brightness, lower edge and reach in each column.
	tone  []float64
	shade []float64
	rows  []float64
	aur   []float64
}

// newLandscape makes a landscape of a setting, choosing its elements with r.
func newLandscape(r *rand.Rand, set setting) *landscape {
	rng := rand.New(rand.NewPCG(r.Uint64(), r.Uint64()))
	l := &landscape{set: set, rng: rng, aim: 0.5}
	l.water = rng.Float64() < set.water
	l.tod0 = set.tod[0] + (set.tod[1]-set.tod[0])*rng.Float64()
	if rng.Float64() < set.body {
		l.body = bodySun
		l.bodyY = 0.02 + 0.3*rng.Float64()
		if l.tod0 < 0.2 {
			l.body = bodyMoon
			l.bodyY = 0.45 + 0.35*rng.Float64()
		}
	}
	l.bodyX = 0.2 + 0.6*rng.Float64()
	l.aurora = rng.Float64() < set.aurora
	l.weather = weather(pick(rng, set.weather[:]))
	l.horizon = 0.72 + 0.08*rng.Float64()
	if l.water {
		l.horizon = 0.55 + 0.1*rng.Float64()
	}
	n := set.ridges[0] + rng.IntN(set.ridges[1]-set.ridges[0]+1)
	var at []int
	switch n {
	case 1:
		at = []int{1 + rng.IntN(2)}
	case 2:
		at = []int{0, 2}
	case 3:
		at = []int{0, 1, 2}
	}
	for _, d := range at {
		l.ridges = append(l.ridges, ridge{depth: depths[d], profile: ridgeProfile(rng.IntN(1 << 20)), off: rng.Float64() * ridgeLen})
	}
	l.clouds = cloudTexture(rng.IntN(1 << 20))
	for range 200 {
		b := rng.Float64()
		l.stars = append(l.stars, [4]float64{rng.Float64(), rng.Float64() * 0.97, 0.45 + 0.55*b*b, rng.Float64()})
	}
	for range 220 {
		l.fall = append(l.fall, mote{rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64()})
	}
	for range 70 {
		l.rise = append(l.rise, mote{rng.Float64(), l.horizon + 0.15*(rng.Float64()-0.3), rng.Float64(), rng.Float64(), rng.Float64()})
	}
	return l
}

// ridgeProfile returns ridgeLen heights from 0 to 1 that wrap round: five
// octaves of smoothed noise drawn from seed.
func ridgeProfile(seed int) []float64 {
	p := make([]float64, ridgeLen)
	amp := 1.0
	for o, cells := 0, 4; cells <= 64; o, cells = o+1, cells*2 {
		for i := range p {
			u := float64(i) * float64(cells) / ridgeLen
			k := int(u)
			fr := u - float64(k)
			a, b := hash(seed+o*977, k%cells), hash(seed+o*977, (k+1)%cells)
			p[i] += amp * (a + (b-a)*fr*fr*(3-2*fr))
		}
		amp *= 0.5
	}
	normalise(p)
	return p
}

// cloudTexture returns cloudW by cloudH values from 0 to 1 that wrap round:
// three octaves of smoothed noise drawn from seed.
func cloudTexture(seed int) []float64 {
	t := make([]float64, cloudW*cloudH)
	amp := 1.0
	for o, cells := 0, 8; cells <= 32; o, cells = o+1, cells*2 {
		rows := max(cells/4, 2)
		for y := range cloudH {
			v := float64(y) * float64(rows) / cloudH
			ky := int(v)
			fy := v - float64(ky)
			fy = fy * fy * (3 - 2*fy)
			for x := range cloudW {
				u := float64(x) * float64(cells) / cloudW
				kx := int(u)
				fx := u - float64(kx)
				fx = fx * fx * (3 - 2*fx)
				s := seed + o*7919
				at := func(i, j int) float64 { return hash(s+(j%rows)*131, i%cells) }
				top := at(kx, ky) + (at(kx+1, ky)-at(kx, ky))*fx
				bot := at(kx, ky+1) + (at(kx+1, ky+1)-at(kx, ky+1))*fx
				t[y*cloudW+x] += amp * (top + (bot-top)*fy)
			}
		}
		amp *= 0.5
	}
	normalise(t)
	return t
}

// normalise scales v to run from 0 to 1.
func normalise(v []float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range v {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	if hi-lo < 1e-9 {
		return
	}
	for i := range v {
		v[i] = (v[i] - lo) / (hi - lo)
	}
}

// cloudAt samples the cloud texture at (u, v) in texels, wrapping across and
// holding at the top and bottom.
func (l *landscape) cloudAt(u, v float64) float64 {
	u -= math.Floor(u/cloudW) * cloudW
	v = math.Min(math.Max(v, 0), cloudH-1)
	x0, y0 := int(u), int(v)
	if x0 < 0 || x0 >= cloudW {
		return 0
	}
	x1, y1 := (x0+1)%cloudW, min(y0+1, cloudH-1)
	fx, fy := u-float64(x0), v-float64(y0)
	c := l.clouds
	top := c[y0*cloudW+x0] + (c[y0*cloudW+x1]-c[y0*cloudW+x0])*fx
	bot := c[y1*cloudW+x0] + (c[y1*cloudW+x1]-c[y1*cloudW+x0])*fx
	return top + (bot-top)*fy
}

func (l *landscape) tones() []float64 { return l.tone }

func (l *landscape) draw(f *frame, v [4]float64, clock float64, buf []float64) {
	n := f.fw * f.fh
	if len(l.tone) != n {
		l.tone, l.shade = make([]float64, n), make([]float64, n)
	}
	if len(l.rows) != f.fh {
		l.rows = make([]float64, f.fh)
	}
	if len(l.aur) != 3*f.fw {
		l.aur = make([]float64, 3*f.fw)
	}
	l.t += f.dt
	l.feel(f)
	hy := min(max(int(l.horizon*float64(f.fh)+0.5), 1), f.fh-1)
	l.sky(f, buf, hy)
	l.starlight(f, buf, hy)
	l.hills(f, buf, hy)
	if l.water {
		l.reflect(f, buf, hy)
	} else {
		l.ground(f, buf, hy)
	}
	l.motes(f, buf)
	if l.flash > 0.25 && len(l.bolt) > 1 {
		for i := 1; i < len(l.bolt); i++ {
			a, b := l.bolt[i-1], l.bolt[i]
			l.line(f, buf, a[0]*float64(f.fw), a[1]*float64(f.fh), b[0]*float64(f.fw), b[1]*float64(f.fh), l.flash)
		}
	}
	for i := range buf {
		buf[i] = clamp01(buf[i])
		l.tone[i] = clamp01(l.tone[i])
	}
}

// feel moves the landscape's light and weather towards what the music asks
// for. Tension gathers cloud, raises the wind, dims the light and moves the
// water; a release breaks the light through; a hush stills the water and
// settles the light towards night.
func (l *landscape) feel(f *frame) {
	sig, dt := f.sig, f.dt
	todAt := func() float64 {
		return clamp01(l.tod0 + 0.06*wave(l.t/150) - 0.35*l.hush + 0.12*l.release)
	}
	litAt := func() float64 { return (1 - 0.4*l.tension) * (1 + 0.4*l.release) }
	coverAt := func() float64 {
		c := l.set.cloud
		if l.weather == weatherCloud {
			c += 0.35
		}
		return clamp01(c + 0.55*l.tension - 0.6*l.release)
	}
	windAt := func() float64 {
		w := 0.12 + 0.9*l.tension
		if l.set.storm {
			w += 0.3
		}
		return w
	}
	rippleAt := func() float64 {
		return (3*sig.Bass + 2*l.tension) * (1 - 0.9*l.hush) * float64(f.fw) / 200
	}
	pulseAt := 0.0
	if l.set.pulse {
		pulseAt = sig.Kick
		if sig.Tempo > 0 {
			p := 1 - sig.Phase
			pulseAt = sig.blendBeat(p*p*p*p, sig.Kick)
		}
	}
	if !l.ready {
		l.tension, l.release, l.hush = sig.Tension, sig.Release, sig.Hush
		l.tod, l.lit, l.cover, l.wind, l.ripple = todAt(), litAt(), coverAt(), windAt(), rippleAt()
		l.ready = true
	}
	l.tension = glide(l.tension, sig.Tension, dt, 1)
	l.release = glide(l.release, sig.Release, dt, 0.3)
	l.hush = glide(l.hush, sig.Hush, dt, 1.5)
	l.tod = glide(l.tod, todAt(), dt, 5)
	l.lit = glide(l.lit, litAt(), dt, 1.2)
	l.cover = glide(l.cover, coverAt(), dt, 3)
	l.wind = glide(l.wind, windAt(), dt, 2)
	l.ripple = glide(l.ripple, rippleAt(), dt, 0.3)
	l.pulse = glide(l.pulse, pulseAt, dt, 0.05)
	l.drift += dt * l.wind * 1.5
	for i := range l.ridges {
		l.ridges[i].off += dt * l.wind * l.ridges[i].speed
	}

	l.flash *= math.Exp(-dt / 0.15)
	l.wait -= dt
	if l.set.storm && sig.Snap > 0.6 && l.wait <= 0 {
		l.flash, l.wait = 1, 1.2+l.rng.Float64()
		x, y := 0.15+0.7*l.rng.Float64(), 0.05
		l.bolt = append(l.bolt[:0], [2]float64{x, y})
		for y < l.horizon-0.08 {
			y += 0.03 + 0.03*l.rng.Float64()
			x += (l.rng.Float64() - 0.5) * 0.05
			l.bolt = append(l.bolt, [2]float64{x, y})
		}
	}

	l.aimAt -= dt
	if l.aimAt <= 0 && f.couple > 0 && len(f.env) == f.fw*f.fh {
		bx, _ := brightest(f)
		l.aim = clamp01((bx*f.radius + f.cx) / float64(f.fw))
		l.aimAt = 1
	}
}

// sky draws the rows above the horizon: the gradient of the time of day, the
// sun or moon, the aurora and the cloud.
func (l *landscape) sky(f *frame, buf []float64, hy int) {
	fw := f.fw
	tod := l.tod
	glow := clamp01(1 - math.Abs(tod-0.45)/0.3)
	zen := (0.02 + 0.38*tod) * l.lit
	hor := (0.1 + 0.32*tod + 0.3*glow) * l.lit
	for y := range hy {
		fr := (float64(y) + 0.5) / float64(hy)
		l.rows[y] = zen + (hor-zen)*fr*fr + 0.5*l.flash
	}

	var mx, my, r, r2, bodyV, glowA, glowK float64
	if l.body != bodyNone {
		mx = l.bodyX * float64(fw)
		my = float64(hy) * (1 - l.bodyY)
		r = 0.05 * float64(f.fh) * f.aspect
		bodyV, glowA, glowK = 0.95, 0.18*(1+l.release), 1/(r*r*5)
		if l.body == bodySun {
			r *= 1.5
			bodyV = 1
			glowA = (0.3 + 0.3*glow) * (1 + 0.6*l.release) * l.lit
			glowK = 1 / (r * r * 10)
		}
		r2 = r * r
	}
	l.mx, l.r, l.path = mx, r, 0
	if l.body == bodyMoon {
		l.path = 0.35 * (1 + l.release)
	} else if l.body == bodySun {
		l.path = (0.3 + 0.4*glow) * (1 + 0.6*l.release) * l.lit
	}

	aurora := 0.0
	if l.aurora {
		aurora = 0.9 * clamp01(1-1.8*tod) * (1 + 0.6*l.release) * (1 + 0.5*l.pulse) * l.lit
		for x := range fw {
			u := float64(x) / float64(fw)
			i := 0.55 + 0.45*wave(u*1.1+l.t*0.013)
			i *= 0.75 + 0.25*wave(u*2.9-l.t*0.021+0.3)
			ray := 0.5 + 0.5*wave(u*23+0.4*wave(u*3.1+l.t*0.03))
			i *= 0.35 + 0.65*ray*ray
			l.aur[3*x] = aurora * i
			l.aur[3*x+1] = float64(hy) * (0.45 + 0.12*wave(u*0.8+l.t*0.011) + 0.05*wave(u*2.2-l.t*0.017))
			l.aur[3*x+2] = 1 / (float64(hy) * (0.3 + 0.12*wave(u*1.6+l.t*0.009)))
		}
	}

	cover := l.cover
	cloudy := cover > 0.02
	cloudV := (0.1 + 0.45*tod) * l.lit * (1.2 - cover)
	if l.set.storm {
		cloudV *= 0.4
	}
	cloudV += 0.6 * l.flash
	tsky := toneSky - 0.2*l.tension
	cu, cv := 0.8*cloudW/float64(fw), 0.9*cloudH/float64(hy)
	for y := range hy {
		fr := (float64(y) + 0.5) / float64(hy)
		dy := (float64(y) + 0.5 - my) * f.aspect
		dy2 := dy * dy
		thin := 1 - 0.6*fr*fr
		for x := range fw {
			i := y*fw + x
			v := l.rows[y]
			t := tsky + 0.1*fr
			g := 0.0
			if l.body != bodyNone {
				dx := float64(x) + 0.5 - mx
				d2 := dx*dx + dy2
				g = glowA / (1 + d2*glowK)
				v += g
				if c := clamp01((r2-d2)/(2*r) + 0.5); c > 0 {
					v += (bodyV - v) * c
					t += (toneBody - t) * c
				}
			}
			if aurora > 0 {
				a := l.aur[3*x]
				if d := l.aur[3*x+1] - float64(y); d >= 0 {
					a *= math.Max(0, 1-d*l.aur[3*x+2])
				} else {
					a *= math.Max(0, 1+d/1.5)
				}
				v += a
				t += (0.95 - t) * clamp01(a*2)
			}
			a := 0.0
			if cloudy {
				c := l.cloudAt(float64(x)*cu+l.drift, float64(y)*cv)
				a = smooth(1-cover, 1.25-cover, c) * thin
				v += (cloudV + 0.5*g - v) * a
				t += (toneCloud - t) * a
			}
			l.shade[i] = a
			buf[i] = clamp01(v)
			l.tone[i] = t
		}
	}
}

// starlight draws the stars into the sky as it darkens, behind any cloud.
func (l *landscape) starlight(f *frame, buf []float64, hy int) {
	vis := clamp01(1-2.2*l.tod*l.lit) * (0.75 + 0.5*l.hush)
	if vis < 0.01 {
		return
	}
	for _, s := range l.stars {
		px, py := int(s[0]*float64(f.fw)), int(s[1]*float64(hy))
		if px >= f.fw || py >= hy {
			continue
		}
		i := py*f.fw + px
		v := clamp01(s[2] * vis * (0.8 + 0.2*wave(s[3]+l.t*0.3)) * (1 - l.shade[i]))
		if v > buf[i] {
			buf[i] = v
			l.tone[i] = 1
		}
	}
}

// hills draws the ridges from far to near, each standing on the horizon and
// taking on more of the sky's light the further away it is.
func (l *landscape) hills(f *frame, buf []float64, hy int) {
	fw, fh := f.fw, float64(f.fh)
	glow := clamp01(1 - math.Abs(l.tod-0.45)/0.3)
	hor := (0.1 + 0.32*l.tod + 0.3*glow) * l.lit
	for k := range l.ridges {
		r := &l.ridges[k]
		land := clamp01(hor*r.haze*0.75 + 0.012 + 0.15*l.flash*r.haze)
		for x := range fw {
			h := r.at(float64(x)/float64(fw)*r.span*ridgeLen + r.off)
			if l.set.sea {
				h = math.Max(0, h-0.55) / 0.45
			}
			top := float64(hy) - (r.base+r.rise*h)*fh
			for y := max(int(top), 0); y < hy; y++ {
				c := clamp01(float64(y) + 1 - top)
				i := y*fw + x
				buf[i] += (land - buf[i]) * c
				l.tone[i] += (r.tone - l.tone[i]) * c
			}
		}
	}
}

// ground draws the land below the horizon where there is no water: snow
// that holds a little light, or dark earth.
func (l *landscape) ground(f *frame, buf []float64, hy int) {
	glow := clamp01(1 - math.Abs(l.tod-0.45)/0.3)
	hor := (0.1 + 0.32*l.tod + 0.3*glow) * l.lit
	v, t := 0.02+0.1*hor, 0.1
	if l.weather == weatherSnow || l.aurora {
		v, t = 0.04+0.35*hor, 0.6
	}
	for i := hy * f.fw; i < len(buf); i++ {
		buf[i], l.tone[i] = v, t
	}
}

// reflect draws the water: each row below the horizon is the row as far
// above it, dimmed, and moved sideways by the ripples. As the water moves,
// the light of the sun or moon breaks into a path of glints below it. Where
// the layer is coupled the water also reflects the scene's last frame.
func (l *landscape) reflect(f *frame, buf []float64, hy int) {
	fw := f.fw
	depth := float64(f.fh - hy)
	amp := l.ripple
	glint := clamp01(amp / (3 * float64(fw) / 200))
	env := f.couple > 0 && len(f.env) == len(buf)
	for y := hy; y < f.fh; y++ {
		d := float64(y - hy)
		src := max(hy-1-(y-hy), 0)
		shift := 0.0
		if amp > 0.001 {
			shift = amp * (0.4 + 0.6*d/depth) * wave(d/float64(f.fh)*14-l.t*0.35+0.37*wave(d*0.05+l.t*0.1))
		}
		row := buf[src*fw : (src+1)*fw]
		path, inv := 0.0, 0.0
		if l.path > 0 && glint > 0.01 {
			path = l.path * glint * (1 - 0.5*d/depth)
			span := l.r * (0.8 + 1.5*glint + 1.2*d/depth)
			inv = 1 / (span * span)
		}
		for x := range fw {
			sx := math.Min(math.Max(float64(x)+shift, 0), float64(fw-1))
			x0 := int(sx)
			x1 := min(x0+1, fw-1)
			v := row[x0] + (row[x1]-row[x0])*(sx-float64(x0))
			if env {
				v = math.Max(v, f.couple*f.env[src*fw+x])
			}
			v *= reflectivity
			if glint > 0.01 {
				sparkle := 0.5 + 0.5*wave(float64(x)/float64(fw)*9+d*0.19+l.t*0.2)
				v *= 1 - 0.4*glint*sparkle
				if path > 0 {
					dx := float64(x) + 0.5 - l.mx
					v += path * sparkle * sparkle / (1 + dx*dx*inv)
				}
			}
			i := y*fw + x
			buf[i] = v
			l.tone[i] = toneWater
		}
	}
}

// motes moves and draws the weather: snow or rain falling, and embers or
// fireflies rising, more of them on a release.
func (l *landscape) motes(f *frame, buf []float64) {
	dt, fw, fh := f.dt, float64(f.fw), float64(f.fh)
	switch l.weather {
	case weatherSnow:
		density := clamp01(0.35 + 0.5*l.hush + 0.2*l.tension)
		for i := range l.fall {
			m := &l.fall[i]
			m.y += dt * (0.035 + 0.03*m.sp) * (1 + 0.5*l.wind)
			m.x += dt * (0.04*l.wind + 0.012*wave(m.ph+l.t*0.15))
			l.wrap(m)
			if m.u < density {
				l.dot(f, buf, m.x*fw, m.y*fh, (0.55+0.35*m.sp)*clamp01((density-m.u)*8), 1)
			}
		}
	case weatherRain:
		density := clamp01(0.55 + 0.45*l.tension)
		slant := 0.15 * l.wind
		for i := range l.fall {
			m := &l.fall[i]
			m.y += dt * (0.2 + 0.1*m.sp) * (1 + 0.3*l.wind)
			m.x += dt * slant
			l.wrap(m)
			if m.u < density {
				v := 0.8 * clamp01((density-m.u)*8)
				px, py := m.x*fw, m.y*fh
				for k := range 5 {
					l.dot(f, buf, px-float64(k)*slant*2, py-float64(k), v*(1-0.1*float64(k)), 0.6)
				}
			}
		}
	}

	base := 0.0
	if l.weather == weatherEmbers || l.weather == weatherFireflies {
		base = 0.45
	}
	density := clamp01(base + 0.6*l.release)
	if density < 0.01 {
		return
	}
	for i := range l.rise {
		m := &l.rise[i]
		if l.weather == weatherEmbers {
			m.y -= dt * (0.04 + 0.05*m.sp) * (1 + l.release)
			m.x += dt * (0.03*l.wind + 0.01*wave(m.ph+l.t*0.5))
			if m.y < 0 {
				m.y, m.x = 1, l.rng.Float64()
			}
			m.x -= math.Floor(m.x)
			v := (0.5 + 0.4*wave(m.ph*5+l.t*1.3)) * clamp01((density-m.u)*8) * (0.3 + 0.7*m.y)
			if m.u < density {
				l.dot(f, buf, m.x*fw, m.y*fh, v, 1)
			}
			continue
		}
		m.x += dt * (0.015*wave(m.ph+l.t*0.07) + 0.03*f.couple*(l.aim-m.x))
		m.y += dt * (0.01*wave(m.ph*1.7+l.t*0.05) - 0.05*l.release*(0.5+m.sp))
		if m.y < l.horizon-0.4 || m.y > 1 {
			m.x, m.y = l.rng.Float64(), l.horizon+0.15*(l.rng.Float64()-0.3)
		}
		m.x -= math.Floor(m.x)
		if m.u < density {
			v := 0.9 * smooth(0.3, 0.9, wave(m.ph*2.3+l.t*0.2)) * clamp01((density-m.u)*8)
			l.dot(f, buf, m.x*fw, m.y*fh, v, 1)
		}
	}
}

// wrap returns a falling mote that has left the field to the top, and keeps
// it within the width.
func (l *landscape) wrap(m *mote) {
	if m.y > 1 {
		m.y -= 1
		m.x = l.rng.Float64()
	}
	m.x -= math.Floor(m.x)
}

// dot draws a point of brightness v and tone t at pixel (px, py), shared
// between the four pixels around it so that it moves smoothly.
func (l *landscape) dot(f *frame, buf []float64, px, py, v, t float64) {
	x0, y0 := math.Floor(px), math.Floor(py)
	fx, fy := px-x0, py-y0
	for _, c := range [4][3]float64{
		{x0, y0, (1 - fx) * (1 - fy)}, {x0 + 1, y0, fx * (1 - fy)},
		{x0, y0 + 1, (1 - fx) * fy}, {x0 + 1, y0 + 1, fx * fy},
	} {
		ix, iy := int(c[0]), int(c[1])
		if ix >= 0 && iy >= 0 && ix < f.fw && iy < f.fh {
			i := iy*f.fw + ix
			if p := v * math.Sqrt(c[2]); p > buf[i] {
				buf[i] = p
				l.tone[i] += (t - l.tone[i]) * p
			}
		}
	}
}

// line draws points along the segment from (x0, y0) to (x1, y1), in pixels.
func (l *landscape) line(f *frame, buf []float64, x0, y0, x1, y1, v float64) {
	n := int(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		l.dot(f, buf, x0+(x1-x0)*t, y0+(y1-y0)*t, v, 1)
	}
}

// waveTable holds one turn of a sine wave.
var waveTable = func() (t [1025]float64) {
	for i := range t {
		t[i] = math.Sin(2 * math.Pi * float64(i) / 1024)
	}
	return t
}()

// wave returns the sine of x whole turns, from a table.
func wave(x float64) float64 {
	p := (x - math.Floor(x)) * 1024
	i := int(p)
	if i < 0 || i >= 1024 {
		return 0
	}
	return waveTable[i] + (waveTable[i+1]-waveTable[i])*(p-float64(i))
}
