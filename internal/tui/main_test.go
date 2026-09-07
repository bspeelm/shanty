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
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI)
	os.Exit(m.Run())
}
