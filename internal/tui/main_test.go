package tui

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The colour profile is pinned rather than detected. lipgloss otherwise asks
// the terminal what it can do, so the golden files would record the machine
// that generated them and a passing suite here would be a failing one in CI --
// Graybeard row 16, in the one place a test writes its own expectations.
// The palette is off for the same reason the profile is pinned. The golden
// files record layout, and a frame carrying colour codes would make every
// later layout change arrive with them. colour_test.go turns it on for the
// few frames that exist to test the palette itself.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI)
	colour = false
	os.Exit(m.Run())
}
