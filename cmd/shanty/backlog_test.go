package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bspeelm/shanty/internal/backlog"
	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
	"github.com/bspeelm/shanty/internal/tui"
)

// keeping builds an app whose backlog is a file in a temporary directory, and
// whose server behaves as the options say.
func keeping(t *testing.T, malice fake.Malice) (app, *fake.Server, string) {
	t.Helper()
	srv := fake.New(t, fake.Options{User: user, Password: pass, Malice: malice})
	client, err := subsonic.New(srv.URL, subsonic.PasswordAuth(user, pass), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state", "plays.jsonl")
	a := newApp(t.Context(), client, newRecorder(), config.Config{})
	a.backlog = path
	return a, srv, path
}

// played drives one track to its natural end, which is what reports a play.
func played(t *testing.T, a app) app {
	t.Helper()
	a, _ = step(t, a, tui.PlayFrom{Album: threeTracks, Index: 0})
	a, _ = step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: "eof"}))
	return a
}

// TestAPlayTheServerWouldNotTakeIsKept is the whole point. A session reports
// plays where nobody is watching, so one that is dropped is a listening
// history that quietly loses days.
func TestAPlayTheServerWouldNotTakeIsKept(t *testing.T) {
	a, _, path := keeping(t, fake.Malice{RefuseScrobbles: true})

	_ = played(t, a)

	waiting, err := backlog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 1 {
		t.Fatalf("the server refused a play and %d were kept", len(waiting))
	}
	if waiting[0].ID != "tr-1" {
		t.Errorf("the play kept was %q, want tr-1", waiting[0].ID)
	}
	if time.Since(waiting[0].At) > time.Minute {
		t.Errorf("the play is dated %v, which is not when it happened", waiting[0].At)
	}
}

// TestAPlayTheServerTookIsNotKept covers the ordinary case, which must leave
// nothing behind.
func TestAPlayTheServerTookIsNotKept(t *testing.T) {
	a, _, path := keeping(t, fake.Malice{})

	_ = played(t, a)

	waiting, err := backlog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 0 {
		t.Errorf("a play the server accepted was kept anyway: %v", waiting)
	}
}

// TestWhatWasKeptIsSentWhenTheServerTakesItAgain covers catching up, and that
// a late report says when it happened rather than when it was sent.
func TestWhatWasKeptIsSentWhenTheServerTakesItAgain(t *testing.T) {
	a, srv, path := keeping(t, fake.Malice{})
	yesterday := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	for _, id := range []string{"tr-9", "tr-8"} {
		if err := backlog.Add(path, backlog.Play{ID: id, At: yesterday}); err != nil {
			t.Fatal(err)
		}
	}

	_ = played(t, a)

	waiting, err := backlog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 0 {
		t.Errorf("the server took everything and %d plays are still waiting: %v", len(waiting), waiting)
	}

	var reported []string
	var late string
	for _, r := range srv.Requests() {
		if r.Endpoint == "scrobble" {
			reported = append(reported, r.Query.Get("id"))
			if r.Query.Get("id") == "tr-9" {
				late = r.Query.Get("time")
			}
		}
	}
	if len(reported) != 3 {
		t.Errorf("the server was sent %v, want the new play and the two kept", reported)
	}
	if late == "" {
		t.Fatal("a play reported late did not say when it happened")
	}
	if want := yesterday.UnixMilli(); late != sprintf("%d", want) {
		t.Errorf("the late play was dated %s, want %d", late, want)
	}
}

// TestCatchingUpStopsWhenTheServerStopsTakingThem covers the server going away
// again part way through, which must not lose the rest.
func TestCatchingUpStopsWhenTheServerStopsTakingThem(t *testing.T) {
	a, _, path := keeping(t, fake.Malice{RefuseScrobbles: true})
	for _, id := range []string{"tr-9", "tr-8"} {
		if err := backlog.Add(path, backlog.Play{ID: id, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	_ = played(t, a)

	waiting, err := backlog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 3 {
		t.Fatalf("the server took nothing and %d plays are waiting, want 3: %v", len(waiting), waiting)
	}
	ids := []string{waiting[0].ID, waiting[1].ID, waiting[2].ID}
	if strings.Join(ids, ",") != "tr-9,tr-8,tr-1" {
		t.Errorf("the plays waiting are %v, want the two kept and the new one", ids)
	}
}

// TestAnAppWithNowhereToKeepPlaysStillPlays covers the tests and the paths
// that have no state directory. A missing backlog must not stop the music.
func TestAnAppWithNowhereToKeepPlaysStillPlays(t *testing.T) {
	a, _, _ := wired(t)
	if a.backlog != "" {
		t.Fatal("this test wants an app with nowhere to keep plays")
	}

	a = played(t, a)

	if current, _ := a.queue.Current(); current.ID != "tr-2" {
		t.Errorf("the queue did not advance; it is on %v", current.ID)
	}
}

// TestPlaysTheServerTookAreNotSentAgain covers a server that goes away part
// way through catching up. The plays it accepted must not be kept, or the next
// catch-up reports them a second time and the listening history counts them
// twice.
func TestPlaysTheServerTookAreNotSentAgain(t *testing.T) {
	// Two reports are taken: the new play, and the first one waiting.
	a, srv, path := keeping(t, fake.Malice{RefuseScrobblesAfter: 2})
	for _, id := range []string{"tr-9", "tr-8", "tr-7"} {
		if err := backlog.Add(path, backlog.Play{ID: id, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	_ = played(t, a)

	var taken []string
	for _, r := range srv.Requests() {
		if r.Endpoint == "scrobble" {
			taken = append(taken, r.Query.Get("id"))
		}
	}
	if len(taken) < 2 {
		t.Fatalf("the server was sent %v, want at least the two it would take", taken)
	}
	accepted := taken[:2]

	waiting, err := backlog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, play := range waiting {
		for _, done := range accepted {
			if play.ID == done {
				t.Errorf("%s was accepted and is still waiting, so it will be reported twice", play.ID)
			}
		}
	}
	if len(waiting) != 2 {
		t.Errorf("%d plays are waiting, want the two the server refused: %v", len(waiting), waiting)
	}
}
