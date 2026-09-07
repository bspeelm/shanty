package mpv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	// A credential in the parent's environment, to prove the test would
	// notice one: shanty adds nothing of its own, and this asserts the search
	// is looking in the right place.
	p, dir := stub(t, "SHANTY_TEST_CANARY="+fakeToken)

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

	// The canary proves the environment is genuinely being searched; shanty's
	// own credential is what must not be in it, and it is not, because it
	// went over the socket instead.
	environ := report(t, dir, environFile)
	if !strings.Contains(environ, "SHANTY_TEST_CANARY="+fakeToken) {
		t.Fatal("the canary is missing from the child's environment; this test is not searching anything")
	}
	for _, secret := range []string{fakeSalt, "stream.view"} {
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
		// NUL-separated; the token would survive either way.
		if strings.Contains(string(raw), fakeSalt) || strings.Contains(string(raw), "stream.view") {
			t.Errorf("/proc/%d/%s carries the stream URL", pid, f)
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
