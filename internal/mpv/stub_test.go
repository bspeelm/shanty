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

// stub starts a player backed by the re-executed test binary and returns the
// directory it reports into.
func stub(t *testing.T, extraEnv ...string) (*Player, string) {
	t.Helper()
	report := t.TempDir()
	socket := filepath.Join(t.TempDir(), "run", "mpv.sock")

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

	p, err := Start(t.Context(), Options{Binary: self, Socket: socket})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p, report
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
