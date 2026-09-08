package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// hostile is what a server can put in a track title: clear the screen, home
// the cursor, reset the scroll region, then ask the terminal a question whose
// answer some emulators inject back as if it had been typed. Kept as a literal
// here rather than imported from the fake, so this package's tests need
// nothing that can open a socket.
const hostile = "\x1b[2J\x1b[H\x1b[1;1r\x1b[6n\x07\r\n\x9b31m\u202e"

func TestSanitiseRemovesEverythingATerminalWouldObey(t *testing.T) {
	got := Sanitise(hostile + "Slipway")
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("an escape survived: %q", got)
	}
	for _, r := range got {
		if control(r) {
			t.Errorf("control character %U survived in %q", r, got)
		}
	}
	if !strings.HasSuffix(got, "Slipway") {
		t.Errorf("the title itself did not survive: %q", got)
	}
}

func TestSanitiseLeavesOrdinaryNamesAlone(t *testing.T) {
	for _, want := range []string{
		"Aoi",
		"The Bilge Pumps",
		"Sigur Rós",
		"坂本龍一",                   // CJK, which the layout must also measure
		"Umm Kulthum — أم كلثوم", // right-to-left, deliberately not mangled
		"Godspeed You! Black Emperor",
		"?!#$%^&*()[]{}",
	} {
		if got := Sanitise(want); got != want {
			t.Errorf("Sanitise(%q) = %q", want, got)
		}
	}
}

// The characters that force the display order of letters. A server can use one
// to make a title read in an order the characters do not have, and an
// unterminated one changes the layout of the rest of the row it is drawn on.
func TestSanitiseRemovesBidirectionalFormatting(t *testing.T) {
	for _, r := range []rune{
		0x202a, 0x202b, 0x202c, 0x202d, 0x202e, // embeddings, pop, overrides
		0x2066, 0x2067, 0x2068, 0x2069, // isolates
	} {
		in := "Slipway" + string(r) + "Ballast"
		if got := Sanitise(in); got != "SlipwayBallast" {
			t.Errorf("Sanitise(%U) = %q, want the character removed", r, got)
		}
	}
}

// The reversal itself. A right-to-left override makes what follows display
// backwards, so a title can be made to read as something it is not.
func TestSanitiseRemovesTheReversal(t *testing.T) {
	got := Sanitise("cover\u202egnp.exe")
	if strings.ContainsRune(got, 0x202e) {
		t.Fatalf("the override survived: %q", got)
	}
	if got != "covergnp.exe" {
		t.Errorf("Sanitise = %q, want covergnp.exe", got)
	}
}

// Right-to-left scripts render from the directional properties of the letters
// themselves and need no control characters, so removing the control
// characters does not affect them.
func TestSanitiseLeavesRightToLeftLettersAlone(t *testing.T) {
	for _, want := range []string{
		"\u0623\u0645 \u0643\u0644\u062b\u0648\u0645",
		"\u05e9\u05dc\u05de\u05d4 \u05d0\u05e8\u05e6\u05d9",
		"\u0623\u0645 \u0643\u0644\u062b\u0648\u0645 (Umm Kulthum)",
	} {
		if got := Sanitise(want); got != want {
			t.Errorf("Sanitise(%q) = %q", want, got)
		}
	}
}

// The marks order neutral characters and cannot reverse letters, so they are
// kept. This pins the narrower rule rather than leaving it to widen without a
// decision.
func TestSanitiseKeepsTheDirectionalMarks(t *testing.T) {
	for _, r := range []rune{0x200e, 0x200f} {
		in := "Aoi" + string(r) + " (1998)"
		if got := Sanitise(in); got != in {
			t.Errorf("Sanitise removed %U: %q", r, got)
		}
	}
}

// Whatever the server sends, the result is printable and still valid UTF-8.
func TestSanitiseAlwaysReturnsSomethingPrintable(t *testing.T) {
	var b strings.Builder
	for r := rune(0); r < 0x300; r++ {
		b.WriteRune(r)
	}
	got := Sanitise(b.String())

	if !utf8.ValidString(got) {
		t.Fatal("Sanitise produced invalid UTF-8")
	}
	for _, r := range got {
		if control(r) {
			t.Fatalf("control character %U survived", r)
		}
	}
}

// Sanitising twice changes nothing, which is what lets it be called at every
// render without the text drifting.
func TestSanitiseIsIdempotent(t *testing.T) {
	once := Sanitise(hostile + "Ballast")
	if twice := Sanitise(once); twice != once {
		t.Errorf("second pass changed %q to %q", once, twice)
	}
}
