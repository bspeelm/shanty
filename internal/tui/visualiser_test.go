package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/sky"
)

func TestAPlanReachesTheVisualiserWithoutChangingTheScreen(t *testing.T) {
	plan := sky.NewPlan(nil)
	closed := loaded(t)
	if next, cmd := closed.Update(Foresight{Title: "Slipway", Artist: "Aoi", Plan: plan}); cmd != nil || next.(Model).View() != closed.View() {
		t.Error("a plan with the visualiser closed changed something")
	}
	open := send(t, loaded(t), ShowVisualiser{})
	if next, cmd := open.Update(Foresight{Title: "Slipway", Artist: "Aoi", Plan: plan}); cmd != nil || next.(Model).View() != open.View() {
		t.Error("a plan with the visualiser open asked for something or redrew it")
	}
}

// The visualiser is an overlay rather than a screen, and it keeps itself
// turning by asking for the next frame. These cover both: what is on screen
// while it is up, and that nothing keeps asking once it is down.

// turn runs whatever command the model returned and feeds the message back,
// which is one frame of the pattern advancing.
func turn(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("nothing asked for the next frame")
	}
	msg := cmd()
	if _, ok := msg.(skyTurned); !ok {
		t.Fatalf("the command produced %#v, want a frame", msg)
	}
	next, out := m.Update(msg)
	return next.(Model), out
}

func TestTheVisualiserOpensFromACommandAndAJump(t *testing.T) {
	byCommand, msg := typeCommand(t, loaded(t), "visualizer")
	if _, ok := msg.(ShowVisualiser); !ok {
		t.Fatalf(":visualizer emitted %#v, want ShowVisualiser", msg)
	}
	byCommand = send(t, byCommand, msg)
	if !byCommand.ShowingSky() {
		t.Error(":visualizer did not put the visualiser on screen")
	}

	byJump, _ := press(t, loaded(t), "g")
	byJump, _ = press(t, byJump, "v")
	if !byJump.ShowingSky() {
		t.Error("gv did not put the visualiser on screen")
	}
}

func TestTheVisualiserKeepsAskingForTheNextFrame(t *testing.T) {
	m := loaded(t)
	next, cmd := m.Update(ShowVisualiser{})
	m = next.(Model)

	for i := range 3 {
		m, cmd = turn(t, m, cmd)
		if got := m.sky.Frames(); got != i+2 {
			t.Fatalf("after opening and %d frames the visualiser had drawn %d", i+1, got)
		}
	}
}

func TestLeavingTheVisualiserStopsItTurning(t *testing.T) {
	m := loaded(t)
	next, cmd := m.Update(ShowVisualiser{})
	m = next.(Model)
	m, cmd = turn(t, m, cmd)

	// esc takes it away without moving off the screen underneath.
	before := m.Screen()
	m, _ = press(t, m, "esc")
	if m.ShowingSky() {
		t.Fatal("esc did not take the visualiser away")
	}
	if m.Screen() != before {
		t.Errorf("esc left the visualiser and also moved to %s", m.Screen())
	}

	// The frame already in flight arrives after it has gone, and must not ask
	// for another. A detached session runs Update without drawing anything,
	// so a pattern that kept asking would turn for ever with nobody looking.
	after, out := m.Update(skyTurned{})
	if out != nil {
		t.Error("a frame arriving after the visualiser closed asked for another")
	}
	if after.(Model).ShowingSky() {
		t.Error("a late frame put the visualiser back on screen")
	}
}

func TestPausingHoldsThePatternAndResumingStartsItAgain(t *testing.T) {
	m := loaded(t)
	next, cmd := m.Update(ShowVisualiser{})
	m = next.(Model)
	m, cmd = turn(t, m, cmd)

	m = send(t, m, PausedChanged(true))
	// The frame in flight lands, advances once, and asks for nothing more.
	held, out := m.Update(skyTurned{})
	m = held.(Model)
	if out != nil {
		t.Error("the pattern asked for another frame while the music was paused")
	}

	stopped := m.sky.Frames()
	resumed, cmd := m.Update(PausedChanged(false))
	m = resumed.(Model)
	if cmd == nil {
		t.Fatal("resuming did not start the pattern turning again")
	}
	m, _ = turn(t, m, cmd)
	if m.sky.Frames() <= stopped {
		t.Errorf("the pattern did not advance after resuming: %v then %v", stopped, m.sky.Frames())
	}
}

func TestTheVisualiserSaysWhatIsPlayingUnderneath(t *testing.T) {
	m := send(t, loaded(t), nowPlayingSlipway(180*time.Second,
		Progress(83*time.Second), VolumeChanged(80), ShowVisualiser{})...)

	frame := m.View()
	// The bar and the progress line are the reason this is an overlay rather
	// than a screen: it is never a dead end.
	if !strings.Contains(frame, "Slipway · Aoi") {
		t.Errorf("the visualiser hides what is playing:\n%s", frame)
	}
	if !strings.Contains(frame, "1:23 / 3:00") {
		t.Errorf("the visualiser hides how far through it is:\n%s", frame)
	}
	if !strings.Contains(frame, "esc leaves") {
		t.Errorf("the visualiser does not say how to leave:\n%s", frame)
	}
	// And the heading still names where esc goes back to.
	if !strings.Contains(frame, "shanty · artists") {
		t.Errorf("the heading does not name the screen underneath:\n%s", frame)
	}
}

func TestAHeldPatternSaysItIsPausedRatherThanLookingStuck(t *testing.T) {
	m := send(t, loaded(t), nowPlayingSlipway(180*time.Second,
		Progress(83*time.Second), ShowVisualiser{}, PausedChanged(true))...)

	if frame := m.View(); !strings.Contains(frame, "paused") {
		t.Errorf("a held pattern does not say it is paused:\n%s", frame)
	}
}
