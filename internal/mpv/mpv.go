// Package mpv runs one mpv for the life of the program, over a unix socket.
//
// The socket is a security boundary, not a convenience: a stream URL carries
// the credential, and a URL on a command line is readable by every account on
// the machine through ps, which the audit found a shipping client doing. Every
// track is loaded over IPC, so the URL is in mpv's memory and nowhere
// inspectable (§7).
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
	"time"
)

// Long enough for a loaded machine, short enough that a broken install is a
// message rather than a hang.
const startTimeout = 10 * time.Second

// Options configure the one player. No field for extra mpv flags: §3 makes
// shanty not a manager of anyone's mpv config, and a passthrough is how that
// promise gets broken one bug report at a time.
type Options struct {
	// Binary is the mpv to run. Empty means "mpv", resolved on PATH.
	Binary string
	// Socket is where mpv listens. Its directory is created 0700.
	Socket string
	// Stderr is where mpv's own diagnostics go. Nil discards them, which is
	// right for a TUI that owns the screen -- but a player that fails without
	// saying why leaves nothing to report, so doctor and the integration test
	// pass a buffer.
	Stderr io.Writer
}

// Player is a running mpv.
type Player struct {
	cmd  *exec.Cmd
	conn net.Conn

	mu       sync.Mutex
	nextID   int
	observed int
	pending  map[int]chan reply

	writeMu sync.Mutex

	events chan Event

	// done closes when the player stops, and cause says why. Nothing restarts
	// it: a loop turns "mpv is not installed" into a machine that is merely
	// slow (§7).
	done     chan struct{}
	closeOne sync.Once
	causeMu  sync.Mutex
	cause    error
}

// Start launches mpv and connects to it. The flags are fixed: --no-config
// leaves the user's own setup as it was found, --idle waits for a track rather
// than exiting, and the prefetch makes gapless playback mpv's job, not ours.
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
	// A socket left by a previous run refuses the bind and looks exactly like
	// a permissions problem.
	if err := os.Remove(opt.Socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binary,
		"--no-config",
		"--no-video",
		"--idle=yes",
		"--prefetch-playlist=yes",
		"--input-ipc-server="+opt.Socket,
	)
	// The environment is inherited because audio depends on it: PipeWire and
	// PulseAudio read XDG_RUNTIME_DIR, ALSA reads HOME. Nothing of shanty's is
	// added -- the credential goes over the socket, which is the point of it.
	cmd.Env = os.Environ()
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

	p := &Player{
		cmd:     cmd,
		conn:    conn,
		pending: map[int]chan reply{},
		events:  make(chan Event, 64),
		done:    make(chan struct{}),
	}
	go p.read()
	go p.reap()
	return p, nil
}

// dial waits for mpv to bind the socket, which it does after startup: the
// first several failures are the normal case, not an error worth reporting.
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

// reap records why the process ended, so a caller gets the exit status rather
// than a closed socket.
func (p *Player) reap() {
	err := p.cmd.Wait()
	if err == nil {
		err = errors.New("mpv exited")
	}
	p.stop(fmt.Errorf("mpv stopped: %w", err))
}

// read demultiplexes: replies go to whoever waits on the request id, the rest
// are events.
func (p *Player) read() {
	scan := bufio.NewScanner(p.conn)
	scan.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scan.Scan() {
		var msg message
		if err := json.Unmarshal(scan.Bytes(), &msg); err != nil {
			// A line we cannot parse is mpv's problem, not a reason to stop
			// listening to the ones we can.
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

// stop keeps the first cause: the socket closing because the process died is
// not news.
func (p *Player) stop(cause error) {
	p.closeOne.Do(func() {
		p.causeMu.Lock()
		p.cause = cause
		p.causeMu.Unlock()
		close(p.done)
		close(p.events)
	})
}

// Done closes when the player stops. Nothing here restarts it.
func (p *Player) Done() <-chan struct{} { return p.done }

// Err is why the player stopped, or nil while it is running.
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

// Events carries mpv's notifications. It closes when the player stops.
func (p *Player) Events() <-chan Event { return p.events }

// Close shuts mpv down and waits for it.
func (p *Player) Close() error {
	// Ask, then close the socket, then kill. Each step is unchecked because
	// the next covers it and only the process ending matters.
	_, _ = p.command(context.Background(), "quit")
	_ = p.conn.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	<-p.done
	return nil
}
