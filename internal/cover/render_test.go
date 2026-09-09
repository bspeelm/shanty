package cover

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strconv"
	"strings"
	"testing"
)

// pngOf is an image of a given size, as a server would send it.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// noisyPNG is an image that does not compress, so that its encoding is
// genuinely larger than one escape sequence can carry.
func noisyPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := uint32(1)
	for y := range h {
		for x := range w {
			r = r*1664525 + 1013904223
			img.Set(x, y, color.RGBA{R: uint8(r >> 24), G: uint8(r >> 16), B: uint8(r >> 8), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestTheTerminalDecidesTheProtocol holds the closed set of terminals against
// the answers. A terminal that is not here gets text, which is the whole point
// of the fallback.
func TestTheTerminalDecidesTheProtocol(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want Protocol
	}{
		{"kitty by its own variable", map[string]string{"TERM": "xterm-256color", "KITTY_WINDOW_ID": "1"}, Kitty},
		{"kitty by TERM", map[string]string{"TERM": "xterm-kitty"}, Kitty},
		// Read off a running Ghostty on Linux: it sets TERM and nothing else.
		{"ghostty by TERM", map[string]string{"TERM": "xterm-ghostty"}, Kitty},
		{"ghostty naming itself", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "ghostty"}, Kitty},
		{"wezterm by its own variable", map[string]string{"TERM": "xterm-256color", "WEZTERM_PANE": "0"}, Kitty},
		{"wezterm naming itself", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "WezTerm"}, Kitty},
		{"iterm2", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app"}, ITerm2},
		{"iterm2 over ssh", map[string]string{"TERM": "xterm-256color", "LC_TERMINAL": "iTerm2"}, ITerm2},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, None},
		{"screen", map[string]string{"TERM": "screen"}, None},
		{"dumb", map[string]string{"TERM": "dumb", "KITTY_WINDOW_ID": "1"}, None},
		{"no terminal at all", map[string]string{}, None},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Detect(func(k string) string { return tc.env[k] })
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestKittySendsAPNGWhateverTheServerSent covers the one format conversion
// here: the kitty protocol takes PNG, and a server sends what it kept.
func TestKittySendsAPNGWhateverTheServerSent(t *testing.T) {
	for name, data := range map[string][]byte{"png": pngOf(t, 8, 8), "jpeg": jpegOf(t, 8, 8)} {
		t.Run(name, func(t *testing.T) {
			out := Render(Kitty, data, 20, 10)
			if !strings.HasPrefix(out, "\x1b_G") {
				t.Fatalf("not a kitty sequence: %q", out[:min(16, len(out))])
			}
			if !strings.Contains(out, "f=100") {
				t.Error("the sequence does not say the payload is a PNG")
			}
			if !strings.Contains(out, "c=20,r=10") {
				t.Error("the sequence does not carry the size in cells")
			}
			// Without this the terminal moves the cursor past the picture and
			// the rows reserved for it are added to the rows it already took.
			if !strings.Contains(out, "C=1") {
				t.Error("the sequence lets the terminal move the cursor")
			}
			// The payload decodes to a PNG, whatever arrived.
			body := out[strings.Index(out, ";")+1:]
			body = body[:strings.Index(body, "\x1b\\")]
			raw, err := base64.StdEncoding.DecodeString(body)
			if err != nil {
				t.Fatalf("the payload is not base64: %v", err)
			}
			if !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
				t.Error("the payload is not a PNG")
			}
		})
	}
}

// TestAPictureTooBigForOneSequenceIsChunked covers the protocol's limit. A
// single escape sequence carrying a whole image is what silently truncates.
func TestAPictureTooBigForOneSequenceIsChunked(t *testing.T) {
	out := Render(Kitty, noisyPNG(t, 200, 200), 20, 10)

	chunks := strings.Count(out, "\x1b_G")
	if chunks < 2 {
		t.Fatalf("a large image went out in %d sequence(s)", chunks)
	}
	if strings.Count(out, "a=T") != 1 {
		t.Errorf("the placement is repeated across %d chunks", strings.Count(out, "a=T"))
	}
	if strings.Count(out, "m=0;") != 1 {
		t.Errorf("%d chunks say they are the last", strings.Count(out, "m=0;"))
	}
	if !strings.Contains(out[:strings.Index(out, "\x1b\\")], "m=1;") {
		t.Error("the first chunk does not say more is coming")
	}
}

func TestITerm2SendsWhatTheServerSentAndItsLength(t *testing.T) {
	data := jpegOf(t, 8, 8)
	out := Render(ITerm2, data, 20, 10)

	if !strings.HasPrefix(out, "\x1b]1337;File=inline=1;") {
		t.Fatalf("not an iTerm2 sequence: %q", out[:min(24, len(out))])
	}
	if !strings.Contains(out, "size="+strconv.Itoa(len(data))) {
		t.Error("the sequence does not carry the byte length")
	}
	if !strings.Contains(out, "width=20;height=10") {
		t.Error("the sequence does not carry the size in cells")
	}
	if !strings.HasSuffix(out, "\a") {
		t.Error("the sequence is not terminated")
	}
	if !strings.Contains(out, base64.StdEncoding.EncodeToString(data)) {
		t.Error("the bytes sent are not the bytes given")
	}
}

// TestAnImageWhoseHeaderIsHugeIsNeverDecoded is the guard the byte cap does not
// give. A small file describes an image of any dimensions, and decoding one
// allocates four bytes a pixel.
func TestAnImageWhoseHeaderIsHugeIsNeverDecoded(t *testing.T) {
	// A well formed PNG that says it is 30000x30000, which is 900 million
	// pixels and 3.6 GB decoded. The checksum is recomputed, or the decoder
	// rejects it for being corrupt and the size is never reached.
	bomb := pngOf(t, 1, 1)
	tag := bytes.Index(bomb, []byte("IHDR"))
	copy(bomb[tag+4:tag+12], []byte{0, 0, 0x75, 0x30, 0, 0, 0x75, 0x30})
	sum := crc32.ChecksumIEEE(bomb[tag : tag+17])
	binary.BigEndian.PutUint32(bomb[tag+17:tag+21], sum)

	if err := check(bomb); err == nil {
		t.Fatal("an image of 900 million pixels was accepted")
	} else if !strings.Contains(err.Error(), "pixels") {
		t.Errorf("the error is %q and should say what the limit is", err)
	}

	// And the renderers draw nothing rather than failing.
	for _, p := range []Protocol{Kitty, ITerm2} {
		if out := Render(p, bomb, 4, 3); out != "" {
			t.Errorf("%s drew %q for an image it must not decode", p, out)
		}
	}
}

// TestSomethingThatIsNotAnImageDrawsNothing covers the renderers being unable
// to fail. Nothing about a cover is worth interrupting music for, and a box
// where a picture is not is a row of the screen saying so.
func TestSomethingThatIsNotAnImageDrawsNothing(t *testing.T) {
	for _, p := range []Protocol{Kitty, ITerm2, None} {
		if out := Render(p, []byte("<html>not a picture</html>"), 6, 3); out != "" {
			t.Errorf("%s drew %q for something that is not a picture", p, out)
		}
	}
}

func TestNoRoomIsNothingRatherThanABrokenBox(t *testing.T) {
	for _, size := range [][2]int{{0, 5}, {5, 0}, {-1, -1}} {
		if out := Render(Kitty, pngOf(t, 8, 8), size[0], size[1]); out != "" {
			t.Errorf("%v gave %q", size, out)
		}
	}
}

// TestABlockIsAlwaysTheRowsItWasAskedFor is what lets a screen count rows
// without knowing which terminal it is on.
func TestABlockIsAlwaysTheRowsItWasAskedFor(t *testing.T) {
	data := pngOf(t, 8, 8)
	if got := Block(None, data, 12, 4); got != nil {
		t.Errorf("a terminal that draws nothing was given %d rows", len(got))
	}
	for _, p := range []Protocol{Kitty, ITerm2} {
		for _, rows := range []int{1, 3, 10} {
			got := Block(p, data, 12, rows)
			if len(got) != rows {
				t.Errorf("%s at %d rows gave %d lines", p, rows, len(got))
			}
		}
	}
	if got := Detect(nil); got != None {
		t.Errorf("no environment at all gave %s", got)
	}
	if got := Block(Kitty, data, 0, 4); got != nil {
		t.Errorf("no width gave %v", got)
	}
}

// TestOnlyTheFirstLineOfAPictureCarriesTheSequence covers the padding: the
// rest of the block has to be blank, or a terminal draws the image again.
func TestOnlyTheFirstLineOfAPictureCarriesTheSequence(t *testing.T) {
	for _, p := range []Protocol{Kitty, ITerm2} {
		got := Block(p, pngOf(t, 8, 8), 12, 4)
		if !strings.Contains(got[0], "\x1b") {
			t.Errorf("%s put no escape sequence on the first line", p)
		}
		for _, line := range got[1:] {
			if strings.Contains(line, "\x1b") {
				t.Errorf("%s repeated the sequence on a later line", p)
			}
			if line != strings.Repeat(" ", 12) {
				t.Errorf("%s padded with %q", p, line)
			}
		}
	}
}

// TestOnlyAnOverlayNeedsTakingAway covers which protocols leave something
// behind. Getting this wrong in either direction is visible: too little and a
// picture sits over the next screen, too much and every frame carries an
// escape sequence for nothing.
func TestOnlyAnOverlayNeedsTakingAway(t *testing.T) {
	if got := Clear(Kitty); got == "" {
		t.Error("a kitty image is an overlay and is not taken away")
	} else if !strings.HasPrefix(got, "\x1b_G") || !strings.Contains(got, "a=d") {
		t.Errorf("the kitty sequence is %q, which is not a delete", got)
	}
	for _, p := range []Protocol{None, ITerm2} {
		if got := Clear(p); got != "" {
			t.Errorf("%s is drawn into the screen and needs no clearing, but sends %q", p, got)
		}
	}
}

// TestABoxKeepsThePicturesShape covers the stretching. Handing a terminal a
// width and a height makes it fill both, so the box has to be the right shape
// before the picture is sent.
func TestABoxKeepsThePicturesShape(t *testing.T) {
	for _, tc := range []struct {
		name             string
		w, h             int
		maxCols, maxRows int
		cols, rows       int
	}{
		// A cell is twice as tall as it is wide, so a square picture wants
		// twice as many columns as rows.
		{"square, room to spare", 500, 500, 80, 20, 40, 20},
		{"square, too wide for the box", 500, 500, 24, 40, 24, 12},
		{"wide picture", 1000, 500, 80, 20, 80, 20},
		{"tall picture", 500, 1000, 80, 20, 20, 20},
		{"no room", 500, 500, 0, 10, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cols, rows := Fit(pngOf(t, tc.w, tc.h), tc.maxCols, tc.maxRows)
			if cols != tc.cols || rows != tc.rows {
				t.Errorf("got %dx%d cells, want %dx%d", cols, rows, tc.cols, tc.rows)
			}
			if cols > tc.maxCols || rows > tc.maxRows {
				t.Errorf("%dx%d does not fit in %dx%d", cols, rows, tc.maxCols, tc.maxRows)
			}
		})
	}
}

// TestSomethingThatIsNotAPictureFillsTheBoxAsked covers Fit having nothing to
// measure. The placeholder is drawn to the box it was given.
func TestSomethingThatIsNotAPictureFillsTheBoxAsked(t *testing.T) {
	cols, rows := Fit([]byte("<html>not a picture</html>"), 30, 10)
	if cols != 30 || rows != 10 {
		t.Errorf("got %dx%d, want the box asked for", cols, rows)
	}
}
