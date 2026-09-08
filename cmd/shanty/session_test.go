package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/control"
	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/tui"
)

// twoTracks is an album long enough to end one track and still have one left.
var twoTracks = subsonic.Album{ID: "al-1", Songs: []subsonic.Song{
	{ID: "tr-1", Title: "Slipway", Duration: 180},
	{ID: "tr-2", Title: "Ballast", Duration: 200},
}}

// TestASessionStillReportsPlays is the claim the whole feature rests on. A
// session has no interface, but it is still the thing that tells the server
// what was listened to, which is what leaving mpv on its own would have lost.
func TestASessionStillReportsPlays(t *testing.T) {
	a, _, srv := wired(t)
	a.headless = true
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})

	a, _ = step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}))

	var played int
	for _, r := range srv.Requests() {
		if r.Endpoint == "scrobble" {
			played++
		}
	}
	if played == 0 {
		t.Fatal("a session played a track through and told the server nothing")
	}
	if current, _ := a.queue.Current(); current.ID != "tr-2" {
		t.Errorf("the session is on %q, want tr-2", current.ID)
	}
}

// TestASessionEndsWhenItsQueueRunsOut covers the lifetime. A session nobody
// can see must not sit there after the music has finished.
func TestASessionEndsWhenItsQueueRunsOut(t *testing.T) {
	a, _, _ := wired(t)
	a.headless = true
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 1}) // the last track

	_, msgs := step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}))

	if !quits(msgs) {
		t.Error("a session whose queue ran out kept running")
	}
}

// TestAnInterfaceDoesNotEndWhenItsQueueRunsOut is the same event in the other
// mode. A screen showing an empty player is correct; a process nobody can see
// is not.
func TestAnInterfaceDoesNotEndWhenItsQueueRunsOut(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 1})

	_, msgs := step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}))

	if quits(msgs) {
		t.Error("the interface quit because the album finished")
	}
}

// TestASessionEndsWhenItsPlayerDies covers a player killed out from under a
// session. An interface shows the message and waits; a session has nobody to
// show it to and nothing left to play, so it goes.
func TestASessionEndsWhenItsPlayerDies(t *testing.T) {
	a, _, _ := wired(t)
	a.headless = true
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})

	_, msgs := step(t, a, playerGone{err: errors.New("mpv exited")})
	if !quits(msgs) {
		t.Error("a session outlived the player it was driving")
	}
}

// TestAnInterfaceSurvivesItsPlayerDying is the same event with a screen to
// report it on.
func TestAnInterfaceSurvivesItsPlayerDying(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})

	a, msgs := step(t, a, playerGone{err: errors.New("mpv exited")})
	if quits(msgs) {
		t.Fatal("the interface quit instead of reporting that mpv stopped")
	}
	if !strings.Contains(a.ui.Status(), "mpv stopped") {
		t.Errorf("the message reads %q", a.ui.Status())
	}
}

// TestATrackEndingDuringTheHandoverIsReportedOnce covers the window in which
// both processes are alive. The session owns the queue from the moment it is
// started, so this one must stop advancing it: otherwise the track is
// reported twice and the next one is never played.
func TestATrackEndingDuringTheHandoverIsReportedOnce(t *testing.T) {
	a, _, srv := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})
	a.detaching = true

	a, _ = step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}))

	for _, r := range srv.Requests() {
		if r.Endpoint == "scrobble" {
			t.Fatal("the interface reported a play the session is responsible for")
		}
	}
	if current, _ := a.queue.Current(); current.ID != "tr-1" {
		t.Errorf("the interface advanced to %q, which the session will do again", current.ID)
	}
}

// TestDetachingRefusesWithNothingPlaying covers the empty case. A session with
// nothing to play would exit the moment it started, which is indistinguishable
// from a failure.
func TestDetachingRefusesWithNothingPlaying(t *testing.T) {
	a, _, _ := wired(t)
	a.detach = func(handoff) error { return nil }

	a, _ = step(t, a, tui.Detach{})

	if a.released {
		t.Fatal("a session was started with nothing to play")
	}
	if !strings.Contains(a.ui.Status(), "nothing playing") {
		t.Errorf("the refusal reads %q", a.ui.Status())
	}
}

// TestDetachingIsAllowedWhilePaused covers the case a refusal must not catch.
// Pausing and then wanting the terminal back is the reason to detach.
func TestDetachingIsAllowedWhilePaused(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})
	a, _ = step(t, a, tui.PausedChanged(true))

	var given handoff
	a.detach = func(h handoff) error { given = h; return nil }
	a, msgs := step(t, a, tui.Detach{})

	if !a.detaching {
		t.Error("the interface kept the queue after handing it over")
	}
	if !contains[detached](msgs) {
		t.Fatalf("detaching produced %v", msgs)
	}
	if !given.Paused {
		t.Error("the session was not told the music was paused")
	}
}

// TestTheHandoffCarriesTheQueueAndNoCredential covers what is written down.
// The session builds its own stream URLs from the credential it reads itself,
// so nothing here needs to carry one.
func TestTheHandoffCarriesTheQueueAndNoCredential(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 1})

	var given handoff
	a.detach = func(h handoff) error { given = h; return nil }
	_, _ = step(t, a, tui.Detach{})

	if len(given.Tracks) != 2 || given.At != 1 {
		t.Fatalf("the session was given %d tracks at %d, want 2 at 1", len(given.Tracks), given.At)
	}
	doc, err := json.Marshal(given)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{pass, "stream", "&p=", "&t=", "&s="} {
		if bytes.Contains(doc, []byte(secret)) {
			t.Errorf("the handoff contains %q:\n%s", secret, doc)
		}
	}
}

// TestAFailedHandoverLeavesTheMusicPlayingHere covers the safe failure. If the
// session does not start, nothing has been given away.
func TestAFailedHandoverLeavesTheMusicPlayingHere(t *testing.T) {
	a, _, _ := wired(t)
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})
	a.detach = func(handoff) error { return errors.New("no such file") }

	a, msgs := step(t, a, tui.Detach{})
	if !contains[detachFailed](msgs) {
		t.Fatalf("a failed handover produced %v", msgs)
	}
	for _, m := range msgs {
		if f, ok := m.(detachFailed); ok {
			a, _ = step(t, a, f)
		}
	}

	if a.released {
		t.Error("the player was given away to a session that never started")
	}
	if a.detaching {
		t.Error("the interface is still holding back after the handover failed")
	}
	if !strings.Contains(a.ui.Status(), "still playing here") {
		t.Errorf("the message reads %q", a.ui.Status())
	}
}

// TestTheHandoffSurvivesTheRoundTrip covers what a session reads back.
func TestTheHandoffSurvivesTheRoundTrip(t *testing.T) {
	sent := handoff{
		Protocol: control.Protocol,
		Tracks: []queue.Track{
			{ID: "tr-1", Title: "Slipway", Artist: "Aoi", Duration: 3 * time.Minute},
			{ID: "tr-2", Title: "Ballast", Artist: "Aoi"},
		},
		At: 1, Volume: 40, Paused: true,
	}
	doc, err := json.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}

	got, err := readHandoff(bytes.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	q := got.resume()
	if track, ok := q.Current(); !ok || track.ID != "tr-2" {
		t.Errorf("the session resumed on %+v, want tr-2", track)
	}
	if got.Volume != 40 || !got.Paused {
		t.Errorf("the session read volume %d paused %v, want 40 true", got.Volume, got.Paused)
	}
	playing, _ := got.nowPlaying()
	if playing.Title != "Ballast" {
		t.Errorf("the session is showing %q, want Ballast", playing.Title)
	}
}

// TestASessionRefusesAHandoffItCannotUse covers what arrives from a version
// that is not this one, and the empty stdin that a broken handover produces.
func TestASessionRefusesAHandoffItCannotUse(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"nothing at all", "", "sent nothing"},
		{"not JSON", "{{{", "could not be read"},
		{"another version", `{"protocol":99,"tracks":[{"ID":"tr-1"}]}`, "different versions"},
		{"no tracks", `{"protocol":1,"tracks":[]}`, "sent nothing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readHandoff(strings.NewReader(tc.doc))
			if err == nil {
				t.Fatal("the session accepted a handoff it cannot use")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the message reads %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// contains reports whether any message has the given type.
func contains[T tea.Msg](msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}

// quits reports whether the batch asked bubbletea to end the program.
func quits(msgs []tea.Msg) bool { return contains[tea.QuitMsg](msgs) }

// TestHandingOverReleasesThePlayerAndTheQueue covers what a session does when
// an interface takes it back. It must let go of the player rather than
// stopping it, and stop advancing a queue that is no longer its own.
func TestHandingOverReleasesThePlayerAndTheQueue(t *testing.T) {
	a, _, srv := wired(t)
	a.headless = true
	a, _ = step(t, a, tui.PlayFrom{Album: twoTracks, Index: 0})

	reply := make(chan handoff, 1)
	a, _ = step(t, a, handoverRequest{reply: reply})
	given := <-reply

	if !a.released {
		t.Error("the session will stop the player it handed over")
	}
	if !a.detaching {
		t.Error("the session is still advancing a queue it gave away")
	}
	if len(given.Tracks) != 2 {
		t.Errorf("the interface was given %d tracks, want 2", len(given.Tracks))
	}

	// A track ending now belongs to whoever took over.
	a, _ = step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}))
	for _, r := range srv.Requests() {
		if r.Endpoint == "scrobble" {
			t.Error("the session reported a play after handing the queue over")
		}
	}
	if current, _ := a.queue.Current(); current.ID != "tr-1" {
		t.Errorf("the session advanced to %q after handing over", current.ID)
	}
}

// TestUninstallRefusesWhileASessionIsPlaying covers a command this feature
// made dangerous. The runtime directory holds the socket a session is reached
// through, so removing it while one is playing would leave a player nothing
// could stop.
func TestUninstallRefusesWhileASessionIsPlaying(t *testing.T) {
	env, _ := scratch(t)
	env = shortRuntime(t, env)
	if err := env.Paths.EnsureRuntime(); err != nil {
		t.Fatal(err)
	}
	// Something for uninstall to delete, so "it deleted nothing" is a claim
	// with content.
	if err := os.MkdirAll(env.Paths.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := control.Listen(env.Paths.ControlSocket())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		_ = control.Serve(l, func(control.Request) control.Response {
			return control.Response{OK: true, State: &control.State{}}
		})
	}()

	err = runUninstall(t.Context(), env, nil)
	if err == nil {
		t.Fatal("uninstall removed the directories under a session that was playing")
	}
	if !strings.Contains(err.Error(), "shanty stop") {
		t.Errorf("the refusal does not say what to do: %q", err)
	}
	if _, err := os.Stat(env.Paths.Config); err != nil {
		t.Errorf("uninstall deleted something before refusing: %v", err)
	}
}

// TestUninstallProceedsWithNoSession covers the ordinary case, so the refusal
// cannot quietly become permanent.
func TestUninstallProceedsWithNoSession(t *testing.T) {
	env, _ := scratch(t)
	if err := env.Paths.EnsureRuntime(); err != nil {
		t.Fatal(err)
	}

	if err := runUninstall(t.Context(), env, nil); err != nil {
		t.Fatalf("uninstall refused with no session running: %v", err)
	}
	if _, err := os.Stat(env.Paths.Runtime); !os.IsNotExist(err) {
		t.Error("uninstall left the runtime directory behind")
	}
}

// TestTheDoctorReportsEveryStateASessionCanBeIn covers what a background
// process cannot report about itself. Each of the four combinations of a
// session and a player says something different, and the two that are wrong
// carry the command that fixes them.
func TestTheDoctorReportsEveryStateASessionCanBeIn(t *testing.T) {
	for _, tc := range []struct {
		name            string
		session, player bool
		severity        Severity
		want            string
	}{
		{"nothing running", false, false, Pass, "nothing is playing"},
		{"a session playing", true, true, Pass, "a session is playing"},
		{"a session whose player died", true, false, Warn, "player has stopped"},
		{"an mpv nobody owns", false, true, Warn, "no session owns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := scratch(t)
			env = shortRuntime(t, env)
			if err := env.Paths.EnsureRuntime(); err != nil {
				t.Fatal(err)
			}
			if tc.session {
				serveOn(t, env.Paths.ControlSocket())
			}
			if tc.player {
				listenOn(t, env.Paths.Socket())
			}

			got := checkSession(env)
			if got.Severity != tc.severity {
				t.Errorf("severity %v, want %v (%s)", got.Severity, tc.severity, got.Summary)
			}
			if !strings.Contains(got.Summary, tc.want) {
				t.Errorf("the report reads %q, want it to mention %q", got.Summary, tc.want)
			}
			if tc.severity != Pass && got.Fix == "" {
				t.Error("a warning with nothing to do about it")
			}
		})
	}
}

// shortDir is a directory whose path is short enough to hold a socket.
//
// A unix socket path is limited to around a hundred bytes. A directory from
// t.TempDir is named after the test, sits under whatever TMPDIR holds, and
// exceeds that on its own. Where shanty really puts its sockets is short,
// which is why nothing outside these tests has to think about it.
func shortDir(t *testing.T) string {
	t.Helper()
	base := "/tmp"
	if _, err := os.Stat(base); err != nil {
		t.Skip("no short path to put a socket in")
	}
	dir, err := os.MkdirTemp(base, "sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// shortRuntime points the environment at a runtime directory a socket fits in.
func shortRuntime(t *testing.T, env Env) Env {
	t.Helper()
	env.Paths.Runtime = shortDir(t)
	return env
}

// serveOn answers on the control socket the way a session does.
func serveOn(t *testing.T, socket string) {
	t.Helper()
	l, err := control.Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		_ = control.Serve(l, func(control.Request) control.Response {
			return control.Response{OK: true, State: &control.State{}}
		})
	}()
}

// listenOn accepts connections the way mpv's socket does, without answering.
func listenOn(t *testing.T, socket string) {
	t.Helper()
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
}

// TestStoppingCoversEveryStateThereIsToStop covers the one command that has
// work to do when there is no session. A player left by a session that was
// killed is exactly what somebody typing `shanty stop` wants gone, and telling
// them there is no session would be true and useless.
func TestStoppingCoversEveryStateThereIsToStop(t *testing.T) {
	t.Run("nothing at all", func(t *testing.T) {
		env, out := scratchOut(t)
		if err := commandSession("stop", control.Stop)(t.Context(), env, nil); err != nil {
			t.Fatalf("stopping nothing failed: %v", err)
		}
		if !strings.Contains(out.String(), "nothing was playing") {
			t.Errorf("it said %q", out)
		}
	})

	t.Run("a session playing", func(t *testing.T) {
		env, out := scratchOut(t)
		env = shortRuntime(t, env)
		if err := env.Paths.EnsureRuntime(); err != nil {
			t.Fatal(err)
		}
		serveOn(t, env.Paths.ControlSocket())

		if err := commandSession("stop", control.Stop)(t.Context(), env, nil); err != nil {
			t.Fatalf("stopping a session failed: %v", err)
		}
		if !strings.Contains(out.String(), "stopped") {
			t.Errorf("it said %q", out)
		}
	})

	t.Run("a player no session owns", func(t *testing.T) {
		env, out := scratchOut(t)
		env = shortRuntime(t, env)
		if err := env.Paths.EnsureRuntime(); err != nil {
			t.Fatal(err)
		}
		listenOn(t, env.Paths.Socket())

		if err := commandSession("stop", control.Stop)(t.Context(), env, nil); err != nil {
			t.Fatalf("stopping an orphaned player failed: %v", err)
		}
		if !strings.Contains(out.String(), "no session owned") {
			t.Errorf("it said %q", out)
		}
	})
}

// scratchOut is scratch with somewhere to read the output back from.
func scratchOut(t *testing.T) (Env, *bytes.Buffer) {
	t.Helper()
	env, _ := scratch(t)
	out := &bytes.Buffer{}
	env.Stdout = out
	return env, out
}
