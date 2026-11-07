package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// escapes matches one SGR sequence.
var escapes = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// hasColour says whether any escape in s sets a foreground colour. The colour
// is not always the first parameter -- bold and a colour arrive together as
// one sequence -- so each parameter is read rather than the sequence matched
// as a whole.
func hasColour(s string) bool {
	for _, seq := range escapes.FindAllStringSubmatch(s, -1) {
		for _, p := range strings.Split(seq[1], ";") {
			switch {
			case p == "38":
				return true
			case len(p) == 2 && (p[0] == '3' || p[0] == '9') && p[1] >= '0' && p[1] <= '7':
				return true
			}
		}
	}
	return false
}

// withColour turns the palette on for one test and off again after.
func withColour(t *testing.T) {
	t.Helper()
	colour = true
	t.Cleanup(func() { colour = false })
}

func TestTheGoldenFramesCarryNoColour(t *testing.T) {
	// The default for the suite, set in TestMain. A frame with colour in it
	// would make every later layout change arrive carrying the palette.
	m := send(t, loaded(t), nowPlayingSlipway(180*time.Second,
		Progress(83*time.Second))...)
	if hasColour(m.View()) {
		t.Errorf("a frame carries a colour with the palette off:\n%q", m.View())
	}
}

func TestThePaletteColoursTheMusicAndTheWarnings(t *testing.T) {
	withColour(t)

	onAlbum := send(t, loaded(t), ArtistLoaded(library()[0]),
		AlbumLoaded(library()[0].Albums[0]))
	playing := send(t, onAlbum,
		nowPlayingSlipway(180*time.Second, Progress(83*time.Second))...)
	if !hasColour(playing.View()) {
		t.Error("with the palette on, nothing in a playing frame is coloured")
	}

	// The mark on the playing row and the played part of the progress line are
	// the only two things the accent is allowed to touch.
	if !hasColour(playing.playing("tr-1")) {
		t.Error("the mark on the playing track is not coloured")
	}
	if !hasColour(playing.progress(20)) {
		t.Error("the played part of the progress line is not coloured")
	}

	asking := send(t, loaded(t), Confirm{Question: "delete \"Evening\"?"})
	if !hasColour(asking.footer(64)) {
		t.Error("a question that cannot be undone is not coloured")
	}

	failed := send(t, loaded(t), Failed{Message: "the server refused"})
	if !hasColour(failed.footer(64)) {
		t.Error("a refusal is not coloured")
	}
	// A notice is not a refusal and is not warned about.
	notice := send(t, loaded(t), Notice("playing Ballast next"))
	if hasColour(notice.footer(64)) {
		t.Error("an ordinary notice is coloured as a warning")
	}
}

// This is the test of whether the palette is decoration or load-bearing. Strip
// every escape from both frames and they must be the same characters in the
// same places: colour says nothing the glyphs do not.
func TestTheLayoutIsTheSameWithTheColourAsWithoutIt(t *testing.T) {
	frame := func() string {
		m := send(t, loaded(t), ArtistLoaded(library()[0]),
			AlbumLoaded(library()[0].Albums[0]), starredChanged())
		m = send(t, m, nowPlayingSlipway(180*time.Second, Progress(83*time.Second))...)
		return m.View()
	}

	mono := frame()
	withColour(t)
	lit := frame()

	if plain(mono) != plain(lit) {
		t.Errorf("the palette changes the layout.\n--- without ---\n%s\n--- with ---\n%s",
			plain(mono), plain(lit))
	}
}

// plain is a frame with every escape sequence removed.
func plain(frame string) string { return escapes.ReplaceAllString(frame, "") }

func TestEveryColourHasAValueForEitherKindOfTerminal(t *testing.T) {
	// One of the two is unreadable on the other, so neither may be empty.
	for name, c := range map[string]struct{ Light, Dark string }{
		"accent":  {accent.Light, accent.Dark},
		"warning": {warning.Light, warning.Dark},
		"muted":   {muted.Light, muted.Dark},
	} {
		if c.Light == "" || c.Dark == "" {
			t.Errorf("%s is %q on light and %q on dark", name, c.Light, c.Dark)
		}
		if c.Light == c.Dark {
			t.Errorf("%s is the same on both, which cannot be right for either", name)
		}
		for _, v := range []string{c.Light, c.Dark} {
			if !strings.HasPrefix(v, "#") || len(v) != 7 {
				t.Errorf("%s has %q, which is not a hex colour", name, v)
			}
		}
	}
}
