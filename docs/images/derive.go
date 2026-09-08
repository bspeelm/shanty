//go:build ignore

// Derive the README’s two images from the source artwork.
//
//	go run docs/images/derive.go
//
// The source is light glyphs on a dark ground. Brightness becomes opacity,
// the empty border is trimmed, and the result is flooded with one colour.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// Pixels below lo become fully transparent and above hi fully opaque, which
// removes the anti-aliasing around each character.
const lo, hi = 0.20, 0.92

// The fill colour for each variant.
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

// opacity returns per-pixel alpha derived from brightness, and the box
// outside which every pixel is transparent.
func opacity(img image.Image) ([]uint8, image.Rectangle) {
	b := img.Bounds()
	out := make([]uint8, b.Dx()*b.Dy())
	// The bounds are tracked as four integers. image.Rect orders its arguments,
	// so an inverted rectangle cannot be used as a starting value.
	minX, minY, maxX, maxY := b.Dx(), b.Dy(), -1, -1

	for y := range b.Dy() {
		for x := range b.Dx() {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// Rec. 709 luminance, scaled to 0..1.
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
	// Padding, so the glyphs are not flush against the edge.
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
