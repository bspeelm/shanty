// Package mpv starts one mpv process and controls it over a unix socket.
//
// Tracks are loaded over the socket rather than passed as command-line
// arguments, because a stream URL contains the credential.
package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// startTimeout is how long mpv is given to create its socket.
const startTimeout = 10 * time.Second

// Options configure the player. There is no field for extra mpv flags.
type Options struct {
	// Binary is the mpv executable. Empty means "mpv", resolved on PATH.
	Binary string
	// Socket is the path mpv listens on. Its directory is created 0700.
	Socket string
	// Stderr receives mpv’s diagnostics. Nil discards them.
	Stderr io.Writer
}

// Player is a running mpv process.
type Player struct {
	cmd    *exec.Cmd
	conn   net.Conn
	socket string

	mu       sync.Mutex
	nextID   int
	observed int
	pending  map[int]chan reply

	writeMu sync.Mutex

	events chan Event

	// done closes when the player stops, and cause records why. Nothing restarts
	// it.
	done     chan struct{}
	closeOne sync.Once
	causeMu  sync.Mutex
	cause    error
}

// Start launches mpv and connects to its socket. The flags are fixed:
// --no-config so the user’s own mpv configuration is not read, --no-video,
// --idle so it waits for a track, and --prefetch-playlist so the next track
// is opened early.
func Start(ctx context.Context, opt Options) (*Player, error) {
	binary := opt.Binary
	if binary == "" {
		binary = "mpv"
	}
	if opt.Socket == "" {
		return nil, errors.New("no IPC socket path was given")
	}
	if err := os.MkdirAll(filepath.Dir(opt.Socket), 0o700); err != nil {
		return nil, err
	}
	if err := clearSocket(opt.Socket); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binary,
		"--no-config",
		"--no-video",
		"--idle=yes",
		"--prefetch-playlist=yes",
		"--input-ipc-server="+opt.Socket,
	)
	// The environment is inherited because PipeWire, PulseAudio and ALSA read
	// from it. Nothing of shanty’s is added.
	cmd.Env = os.Environ()
	// mpv runs in its own session, so the terminal’s signals do not reach it
	// and Close is the only thing that stops it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stderr = opt.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start %s: %w\nInstall mpv, or set its path in config.toml", binary, err)
	}

	conn, err := dial(ctx, opt.Socket)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	p := newPlayer(cmd, conn, opt.Socket)
	go p.read()
	go p.reap()
	return p, nil
}

// newPlayer builds a Player around an open connection. cmd is nil for a player
// this process did not start.
func newPlayer(cmd *exec.Cmd, conn net.Conn, socket string) *Player {
	return &Player{
		cmd:     cmd,
		conn:    conn,
		socket:  socket,
		pending: map[int]chan reply{},
		events:  make(chan Event, 64),
		done:    make(chan struct{}),
	}
}

// ErrPlayerRunning reports that a player is already listening on the socket.
// The path is carried so the caller can name it.
type ErrPlayerRunning struct{ Socket string }

func (e ErrPlayerRunning) Error() string {
	return "a player is already running on " + e.Socket
}

// clearSocket removes a socket left behind by a player that is gone. A socket
// something is still listening on is reported as ErrPlayerRunning and left
// alone.
func clearSocket(socket string) error {
	if conn, err := net.Dial("unix", socket); err == nil {
		_ = conn.Close()
		return ErrPlayerRunning{Socket: socket}
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Attach connects to a player that is already running on the socket. The
// returned Player controls it in every way Start's does, except that it did
// not launch the process and so has none to wait for.
func Attach(ctx context.Context, socket string) (*Player, error) {
	if socket == "" {
		return nil, errors.New("no IPC socket path was given")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("no player is listening on %s: %w", socket, err)
	}
	p := newPlayer(nil, conn, socket)
	go p.read()
	return p, nil
}

// dial connects to the socket, retrying until mpv has created it or
// startTimeout passes.
func dial(ctx context.Context, socket string) (net.Conn, error) {
	deadline := time.Now().Add(startTimeout)
	for {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("mpv did not open %s within %s: %w", socket, startTimeout, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// reap waits for the process and records why it ended.
func (p *Player) reap() {
	err := p.cmd.Wait()
	if err == nil {
		err = errors.New("mpv exited")
	}
	p.stop(fmt.Errorf("mpv stopped: %w", err))
}

// read dispatches replies to whoever is waiting on the request id, and
// everything else to Events.
func (p *Player) read() {
	scan := bufio.NewScanner(p.conn)
	scan.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scan.Scan() {
		var msg message
		if err := json.Unmarshal(scan.Bytes(), &msg); err != nil {
			// An unparseable line is skipped.
			continue
		}
		if msg.Event != "" {
			select {
			case p.events <- Event{Name: msg.Event, Property: msg.Name, Reason: msg.Reason, Data: msg.Data}:
			default: // A slow reader loses events rather than blocking playback.
			}
			continue
		}
		p.mu.Lock()
		ch, waiting := p.pending[msg.RequestID]
		delete(p.pending, msg.RequestID)
		p.mu.Unlock()
		if waiting {
			ch <- reply{Error: msg.Error, Data: msg.Data}
		}
	}
	p.stop(errors.New("the connection to mpv closed"))
}

// stop records the first cause and releases everyone waiting.
func (p *Player) stop(cause error) {
	p.closeOne.Do(func() {
		p.causeMu.Lock()
		p.cause = cause
		p.causeMu.Unlock()
		close(p.done)
		close(p.events)
	})
}

// Socket is the path of the socket this player is controlled through.
func (p *Player) Socket() string { return p.socket }

// Done closes when the player stops.
func (p *Player) Done() <-chan struct{} { return p.done }

// Err returns why the player stopped, or nil while it runs.
func (p *Player) Err() error {
	select {
	case <-p.done:
		p.causeMu.Lock()
		defer p.causeMu.Unlock()
		return p.cause
	default:
		return nil
	}
}

// Events carries mpv’s notifications. It closes when the player stops.
func (p *Player) Events() <-chan Event { return p.events }

// Close stops mpv and waits for it to exit.
func (p *Player) Close() error {
	// Ask, then close the socket, then kill. Each step is unchecked because the
	// next covers it.
	_, _ = p.command(context.Background(), "quit")
	_ = p.conn.Close()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	<-p.done
	return nil
}

// Detach closes the connection and leaves mpv running. The Player is finished
// afterwards and the process it was controlling carries on playing.
func (p *Player) Detach() error {
	_ = p.conn.Close()
	<-p.done
	return nil
}
