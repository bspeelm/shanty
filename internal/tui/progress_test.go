package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TestTheProgressBarAlwaysFillsItsWidth covers the positions the golden files
// do not: a width of zero, a position past the end of the track, and a
// negative position. Each must produce a bar exactly as wide as it was asked
// for.
func TestTheProgressBarAlwaysFillsItsWidth(t *testing.T) {
	for _, c := range []struct {
		name     string
		position time.Duration
		duration time.Duration
		width    int
		want     int
	}{
		{"start", 0, time.Minute, 10, 0},
		{"halfway", 30 * time.Second, time.Minute, 10, 5},
		{"end", time.Minute, time.Minute, 10, 10},
		{"past the end", 2 * time.Minute, time.Minute, 10, 10},
		{"negative position", -time.Second, time.Minute, 10, 0},
		{"length not reported", 30 * time.Second, 0, 10, 0},
		{"negative length", 30 * time.Second, -time.Minute, 10, 0},
		{"zero width", 30 * time.Second, time.Minute, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := Model{position: c.position, duration: c.duration}
			got := m.progress(c.width)
			if n := lipgloss.Width(got); n != max(1, c.width) {
				t.Fatalf("bar is %d cells wide, asked for %d: %q", n, c.width, got)
			}
			if n := strings.Count(got, "━"); n != c.want {
				t.Errorf("%d cells played, want %d: %q", n, c.want, got)
			}
		})
	}
}
