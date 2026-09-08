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

// insert places a track at index i, which must be within the queue.
func (q Queue) insert(i int, t Track) Queue {
	tracks := make([]Track, 0, len(q.tracks)+1)
	tracks = append(tracks, q.tracks[:i]...)
	tracks = append(tracks, t)
	tracks = append(tracks, q.tracks[i:]...)
	q.tracks = tracks
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
