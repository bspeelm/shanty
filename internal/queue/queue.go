// Package queue holds a list of tracks and the current position in it.
//
// Every operation returns a new Queue rather than modifying the receiver.
package queue

import (
	"math/rand/v2"
	"time"
)

// Track is one track in the queue.
type Track struct {
	ID       string
	Title    string
	Album    string
	Artist   string
	Duration time.Duration
}

// Queue is a list of tracks and a position in it. The position ranges from 0
// to Len inclusive; Len means the queue has finished.
type Queue struct {
	tracks []Track
	at     int
}

// New returns a queue of the given tracks, positioned at the first. The slice
// is copied.
func New(tracks ...Track) Queue {
	return Queue{tracks: append([]Track(nil), tracks...)}
}

func (q Queue) Len() int    { return len(q.tracks) }
func (q Queue) At() int     { return q.at } // 0 to Len inclusive
func (q Queue) Empty() bool { return len(q.tracks) == 0 }

// Done reports whether the position is past the last track. An empty queue is
// always Done.
func (q Queue) Done() bool { return q.at >= len(q.tracks) }

// Current returns the track that should be playing. The second result is false
// once the queue is Done.
func (q Queue) Current() (Track, bool) {
	if q.Done() {
		return Track{}, false
	}
	return q.tracks[q.at], true
}

// Upcoming returns the track after the current one. It is used to tell mpv
// which file to open in advance, so that albums play without a gap between
// tracks. That is the only reason this package needs to look ahead.
func (q Queue) Upcoming() (Track, bool) {
	if next := q.at + 1; next < len(q.tracks) {
		return q.tracks[next], true
	}
	return Track{}, false
}

// Tracks returns a copy of the track list.
func (q Queue) Tracks() []Track { return append([]Track(nil), q.tracks...) }

// InsertNext returns a queue with the track placed after the one playing, so
// that it is what plays when the current track ends. The position does not
// move, so whatever is playing keeps playing.
//
// A queue that has finished, and an empty one, have nothing to insert after,
// and the track goes on the end.
func (q Queue) InsertNext(t Track) Queue {
	return q.insert(min(q.at+1, len(q.tracks)), t)
}

// Append returns a queue with the track added to the end.
func (q Queue) Append(t Track) Queue { return q.insert(len(q.tracks), t) }

// AppendAll returns a queue with the tracks added to the end, in order.
func (q Queue) AppendAll(tracks ...Track) Queue { return q.insert(len(q.tracks), tracks...) }

// InsertAllNext returns a queue with the tracks placed after the one playing,
// in order. A queue that has finished, and an empty one, take them on the end.
func (q Queue) InsertAllNext(tracks ...Track) Queue {
	return q.insert(min(q.at+1, len(q.tracks)), tracks...)
}

// insert places tracks at index i, which must be within the queue.
func (q Queue) insert(i int, more ...Track) Queue {
	tracks := make([]Track, 0, len(q.tracks)+len(more))
	tracks = append(tracks, q.tracks[:i]...)
	tracks = append(tracks, more...)
	tracks = append(tracks, q.tracks[i:]...)
	q.tracks = tracks
	return q
}

// Remove returns a queue without the track at index i. The track that was
// current stays current. Where the current track is the one removed, the one
// that followed it is current, and the queue is done if none did. An index
// outside the queue returns it unchanged.
func (q Queue) Remove(i int) Queue {
	if i < 0 || i >= len(q.tracks) {
		return q
	}
	tracks := make([]Track, 0, len(q.tracks)-1)
	q.tracks = append(append(tracks, q.tracks[:i]...), q.tracks[i+1:]...)
	if i < q.at {
		q.at--
	}
	return q
}

// Move returns a queue with the track at index from placed at index to. The
// track that was current stays current, wherever it now is. An index outside
// the queue returns it unchanged.
func (q Queue) Move(from, to int) Queue {
	if from < 0 || from >= len(q.tracks) || to < 0 || to >= len(q.tracks) {
		return q
	}
	moved, at := q.tracks[from], q.at
	q = q.Remove(from).insert(to, moved)
	switch {
	case from == at:
		q.at = to
	case from < at && to >= at:
		q.at = at - 1
	case from > at && to <= at:
		q.at = at + 1
	default:
		q.at = at
	}
	return q
}

// Shuffle returns a queue holding the same tracks in a random order, starting
// at the first.
//
// The randomness is the caller's, so that a test gets the same order twice and
// the shuffling itself stays a pure function over data like everything else
// here.
func (q Queue) Shuffle(rng *rand.Rand) Queue {
	tracks := append([]Track(nil), q.tracks...)
	rng.Shuffle(len(tracks), func(i, j int) { tracks[i], tracks[j] = tracks[j], tracks[i] })
	return Queue{tracks: tracks}
}

// ShuffleRest returns a queue with the tracks after the current one in a
// random order. The current track, those before it and the position are as
// they were. A queue that is done, or has fewer than two tracks after the
// current one, is returned unchanged.
func (q Queue) ShuffleRest(rng *rand.Rand) Queue {
	from := q.at + 1
	if q.Done() || len(q.tracks)-from < 2 {
		return q
	}
	tracks := append([]Track(nil), q.tracks...)
	rest := tracks[from:]
	rng.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	q.tracks = tracks
	return q
}

func (q Queue) Next() Queue    { return q.jump(q.at + 1) }
func (q Queue) Restart() Queue { return q.jump(0) }

// Previous moves back one track. From the Done position it selects the last
// track.
func (q Queue) Previous() Queue { return q.jump(q.at - 1) }

// Jump moves to the track at index i, clamped to the queue.
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
