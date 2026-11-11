package tui

import "github.com/charmbracelet/lipgloss"

// The three colours the interface uses, and no others outside the visualiser,
// which draws in palettes of its own (ADR-028). accent means this is
// the music: the mark on the track playing and the part of the progress line
// already played. warning means a question that cannot be undone or a refusal.
// muted is what faint already says, named rather than left to a terminal's own
// idea of dim.
//
// Each carries a value for a light terminal and one for a dark one, because
// one of the two is unreadable on the other.
var (
	accent  = lipgloss.AdaptiveColor{Light: "#9A4A1E", Dark: "#E08A4F"}
	warning = lipgloss.AdaptiveColor{Light: "#9B3B2B", Dark: "#E08478"}
	muted   = lipgloss.AdaptiveColor{Light: "#6E665C", Dark: "#8A8176"}
)

// colour says whether the three are applied. It is read when a style is built
// rather than when this package loads, so a test can turn it off.
var colour = true

// tint adds a colour to a style, or leaves the style as it is.
func tint(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style {
	if !colour {
		return s
	}
	return s.Foreground(c)
}

// faint is for rules, the key help, group headings, an empty list and the
// visualiser.
func faint() lipgloss.Style { return tint(lipgloss.NewStyle().Faint(true), muted) }

// played is the part of the progress line behind the playhead.
func played() lipgloss.Style { return tint(lipgloss.NewStyle().Bold(true), accent) }

// mark is the glyph against the track playing, wherever it is listed.
func mark() lipgloss.Style { return tint(lipgloss.NewStyle(), accent) }

// alarm is a question that cannot be undone, and a refusal.
func alarm() lipgloss.Style { return tint(lipgloss.NewStyle().Bold(true), warning) }
