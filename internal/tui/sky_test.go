package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// skyFrameAtZero is the frame sky draws for a 64 by 10 grid at t = 0.
const skyFrameAtZero = `·             ·                 ░░▒▒▒▒▒░░░      ·           ░░░░
                                ░▒▓▓▓▓▒░                    ░░░░
                           ░░░  ░▓███▓░                    ░░░░░
                ·       ░░▒████░░████▒░          ░░░       ░░░░░
             ░░░░░░░░░░▒▓███████▒███▓░         ░░░░░░░░  ░░░░░░░
            ░░▒▒▒▒▒▒▒▒▓▓█████▒▓█ ▒▓▓▒░       ░░▒▒▒▒▒░░░░░░░░░░▒▒
           ░░▒▒▓▓▓▓▓▓▓▓▓▓▒▒░ ░████▓▒▒▒▒░░░░░▒▒▒▒▓▓▒▒▒░░░░░░░▒▒▒▒
          ░░▒▒▓▓▓▓▓▓▓▒▒░░     ▒████▓▓▒▒▒▒▒▒▒▒▓▓▓▓▓▓▒▒▒░░░░░░▒▒▒▒
 ░░░      ░░▒▒▒▓▓▓▓▓▒▒░       ░▒▓▓▓▒▒▒░░▒▒▒▒▓▓▓▓▓▓▒▒▒░░░░░░▒▒▒▒▒
 ░░░░░░  ░░░░▒▒▒▓▓▓▓▒▒░         ░░░░░░░░░▒▒▒▓▓▓▓▓▒▒▒░░░░░░░▒▒▒▒░`

// TestSkyFrameAtZeroMatchesGolden compares the frame at t = 0 with the
// recorded one, line by line.
func TestSkyFrameAtZeroMatchesGolden(t *testing.T) {
	want := strings.Split(skyFrameAtZero, "\n")
	got := sky(64, 10, 0.0)
	if len(got) != len(want) {
		t.Fatalf("sky(64, 10, 0) returned %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

// TestSkyIsDeterministic draws the same frame twice and expects the same
// bytes.
func TestSkyIsDeterministic(t *testing.T) {
	first := sky(48, 12, 3.7)
	second := sky(48, 12, 3.7)
	if strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Error("two calls with the same arguments drew different frames")
	}
}

// TestSkyStarsDoNotMove checks that a cell holding a star at one time holds
// either a star or a shade at another, and never a blank.
func TestSkyStarsDoNotMove(t *testing.T) {
	early := sky(64, 10, 0.0)
	later := sky(64, 10, 9.3)
	for r := range early {
		a := []rune(early[r])
		b := []rune(later[r])
		for c := range a {
			star := skyStar(c, r)
			if !star && (a[c] == '·' || b[c] == '·') {
				t.Fatalf("a star appeared at column %d of line %d where the hash says there is none", c, r)
			}
			if star && (a[c] == ' ' || b[c] == ' ') {
				t.Fatalf("the star at column %d of line %d is missing from a frame", c, r)
			}
		}
	}
}

// TestSkyFillsTheFrame checks that every frame has one line per row and that
// each line is as wide as the frame.
func TestSkyFillsTheFrame(t *testing.T) {
	sizes := []struct{ w, h int }{{64, 10}, {100, 24}, {1, 1}, {8, 3}}
	for _, size := range sizes {
		lines := sky(size.w, size.h, 1.25)
		if len(lines) != size.h {
			t.Errorf("sky(%d, %d, 1.25) returned %d lines, want %d", size.w, size.h, len(lines), size.h)
			continue
		}
		for i, line := range lines {
			if got := lipgloss.Width(line); got != size.w {
				t.Errorf("sky(%d, %d, 1.25) line %d is %d columns wide, want %d", size.w, size.h, i, got, size.w)
			}
		}
	}
}

// TestSkyRefusesEmptyFrames checks that a frame with no area draws nothing.
func TestSkyRefusesEmptyFrames(t *testing.T) {
	sizes := []struct{ w, h int }{{0, 10}, {64, 0}, {0, 0}, {-3, 8}, {8, -3}}
	for _, size := range sizes {
		if lines := sky(size.w, size.h, 0.5); lines != nil {
			t.Errorf("sky(%d, %d, 0.5) returned %d lines, want nil", size.w, size.h, len(lines))
		}
	}
}

// TestSkyMoves checks that the frame changes as time passes.
func TestSkyMoves(t *testing.T) {
	before := strings.Join(sky(64, 10, 0.0), "\n")
	after := strings.Join(sky(64, 10, 0.8), "\n")
	if before == after {
		t.Error("the frame at t = 0.8 is the same as the frame at t = 0")
	}
}

// TestSkyDrawsOnlyItsOwnGlyphs checks that no rune outside the shades, the
// star and the blank reaches the frame.
func TestSkyDrawsOnlyItsOwnGlyphs(t *testing.T) {
	allowed := skyShades + "·"
	for _, step := range []float64{0, 0.4, 1.9, 12.5, 100} {
		for _, line := range sky(80, 20, step) {
			for _, r := range line {
				if !strings.ContainsRune(allowed, r) {
					t.Fatalf("sky drew %q at t = %v, which is not one of %q", r, step, allowed)
				}
			}
		}
	}
}
