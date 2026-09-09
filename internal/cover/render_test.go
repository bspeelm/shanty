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
		{"ghostty", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "ghostty"}, Kitty},
		{"wezterm", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "WezTerm"}, Kitty},
		{"iterm2", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app"}, ITerm2},
		{"iterm2 over ssh", map[string]string{"TERM": "xterm-256color", "LC_TERMINAL": "iTerm2"}, ITerm2},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, Text},
		{"screen", map[string]string{"TERM": "screen"}, Text},
		{"dumb", map[string]string{"TERM": "dumb", "KITTY_WINDOW_ID": "1"}, Text},
		{"no terminal at all", map[string]string{}, Text},
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

	// And the renderers answer with the placeholder rather than an error.
	for _, p := range []Protocol{Kitty, ITerm2} {
		if out := Render(p, bomb, 4, 3); !strings.Contains(out, "╭") {
			t.Errorf("%s rendered something other than the placeholder", p)
		}
	}
}

// TestSomethingThatIsNotAnImageIsThePlaceholder covers the renderers being
// unable to fail. Nothing about a cover is worth interrupting music for.
func TestSomethingThatIsNotAnImageIsThePlaceholder(t *testing.T) {
	for _, p := range []Protocol{Kitty, ITerm2, Text} {
		out := Render(p, []byte("<html>not a picture</html>"), 6, 3)
		if !strings.Contains(out, "╭") {
			t.Errorf("%s gave %q, want the placeholder", p, out)
		}
	}
}

// TestThePlaceholderIsExactlyTheBoxItWasAskedFor covers the layout promise: a
// screen keeps its shape whether or not the terminal shows pictures.
func TestThePlaceholderIsExactlyTheBoxItWasAskedFor(t *testing.T) {
	for _, size := range [][2]int{{10, 5}, {2, 2}, {1, 1}, {3, 1}} {
		cols, rows := size[0], size[1]
		out := Render(Text, pngOf(t, 8, 8), cols, rows)
		lines := strings.Split(out, "\n")
		if len(lines) != rows {
			t.Errorf("%dx%d gave %d rows", cols, rows, len(lines))
		}
		for _, line := range lines {
			if n := len([]rune(line)); n != cols {
				t.Errorf("%dx%d gave a row of %d columns: %q", cols, rows, n, line)
			}
		}
	}
}

func TestNoRoomIsNothingRatherThanABrokenBox(t *testing.T) {
	for _, size := range [][2]int{{0, 5}, {5, 0}, {-1, -1}} {
		if out := Render(Text, pngOf(t, 8, 8), size[0], size[1]); out != "" {
			t.Errorf("%v gave %q", size, out)
		}
	}
}
