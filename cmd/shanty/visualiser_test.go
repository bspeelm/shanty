package main

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/tui"
)

// readings returns the heard messages among msgs.
func readings(msgs []tea.Msg) []heard {
	var got []heard
	for _, m := range msgs {
		if h, ok := m.(heard); ok {
			got = append(got, h)
		}
	}
	return got
}

func TestOpeningTheVisualiserStartsReadingTheAudio(t *testing.T) {
	a, rec, _ := wired(t)
	a, msgs := step(t, a, tui.ShowVisualiser{})
	got := readings(msgs)
	if len(got) != 1 || !got[0].ok {
		t.Fatalf("opening the visualiser produced %d readings, want 1: %#v", len(got), msgs)
	}
	if !slices.Contains(rec.said(), "hearing") {
		t.Errorf("mpv was never asked for a reading: %v", rec.said())
	}

	// A reading is passed on and the next one is asked for.
	_, msgs = step(t, a, got[0])
	if len(readings(msgs)) != 1 {
		t.Errorf("a reading while the visualiser shows did not ask for the next: %#v", msgs)
	}
}

func TestClosingTheVisualiserStopsReadingTheAudio(t *testing.T) {
	a, _, _ := wired(t)
	a, msgs := step(t, a, tui.ShowVisualiser{})
	inFlight := readings(msgs)
	a, _ = step(t, a, tea.KeyMsg{Type: tea.KeyEsc})
	if a.ui.ShowingSky() {
		t.Fatal("esc did not close the visualiser")
	}
	// The reading already asked for arrives after it closed, and must not ask
	// for another, or mpv is read for ever with nothing drawing.
	_, msgs = step(t, a, inFlight[0])
	if len(readings(msgs)) != 0 {
		t.Errorf("a reading after the visualiser closed asked for another: %#v", msgs)
	}
}

func TestNoReadingKeepsAskingWhileTheVisualiserShows(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.ShowVisualiser{})
	_, msgs := step(t, a, heard{ok: false})
	if len(readings(msgs)) != 1 {
		t.Errorf("a missing reading stopped the audio being read: %#v", msgs)
	}
}

func TestAVisualiserAlreadyShowingDoesNotStartASecondReader(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.ShowVisualiser{})
	_, msgs := step(t, a, tui.ShowVisualiser{})
	if n := len(readings(msgs)); n != 0 {
		t.Errorf("opening it again while it showed started %d more readers", n)
	}
}
