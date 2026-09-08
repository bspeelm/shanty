//go:build ignore

// Derive the README's two images from the source artwork.
//
//	go run docs/images/derive.go
//
// bothy does this with ImageMagick and one -negate: its source is dark ink on
// white, so darkness becomes opacity. shanty's source is the other way round --
// light glyphs on a dark ground -- so brightness becomes opacity and there is
// nothing to negate. Everything after that is the same idea: clip the
// anti-aliasing that would leave a haze around every character, trim the empty
// border, and flood what remains with one colour.
//
// It is Go rather than a shell script because that is the language already
// here, and because the whole transform is thirty lines of image/png.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// The clip. Below lo the pixel is background and becomes fully transparent;
// above hi it is a glyph and becomes fully opaque. The source's ground sits at
// 0.13 luminance and its glyphs at 0.97, so this has room on both sides.
const lo, hi = 0.20, 0.92

// Provisional, and the only colours in the project. shanty ships no palette
// yet -- themes are v0.3 -- so these are chosen to sit on a dark and a light
// page respectively and will be replaced from the palette when there is one.
var variants = map[string]color.NRGBA{
	"dark":  {0x8b, 0xe9, 0xfd, 0xff},
	"light": {0x31, 0x68, 0x7a, 0xff},
}

func main() {
	dir := filepath.Dir(os.Args[0])
	if wd, err := os.Getwd(); err == nil {
		dir = filepath.Join(wd, "docs", "images")
	}

	src := decode(filepath.Join(dir, "shanty-source.png"))
	alpha, box := opacity(src)

	for name, tint := range variants {
		out := image.NewNRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
		for y := range box.Dy() {
			for x := range box.Dx() {
				a := alpha[(y+box.Min.Y)*src.Bounds().Dx()+(x+box.Min.X)]
				out.SetNRGBA(x, y, color.NRGBA{tint.R, tint.G, tint.B, a})
			}
		}
		write(filepath.Join(dir, "shanty-"+name+".png"), out)
		fmt.Printf("wrote shanty-%s.png  %dx%d\n", name, box.Dx(), box.Dy())
	}
}

// opacity turns brightness into alpha and reports the box outside which
// everything is transparent.
func opacity(img image.Image) ([]uint8, image.Rectangle) {
	b := img.Bounds()
	out := make([]uint8, b.Dx()*b.Dy())
	// Tracked by hand rather than by unioning rectangles: image.Rect puts its
	// arguments in order, so an inverted rectangle to start from is silently
	// canonicalised into the whole image and nothing is ever trimmed.
	minX, minY, maxX, maxY := b.Dx(), b.Dy(), -1, -1

	for y := range b.Dy() {
		for x := range b.Dx() {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// Rec. 709 luminance, on the 0..1 scale RGBA's 16-bit values need.
			l := (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(bl)) / 65535
			a := (l - lo) / (hi - lo)
			switch {
			case a <= 0:
				continue // background: leave it transparent and out of the box
			case a > 1:
				a = 1
			}
			out[y*b.Dx()+x] = uint8(a*255 + 0.5)
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	if maxX < 0 {
		must(fmt.Errorf("every pixel is below the %.2f clip; the source is not light-on-dark", lo))
	}
	// A little air, so the glyphs are not flush against the edge.
	const pad = 8
	return out, image.Rect(
		max(0, minX-pad), max(0, minY-pad),
		min(b.Dx(), maxX+1+pad), min(b.Dy(), maxY+1+pad))
}

func decode(path string) image.Image {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	img, err := png.Decode(f)
	must(err)
	return img
}

func write(path string, img image.Image) {
	f, err := os.Create(path)
	must(err)
	defer f.Close()
	must(png.Encode(f, img))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "derive:", err)
		os.Exit(1)
	}
}
