package queue

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
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
		// want is every track the queue should hold, in no particular order.
		want := map[string]int{}
		for _, t := range start.Tracks() {
			want[t.ID]++
		}
		var history []string
		inserted := 0

		for range steps {
			var op string
			// playing is what should still be playing after the step. An
			// insertion must not change it; a movement is the only thing that
			// may, and then only to another track already in the queue.
			playing, wasPlaying := q.Current()

			switch rng.IntN(6) {
			case 0:
				op, q = "Next", q.Next()
			case 1:
				op, q = "Previous", q.Previous()
			case 2:
				i := rng.IntN(size+3) - 1
				op, q = fmt.Sprintf("Jump(%d)", i), q.Jump(i)
			case 3:
				op, q = "Restart", q.Restart()
			case 4:
				t := Track{ID: fmt.Sprintf("new-%d", inserted), Title: "Added"}
				inserted++
				want[t.ID]++
				op, q = "InsertNext("+t.ID+")", q.InsertNext(t)
			case 5:
				t := Track{ID: fmt.Sprintf("new-%d", inserted), Title: "Added"}
				inserted++
				want[t.ID]++
				op, q = "Append("+t.ID+")", q.Append(t)
			}
			history = append(history, op)
			inserting := strings.HasPrefix(op, "InsertNext") || strings.HasPrefix(op, "Append")

			fail := func(format string, args ...any) {
				t.Helper()
				t.Fatalf("seed %d, walk %d, size %d, after %v:\n  "+format,
					append([]any{seed, walk, size, history}, args...)...)
			}

			// The queue holds exactly what was put in it: no operation may
			// drop or duplicate a track, and only an insertion may add one.
			// This is what catches a play-next that overwrites rather than
			// inserts.
			got := map[string]int{}
			for _, t := range q.Tracks() {
				got[t.ID]++
			}
			if !reflect.DeepEqual(got, want) {
				fail("the queue holds %v, want %v", got, want)
			}
			// Inserting never disturbs what is playing. A queue with nothing
			// playing is the exception: the inserted track becomes current,
			// which is the only sensible answer.
			if inserting && wasPlaying {
				if now, ok := q.Current(); !ok || now.ID != playing.ID {
					fail("inserting changed the playing track from %s to %v", playing.ID, now.ID)
				}
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
				if up != q.Tracks()[q.At()+1] {
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

// TestInsertNextPlaysAfterWhatIsPlaying covers the point of the operation. The
// track goes after the current one rather than at the end, and whatever is
// playing keeps playing.
func TestInsertNextPlaysAfterWhatIsPlaying(t *testing.T) {
	q := New(tracks(4)...).Jump(1)
	added := Track{ID: "new", Title: "Added"}

	q = q.InsertNext(added)

	if current, _ := q.Current(); current.ID != "tr-1" {
		t.Errorf("inserting moved playback to %s", current.ID)
	}
	if up, ok := q.Upcoming(); !ok || up.ID != "new" {
		t.Errorf("the inserted track is not next; next is %v", up.ID)
	}
	if got := ids(q); !reflect.DeepEqual(got, []string{"tr-0", "tr-1", "new", "tr-2", "tr-3"}) {
		t.Errorf("the queue is %v", got)
	}
}

// TestAppendGoesToTheEndHowevrFarThroughTheQueueIs covers the other key, which
// adds to the end rather than after what is playing.
func TestAppendGoesToTheEndHowevrFarThroughTheQueueIs(t *testing.T) {
	q := New(tracks(3)...).Jump(0).Append(Track{ID: "new"})

	if got := ids(q); !reflect.DeepEqual(got, []string{"tr-0", "tr-1", "tr-2", "new"}) {
		t.Errorf("the queue is %v", got)
	}
	if current, _ := q.Current(); current.ID != "tr-0" {
		t.Errorf("appending moved playback to %s", current.ID)
	}
}

// TestInsertingIntoAQueueWithNothingPlaying covers the two positions that have
// nothing to insert after: an empty queue, and one that has finished. The
// track becomes what plays, which is the only sensible answer to being asked
// to play something next when nothing is playing.
func TestInsertingIntoAQueueWithNothingPlaying(t *testing.T) {
	for _, tc := range []struct {
		name string
		q    Queue
		want []string
	}{
		{"empty", New(), []string{"new"}},
		{"finished", New(tracks(2)...).Jump(2), []string{"tr-0", "tr-1", "new"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q.InsertNext(Track{ID: "new"})

			if got := ids(q); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("the queue is %v, want %v", got, tc.want)
			}
			if q.Done() {
				t.Error("the queue is still finished with a track waiting in it")
			}
			if current, ok := q.Current(); !ok || current.ID != "new" {
				t.Errorf("the inserted track is not playing; %v is", current.ID)
			}
		})
	}
}

// TestInsertingDoesNotDisturbTheQueueItCameFrom covers the immutability every
// other operation here keeps.
func TestInsertingDoesNotDisturbTheQueueItCameFrom(t *testing.T) {
	before := New(tracks(3)...).Jump(1)
	was := ids(before)

	_ = before.InsertNext(Track{ID: "a"})
	_ = before.Append(Track{ID: "b"})

	if got := ids(before); !reflect.DeepEqual(got, was) {
		t.Errorf("the original queue became %v, want %v", got, was)
	}
}

// ids is the track identifiers in a queue, in order.
func ids(q Queue) []string {
	out := make([]string, 0, q.Len())
	for _, t := range q.Tracks() {
		out = append(out, t.ID)
	}
	return out
}

// TestShufflingKeepsEveryTrack covers the property that matters: a shuffle
// that loses or repeats a track is a shuffle that has eaten somebody's
// playlist.
func TestShufflingKeepsEveryTrack(t *testing.T) {
	start := New(tracks(20)...)

	for seed := range uint64(50) {
		got := start.Shuffle(rand.New(rand.NewPCG(seed, 0)))

		if got.Len() != start.Len() {
			t.Fatalf("seed %d: shuffled %d tracks into %d", seed, start.Len(), got.Len())
		}
		seen := map[string]int{}
		for _, track := range got.Tracks() {
			seen[track.ID]++
		}
		for _, track := range start.Tracks() {
			if seen[track.ID] != 1 {
				t.Fatalf("seed %d: %s appears %d times", seed, track.ID, seen[track.ID])
			}
		}
		if got.At() != 0 {
			t.Errorf("seed %d: a shuffled queue starts at %d", seed, got.At())
		}
	}
}

// TestShufflingActuallyChangesTheOrder covers the other half. Keeping every
// track is easy to do by returning the queue unchanged.
func TestShufflingActuallyChangesTheOrder(t *testing.T) {
	start := New(tracks(20)...)

	var moved int
	for seed := range uint64(20) {
		got := start.Shuffle(rand.New(rand.NewPCG(seed, 0)))
		if !reflect.DeepEqual(ids(got), ids(start)) {
			moved++
		}
	}
	if moved == 0 {
		t.Error("twenty shuffles of twenty tracks all came back in the same order")
	}
}

// TestShufflingDoesNotDisturbTheQueueItCameFrom covers the immutability the
// rest of this package keeps.
func TestShufflingDoesNotDisturbTheQueueItCameFrom(t *testing.T) {
	start := New(tracks(10)...).Jump(3)
	was := ids(start)

	_ = start.Shuffle(rand.New(rand.NewPCG(1, 2)))

	if got := ids(start); !reflect.DeepEqual(got, was) {
		t.Errorf("shuffling reordered the queue it came from")
	}
	if start.At() != 3 {
		t.Errorf("shuffling moved the original to %d", start.At())
	}
}

// TestShufflingNothingIsNothing covers the empty queue, which `:shuffle` with
// nothing loaded would reach.
func TestShufflingNothingIsNothing(t *testing.T) {
	got := New().Shuffle(rand.New(rand.NewPCG(1, 2)))

	if !got.Empty() || !got.Done() {
		t.Errorf("shuffling an empty queue gave %+v", got)
	}
}
