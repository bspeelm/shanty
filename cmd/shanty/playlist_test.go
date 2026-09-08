package main

import (
	"math/rand/v2"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
	"github.com/bspeelm/shanty/internal/tui"
)

// withPlaylists builds an app against a server already holding some.
func withPlaylists(t *testing.T, lists map[string][]string) (app, *fake.Server) {
	t.Helper()
	srv := fake.New(t, fake.Options{User: user, Password: pass})
	srv.SetPlaylists(lists)
	client, err := subsonic.New(srv.URL, subsonic.PasswordAuth(user, pass), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(t.Context(), client, newRecorder())
	a.random = rand.New(rand.NewPCG(1, 2))
	return a, srv
}

// runTo drives every message a step produced back into the app, which is what
// bubbletea does and what a command needing two round trips needs.
func runTo(t *testing.T, a app, msg tea.Msg) (app, []tea.Msg) {
	t.Helper()
	a, msgs := step(t, a, msg)
	for _, m := range msgs {
		if m == nil {
			continue
		}
		var more []tea.Msg
		a, more = step(t, a, m)
		msgs = append(msgs, more...)
	}
	return a, msgs
}

// TestDeletingAPlaylistAsksFirstAndNamesIt is what issue #6 requires: the one
// thing here that cannot be undone from inside shanty.
func TestDeletingAPlaylistAsksFirstAndNamesIt(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2"}})

	a, msgs := runTo(t, a, tui.Playlist{Verb: "delete", Name: "Evening"})

	var asked *tui.Confirm
	for _, m := range msgs {
		if c, ok := m.(tui.Confirm); ok {
			asked = &c
		}
	}
	if asked == nil {
		t.Fatalf("deleting did not ask; it produced %v", msgs)
	}
	if !strings.Contains(asked.Question, "Evening") {
		t.Errorf("the question does not name the playlist: %q", asked.Question)
	}
	if !strings.Contains(asked.Question, "2 tracks") {
		t.Errorf("the question does not say what is in it: %q", asked.Question)
	}

	// Nothing has happened yet.
	for _, r := range srv.Requests() {
		if r.Endpoint == "deletePlaylist" {
			t.Fatal("the playlist was deleted before the question was answered")
		}
	}
	if len(srv.Playlists()) != 1 {
		t.Error("the playlist is gone")
	}
}

// TestAnsweringNoLeavesThePlaylistAlone covers the other half of asking.
func TestAnsweringNoLeavesThePlaylistAlone(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1"}})
	a, _ = runTo(t, a, tui.Playlist{Verb: "delete", Name: "Evening"})
	a, _ = step(t, a, tui.Confirm{Question: "delete?", Do: deletePlaylist{id: srv.Playlists()[0].ID, name: "Evening"}})

	// Every key but yes means no.
	for _, key := range []string{"n", "esc", "q", "d"} {
		next, _ := a.ui.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		ui := next.(tui.Model)
		if ui.Confirming() {
			t.Errorf("%q left the question waiting", key)
		}
		if !strings.Contains(ui.Status(), "nothing was changed") {
			t.Errorf("%q said %q", key, ui.Status())
		}
	}
	if len(srv.Playlists()) != 1 {
		t.Error("the playlist was deleted by an answer that was not yes")
	}
}

// TestAnsweringYesDeletesIt covers the path the question guards.
func TestAnsweringYesDeletesIt(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1"}})
	id := srv.Playlists()[0].ID

	a, _ = step(t, a, tui.Confirm{Question: "delete?", Do: deletePlaylist{id: id, name: "Evening"}})
	next, cmd := a.ui.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	a.ui = next.(tui.Model)
	if cmd == nil {
		t.Fatal("answering yes did nothing")
	}
	a, _ = runTo(t, a, cmd())

	if len(srv.Playlists()) != 0 {
		t.Errorf("the playlist survived: %+v", srv.Playlists())
	}
}

// TestTwoPlaylistsWithOneNameIsAQuestionNotAGuess covers what a server allows
// and a name does not settle: identifiers are unique, names are not.
func TestTwoPlaylistsWithOneNameIsAQuestionNotAGuess(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {"tr-1"}})
	srv.SetPlaylists(map[string][]string{"Evening": {"tr-2"}})

	a, _ = runTo(t, a, tui.Playlist{Verb: "delete", Name: "Evening"})

	if a.ui.Confirming() {
		t.Fatal("shanty picked one of two playlists with the same name")
	}
	if !strings.Contains(a.ui.Status(), "cannot tell which") {
		t.Errorf("it said %q", a.ui.Status())
	}
}

// TestEditingMarksWhatIsAlreadyInThePlaylist covers the mode: the library is
// on screen, and the tracks already in the playlist are marked.
func TestEditingMarksWhatIsAlreadyInThePlaylist(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1"}})

	a, _ = runTo(t, a, tui.Playlist{Verb: "edit", Name: "Evening"})
	a, _ = step(t, a, tui.AlbumLoaded(threeTracks))

	if a.ui.Editing().Name != "Evening" {
		t.Fatalf("editing is %q", a.ui.Editing().Name)
	}
	frame := a.View()
	if !strings.Contains(frame, "(A)") {
		t.Errorf("the track already in the playlist is not marked:\n%s", frame)
	}
	if strings.Count(frame, "(A)") != 1 {
		t.Errorf("%d tracks are marked, want the one that is in it:\n%s", strings.Count(frame, "(A)"), frame)
	}
}

// TestAddingAndRemovingWhileEditingReachesTheServer covers a and r, and that
// the marks follow what the server has rather than what was pressed.
func TestAddingAndRemovingWhileEditingReachesTheServer(t *testing.T) {
	a, srv := withPlaylists(t, map[string][]string{"Evening": {}})

	a, _ = runTo(t, a, tui.Playlist{Verb: "edit", Name: "Evening"})
	a, _ = step(t, a, tui.AlbumLoaded(threeTracks))

	// a adds the selected track.
	next, cmd := a.ui.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	a.ui = next.(tui.Model)
	if cmd == nil {
		t.Fatal("a did nothing while editing")
	}
	a, _ = runTo(t, a, cmd())

	if got := srv.Playlists()[0].Songs; len(got) != 1 || got[0].ID != "tr-1" {
		t.Fatalf("the playlist holds %+v, want tr-1", got)
	}
	if !strings.Contains(a.View(), "(A)") {
		t.Error("the added track is not marked")
	}

	// r takes it out again.
	next, cmd = a.ui.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	a.ui = next.(tui.Model)
	if cmd == nil {
		t.Fatal("r did nothing while editing")
	}
	a, _ = runTo(t, a, cmd())

	if got := srv.Playlists()[0].Songs; len(got) != 0 {
		t.Errorf("the playlist still holds %+v", got)
	}
}

// TestOutsideEditModeAStillQueues covers the modal binding. The two meanings
// must not leak into each other.
func TestOutsideEditModeAStillQueues(t *testing.T) {
	a, _ := withPlaylists(t, nil)
	a, _ = step(t, a, tui.AlbumLoaded(threeTracks))

	_, cmd := a.ui.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if cmd == nil {
		t.Fatal("a did nothing outside edit mode")
	}
	if _, ok := cmd().(tui.Enqueue); !ok {
		t.Errorf("a emitted %#v outside edit mode, want Enqueue", cmd())
	}
}

// TestLeavingEditModeStopsTheMarks covers esc, which has to leave the playlist
// before it leaves the screen.
func TestLeavingEditModeStopsTheMarks(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1"}})
	a, _ = runTo(t, a, tui.Playlist{Verb: "edit", Name: "Evening"})
	a, _ = step(t, a, tui.AlbumLoaded(threeTracks))

	next, _ := a.ui.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.ui = next.(tui.Model)

	if a.ui.Editing().ID != "" {
		t.Error("esc did not leave the playlist")
	}
	if strings.Contains(a.View(), "(A)") {
		t.Error("the marks are still there after leaving")
	}
	if !strings.Contains(a.ui.Status(), "finished with Evening") {
		t.Errorf("it said %q", a.ui.Status())
	}
}

// TestShufflingAPlaylistPlaysItInAnotherOrder covers the command, and that the
// same shuffle serves the whole library.
func TestShufflingAPlaylistPlaysItInAnotherOrder(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1", "tr-2", "tr-3", "tr-4"}})

	a, _ = runTo(t, a, tui.Playlist{Verb: "shuffle", Name: "Evening"})

	if a.queue.Len() != 4 {
		t.Fatalf("shuffling queued %d tracks, want 4", a.queue.Len())
	}
	seen := map[string]int{}
	for _, track := range a.queue.Tracks() {
		seen[track.ID]++
	}
	for _, id := range []string{"tr-1", "tr-2", "tr-3", "tr-4"} {
		if seen[id] != 1 {
			t.Errorf("%s appears %d times in the shuffled queue", id, seen[id])
		}
	}
	if _, playing := a.queue.Current(); !playing {
		t.Error("shuffling queued tracks and played nothing")
	}
}

// TestShufflingEverythingReadsTheWholeLibrary covers :shuffle, which is the
// same shuffle over everything the server has.
func TestShufflingEverythingReadsTheWholeLibrary(t *testing.T) {
	a, _ := withPlaylists(t, nil)

	a, _ = runTo(t, a, tui.Shuffle{})

	if a.queue.Len() != 4 {
		t.Fatalf("shuffling everything queued %d tracks, want the library's 4", a.queue.Len())
	}
	if !strings.Contains(a.ui.Status(), "everything") {
		t.Errorf("it said %q", a.ui.Status())
	}
}

// TestShufflingNothingSaysSo covers a playlist with nothing in it.
func TestShufflingNothingSaysSo(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Empty": {}})

	a, _ = runTo(t, a, tui.Playlist{Verb: "shuffle", Name: "Empty"})

	if !strings.Contains(a.ui.Status(), "nothing to shuffle") {
		t.Errorf("it said %q", a.ui.Status())
	}
}

// TestAPlaylistThatIsNotThereIsReported covers a name that matches nothing.
func TestAPlaylistThatIsNotThereIsReported(t *testing.T) {
	a, _ := withPlaylists(t, map[string][]string{"Evening": {"tr-1"}})

	a, _ = runTo(t, a, tui.Playlist{Verb: "edit", Name: "Morning"})

	if !strings.Contains(a.ui.Status(), "no playlist called") {
		t.Errorf("it said %q", a.ui.Status())
	}
}
