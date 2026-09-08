package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/control"
	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/tui"
)

// sessionArg is the argument that runs a session. It is not in the command
// table and does not appear in help, because it is started by :headless rather
// than typed.
const sessionArg = "_session"

// sessionStart is how long a session is given to take the player over.
const sessionStart = 10 * time.Second

// detacher starts a session holding the given queue and returns once it has
// taken the player over.
func detacher(env Env) func(handoff) error {
	return func(h handoff) error {
		self, err := env.Executable()
		if err != nil {
			return err
		}
		doc, err := json.Marshal(h)
		if err != nil {
			return err
		}

		// The handoff travels on a real pipe rather than through os/exec's
		// copier, which runs on a goroutine that dies with this process. The
		// parent quits as soon as the session is up, so the copy would not
		// finish and the session would start with nothing to play.
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		defer func() { _ = w.Close() }()

		cmd := exec.Command(self, sessionArg)
		cmd.Stdin = r
		// The environment is inherited for the same reason mpv's is: the sound
		// server is found through it. Nothing of shanty's is added.
		cmd.Env = os.Environ()
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			_ = r.Close()
			return err
		}
		_ = r.Close()
		if _, err := w.Write(doc); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
		_ = w.Close()

		if err := waitForSession(env.Paths.ControlSocket()); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
		return nil
	}
}

// waitForSession returns once the session is answering on its socket.
func waitForSession(socket string) error {
	deadline := time.Now().Add(sessionStart)
	for {
		if _, err := control.Send(socket, control.Request{Verb: control.Status}); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("it did not answer within %s", sessionStart)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// runSession plays what the interface handed over, and answers commands until
// the queue runs out or it is told to stop.
func runSession(ctx context.Context, env Env) error {
	h, err := readHandoff(env.Stdin)
	if err != nil {
		return err
	}
	cfg, creds, err := loadOrSetUp(ctx, env)
	if err != nil {
		return err
	}
	client, err := dial(env, cfg, creds)
	if err != nil {
		return err
	}

	p, err := mpv.Attach(ctx, env.Paths.Socket())
	if err != nil {
		return err
	}
	// The player is stopped when the session ends, unless an interface has
	// taken it back.
	released := false
	defer func() {
		if released {
			_ = p.Detach()
			return
		}
		_ = p.Close()
	}()

	a := newApp(ctx, client, p)
	a.headless = true
	a.queue = h.resume()
	a.volume = h.Volume
	playing, _ := h.nowPlaying()
	ui, _ := a.ui.Update(playing)
	a.ui = ui.(tui.Model)

	program := tea.NewProgram(a, tea.WithContext(ctx),
		tea.WithoutRenderer(), tea.WithInput(nil))

	l, err := control.Listen(env.Paths.ControlSocket())
	if err != nil {
		return err
	}
	// The socket says a session is running, so it goes when the session does.
	defer func() {
		_ = l.Close()
		_ = os.Remove(env.Paths.ControlSocket())
	}()
	go func() { _ = control.Serve(l, commanded(program)) }()

	final, err := program.Run()
	if m, ok := final.(app); ok && m.released {
		released = true
	}
	return err
}
