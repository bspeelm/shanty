package cover

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"

	// Registered for their decoders. A server sends whichever it kept.
	_ "image/gif"
	_ "image/jpeg"
)

// MaxPixels is the largest decoded image that will be built, about 2048 by
// 2048.
//
// The byte cap on the download does not bound this. A few hundred kilobytes of
// PNG describes an image of any dimensions it likes, and decoding it allocates
// four bytes for every pixel, so the header is read and the dimensions are
// checked before anything is decoded.
const MaxPixels = 4 << 20

// Protocol is a way of putting a picture in a terminal.
type Protocol int

const (
	// None is a terminal that draws no pictures. Nothing is fetched for one
	// and nothing is drawn: a box where a cover would have been is a row of
	// the screen spent saying that a picture is missing.
	None Protocol = iota
	// Kitty is the kitty graphics protocol.
	Kitty
	// ITerm2 is the iTerm2 inline image protocol.
	ITerm2
)

func (p Protocol) String() string {
	switch p {
	case Kitty:
		return "kitty"
	case ITerm2:
		return "iterm2"
	}
	return "none"
}

// Detect says how a terminal can be sent a picture.
//
// It reads the environment and nothing else. The other way to ask is to write
// an escape sequence to the terminal and read its reply, which races with
// everything else sharing that file descriptor and waits on a terminal that
// will never answer. A terminal this does not recognise gets text, which is
// the same answer as a terminal that cannot show pictures at all and is why
// being uncertain is safe.
func Detect(env func(string) string) Protocol {
	if env == nil {
		return None
	}
	term, program := env("TERM"), env("TERM_PROGRAM")
	if term == "" || term == "dumb" {
		return None
	}
	// TERM is what a terminal always sets, and several of these set nothing
	// else. TERM_PROGRAM is checked as well because the same terminal sets it
	// on macOS and not on Linux.
	switch {
	case env("KITTY_WINDOW_ID") != "", strings.Contains(term, "kitty"):
		return Kitty
	case strings.Contains(term, "ghostty"), program == "ghostty", program == "Ghostty":
		return Kitty
	case env("WEZTERM_PANE") != "", strings.Contains(term, "wezterm"), program == "WezTerm":
		return Kitty
	case program == "iTerm.app", env("LC_TERMINAL") == "iTerm2":
		return ITerm2
	}
	return None
}

// Render turns the bytes of an image into what a terminal is sent to show it
// in a box that many columns wide and rows tall.
//
// Anything that cannot be drawn is nothing rather than an error. A cover is
// not worth interrupting somebody's music for, and a placeholder in its place
// is a row of the screen spent on a picture that is not there.
func Render(p Protocol, data []byte, cols, rows int) string {
	if cols < 1 || rows < 1 {
		return ""
	}
	switch p {
	case Kitty:
		img, err := decode(data)
		if err != nil {
			return ""
		}
		var png bytes.Buffer
		if err := encodePNG(&png, img); err != nil {
			return ""
		}
		return kitty(png.Bytes(), cols, rows)
	case ITerm2:
		if err := check(data); err != nil {
			return ""
		}
		return iterm2(data, cols, rows)
	}
	return ""
}

// check reads an image's header and reports whether it is one this will
// decode. Nothing is decoded.
func check(data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("that is not an image this can read: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return fmt.Errorf("the image is %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width*cfg.Height > MaxPixels {
		return fmt.Errorf("the image is %dx%d, which is more than the %d pixels one is decoded under",
			cfg.Width, cfg.Height, MaxPixels)
	}
	return nil
}

// decode reads an image, after its header says the size is one worth building.
func decode(data []byte) (image.Image, error) {
	if err := check(data); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// encodePNG writes an image as a PNG, which is the one format the kitty
// protocol takes without the pixels being sent raw.
func encodePNG(w *bytes.Buffer, img image.Image) error { return png.Encode(w, img) }

// kittyChunk is the most base64 the kitty protocol takes in one escape
// sequence.
const kittyChunk = 4096

// kitty is the kitty graphics protocol: base64 PNG in chunks, the first
// carrying the placement and the last saying it is the last.
func kitty(data []byte, cols, rows int) string {
	payload := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for first := true; len(payload) > 0; first = false {
		chunk := payload
		if len(chunk) > kittyChunk {
			chunk = chunk[:kittyChunk]
		}
		payload = payload[len(chunk):]

		more := 1
		if len(payload) == 0 {
			more = 0
		}
		b.WriteString("\x1b_G")
		if first {
			// C=1 is the cursor movement policy: leave the cursor where it
			// was. Without it the terminal moves past the picture, and the
			// blank lines that reserve the rows land below it instead of
			// under it, making the frame twice as tall as the space it asked
			// for.
			fmt.Fprintf(&b, "a=T,f=100,c=%d,r=%d,C=1,", cols, rows)
		}
		fmt.Fprintf(&b, "m=%d;%s\x1b\\", more, chunk)
	}
	return b.String()
}

// iterm2 is the iTerm2 inline image protocol, which takes whichever format the
// server sent and sizes it in cells.
func iterm2(data []byte, cols, rows int) string {
	return fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1:%s\a",
		len(data), cols, rows, base64.StdEncoding.EncodeToString(data))
}

// cellAspect is how many times taller a terminal cell is than it is wide. It
// is not asked of the terminal: the answer varies by a little between fonts
// and by nothing that matters at the size a cover is drawn.
const cellAspect = 2.0

// Fit is the box of cells a picture should be drawn in to keep its shape,
// inside the space there is for it.
//
// Giving a terminal both a width and a height in cells makes it stretch the
// picture to fill them. The way to keep a square cover square is to ask for a
// box that is already the right shape.
func Fit(data []byte, maxCols, maxRows int) (cols, rows int) {
	if maxCols < 1 || maxRows < 1 {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return maxCols, maxRows
	}
	// How many columns this picture wants for every row it is given.
	wide := float64(cfg.Width) * cellAspect / float64(cfg.Height)
	if c := int(math.Round(float64(maxRows) * wide)); c <= maxCols {
		return max(1, c), maxRows
	}
	return maxCols, max(1, int(math.Round(float64(maxCols)/wide)))
}

// Clear is what a terminal is sent to take away a picture already placed.
//
// A kitty graphics image is not text. It is an overlay the terminal keeps
// until it is told otherwise, so drawing a different screen over it leaves it
// where it was. An iTerm2 image occupies cells and goes when they are written
// over, and a text placeholder is text, so both are nothing.
func Clear(p Protocol) string {
	if p == Kitty {
		// d=A takes the placement and the image data with it, because the
		// picture is sent again whenever it is shown again.
		return "\x1b_Ga=d,d=A\x1b\\"
	}
	return ""
}

// Block renders an image as exactly that many lines, so that a caller laying
// out a screen can count rows without knowing which protocol was used.
//
// A protocol that draws with an escape sequence puts it on the first line and
// pads the rest with spaces. The padding is what moves the cursor past the
// picture: how far a terminal moves it on its own varies, and blank lines are
// the one thing every terminal agrees about.
func Block(p Protocol, data []byte, cols, rows int) []string {
	if cols < 1 || rows < 1 {
		return nil
	}
	out := Render(p, data, cols, rows)
	if out == "" {
		return nil
	}
	lines := make([]string, rows)
	lines[0] = out
	for i := 1; i < rows; i++ {
		lines[i] = strings.Repeat(" ", cols)
	}
	return lines
}
