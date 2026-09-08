package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
	"github.com/bspeelm/shanty/internal/tui"
)

// recorder is a player that records instead of playing. The wiring is what is
// under test here; that the commands reach a real mpv correctly is
// internal/mpv's business, and that mpv obeys them is the integration job's.
type recorder struct {
	mu     sync.Mutex
	calls  []string
	events chan mpv.Event
	fail   error
}

func newRecorder() *recorder { return &recorder{events: make(chan mpv.Event, 8)} }

func (r *recorder) note(format string, args ...any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, sprintf(format, args...))
	return r.fail
}

func (r *recorder) said() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recorder) Load(_ context.Context, url string) error   { return r.note("load %s", url) }
func (r *recorder) Append(_ context.Context, url string) error { return r.note("append %s", url) }
func (r *recorder) SetPause(_ context.Context, p bool) error   { return r.note("pause %v", p) }
func (r *recorder) Seek(_ context.Context, d time.Duration) error {
	return r.note("seek %s", d)
}
func (r *recorder) SeekTo(_ context.Context, d time.Duration) error {
	return r.note("seek to %s", d)
}
func (r *recorder) SetVolume(_ context.Context, v int) error { return r.note("volume %d", v) }
func (r *recorder) Observe(_ context.Context, p string) error {
	return r.note("observe %s", p)
}
func (r *recorder) Events() <-chan mpv.Event { return r.events }
func (r *recorder) Detach() error            { return r.note("detach") }
func (r *recorder) Prefetch(_ context.Context, url string) error {
	if url == "" {
		return r.note("prefetch nothing")
	}
	return r.note("prefetch %s", url)
}
func (r *recorder) Close() error { return r.note("close") }
func (r *recorder) Err() error   { return nil }

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// wired builds the app against the fake server and a recording player.
func wired(t *testing.T) (app, *recorder, *fake.Server) {
	t.Helper()
	srv := fake.New(t, fake.Options{User: user, Password: pass})
	client, err := subsonic.New(srv.URL, subsonic.PasswordAuth(user, pass), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rec := newRecorder()
	return newApp(t.Context(), client, rec), rec, srv
}

// step applies a message and runs everything the command produced, returning
// the messages that came back. This is the loop bubbletea runs, made
// synchronous so a test can assert on each turn.
func step(t *testing.T, a app, msg tea.Msg) (app, []tea.Msg) {
	t.Helper()
	next, cmd := a.Update(msg)
	return next.(app), runAll(cmd)
}

// runAll expands tea.Batch, which returns its members as a message rather than
// running them -- a helper that stopped at the batch would report that a
// batched command had produced nothing, which is how the volume key looked
// wired up while doing nothing.
//
// It gives up after a moment rather than failing: watchPlayer blocks on the
// event channel by design, and a batch containing it never finishes.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	var (
		mu  sync.Mutex
		got []tea.Msg
		wg  sync.WaitGroup
		run func(tea.Cmd)
	)
	run = func(c tea.Cmd) {
		defer wg.Done()
		m := c()
		if batch, isBatch := m.(tea.BatchMsg); isBatch {
			for _, sub := range batch {
				wg.Add(1)
				go run(sub)
			}
			return
		}
		if m == nil {
			return
		}
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	}
	wg.Add(1)
	go run(cmd)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]tea.Msg(nil), got...)
}

// only returns the one message of a kind, so a test says what it is looking
// for rather than indexing into whatever order the commands finished in.
func only[T tea.Msg](t *testing.T, msgs []tea.Msg) T {
	t.Helper()
	var zero T
	for _, m := range msgs {
		if got, ok := m.(T); ok {
			return got
		}
	}
	t.Fatalf("no %T among %v", zero, msgs)
	return zero
}

// The vertical slice, without a server on the network or a player on the
// machine: browse to an album, play a track, and finish it.
func TestBrowsingToAnAlbumAndPlayingIt(t *testing.T) {
	a, rec, _ := wired(t)

	a, out := step(t, a, tui.OpenArtist{ID: "ar-1"})
	artist := only[tui.ArtistLoaded](t, out)
	a, _ = step(t, a, artist)

	a, out = step(t, a, tui.OpenAlbum{ID: "al-1"})
	album := only[tui.AlbumLoaded](t, out)
	a, _ = step(t, a, album)

	a, out = step(t, a, tui.PlayFrom{Album: subsonic.Album(album), Index: 0})
	playing := only[tui.NowPlaying](t, out)
	if playing.Title != "Slipway" {
		t.Errorf("now playing %q, want Slipway", playing.Title)
	}

	said := rec.said()
	if len(said) < 2 {
		t.Fatalf("the player was told %v", said)
	}
	// The first track is loaded and the second appended, which is what makes
	// mpv's prefetch gapless.
	if !strings.HasPrefix(said[0], "load ") || !strings.Contains(said[0], "id=tr-1") {
		t.Errorf("first command was %q", said[0])
	}
	if !strings.HasPrefix(said[1], "append ") || !strings.Contains(said[1], "id=tr-2") {
		t.Errorf("second command was %q", said[1])
	}
}

// The credential reaches mpv over the socket and never on a command line. The
// URL is built here, so this asserts the app hands over a complete one -- the
// argv half is internal/mpv's TestChildProcessHygiene.
func TestTheStreamURLIsHandedOverWholeAndCarriesTheCredential(t *testing.T) {
	a, rec, _ := wired(t)
	album := subsonic.Album{ID: "al-1", Name: "Harbour", Songs: []subsonic.Song{{ID: "tr-1", Title: "Slipway"}}}

	step(t, a, tui.PlayFrom{Album: album, Index: 0})

	said := rec.said()
	if len(said) == 0 {
		t.Fatal("nothing was loaded")
	}
	for _, want := range []string{"id=tr-1", "c=shanty", "t=", "s=", "u=" + user} {
		if !strings.Contains(said[0], want) {
			t.Errorf("the loaded URL carries no %s: %q", want, said[0])
		}
	}
	if strings.Contains(said[0], "&p=") {
		t.Errorf("the loaded URL carries the legacy password parameter: %q", said[0])
	}
}

// A track ending of its own accord advances the queue and tells the server it
// was played. A track ended by a skip must not, or the skip would advance
// twice and lose one.
func TestOnlyAnEndOfFileAdvancesTheQueue(t *testing.T) {
	for _, tc := range []struct {
		reason    string
		wantTrack string
		wantPlay  bool
	}{
		{reason: "eof", wantTrack: "tr-2", wantPlay: true},
		{reason: "stop", wantTrack: "tr-1", wantPlay: false},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			a, rec, srv := wired(t)
			album := subsonic.Album{ID: "al-1", Songs: []subsonic.Song{
				{ID: "tr-1", Title: "Slipway"}, {ID: "tr-2", Title: "Ballast"},
			}}
			a, _ = step(t, a, tui.PlayFrom{Album: album, Index: 0})
			before := len(rec.said())

			a, _ = step(t, a, playerEvent(mpv.Event{Name: "end-file", Reason: tc.reason}))
			_ = before

			if current, _ := a.queue.Current(); current.ID != tc.wantTrack {
				t.Errorf("after end-file %q the queue is on %q, want %q", tc.reason, current.ID, tc.wantTrack)
			}

			var scrobbles int
			for _, r := range srv.Requests() {
				if r.Endpoint == "scrobble" {
					scrobbles++
				}
			}
			if tc.wantPlay && scrobbles == 0 {
				t.Error("a finished track was not scrobbled")
			}
			if !tc.wantPlay && scrobbles != 0 {
				t.Error("a skipped track was scrobbled as played")
			}
		})
	}
}

func TestPlaybackKeysReachThePlayer(t *testing.T) {
	a, rec, _ := wired(t)
	for _, msg := range []tea.Msg{
		tui.SeekBy{By: 10 * time.Second},
		tui.VolumeBy{Delta: -5},
		tui.TogglePause{},
	} {
		a, _ = step(t, a, msg)
	}

	said := strings.Join(rec.said(), "\n")
	for _, want := range []string{"seek 10s", "volume 95", "pause true"} {
		if !strings.Contains(said, want) {
			t.Errorf("the player never heard %q; it heard:\n%s", want, said)
		}
	}
}

// The volume the screen shows is the volume the player was given.
func TestVolumeClampsAndTheScreenAgrees(t *testing.T) {
	a, rec, _ := wired(t)
	for range 30 {
		a, _ = step(t, a, tui.VolumeBy{Delta: 5})
	}
	if a.volume != 100 {
		t.Errorf("volume ran to %d", a.volume)
	}
	said := rec.said()
	if last := said[len(said)-1]; !strings.Contains(last, "volume 100") {
		t.Errorf("the last thing the player heard was %q", last)
	}
}

// §7: a dead player is reported and never respawned.
func TestADeadPlayerBecomesAMessageAndNotARestart(t *testing.T) {
	a, rec, _ := wired(t)
	close(rec.events)

	// With a list on screen, which is when the player is most likely to die.
	a, _ = step(t, a, tui.ArtistsLoaded([]subsonic.Artist{{ID: "ar-1", Name: "Aoi"}}))

	next, _ := a.Update(playerGone{err: context.Canceled})
	a = next.(app)

	// Asserted on the rendered frame rather than on the model field. A test
	// that reads the field passes whether or not the message reaches the
	// screen, which is how this went unnoticed.
	frame := a.View()
	if !strings.Contains(frame, "mpv stopped") {
		t.Errorf("the frame does not say the player died:\n%s", frame)
	}
	if !strings.Contains(frame, "restart shanty") {
		t.Errorf("the frame does not say what to do:\n%s", frame)
	}
}

// Every intent the screen can emit is one the app acts on. An intent that fell
// through to the screen unhandled would be a key that silently does nothing.
func TestEveryIntentIsWiredUp(t *testing.T) {
	a, rec, _ := wired(t)
	album := subsonic.Album{ID: "al-1", Songs: []subsonic.Song{{ID: "tr-1"}}}

	for name, msg := range map[string]tea.Msg{
		"OpenArtist":    tui.OpenArtist{ID: "ar-1"},
		"OpenAlbum":     tui.OpenAlbum{ID: "al-1"},
		"PlayFrom":      tui.PlayFrom{Album: album},
		"TogglePause":   tui.TogglePause{},
		"SkipNext":      tui.SkipNext{},
		"SkipPrev":      tui.SkipPrev{},
		"SeekBy":        tui.SeekBy{By: time.Second},
		"VolumeBy":      tui.VolumeBy{Delta: 1},
		"VolumeSet":     tui.VolumeSet(40),
		"SeekToPercent": tui.SeekToPercent(50),
		"Search":        tui.Search("water"),
		"PlayNext":      tui.PlayNext{Album: album},
		"Enqueue":       tui.Enqueue{Album: album},
		"JumpTo":        tui.JumpTo(0),
		"SetPaused":     tui.SetPaused(true),
		"SeekTo":        tui.SeekTo(time.Minute),
	} {
		t.Run(name, func(t *testing.T) {
			next, cmd := a.Update(msg)
			if cmd == nil {
				t.Errorf("%s produced no command, so the key does nothing", name)
			}
			_ = next
		})
	}
	_ = rec
}

// The command line sets an absolute volume where the keys change it by steps.
func TestAnAbsoluteVolumeReachesThePlayer(t *testing.T) {
	a, rec, _ := wired(t)
	a, _ = step(t, a, tui.VolumeSet(40))

	if a.volume != 40 {
		t.Errorf("volume is %d, want 40", a.volume)
	}
	if said := strings.Join(rec.said(), "\n"); !strings.Contains(said, "volume 40") {
		t.Errorf("the player never heard it:\n%s", said)
	}
}

func TestAnAbsoluteVolumeIsClamped(t *testing.T) {
	a, rec, _ := wired(t)
	a, _ = step(t, a, tui.VolumeSet(400))
	if a.volume != 100 {
		t.Errorf("volume is %d, want 100", a.volume)
	}
	said := rec.said()
	if !strings.Contains(said[len(said)-1], "volume 100") {
		t.Errorf("the player heard %q", said[len(said)-1])
	}
}
