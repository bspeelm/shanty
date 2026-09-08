package mpv

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func waitABit() { time.Sleep(5 * time.Millisecond) }

// The credential-shaped strings these tests hunt for. A stream URL carries
// both, which is exactly why it never reaches a command line.
const (
	fakeToken = "deadbeefdeadbeefdeadbeefdeadbeef"
	fakeSalt  = "brackish"
)

func streamURL() string {
	return "https://music.example.org/rest/stream.view?id=tr-1&u=skipper&t=" + fakeToken + "&s=" + fakeSalt
}

// §5's named gate: no token in any child argv or env.
//
// The child reports its own os.Args and os.Environ, which is the authoritative
// answer to "what did we hand it" and works on every platform. On Linux the
// kernel's own view is checked too, because /proc is what the standard says to
// read and a discrepancy between the two would be worth knowing about.
func TestChildProcessHygiene(t *testing.T) {
	// The canary is a value no part of shanty uses, so that the credential
	// itself stays searchable in the child's environment. Planting the token
	// as the canary would make the one value that matters most impossible to
	// look for.
	p, dir := stub(t, "SHANTY_TEST_CANARY=not-a-credential")

	if err := p.Load(t.Context(), streamURL()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(commands(t, dir), fakeToken) {
		t.Fatal("the stream URL never reached mpv, so this test proves nothing about where it did not go")
	}

	argv := report(t, dir, argvFile)
	for _, secret := range []string{fakeToken, fakeSalt, "stream.view"} {
		if strings.Contains(argv, secret) {
			t.Errorf("the child's argv carries %q:\n%s", secret, argv)
		}
	}

	// The canary proves the environment is genuinely being searched. shanty's
	// own credential must not be in it, because it went over the socket.
	environ := report(t, dir, environFile)
	if !strings.Contains(environ, "SHANTY_TEST_CANARY=not-a-credential") {
		t.Fatal("the canary is missing from the child's environment; this test is not searching anything")
	}
	for _, secret := range []string{fakeToken, fakeSalt, "stream.view"} {
		if strings.Contains(environ, secret) {
			t.Errorf("the child's environment carries %q", secret)
		}
	}

	if runtime.GOOS != "linux" {
		return
	}
	pid := p.cmd.Process.Pid
	for _, f := range []string{"cmdline", "environ"} {
		raw, err := os.ReadFile(filepath.Join("/proc", itoa(pid), f))
		if err != nil {
			t.Fatalf("reading /proc/%d/%s: %v", pid, f, err)
		}
		// NUL-separated, so a credential is still found by a substring search.
		for _, secret := range []string{fakeToken, fakeSalt, "stream.view"} {
			if strings.Contains(string(raw), secret) {
				t.Errorf("/proc/%d/%s carries %q", pid, f, secret)
			}
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// §3: shanty is not a manager of anyone's mpv config. The flags are fixed and
// there is no passthrough, so the user's own setup is left as it was found.
func TestTheFlagsAreFixedAndLeaveTheUsersMpvAlone(t *testing.T) {
	_, dir := stub(t)
	argv := report(t, dir, argvFile)

	for _, want := range []string{"--no-config", "--no-video", "--idle=yes", "--prefetch-playlist=yes", "--input-ipc-server="} {
		if !strings.Contains(argv, want) {
			t.Errorf("the child was not given %s:\n%s", want, argv)
		}
	}
}

func TestTheSocketDirectoryIs0700(t *testing.T) {
	p, _ := stub(t)
	info, err := os.Stat(filepath.Dir(p.cmd.Args[len(p.cmd.Args)-1][len("--input-ipc-server="):]))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("socket directory mode = %04o, want 0700", got)
	}
}

// A socket file left by a crashed run refuses the bind and looks exactly like
// a permissions problem, which is a bad hour for someone whose machine just
// lost power.
func TestAStaleSocketDoesNotBlockStartup(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "mpv.sock")
	if err := os.WriteFile(socket, []byte("left by a crash"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(stubEnv, "1")
	t.Setenv(reportEnv, t.TempDir())
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := Start(t.Context(), Options{Binary: self, Socket: socket})
	if err != nil {
		t.Fatalf("a stale socket stopped startup: %v", err)
	}
	_ = p.Close()
}

func TestCommandsSpeakMpvsProtocol(t *testing.T) {
	p, dir := stub(t)
	ctx := t.Context()

	for _, step := range []struct {
		do   func() error
		want string
	}{
		{func() error { return p.Load(ctx, "https://example.org/a") }, `["loadfile","https://example.org/a","replace"]`},
		{func() error { return p.Append(ctx, "https://example.org/b") }, `["loadfile","https://example.org/b","append"]`},
		{func() error { return p.SetPause(ctx, true) }, `["set_property","pause",true]`},
		{func() error { return p.Seek(ctx, -10*time.Second) }, `["seek",-10,"relative"]`},
		{func() error { return p.SetVolume(ctx, 55) }, `["set_property","volume",55]`},
		{func() error { return p.Observe(ctx, "time-pos") }, `["observe_property",1,"time-pos"]`},
		{func() error { return p.Stop(ctx) }, `["stop"]`},
	} {
		if err := step.do(); err != nil {
			t.Fatalf("%s: %v", step.want, err)
		}
	}

	got := commands(t, dir)
	for _, want := range []string{
		`["loadfile","https://example.org/a","replace"]`,
		`["loadfile","https://example.org/b","append"]`,
		`["set_property","pause",true]`,
		`["seek",-10,"relative"]`,
		`["set_property","volume",55]`,
		`["observe_property",1,"time-pos"]`,
		`["stop"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mpv never received %s\nreceived:\n%s", want, got)
		}
	}
}

// The caller is a keypress that can be held down.
func TestVolumeClamps(t *testing.T) {
	p, dir := stub(t)
	for _, v := range []int{-40, 250} {
		if err := p.SetVolume(t.Context(), v); err != nil {
			t.Fatal(err)
		}
	}
	got := commands(t, dir)
	if !strings.Contains(got, `["set_property","volume",0]`) || !strings.Contains(got, `["set_property","volume",100]`) {
		t.Errorf("volume was not clamped to 0 and 100:\n%s", got)
	}
}

func TestEventsReachTheCaller(t *testing.T) {
	p, _ := stub(t)
	if _, err := p.command(t.Context(), "shanty-emit", map[string]any{
		"event": "property-change", "name": "time-pos", "data": 12.5,
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-p.Events():
		if e.Name != "property-change" || e.Property != "time-pos" {
			t.Fatalf("event = %+v", e)
		}
		if pos, ok := e.Float(); !ok || pos != 12.5 {
			t.Errorf("position = %v, %v; want 12.5", pos, ok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
	}
}

// §7: a dead player is reported and offered back to the user, never respawned
// in a loop -- a loop turns "mpv is not installed" into a machine that is
// merely slow.
func TestADeadPlayerIsReportedAndNotRestarted(t *testing.T) {
	p, _ := stub(t)
	before := p.cmd.Process.Pid

	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the player did not report its own death")
	}
	if p.Err() == nil {
		t.Error("a dead player reports no reason")
	}
	if p.cmd.Process.Pid != before {
		t.Error("something restarted the player")
	}
	// And commands fail rather than hanging, so a keypress after a crash is a
	// message and not a frozen interface.
	if err := p.Load(t.Context(), "https://example.org/a"); err == nil {
		t.Error("a command against a dead player succeeded")
	}
}

func TestAMissingBinarySaysWhatToDo(t *testing.T) {
	_, err := Start(t.Context(), Options{
		Binary: filepath.Join(t.TempDir(), "not-mpv"),
		Socket: filepath.Join(t.TempDir(), "mpv.sock"),
	})
	if err == nil {
		t.Fatal("starting a binary that does not exist succeeded")
	}
	if !strings.Contains(err.Error(), "Install mpv") {
		t.Errorf("the failure does not say what to do:\n%s", err)
	}
}

// Commands from several goroutines must not interleave on the socket. Run
// under -race, which make check does.
func TestConcurrentCommandsDoNotInterleave(t *testing.T) {
	p, dir := stub(t)

	done := make(chan error, 8)
	for i := range 8 {
		go func() { done <- p.SetVolume(t.Context(), i*10) }()
	}
	for range 8 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	for line := range strings.SplitSeq(strings.TrimSpace(commands(t, dir)), "\n") {
		if line == "" {
			continue
		}
		var probe map[string]any
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("a line on the socket is not valid JSON, so two writes interleaved:\n%s", line)
		}
	}
}

// TestThePlayerRunsInItsOwnSession covers the ownership of the mpv process.
// mpv is put in a session of its own, so signals sent to the terminal's
// process group reach shanty and not the player, and Close is the only thing
// that stops it.
func TestThePlayerRunsInItsOwnSession(t *testing.T) {
	p, _ := stub(t)

	child := p.cmd.Process.Pid
	group, err := syscall.Getpgid(child)
	if err != nil {
		t.Fatal(err)
	}
	if group != child {
		t.Errorf("the player is in process group %d and leads none of its own; it should lead group %d", group, child)
	}

	ours, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if group == ours {
		t.Errorf("the player shares process group %d with the process that started it, so a signal to the terminal would reach both", ours)
	}
}

// TestALiveSocketIsNotDestroyed covers starting a second player while one is
// already running. The socket is a file, and removing it to make room for a
// new player would leave the running one unreachable for the rest of its life,
// still holding a credentialed stream URL.
func TestALiveSocketIsNotDestroyed(t *testing.T) {
	first, dir := stub(t)

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	socket := first.Socket()
	second, err := Start(t.Context(), Options{Binary: self, Socket: socket})
	if err == nil {
		_ = second.Close()
		t.Fatal("a second player started on a socket that was already in use")
	}
	var running ErrPlayerRunning
	if !errors.As(err, &running) {
		t.Fatalf("got %v, want an ErrPlayerRunning naming the socket", err)
	}
	if running.Socket != socket {
		t.Errorf("the error names %q, want %q", running.Socket, socket)
	}

	// The player that was already running is still usable.
	if err := first.SetVolume(t.Context(), 55); err != nil {
		t.Fatalf("the running player stopped answering: %v", err)
	}
	if log := report(t, dir, commandLog); !strings.Contains(log, `"volume"`) {
		t.Errorf("the running player did not receive the command; log was %q", log)
	}
}

// TestAttachDrivesAPlayerItDidNotStart covers connecting to a player this
// process did not launch, which is how a session and an interface hand the
// same mpv back and forth.
func TestAttachDrivesAPlayerItDidNotStart(t *testing.T) {
	started, dir := stub(t)

	attached, err := Attach(t.Context(), started.Socket())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = attached.Detach() }()

	if err := attached.SetVolume(t.Context(), 33); err != nil {
		t.Fatalf("the attached player could not send a command: %v", err)
	}
	if log := report(t, dir, commandLog); !strings.Contains(log, `"volume"`) {
		t.Errorf("the command did not reach mpv; log was %q", log)
	}

	// Detaching leaves the player running for whoever holds it next.
	if err := attached.Detach(); err != nil {
		t.Fatal(err)
	}
	if err := started.SetVolume(t.Context(), 44); err != nil {
		t.Errorf("detaching stopped the player it was only borrowing: %v", err)
	}
}

// TestClosingAnAttachedPlayerStopsIt covers the other half of Attach. Detach
// lets a borrowed player carry on; Close ends it, and has no process of its
// own to wait for.
func TestClosingAnAttachedPlayerStopsIt(t *testing.T) {
	started, dir := stub(t)

	attached, err := Attach(t.Context(), started.Socket())
	if err != nil {
		t.Fatal(err)
	}
	if err := attached.Close(); err != nil {
		t.Fatalf("closing an attached player: %v", err)
	}
	if log := report(t, dir, commandLog); !strings.Contains(log, `"quit"`) {
		t.Errorf("closing did not ask the player to quit; log was %q", log)
	}
}

// TestCancellingTheContextDoesNotStopThePlayer covers the other half of who
// owns mpv. The context bounds how long Start waits for the socket; it does
// not own the process afterwards, so a cancellation leaves the player running
// and Close is what ends it.
func TestCancellingTheContextDoesNotStopThePlayer(t *testing.T) {
	report := t.TempDir()
	socket := filepath.Join(t.TempDir(), "run", "mpv.sock")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stubEnv, "1")
	t.Setenv(reportEnv, report)

	ctx, cancel := context.WithCancel(t.Context())
	p, err := Start(ctx, Options{Binary: self, Socket: socket})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })

	cancel()
	select {
	case <-p.Done():
		t.Fatal("cancelling the context stopped the player")
	case <-time.After(100 * time.Millisecond):
	}
	if err := p.SetVolume(t.Context(), 60); err != nil {
		t.Fatalf("the player stopped answering after the context was cancelled: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	<-p.Done()
}

// TestClosingAPlayerThatWillNotQuitDoesNotHang covers a player that has
// stopped answering. Close asks it to quit, and the asking is bounded, so a
// player that never replies is killed rather than waited on for ever.
func TestClosingAPlayerThatWillNotQuitDoesNotHang(t *testing.T) {
	p, _ := stub(t, wedgedEnv+"=1")

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_ = p.Close()
		done <- time.Since(start)
	}()

	select {
	case took := <-done:
		if took > 5*stopGrace {
			t.Errorf("closing a wedged player took %v, which is not a bound", took)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("closing a player that will not quit never returned")
	}
}
