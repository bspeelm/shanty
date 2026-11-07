package mpv

import (
	"strings"
	"testing"
)

// mpv's pause is a property of the player rather than of a track, and it
// survives loadfile. A track loaded while paused sits at position zero and is
// never heard, so loading one resumes.
func TestLoadingATrackResumesAPausedPlayer(t *testing.T) {
	p, report := stub(t)

	if err := p.SetPause(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if err := p.Load(t.Context(), "http://example.invalid/a.flac"); err != nil {
		t.Fatal(err)
	}

	log := commands(t, report)
	loaded := strings.Index(log, `"loadfile"`)
	resumed := strings.Index(log, `"set_property","pause",false`)
	if loaded < 0 {
		t.Fatalf("no loadfile reached the player:\n%s", log)
	}
	if resumed < 0 {
		t.Fatalf("loading a track did not clear pause:\n%s", log)
	}
	// After, not before: clearing it first would be undone by nothing, but the
	// order is what makes the track audible rather than merely unpaused.
	if resumed < loaded {
		t.Errorf("pause was cleared before the load rather than after:\n%s", log)
	}
}

// Appending must not resume. It queues a track for later and says nothing
// about whether the one playing should be heard now.
func TestAppendingDoesNotResume(t *testing.T) {
	p, report := stub(t)

	if err := p.SetPause(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if err := p.Append(t.Context(), "http://example.invalid/b.flac"); err != nil {
		t.Fatal(err)
	}

	if log := commands(t, report); strings.Contains(log, `"pause",false`) {
		t.Errorf("appending a track resumed the player:\n%s", log)
	}
}
