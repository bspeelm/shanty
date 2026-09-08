// Package tui renders shanty’s screens and reports what the user asked for.
// It performs no I/O.
package tui

import "strings"

// Sanitise removes the characters a terminal interprets as instructions,
// leaving the printable text. Server-supplied names pass through it before
// they are drawn.
func Sanitise(s string) string {
	if !needsSanitising(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !control(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// control reports the characters Sanitise removes: the C0 range, DEL, and C1.
// Bidirectional override characters are left in place.
func control(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

func needsSanitising(s string) bool {
	for _, r := range s {
		if control(r) {
			return true
		}
	}
	return false
}
