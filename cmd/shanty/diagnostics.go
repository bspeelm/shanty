package main

import (
	"bytes"
	"strings"
	"sync"
)

// diagnostics keeps the last few lines mpv wrote to its standard error. mpv is
// the only thing that can say why its audio output never started, and that is
// where it says it.
//
// Nothing is written to disk. A stream URL carries the credential and mpv
// names the stream it is opening, so every URL has its query removed before a
// line is kept.
type diagnostics struct {
	mu    sync.Mutex
	lines []string
	part  []byte
}

const (
	// kept is how many lines are held. mpv's account of a failure is the last
	// thing it says before it stops saying anything.
	kept = 5
	// partial is the longest run of bytes held while waiting for a newline.
	// Beyond it the run is dropped, so a process writing without newlines
	// cannot grow this without limit.
	partial = 8 << 10
)

// Write keeps whole lines and discards the rest.
func (d *diagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.part = append(d.part, p...)
	// mpv ends its status line with a carriage return and its messages with a
	// newline, so both count as the end of a line.
	for {
		at := bytes.IndexAny(d.part, "\r\n")
		if at < 0 {
			break
		}
		if line := redact(string(d.part[:at])); line != "" {
			d.lines = append(d.lines, line)
			if len(d.lines) > kept {
				d.lines = d.lines[len(d.lines)-kept:]
			}
		}
		d.part = d.part[at+1:]
	}
	if len(d.part) > partial {
		d.part = d.part[:0]
	}
	return len(p), nil
}

// said returns the lines held, oldest first.
func (d *diagnostics) said() []string {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.lines...)
}

// redact removes the query from every URL in a line and collapses the spacing.
// Everything shanty asks of a server is a query parameter, the credential
// among them.
func redact(line string) string {
	fields := strings.Fields(line)
	for i, field := range fields {
		if !strings.Contains(field, "://") {
			continue
		}
		if at := strings.Index(field, "?"); at >= 0 {
			fields[i] = field[:at] + "?<redacted>"
		}
	}
	return strings.Join(fields, " ")
}
