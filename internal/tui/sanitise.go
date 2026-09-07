// Package tui renders the screens and emits intents. It does no I/O, which
// `make budgets` asserts by refusing it net, os and os/exec: that is what
// makes a golden test of every screen possible without a server, a player or a
// filesystem (§4).
package tui

import "strings"

// Sanitise removes the bytes a terminal reads as instructions rather than
// text.
//
// Every artist name and track title on these screens was chosen by the server,
// and a terminal is an interpreter: escape sequences move the cursor, clear the
// screen, change the scroll region, and on many emulators ask a question whose
// answer is injected back as though the user had typed it. This is the
// cheapest attack a hostile server has and it needs no bug to work (ADR-013).
//
// Stripping happens here, at the point a string stops being data and becomes
// an instruction, rather than at the decode boundary -- altering what the
// server sent would make the scrobble shanty sends back describe a track the
// server does not have.
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

// control is the C0 range, DEL, and C1 -- every code point a terminal can read
// as the start of a sequence. Newline and tab go with them: a title occupies
// one line, and one containing a newline breaks the layout whether or not
// anybody meant it to.
//
// The bidirectional overrides are deliberately left in. They can reorder text
// on screen, but an artist name in Arabic or Hebrew is an ordinary thing for
// this program to show, and mangling those would be a bug for real users in
// exchange for a display trick against none.
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
