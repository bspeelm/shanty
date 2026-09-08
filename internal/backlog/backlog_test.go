package backlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func file(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state", "plays.jsonl")
}

func TestAPlayIsKeptAndReadBack(t *testing.T) {
	path := file(t)
	at := time.Date(2026, 9, 8, 15, 4, 5, 0, time.UTC)

	if err := Add(path, Play{ID: "tr-1", At: at}); err != nil {
		t.Fatal(err)
	}
	if err := Add(path, Play{ID: "tr-2", At: at.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}

	plays, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(plays) != 2 {
		t.Fatalf("kept %d plays, want 2", len(plays))
	}
	if plays[0].ID != "tr-1" || plays[1].ID != "tr-2" {
		t.Errorf("plays came back as %v, want them oldest first", plays)
	}
	// The time is what makes a late report honest about when it happened.
	if !plays[0].At.Equal(at) {
		t.Errorf("the play is dated %v, want %v", plays[0].At, at)
	}
}

// TestNoBacklogIsNotAnError covers the ordinary case, where every play has
// been accepted and there is no file at all.
func TestNoBacklogIsNotAnError(t *testing.T) {
	plays, err := Load(filepath.Join(t.TempDir(), "nothing.jsonl"))
	if err != nil {
		t.Fatalf("reading a backlog that does not exist failed: %v", err)
	}
	if len(plays) != 0 {
		t.Errorf("a file that is not there held %d plays", len(plays))
	}
}

// TestAHalfWrittenLineDoesNotLoseTheRest covers a process killed while a play
// was being appended. The plays written before it are still worth having.
func TestAHalfWrittenLineDoesNotLoseTheRest(t *testing.T) {
	path := file(t)
	for _, id := range []string{"tr-1", "tr-2", "tr-3"} {
		if err := Add(path, Play{ID: id, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	// Cut the file mid-line, which is what a partial append leaves.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body[:len(body)-12], 0o600); err != nil {
		t.Fatal(err)
	}

	plays, err := Load(path)
	if err != nil {
		t.Fatalf("a torn file failed to load: %v", err)
	}
	if len(plays) != 2 {
		t.Fatalf("a torn file gave %d plays, want the two written whole", len(plays))
	}
	if plays[0].ID != "tr-1" || plays[1].ID != "tr-2" {
		t.Errorf("the surviving plays are %v", plays)
	}
}

// TestRubbishInTheFileIsSkippedRatherThanFatal covers a file somebody has
// edited, or one from a version that wrote something else.
func TestRubbishInTheFileIsSkippedRatherThanFatal(t *testing.T) {
	path := file(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"id":"tr-1","at":"2026-09-08T15:04:05Z"}`,
		`not json at all`,
		``,
		`{"at":"2026-09-08T15:04:05Z"}`, // no id, so nothing to report
		`{"id":"tr-2","at":"2026-09-08T15:05:05Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	plays, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(plays) != 2 {
		t.Fatalf("got %d plays, want the two that were readable: %v", len(plays), plays)
	}
}

// TestReplaceLeavesNothingBehindWhenTheBacklogIsCleared covers catching up. A
// machine with nothing waiting should have no file, so the state directory is
// empty again.
func TestReplaceLeavesNothingBehindWhenTheBacklogIsCleared(t *testing.T) {
	path := file(t)
	if err := Add(path, Play{ID: "tr-1", At: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if err := Replace(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("clearing the backlog left the file behind: %v", err)
	}
	if err := Replace(path, nil); err != nil {
		t.Errorf("clearing a backlog that is already gone failed: %v", err)
	}
}

// TestReplaceKeepsWhatIsStillWaiting covers a partial catch-up, where some
// plays were accepted and others were not.
func TestReplaceKeepsWhatIsStillWaiting(t *testing.T) {
	path := file(t)
	for _, id := range []string{"tr-1", "tr-2", "tr-3"} {
		if err := Add(path, Play{ID: id, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	if err := Replace(path, []Play{{ID: "tr-3", At: time.Now()}}); err != nil {
		t.Fatal(err)
	}

	plays, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(plays) != 1 || plays[0].ID != "tr-3" {
		t.Errorf("the backlog holds %v, want only tr-3", plays)
	}
	// Nothing is left beside it.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the state directory holds %d files, want only the backlog", len(entries))
	}
}

// TestTheBacklogDoesNotGrowForEver covers a server unreachable for a long
// time. The oldest plays go first, because the recent ones are the ones a
// listening history is missing.
func TestTheBacklogDoesNotGrowForEver(t *testing.T) {
	path := file(t)
	plays := make([]Play, Limit+10)
	for i := range plays {
		plays[i] = Play{ID: string(rune('a' + i%26)), At: time.Now()}
	}
	plays[len(plays)-1].ID = "newest"
	plays[0].ID = "oldest"

	if err := Replace(path, plays); err != nil {
		t.Fatal(err)
	}

	kept, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != Limit {
		t.Fatalf("kept %d plays, want the limit of %d", len(kept), Limit)
	}
	if kept[len(kept)-1].ID != "newest" {
		t.Error("the newest play was dropped")
	}
	for _, play := range kept {
		if play.ID == "oldest" {
			t.Error("the oldest play was kept over newer ones")
		}
	}
}

// TestTheBacklogIsNotReadableByAnybodyElse covers the file's permissions. It
// names what somebody listened to and when.
func TestTheBacklogIsNotReadableByAnybodyElse(t *testing.T) {
	path := file(t)
	if err := Add(path, Play{ID: "tr-1", At: time.Now()}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the backlog is mode %04o, want 0600", mode)
	}

	if err := Replace(path, []Play{{ID: "tr-2", At: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("rewriting left the backlog mode %04o, want 0600", mode)
	}
}
