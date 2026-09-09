package mpv

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// The IPC format. mpv exchanges newline-delimited JSON: commands carry a
// request_id and are answered, everything else is an event.
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
	// Name is mpv’s event name, such as "end-file" or "property-change".
	Name string
	// Property is the property that changed, for a property-change event.
	Property string
	// Reason is why a track ended. "eof" means it played to the end.
	Reason string
	Data   json.RawMessage
}

// Float decodes a numeric property-change payload.
func (e Event) Float() (float64, bool) {
	var f float64
	if err := json.Unmarshal(e.Data, &f); err != nil {
		return 0, false
	}
	return f, true
}

// command sends one command and waits for its reply, the player stopping, or
// the context ending.
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

	// Writes are serialised separately from the pending map, so a blocked write
	// cannot stop the reader from dispatching replies.
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

// Load plays a track, replacing whatever is playing.
func (p *Player) Load(ctx context.Context, url string) error {
	_, err := p.command(ctx, "loadfile", url, "replace")
	return err
}

// Append adds a track after the current one, so mpv opens it early and the
// album plays without a gap.
func (p *Player) Append(ctx context.Context, url string) error {
	_, err := p.command(ctx, "loadfile", url, "append")
	return err
}

// Stop clears the playlist. mpv stays running and idle.
// Prefetch replaces whatever mpv has queued after the current track, so that
// it opens the right one in advance. An empty url leaves nothing queued.
//
// mpv is given the next track early to play an album without a gap, so
// changing what comes next has to change what mpv is holding, or it plays a
// moment of the track that used to be next.
func (p *Player) Prefetch(ctx context.Context, url string) error {
	if _, err := p.command(ctx, "playlist-clear"); err != nil {
		return err
	}
	if url == "" {
		return nil
	}
	_, err := p.command(ctx, "loadfile", url, "append")
	return err
}

func (p *Player) Stop(ctx context.Context) error {
	_, err := p.command(ctx, "stop")
	return err
}

func (p *Player) SetPause(ctx context.Context, paused bool) error {
	return p.setProperty(ctx, "pause", paused)
}

// SeekTo moves the position to d measured from the start of the track.
func (p *Player) SeekTo(ctx context.Context, d time.Duration) error {
	_, err := p.command(ctx, "seek", d.Seconds(), "absolute")
	return err
}

// Seek moves the position by d, forwards or back.
func (p *Player) Seek(ctx context.Context, d time.Duration) error {
	_, err := p.command(ctx, "seek", d.Seconds(), "relative")
	return err
}

// SetVolume sets the volume, clamped to 0 and 100.
func (p *Player) SetVolume(ctx context.Context, percent int) error {
	percent = max(0, min(100, percent))
	return p.setProperty(ctx, "volume", percent)
}

// SetReplayGain turns loudness levelling on or off.
//
// The gain is a number a tagger measured and wrote into the file, and mpv
// applies it as a volume change. A track without one is played untouched, so
// this is safe over a library that is only partly tagged.
func (p *Player) SetReplayGain(ctx context.Context, on bool) error {
	mode := "no"
	if on {
		mode = "track"
	}
	return p.setProperty(ctx, "replaygain", mode)
}

// Observe asks mpv to report a property whenever it changes, delivered on
// Events.
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
