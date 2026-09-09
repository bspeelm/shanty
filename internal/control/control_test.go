package control

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shortDir is a directory whose path is short enough to hold a socket.
//
// A unix socket path is limited to around a hundred bytes. A directory from
// t.TempDir is named after the test and sits under whatever TMPDIR holds,
// which on macOS exceeds the limit before a socket name is added.
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

// serve runs a handler on a socket in a temporary directory and returns its
// path. The listener is closed when the test ends.
func serve(t *testing.T, h Handler) string {
	t.Helper()
	socket := filepath.Join(shortDir(t), "control.sock")
	l, err := Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() { _ = Serve(l, h) }()
	return socket
}

// echoes answers every request by naming the verb it received.
func echoes(seen *[]Request) Handler {
	return func(req Request) Response {
		*seen = append(*seen, req)
		return Response{OK: true, State: &State{Title: string(req.Verb), Volume: 80}}
	}
}

func TestEveryVerbReachesTheSession(t *testing.T) {
	var seen []Request
	socket := serve(t, echoes(&seen))

	for _, verb := range Typed() {
		arg := ""
		if TakesArgument(verb) {
			arg = "40"
		}
		res, err := Send(socket, Request{Verb: verb, Arg: arg})
		if err != nil {
			t.Fatalf("%s: %v", verb, err)
		}
		if !res.OK {
			t.Errorf("%s was refused: %s", verb, res.Error)
		}
		if res.State == nil || res.State.Title != string(verb) {
			t.Errorf("%s: the session answered about something else: %+v", verb, res.State)
		}
	}
	if len(seen) != len(Typed()) {
		t.Errorf("the session saw %d requests and %d verbs were sent", len(seen), len(Typed()))
	}
}

func TestTheArgumentSurvivesTheJourney(t *testing.T) {
	var seen []Request
	socket := serve(t, echoes(&seen))

	if _, err := Send(socket, Request{Verb: Volume, Arg: "40"}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].Arg != "40" {
		t.Fatalf("the session received %+v, want the argument 40", seen)
	}
}

// TestAVerbTheSessionDoesNotKnowIsRefusedBeforeItArrives covers the closed
// set. A session must not be asked to do something that is not on the list,
// even by a command built against a different version.
func TestAVerbTheSessionDoesNotKnowIsRefusedBeforeItArrives(t *testing.T) {
	var seen []Request
	socket := serve(t, echoes(&seen))

	res, err := Send(socket, Request{Verb: "delete-everything"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Error("a verb that is not in the set was accepted")
	}
	if !strings.Contains(res.Error, "delete-everything") {
		t.Errorf("the refusal does not name the verb: %q", res.Error)
	}
	if len(seen) != 0 {
		t.Errorf("the handler was given %d requests it should never have seen", len(seen))
	}
}

// TestASessionFromAnotherVersionIsRefusedWithSomethingToDo covers a command
// meeting a session started by a different build. The refusal has to name the
// way out, because the session is running code this binary cannot change.
func TestASessionFromAnotherVersionIsRefusedWithSomethingToDo(t *testing.T) {
	var seen []Request
	socket := serve(t, echoes(&seen))

	// Send bypasses the version field, so the request is written by hand.
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	b, _ := json.Marshal(Request{Protocol: Protocol + 1, Verb: Pause})
	if _, err := conn.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
	var res Response
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		t.Fatal(err)
	}

	if res.OK {
		t.Fatal("a request from another protocol version was accepted")
	}
	if !strings.Contains(res.Error, "shanty stop") {
		t.Errorf("the refusal does not say what to do about it: %q", res.Error)
	}
	if len(seen) != 0 {
		t.Errorf("the handler was reached by a request it cannot understand")
	}
}

func TestAMalformedRequestIsAnsweredRatherThanIgnored(t *testing.T) {
	var seen []Request
	socket := serve(t, echoes(&seen))

	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("this is not JSON\n")); err != nil {
		t.Fatal(err)
	}
	var res Response
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		t.Fatalf("a malformed request got no answer at all: %v", err)
	}
	if res.OK {
		t.Error("a malformed request was accepted")
	}
	if len(seen) != 0 {
		t.Error("a malformed request reached the handler")
	}
}

// TestNoSessionIsDistinctFromAFailure covers the common case: the socket is
// not there because nothing is playing. That is a state to report, not an
// error to diagnose.
func TestNoSessionIsDistinctFromAFailure(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "control.sock")

	_, err := Send(socket, Request{Verb: Pause})
	var none ErrNoSession
	if !errors.As(err, &none) {
		t.Fatalf("got %v, want ErrNoSession", err)
	}
	if strings.Contains(err.Error(), socket) {
		t.Errorf("the message shows a path the reader did not ask about: %q", err)
	}
}

// TestASocketLeftByADeadSessionIsCleanedUp covers the crash case. A session
// killed outright leaves its socket behind, and the next command must not
// mistake the file for something that is listening.
func TestASocketLeftByADeadSessionIsCleanedUp(t *testing.T) {
	dir := shortDir(t)
	socket := filepath.Join(dir, "control.sock")

	l, err := Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	// Close the listener without removing the file, which is what a killed
	// session leaves behind.
	_ = l.Close()
	if err := writeStale(socket); err != nil {
		t.Fatal(err)
	}

	if _, err := Send(socket, Request{Verb: Pause}); !errors.As(err, &ErrNoSession{}) {
		t.Fatalf("got %v, want ErrNoSession", err)
	}
	if _, err := Listen(socket); err != nil {
		t.Fatalf("a stale socket stopped a new session from starting: %v", err)
	}
}

// TestASessionThatIsAnsweringIsNotDisplaced covers two sessions. The socket is
// what says who owns the player, so a second session must not take it.
func TestASessionThatIsAnsweringIsNotDisplaced(t *testing.T) {
	var seen []Request
	socket := serve(t, echoes(&seen))

	if _, err := Listen(socket); err == nil {
		t.Fatal("a second session took the socket from one that was answering")
	} else if !strings.Contains(err.Error(), "shanty stop") {
		t.Errorf("the refusal does not say what to do about it: %q", err)
	}

	// The session that was already there still answers.
	if res, err := Send(socket, Request{Verb: Status}); err != nil || !res.OK {
		t.Errorf("the running session stopped answering: %v %+v", err, res)
	}
}

// writeStale puts a plain file where the socket was, which is what remains
// after a session is killed.
func writeStale(socket string) error {
	return os.WriteFile(socket, []byte("left by a killed session"), 0o600)
}

// TestTheResponseIsWrittenBeforeAfterRuns covers a session stopping itself.
// The command that asked has to be answered before the session goes, or a
// stop that worked is reported as a session that would not answer.
func TestTheResponseIsWrittenBeforeAfterRuns(t *testing.T) {
	// Run it many times, because the failure is a race.
	for i := 0; i < 200; i++ {
		answered := make(chan bool, 1)
		socket := serve(t, func(Request) Response {
			return Response{OK: true, After: func() { answered <- true }}
		})

		res, err := Send(socket, Request{Verb: Stop})
		if err != nil {
			t.Fatalf("run %d: the session did not answer the command that stopped it: %v", i, err)
		}
		if !res.OK {
			t.Fatalf("run %d: %s", i, res.Error)
		}
		select {
		case <-answered:
		case <-time.After(time.Second):
			t.Fatalf("run %d: the session answered but never acted", i)
		}
	}
}

// TestAStoppingSessionAnswersFirst pins the ordering. After must not run
// until the response has been written, so a session that stops itself there
// has already told the command it worked. If the order were reversed, an
// After that takes any time at all would delay or lose the answer.
func TestAStoppingSessionAnswersFirst(t *testing.T) {
	release := make(chan struct{})
	socket := serve(t, func(Request) Response {
		return Response{OK: true, After: func() { <-release }}
	})
	defer close(release)

	done := make(chan Response, 1)
	go func() {
		res, err := Send(socket, Request{Verb: Stop})
		if err == nil {
			done <- res
		}
	}()

	select {
	case res := <-done:
		if !res.OK {
			t.Errorf("the session refused: %s", res.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the answer never arrived, because the session acted before it replied")
	}
}

// TestAControlSocketPathTooLongIsRefusedWithTheReason covers the same limit on
// the other socket. Both are in the same directory, so they fail together.
func TestAControlSocketPathTooLongIsRefusedWithTheReason(t *testing.T) {
	long := filepath.Join(t.TempDir(), strings.Repeat("d", 120), "control.sock")

	_, err := Listen(long)
	if err == nil {
		t.Fatal("a socket path over the limit was accepted")
	}
	if !strings.Contains(err.Error(), "over") || !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Errorf("the message reads %v", err)
	}
}
