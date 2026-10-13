package mpv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The stub is this test binary, re-executed.
//
// Graybeard row 16: a test that asks the machine whether mpv is installed
// passes where it was written and fails where the artifact is built. mpv is
// not on the machine this was written on either. So the binary is injectable,
// and everything below drives the real spawn, the real socket and the real
// protocol against a child that is guaranteed to exist because it is us.
const (
	stubEnv     = "SHANTY_TEST_STUB_MPV"
	wedgedEnv   = "SHANTY_TEST_STUB_WEDGED"
	reportEnv   = "SHANTY_TEST_STUB_REPORT"
	commandLog  = "commands.jsonl"
	argvFile    = "argv"
	environFile = "environ"
	// stubSays is the line the stub prints, shaped like mpv's own.
	stubSays = "AO: [stub] 44100Hz stereo 2ch s16"
)

func TestMain(m *testing.M) {
	if os.Getenv(stubEnv) == "1" {
		stubMain()
		return
	}
	os.Exit(m.Run())
}

// stubMain is a mpv-shaped process: it records what it was given, serves the
// IPC socket it was told to serve, and answers commands the way mpv does.
func stubMain() {
	dir := os.Getenv(reportEnv)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "stub:", err)
			os.Exit(2)
		}
	}
	write(argvFile, strings.Join(os.Args, "\n"))
	write(environFile, strings.Join(os.Environ(), "\n"))

	// mpv announces the output it opened on standard output, not standard
	// error, which is what Options.Diagnostics has to be wired to.
	fmt.Println(stubSays)

	var socket string
	for _, arg := range os.Args {
		if s, found := strings.CutPrefix(arg, "--input-ipc-server="); found {
			socket = s
		}
	}
	if socket == "" {
		os.Exit(3)
	}

	ln, err := net.Listen("unix", socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stub:", err)
		os.Exit(4)
	}
	log, err := os.Create(filepath.Join(dir, commandLog))
	if err != nil {
		os.Exit(5)
	}

	var mu sync.Mutex
	for {
		conn, err := ln.Accept()
		if err != nil {
			os.Exit(0)
		}
		go func() {
			scan := bufio.NewScanner(conn)
			for scan.Scan() {
				line := scan.Bytes()
				mu.Lock()
				fmt.Fprintf(log, "%s\n", line)
				mu.Unlock()

				var msg struct {
					Command   []any `json:"command"`
					RequestID int   `json:"request_id"`
				}
				if err := json.Unmarshal(line, &msg); err != nil {
					continue
				}
				if len(msg.Command) > 0 && msg.Command[0] == "quit" {
					// A wedged player reads the command and does nothing,
					// which is what Close has to survive.
					if os.Getenv(wedgedEnv) == "1" {
						continue
					}
					os.Exit(0)
				}
				// A test-only command, so the event path can be driven
				// without waiting on a real track to end.
				if len(msg.Command) == 2 && msg.Command[0] == "shanty-emit" {
					raw, _ := json.Marshal(msg.Command[1])
					mu.Lock()
					fmt.Fprintf(conn, "%s\n", raw)
					mu.Unlock()
				}
				out, _ := json.Marshal(map[string]any{
					"error": "success", "request_id": msg.RequestID, "data": nil,
				})
				mu.Lock()
				fmt.Fprintf(conn, "%s\n", out)
				mu.Unlock()
			}
		}()
	}
}

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

// stub starts a player backed by the re-executed test binary and returns the
// directory it reports into.
func stub(t *testing.T, extraEnv ...string) (*Player, string) {
	p, report, _ := stubSaying(t, extraEnv...)
	return p, report
}

// stubSaying is stub, and also hands back what the child wrote to the stream
// Diagnostics is wired to.
func stubSaying(t *testing.T, extraEnv ...string) (*Player, string, *collected) {
	t.Helper()
	notes := &collected{}
	report := t.TempDir()
	socket := filepath.Join(shortDir(t), "run", "mpv.sock")

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stubEnv, "1")
	t.Setenv(reportEnv, report)
	for _, kv := range extraEnv {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}

	p, err := Start(t.Context(), Options{Binary: self, Socket: socket, Diagnostics: notes})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p, report, notes
}

// collected keeps what the child process wrote, safely: the copy runs on a
// goroutine of os/exec's and the assertion runs on the test's.
type collected struct {
	mu sync.Mutex
	b  strings.Builder
}

func (c *collected) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.Write(p)
}

func (c *collected) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}

func report(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// commands returns every IPC line the stub received, retried because the write
// and the assertion are in different processes.
func commands(t *testing.T, dir string) string {
	t.Helper()
	var last string
	for range 200 {
		b, err := os.ReadFile(filepath.Join(dir, commandLog))
		if err == nil {
			last = string(b)
			if last != "" {
				return last
			}
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		default:
		}
		waitABit()
	}
	return last
}
