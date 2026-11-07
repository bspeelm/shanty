package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// The frame is a character grid, and these are the columns in it that are not
// allowed to move. Each test here measures one row of a list or one half of the
// player line, because the golden files record what the frame looks like and
// say nothing about which parts of it are meant to hold still.

// tenTracks is an album long enough to have two-digit track numbers, which the
// fixtures in golden_test.go deliberately are not: the shipped albums hold two
// and four tracks, and a column that only misbehaves from track ten onwards
// cannot be seen in either.
func tenTracks() subsonic.Album {
	album := subsonic.Album{ID: "al-7", Name: "Harbour", Artist: "Aoi", ArtistID: "ar-1", SongCount: 11}
	for i := 1; i <= 11; i++ {
		album.Songs = append(album.Songs, subsonic.Song{
			ID:       fmt.Sprintf("tr-%02d", i),
			Title:    fmt.Sprintf("Track %02d", i),
			Track:    i,
			Duration: 180,
		})
	}
	return album
}

// columnOf is the column text starts in within a rendered row half, counting
// from one. Measured in terminal cells, because that is what the row is laid
// out in: leading spaces cannot be counted instead, since a marker glyph is
// not a space and is still not the name.
func columnOf(t *testing.T, row, text string) int {
	t.Helper()
	i := strings.Index(row, text)
	if i < 0 {
		t.Fatalf("row %q does not contain %q", row, text)
	}
	return lipgloss.Width(row[:i]) + 1
}

// marks is a queue holding one track, so that exactly one row is the one being
// played.
func marks(id, title string) QueueChanged {
	return QueueChanged{Tracks: []queue.Track{{ID: id, Title: title}}, At: 0}
}

// onPlaylists presses gp, which is how the playlists screen is reached. The
// loaded message only stores them; the screen is reached by the jump.
func onPlaylists(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	return next.(Model)
}

func TestAStarredNameStartsInTheSameColumnAsAnUnstarredOne(t *testing.T) {
	// starredChanged stars ar-1, the first artist, and leaves the second alone.
	m := send(t, loaded(t), starredChanged())

	starred, _ := m.row(0)
	plain, _ := m.row(1)

	got, want := columnOf(t, starred, "Aoi"), columnOf(t, plain, "The Bilge Pumps")
	if got != want {
		t.Errorf("a starred name starts in column %d and an unstarred one in column %d;\n"+
			"  starred: %q\nunstarred: %q", got, want, starred, plain)
	}
	if strings.Contains(starred, "★A") {
		t.Errorf("the star is written against the name with no column between them: %q", starred)
	}
}

func TestATwoDigitTrackNumberKeepsItsDistanceFromTheStar(t *testing.T) {
	album := tenTracks()
	tenth := album.Songs[9]

	m := send(t, sized(New()), ArtistsLoaded(library()), AlbumLoaded(album),
		StarredChanged{IDs: map[string]bool{tenth.ID: true}})

	row := rowContaining(t, m, tenth.Title)
	if strings.Contains(row, "★10") {
		t.Errorf("the star is written against a two-digit track number: %q\n"+
			"the gap after the star was the leading space of %%2d, which track 10 fills", row)
	}
}

func TestEveryRowOfAnAlbumStartsItsTitleInTheSameColumn(t *testing.T) {
	album := tenTracks()
	m := send(t, sized(New()), ArtistsLoaded(library()), AlbumLoaded(album),
		StarredChanged{IDs: map[string]bool{album.Songs[0].ID: true, album.Songs[9].ID: true}},
		marks(album.Songs[4].ID, album.Songs[4].Title))

	columns := map[int][]string{}
	for i := range m.rows() {
		left, _ := m.row(i)
		c := columnOf(t, left, "Track ")
		columns[c] = append(columns[c], left)
	}
	if len(columns) != 1 {
		t.Errorf("album rows start their titles in %d different columns, not one:", len(columns))
		for c, rows := range columns {
			t.Errorf("  column %d: %q", c, rows)
		}
	}
}

func TestThePlayingTrackIsMarkedInAnAlbumAndInAPlaylist(t *testing.T) {
	marked := marks("tr-2", "Ballast")

	album := send(t, sized(New()), ArtistsLoaded(library()),
		AlbumLoaded(library()[0].Albums[0]), marked)
	if row := rowContaining(t, album, "Ballast"); !strings.Contains(row, "▶") {
		t.Errorf("the track being played is not marked in the album it is on: %q", row)
	}

	list := subsonic.Playlist{ID: "pl-1", Name: "Evening", SongCount: 2, Songs: []subsonic.Song{
		{ID: "tr-1", Title: "Slipway", Artist: "Aoi", Duration: 180},
		{ID: "tr-2", Title: "Ballast", Artist: "Aoi", Duration: 200},
	}}
	playlist := send(t, sized(New()), PlaylistsLoaded([]subsonic.Playlist{list}),
		PlaylistLoaded(list), marked)
	if row := rowContaining(t, playlist, "Ballast"); !strings.Contains(row, "▶") {
		t.Errorf("the track being played is not marked in the playlist it is in: %q", row)
	}
}

func TestNothingIsMarkedAsPlayingWhenTheQueueHasRunOut(t *testing.T) {
	finished := QueueChanged{Tracks: queued(), At: len(queued())}
	m := send(t, sized(New()), ArtistsLoaded(library()),
		AlbumLoaded(library()[0].Albums[0]), finished)

	for i := range m.rows() {
		if left, _ := m.row(i); strings.Contains(left, "▶") {
			t.Errorf("a row is marked as playing after the queue finished: %q", left)
		}
	}
}

// rowContaining is the row half whose text holds want.
func rowContaining(t *testing.T, m Model, want string) string {
	t.Helper()
	for i := range m.rows() {
		if left, _ := m.row(i); strings.Contains(left, want) {
			return left
		}
	}
	t.Fatalf("no row contains %q on the %s screen", want, m.screen)
	return ""
}

func TestPausingDoesNotMoveTheTitleInThePlayerLine(t *testing.T) {
	base := send(t, sized(New()),
		NowPlaying{Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second},
		Progress(83*time.Second), VolumeChanged(80))

	playing := base.player()
	paused := send(t, base, PausedChanged(true)).player()

	got, want := columnOf(t, paused, "Slipway"), columnOf(t, playing, "Slipway")
	if got != want {
		t.Errorf("the title starts in column %d while paused and column %d while playing;\n"+
			"playing: %q\n paused: %q", got, want, playing, paused)
	}
}

func TestTheVolumeReachingAHundredDoesNotMoveTheClock(t *testing.T) {
	base := send(t, sized(New()),
		NowPlaying{Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second},
		Progress(83*time.Second))

	at80 := send(t, base, VolumeChanged(80)).player()
	at100 := send(t, base, VolumeChanged(100)).player()

	got, want := columnOf(t, at100, "1:23"), columnOf(t, at80, "1:23")
	if got != want {
		t.Errorf("the elapsed time starts in column %d at full volume and column %d at 80%%;\n"+
			" 80%%: %q\n100%%: %q", got, want, at80, at100)
	}
}

func TestTheListHasTheSameRowsWhetherOrNotThereIsMusic(t *testing.T) {
	silent := send(t, sized(New()), ArtistsLoaded(library()))
	playing := send(t, silent,
		NowPlaying{Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second},
		Progress(83*time.Second))

	if got, want := playing.chrome(), silent.chrome(); got != want {
		t.Errorf("the chrome is %d rows with music and %d without, so starting a track "+
			"shifts every row below the cursor", got, want)
	}
	if got, want := frameRows(playing.View()), frameRows(silent.View()); got != want {
		t.Errorf("the frame is %d rows with music and %d without", got, want)
	}
}

func frameRows(frame string) int { return strings.Count(frame, "\n") + 1 }

func TestTheHelpLineEndsOnAWholeItem(t *testing.T) {
	for _, w := range []int{40, 64, 80, 100, 132} {
		line := helpLine(w)
		if lipgloss.Width(line) > w {
			t.Errorf("at width %d the help line is %d columns: %q", w, lipgloss.Width(line), line)
		}
		for _, item := range strings.Split(strings.TrimSuffix(line, " …"), " · ") {
			if !listed(item) {
				t.Errorf("at width %d the help line stops part way through an item: %q\n"+
					"  the offending piece is %q", w, line, item)
			}
		}
	}
}

func listed(s string) bool {
	for _, item := range helpItems {
		if item == s {
			return true
		}
	}
	return false
}

func TestTheHelpLineSaysWhenItHasLeftSomethingOut(t *testing.T) {
	narrow := helpLine(64)
	if !strings.HasSuffix(narrow, " …") {
		t.Errorf("not every item fits in 64 columns, so the line should say so: %q", narrow)
	}
	wide := helpLine(400)
	if strings.HasSuffix(wide, " …") {
		t.Errorf("everything fits in 400 columns, so nothing should be left out: %q", wide)
	}
	if last := helpItems[len(helpItems)-1]; !strings.Contains(wide, last) {
		t.Errorf("%q is missing from a line with room for it: %q", last, wide)
	}
}

func TestAnEmptyPlaylistsScreenNamesTheCommandThatMakesOne(t *testing.T) {
	frame := onPlaylists(t, send(t, sized(New()), PlaylistsLoaded(nil))).View()
	if !strings.Contains(frame, ":playlist create") {
		t.Errorf("the empty playlists screen does not say how to make one:\n%s", frame)
	}
	if strings.Contains(frame, "nothing here") {
		t.Errorf("the empty playlists screen still uses the generic body:\n%s", frame)
	}
}

func TestAPlaylistNameStartsInTheSameColumnAsAnArtistName(t *testing.T) {
	artist, _ := loaded(t).row(1)

	lists := onPlaylists(t, send(t, sized(New()), PlaylistsLoaded([]subsonic.Playlist{
		{ID: "pl-1", Name: "Evening", SongCount: 12},
	})))
	list, _ := lists.row(0)

	got, want := columnOf(t, list, "Evening"), columnOf(t, artist, "The Bilge Pumps")
	if got != want {
		t.Errorf("a playlist name starts in column %d and an artist name in column %d;\n"+
			"playlist: %q\n  artist: %q", got, want, list, artist)
	}
}
