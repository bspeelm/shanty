package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
	"github.com/bspeelm/shanty/internal/tui"
)

// scanned builds an app against a server that behaves as the options say.
func scanned(t *testing.T, malice fake.Malice) (app, *fake.Server) {
	t.Helper()
	srv := fake.New(t, fake.Options{User: user, Password: pass, Malice: malice})
	client, err := subsonic.New(srv.URL, subsonic.PasswordAuth(user, pass), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return newApp(t.Context(), client, newRecorder(), config.Config{}), srv
}

// TestAScanReportsProgressWithoutStoppingAnything is the design question the
// issue names. A scan takes minutes, so nothing waits for it: each answer
// arrives as a message and asks for the next.
func TestAScanReportsProgressWithoutStoppingAnything(t *testing.T) {
	scanInterval = time.Millisecond
	t.Cleanup(func() { scanInterval = 2 * time.Second })

	a, srv := scanned(t, fake.Malice{ScanSteps: 3})
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})

	a, msgs := step(t, a, tui.Scan{})

	// Drive the scan to its end, the way bubbletea would.
	var progress, finished int
	for range 12 {
		var next []tea.Msg
		for _, m := range msgs {
			if s, ok := m.(scanning); ok {
				a, next = step(t, a, s)
				if strings.Contains(a.ui.Status(), "scanning:") {
					progress++
				}
				if strings.Contains(a.ui.Status(), "finished") {
					finished++
				}
			}
		}
		if finished > 0 || len(next) == 0 {
			break
		}
		msgs = next
	}

	if progress == 0 {
		t.Error("a running scan reported no progress")
	}
	if finished == 0 {
		t.Fatalf("the scan never finished; the screen says %q", a.ui.Status())
	}
	// The music was never touched.
	if current, _ := a.queue.Current(); current.ID != "tr-1" {
		t.Errorf("scanning changed what was playing to %q", current.ID)
	}
	// And it asked the server rather than guessing.
	var asked int
	for _, r := range srv.Requests() {
		if r.Endpoint == "getScanStatus" || r.Endpoint == "startScan" {
			asked++
		}
	}
	if asked < 2 {
		t.Errorf("the server was asked %d times about the scan", asked)
	}
}

// TestAFinishedScanBringsTheNewMusicIn covers the second half of the command:
// a scan that found something is no use until the library is asked for again.
func TestAFinishedScanBringsTheNewMusicIn(t *testing.T) {
	a, srv := scanned(t, fake.Malice{ScanSteps: 1})
	before := len(srv.Requests())

	_, _ = step(t, a, scanning{status: subsonic.Scan{Scanning: false, Count: 42}})

	var reloaded bool
	for _, r := range srv.Requests()[before:] {
		if r.Endpoint == "getArtists" {
			reloaded = true
		}
	}
	if !reloaded {
		t.Error("a finished scan did not ask for the library again")
	}
}

// TestAServerThatCannotScanSaysSoRatherThanLookingBroken covers the server
// without these endpoints. The issue asks for this specifically: it must not
// read as shanty's fault.
func TestAServerThatCannotScanSaysSoRatherThanLookingBroken(t *testing.T) {
	a, _ := scanned(t, fake.Malice{NoScanning: true})

	a, msgs := step(t, a, tui.Scan{})
	for _, m := range msgs {
		if s, ok := m.(scanning); ok {
			a, _ = step(t, a, s)
		}
	}

	status := a.ui.Status()
	if !strings.Contains(status, "does not offer to scan") {
		t.Errorf("the message reads %q", status)
	}
	if strings.Contains(status, "bug in shanty") {
		t.Errorf("a server without the endpoint was reported as shanty's fault: %q", status)
	}
}

// TestAnAccountNotAllowedToScanIsNotToldTheServerCannot covers the refusal
// that is about permission rather than capability. They read differently and
// the fixes are different.
func TestAnAccountNotAllowedToScanIsNotToldTheServerCannot(t *testing.T) {
	a, _ := scanned(t, fake.Malice{})

	a, _ = step(t, a, scanning{err: &subsonic.Error{Code: 50, Message: "User is not authorized"}})

	status := a.ui.Status()
	if strings.Contains(status, "does not offer to scan") {
		t.Errorf("an account without permission was told the server cannot scan: %q", status)
	}
	if !strings.Contains(status, "administrator") {
		t.Errorf("the message does not say who can change it: %q", status)
	}
}
