package tui

import (
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// evening is a playlist open on screen, which is what rename renames.
func evening(t *testing.T) Model {
	t.Helper()
	list := subsonic.Playlist{ID: "pl-1", Name: "Evening", SongCount: 1, Songs: []subsonic.Song{
		{ID: "tr-1", Title: "Slipway", Artist: "Aoi", Duration: 180},
	}}
	return send(t, loaded(t), PlaylistsLoaded([]subsonic.Playlist{list}), PlaylistLoaded(list))
}

func TestRenamingActsOnThePlaylistOnScreen(t *testing.T) {
	_, msg := typeCommand(t, evening(t), "playlist rename Evening Watch")

	got, ok := msg.(RenamePlaylist)
	if !ok {
		t.Fatalf(":playlist rename emitted %#v, want RenamePlaylist", msg)
	}
	// The identifier, not the old name: a server's playlist names are not
	// unique, so renaming by name is a question nobody should have to answer.
	if got.ID != "pl-1" {
		t.Errorf("renamed %q, want the playlist on screen, pl-1", got.ID)
	}
	// The argument is the new name, spaces and all.
	if got.Name != "Evening Watch" {
		t.Errorf("renamed to %q, want %q", got.Name, "Evening Watch")
	}
}

func TestRenamingOffAPlaylistScreenSaysWhereToPressIt(t *testing.T) {
	// The artist list, which is where the program starts.
	m, msg := typeCommand(t, loaded(t), "playlist rename Evening Watch")

	if msg != nil {
		t.Errorf("rename with no playlist on screen emitted %#v, want nothing", msg)
	}
	if !strings.Contains(m.status, "needs a playlist on screen") {
		t.Errorf("the status does not say why nothing happened: %q", m.status)
	}
	// Every failure line in this program carries the thing that fixes it.
	if !strings.Contains(m.status, ":playlist rename") {
		t.Errorf("the status does not name the command to run: %q", m.status)
	}
}

func TestRenamingWithNothingAfterItAsksForTheNewName(t *testing.T) {
	m, msg := typeCommand(t, evening(t), "playlist rename")

	if msg != nil {
		t.Errorf("rename with no argument emitted %#v, want nothing", msg)
	}
	if !strings.Contains(m.status, "new name") {
		t.Errorf("the status should ask for the new name rather than a playlist: %q", m.status)
	}
}

func TestRenameIsOneOfTheVerbsTheCommandOffers(t *testing.T) {
	if !playlistVerbs["rename"] {
		t.Error("rename is not an accepted verb")
	}
	if !strings.Contains(verbList(), "rename") {
		t.Errorf("the verb list does not offer rename: %q", verbList())
	}
	// A verb that is not offered is one nobody finds: the list is what the
	// completion row and the error for a wrong verb both read from.
	_, err := playlistCommand("renaem Evening")
	if err == nil || !strings.Contains(err.Error(), "rename") {
		t.Errorf("a misspelt verb should be answered with the list: %v", err)
	}
}
