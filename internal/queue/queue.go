// Package queue is the play queue, as pure functions over data.
//
// The strange bugs music players accumulate here -- a track that plays twice,
// a skip that loses the album -- are not hard to reason about; they are hard
// to reproduce, because the queue is usually mutable state shared between a UI
// thread and a playback callback. Here it is a value, so each of those bugs is
// a property a test checks without a player, a socket, or a clock (§4).
package queue

import "time"

// Track is this package's own type: importing the Subsonic package would drag
// net/http into the one place meant to be provable by reading it.
type Track struct {
	ID       string
	Title    string
	Album    string
	Artist   string
	Duration time.Duration
}

// Queue is a list of tracks and a position in it. The position may be one past
// the last track: a queue that clamped would leave the last track current
// forever, and nothing could tell playing from finished.
type Queue struct {
	tracks []Track
	at     int
}

// New copies the slice: a caller keeping a reference could otherwise change a
// queue already handed out.
func New(tracks ...Track) Queue {
	return Queue{tracks: append([]Track(nil), tracks...)}
}

func (q Queue) Len() int    { return len(q.tracks) }
func (q Queue) At() int     { return q.at } // 0 to Len inclusive
func (q Queue) Empty() bool { return len(q.tracks) == 0 }

// Done reports a position past the end, which an empty queue always is. Not
// the same question as Empty.
func (q Queue) Done() bool { return q.at >= len(q.tracks) }

// Current is the track that should be playing, and false once Done.
func (q Queue) Current() (Track, bool) {
	if q.Done() {
		return Track{}, false
	}
	return q.tracks[q.at], true
}

// Upcoming is what gets appended to mpv's playlist for gapless prefetch (§7),
// which is the only reason a queue needs to look ahead at all.
func (q Queue) Upcoming() (Track, bool) {
	if next := q.at + 1; next < len(q.tracks) {
		return q.tracks[next], true
	}
	return Track{}, false
}

// Tracks is a copy: a caller able to mutate it could rewrite a queue it does
// not own.
func (q Queue) Tracks() []Track { return append([]Track(nil), q.tracks...) }

func (q Queue) Next() Queue    { return q.jump(q.at + 1) }
func (q Queue) Restart() Queue { return q.jump(0) }

// Previous from Done is the last track, which is what makes a skip at the end
// of an album recoverable.
func (q Queue) Previous() Queue { return q.jump(q.at - 1) }

// Jump clamps rather than refusing an index outside the queue: the caller is a
// keypress, and there is no useful error for pressing down at the bottom.
func (q Queue) Jump(i int) Queue { return q.jump(i) }

func (q Queue) jump(i int) Queue {
	if i < 0 {
		i = 0
	}
	if i > len(q.tracks) {
		i = len(q.tracks)
	}
	q.at = i
	return q
}
