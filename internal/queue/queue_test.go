package queue

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"
)

func tracks(n int) []Track {
	out := make([]Track, n)
	for i := range out {
		out[i] = Track{
			ID:       fmt.Sprintf("tr-%d", i),
			Title:    fmt.Sprintf("Track %d", i),
			Album:    "Harbour",
			Artist:   "Aoi",
			Duration: time.Duration(i+1) * time.Second,
		}
	}
	return out
}

func TestAnAlbumPlaysToCompletion(t *testing.T) {
	q := New(tracks(3)...)

	for i := range 3 {
		got, ok := q.Current()
		if !ok {
			t.Fatalf("queue finished at track %d of 3", i)
		}
		if got.ID != fmt.Sprintf("tr-%d", i) {
			t.Errorf("at position %d the current track is %s", i, got.ID)
		}
		q = q.Next()
	}

	// Finished is a state, not a clamp. A queue that pinned the last track as
	// current would leave nothing able to tell playing from finished.
	if !q.Done() {
		t.Error("after the last track the queue is not Done")
	}
	if _, ok := q.Current(); ok {
		t.Error("a finished queue still reports a current track")
	}
	if q.Len() != 3 {
		t.Errorf("finishing changed the queue's length to %d", q.Len())
	}
}

func TestUpcomingIsWhatMpvPrefetches(t *testing.T) {
	q := New(tracks(2)...)

	next, ok := q.Upcoming()
	if !ok || next.ID != "tr-1" {
		t.Errorf("Upcoming = %v, %v; want tr-1", next.ID, ok)
	}
	if _, ok := q.Next().Upcoming(); ok {
		t.Error("the last track has something after it")
	}
	if _, ok := New().Upcoming(); ok {
		t.Error("an empty queue has something upcoming")
	}
}

// Skipping past the end must be recoverable: Previous from Done is the last
// track, not the one before it.
func TestPreviousFromDoneIsTheLastTrack(t *testing.T) {
	q := New(tracks(3)...).Next().Next().Next()
	if !q.Done() {
		t.Fatal("the queue is not Done")
	}

	got, ok := q.Previous().Current()
	if !ok || got.ID != "tr-2" {
		t.Errorf("Previous from Done gives %q, want tr-2", got.ID)
	}
}

func TestTheEmptyQueueIsDoneAndSaysSo(t *testing.T) {
	q := New()
	if !q.Empty() {
		t.Error("a queue with no tracks is not Empty")
	}
	if !q.Done() {
		t.Error("a queue with no tracks is not Done")
	}
	for _, op := range map[string]Queue{"Next": q.Next(), "Previous": q.Previous(), "Restart": q.Restart(), "Jump": q.Jump(5)} {
		if op.Len() != 0 {
			t.Errorf("an operation on the empty queue produced %d tracks", op.Len())
		}
	}
}

// Purity is the property everything else in this package rests on. A caller
// holding the slice it passed in, or the one it got back, must not be able to
// change a queue that was already handed out.
func TestAQueueCannotBeChangedThroughASliceItGaveOrTook(t *testing.T) {
	in := tracks(3)
	q := New(in...)

	in[0].Title = "rewritten through the input"
	if got, _ := q.Current(); got.Title != "Track 0" {
		t.Errorf("mutating the input slice changed the queue: %q", got.Title)
	}

	out := q.Tracks()
	out[0].Title = "rewritten through the output"
	if got, _ := q.Current(); got.Title != "Track 0" {
		t.Errorf("mutating the returned slice changed the queue: %q", got.Title)
	}
}

// Operations return a new queue and leave the old one alone, which is what
// makes a queue safe to hold across a render without a lock.
func TestOperationsDoNotMutateTheReceiver(t *testing.T) {
	q := New(tracks(3)...)
	before := q.At()

	q.Next()
	q.Jump(2)
	q.Restart()

	if q.At() != before {
		t.Errorf("the receiver moved to %d without being reassigned", q.At())
	}
}

// The properties, checked over random walks rather than chosen cases: the
// interesting queue bugs are sequences, not single calls.
func TestQueueProperties(t *testing.T) {
	const walks, steps = 400, 40
	seed := uint64(time.Now().UnixNano())
	rng := rand.New(rand.NewPCG(seed, 0x5ea17))

	for walk := range walks {
		size := rng.IntN(6)
		start := New(tracks(size)...)
		q := start
		var history []string

		for range steps {
			var op string
			switch rng.IntN(4) {
			case 0:
				op, q = "Next", q.Next()
			case 1:
				op, q = "Previous", q.Previous()
			case 2:
				i := rng.IntN(size+3) - 1
				op, q = fmt.Sprintf("Jump(%d)", i), q.Jump(i)
			case 3:
				op, q = "Restart", q.Restart()
			}
			history = append(history, op)

			fail := func(format string, args ...any) {
				t.Helper()
				t.Fatalf("seed %d, walk %d, size %d, after %v:\n  "+format,
					append([]any{seed, walk, size, history}, args...)...)
			}

			// The multiset is preserved: no operation may add, drop, reorder
			// or duplicate a track. This is the one that catches a play-next
			// implementation that overwrites instead of inserting.
			if !reflect.DeepEqual(q.Tracks(), start.Tracks()) {
				fail("the track list changed to %v", q.Tracks())
			}
			// The position is always addressable or exactly one past the end.
			if q.At() < 0 || q.At() > q.Len() {
				fail("position %d is outside [0,%d]", q.At(), q.Len())
			}
			// Current and Done are one statement, never disagreeing.
			if _, ok := q.Current(); ok == q.Done() {
				fail("Current ok=%v and Done=%v at position %d", ok, q.Done(), q.At())
			}
			// Upcoming is the track after the current one, or nothing.
			if up, ok := q.Upcoming(); ok {
				if q.At()+1 >= q.Len() {
					fail("Upcoming reported %s past the end", up.ID)
				}
				if up != start.Tracks()[q.At()+1] {
					fail("Upcoming is %s, not the track after position %d", up.ID, q.At())
				}
			} else if q.At()+1 < q.Len() {
				fail("Upcoming reported nothing at position %d of %d", q.At(), q.Len())
			}
			// Stepping forward and back is a round trip everywhere but the
			// end, where Next has nowhere left to go.
			if !q.Done() && q.Next().Previous().At() != q.At() {
				fail("Next then Previous moved from %d to %d", q.At(), q.Next().Previous().At())
			}
		}
	}
}
