// Package control carries commands to a session that has outlived its
// terminal.
//
// One request and one response per connection, as JSON on a single line each.
// The verbs are a closed set, so a session can only be asked to do things it
// already knows how to do.
package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Protocol is the version of the wire format. It changes when the shape of a
// request or a response changes, not when shanty is released.
const Protocol = 1

// timeout bounds every exchange. A session that has stopped answering must not
// hold up the command line.
const timeout = 3 * time.Second

// Verb is one thing a session can be asked to do.
type Verb string

const (
	Pause  Verb = "pause"
	Next   Verb = "next"
	Prev   Verb = "prev"
	Volume Verb = "volume"
	Seek   Verb = "seek"
	Status Verb = "status"
	Stop   Verb = "stop"
)

// verbs is every verb, with whether it takes an argument. The set is closed:
// a verb that is not here is refused before it reaches the session.
var verbs = map[Verb]bool{
	Pause: false, Next: false, Prev: false,
	Volume: true, Seek: true,
	Status: false, Stop: false,
}

// Known reports whether the verb is one a session accepts.
func Known(v Verb) bool { _, ok := verbs[v]; return ok }

// TakesArgument reports whether the verb needs something after it.
func TakesArgument(v Verb) bool { return verbs[v] }

// Request is one command sent to a session.
type Request struct {
	Protocol int    `json:"protocol"`
	Verb     Verb   `json:"verb"`
	Arg      string `json:"arg,omitempty"`
}

// Response is what the session sends back.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	State *State `json:"state,omitempty"`

	// After runs once the response has reached the other end. A session that
	// is stopping uses it, so that it does not exit before answering the
	// command that asked it to.
	After func() `json:"-"`
}

// State is what the session is doing, as it should be displayed.
type State struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Paused   bool   `json:"paused"`
	Position string `json:"position"`
	Duration string `json:"duration"`
	Volume   int    `json:"volume"`
	Track    int    `json:"track"`
	Of       int    `json:"of"`
}

// ErrNoSession reports that nothing is listening on the socket.
type ErrNoSession struct{ Socket string }

func (e ErrNoSession) Error() string { return "no session is playing" }

// Handler performs a request and describes the result.
type Handler func(Request) Response

// Serve answers requests until the listener is closed. Each connection carries
// one request and receives one response.
func Serve(l net.Listener, h Handler) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go answer(conn, h)
	}
}

// answer reads one request, hands it to the handler and writes the response.
// A request from a different protocol version is refused without reaching the
// handler.
func answer(conn net.Conn, h Handler) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	var req Request
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	if err := json.Unmarshal(line, &req); err != nil {
		write(conn, Response{Error: "the request could not be read"})
		return
	}
	switch {
	case req.Protocol != Protocol:
		write(conn, Response{Error: fmt.Sprintf(
			"this session speaks version %d and the command speaks version %d\n\nRun `shanty stop` and start it again",
			Protocol, req.Protocol)})
	case !Known(req.Verb):
		write(conn, Response{Error: fmt.Sprintf("a session cannot %q", string(req.Verb))})
	default:
		res := h(req)
		write(conn, res)
		// The connection is closed before After runs, so a session that stops
		// itself there has already answered.
		_ = conn.Close()
		if res.After != nil {
			res.After()
		}
	}
}

func write(conn net.Conn, res Response) {
	b, err := json.Marshal(res)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(b, '\n'))
}

// Send delivers one request to the session listening on the socket. A socket
// nothing answers on is removed, because it was left by a session that is gone.
func Send(socket string, req Request) (Response, error) {
	req.Protocol = Protocol
	conn, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Response{}, ErrNoSession{Socket: socket}
		}
		_ = os.Remove(socket)
		return Response{}, ErrNoSession{Socket: socket}
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	b, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return Response{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return Response{}, fmt.Errorf("the session did not answer: %w", err)
	}
	var res Response
	if err := json.Unmarshal(line, &res); err != nil {
		return Response{}, errors.New("the session's answer could not be read")
	}
	return res, nil
}

// Listen opens the control socket, replacing one left by a session that is
// gone. It refuses to displace a session that is still answering.
func Listen(socket string) (net.Listener, error) {
	if conn, err := net.DialTimeout("unix", socket, timeout); err == nil {
		_ = conn.Close()
		return nil, errors.New("a session is already playing\n\nRun `shanty stop` to end it")
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	// The directory is already 0700. The socket is narrowed to match, because
	// the mode it is created with depends on the umask.
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}
