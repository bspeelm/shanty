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
		if !control(r) && !bidi(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// control reports the terminal control characters Sanitise removes: the C0
// range, DEL, and C1.
func control(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// bidi reports the Unicode bidirectional formatting characters Sanitise
// removes: the embedding and override block, and the isolates. These force the
// display order of letters. The left-to-right and right-to-left marks, U+200E
// and U+200F, are not removed; they order neutral characters only.
func bidi(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

func needsSanitising(s string) bool {
	for _, r := range s {
		if control(r) || bidi(r) {
			return true
		}
	}
	return false
}
