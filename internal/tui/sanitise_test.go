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
const hostile = "\x1b[2J\x1b[H\x1b[1;1r\x1b[6n\x07\r\n\x9b31m"

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

// ADR-013 declines to strip these. An artist name in Arabic or Hebrew is an
// ordinary thing to display, and mangling one to prevent a display trick would
// be a bug for real users.
func TestSanitiseKeepsBidirectionalMarks(t *testing.T) {
	in := "‫العندليب‬"
	if got := Sanitise(in); got != in {
		t.Errorf("Sanitise stripped a bidi mark: %q", got)
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
