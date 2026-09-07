package mpv

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// The IPC wire format. mpv speaks newline-delimited JSON in both directions:
// commands carry a request_id and are answered, everything else is an event.
type message struct {
	Event     string          `json:"event"`
	Name      string          `json:"name"`
	Reason    string          `json:"reason"`
	Data      json.RawMessage `json:"data"`
	Error     string          `json:"error"`
	RequestID int             `json:"request_id"`
}

type reply struct {
	Error string
	Data  json.RawMessage
}

// Event is one notification from mpv.
type Event struct {
	// Name is mpv's event name: "end-file" when a track finishes,
	// "property-change" when something we asked to watch moved.
	Name string
	// Property is which one moved, for a property-change.
	Property string
	// Reason distinguishes an end-file that reached the end of the track from
	// one we caused by skipping, which is the difference between advancing the
	// queue and having already advanced it.
	Reason string
	Data   json.RawMessage
}

// Float reads a property-change payload, for the numeric ones -- position and
// volume are the two that matter here.
func (e Event) Float() (float64, bool) {
	var f float64
	if err := json.Unmarshal(e.Data, &f); err != nil {
		return 0, false
	}
	return f, true
}

// command sends one command and waits for its reply, the player stopping, or
// the context ending -- whichever comes first.
func (p *Player) command(ctx context.Context, args ...any) (json.RawMessage, error) {
	if err := p.Err(); err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.nextID++
	id := p.nextID
	ch := make(chan reply, 1)
	p.pending[id] = ch
	p.mu.Unlock()

	forget := func() {
		p.mu.Lock()
		delete(p.pending, id)
		p.mu.Unlock()
	}

	raw, err := json.Marshal(struct {
		Command   []any `json:"command"`
		RequestID int   `json:"request_id"`
	}{Command: args, RequestID: id})
	if err != nil {
		forget()
		return nil, err
	}

	// Writes are serialised separately from the pending map, so a write that
	// blocks cannot stop the reader from dispatching the replies that would
	// unblock it.
	p.writeMu.Lock()
	_, err = p.conn.Write(append(raw, '\n'))
	p.writeMu.Unlock()
	if err != nil {
		forget()
		return nil, err
	}

	select {
	case r := <-ch:
		if r.Error != "" && r.Error != "success" {
			return nil, fmt.Errorf("mpv refused %v: %s", args[0], r.Error)
		}
		return r.Data, nil
	case <-p.done:
		return nil, p.Err()
	case <-ctx.Done():
		forget()
		return nil, ctx.Err()
	}
}

// Load starts a track, replacing whatever was playing.
//
// This is the only way a URL reaches mpv, and it is why the socket exists: the
// URL carries the credential, and going over IPC keeps it out of argv where
// every account on the machine could read it (§7).
func (p *Player) Load(ctx context.Context, url string) error {
	_, err := p.command(ctx, "loadfile", url, "replace")
	return err
}

// Append queues a track behind the current one. Together with
// --prefetch-playlist this is what makes playback gapless: mpv opens the next
// track early, and shanty decodes nothing.
func (p *Player) Append(ctx context.Context, url string) error {
	_, err := p.command(ctx, "loadfile", url, "append")
	return err
}

// Stop clears the playlist without stopping mpv, which stays idle for the next
// track.
func (p *Player) Stop(ctx context.Context) error {
	_, err := p.command(ctx, "stop")
	return err
}

func (p *Player) SetPause(ctx context.Context, paused bool) error {
	return p.setProperty(ctx, "pause", paused)
}

func (p *Player) Seek(ctx context.Context, d time.Duration) error {
	_, err := p.command(ctx, "seek", d.Seconds(), "relative")
	return err
}

// SetVolume takes 0 to 100 and clamps, because the caller is a keypress that
// can be held down.
func (p *Player) SetVolume(ctx context.Context, percent int) error {
	percent = max(0, min(100, percent))
	return p.setProperty(ctx, "volume", percent)
}

// Observe asks mpv to report a property whenever it changes, delivered on
// Events. "time-pos" is what a progress bar reads.
func (p *Player) Observe(ctx context.Context, property string) error {
	p.mu.Lock()
	p.observed++
	id := p.observed
	p.mu.Unlock()
	_, err := p.command(ctx, "observe_property", id, property)
	return err
}

func (p *Player) setProperty(ctx context.Context, name string, value any) error {
	_, err := p.command(ctx, "set_property", name, value)
	return err
}
