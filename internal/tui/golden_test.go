package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// The screens are rendered through View rather than through a driven terminal.
// teatest drives the real program and captures what bubbletea's renderer
// writes, cursor positioning and all, which makes a regression a diff nobody
// can read -- and readable diffs are the entire reason for golden files. The
// interaction is covered by the update tests; this covers the pixels.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nRun: go test ./internal/tui/ -update", err)
	}
	if got != string(want) {
		t.Errorf("%s changed.\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}

func library() []subsonic.Artist {
	return []subsonic.Artist{
		{ID: "ar-1", Name: "Aoi", AlbumCount: 2, Albums: []subsonic.Album{
			{ID: "al-1", Name: "Harbour", Artist: "Aoi", ArtistID: "ar-1", SongCount: 2, Duration: 360, Songs: []subsonic.Song{
				{ID: "tr-1", Title: "Slipway", Track: 1, Duration: 180},
				{ID: "tr-2", Title: "Ballast", Track: 2, Duration: 180},
			}},
			{ID: "al-2", Name: "Low Water", Artist: "Aoi", ArtistID: "ar-1", SongCount: 1, Duration: 200},
		}},
		{ID: "ar-2", Name: "The Bilge Pumps", AlbumCount: 1},
		{ID: "ar-3", Name: "坂本龍一", AlbumCount: 3},
	}
}

// spoken is a model that has said a few things, with the clock pinned so the
// times in the golden file do not move.
func spoken(t *testing.T, m Model) Model {
	t.Helper()
	at := time.Date(2026, 9, 8, 15, 4, 5, 0, time.UTC)
	m.now = func() time.Time { at = at.Add(37 * time.Second); return at }
	return send(t, m,
		Notice("leaving the interface; the music keeps playing"),
		Failed{Message: "mpv stopped: exit status 1\nrestart shanty to play again"},
		Notice("playing Ballast next"))
}

// onMessages presses :messages, which is how the screen is reached.
func onMessages(t *testing.T, m Model) Model {
	t.Helper()
	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	for _, c := range "messages" {
		m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(c))})
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			m = send(t, m, msg)
		}
	}
	return m
}

// starredChanged is what the server reports starred: one of each kind, so
// every list has a marker in it.
func starredChanged() StarredChanged {
	found := subsonic.Results{
		Artists: []subsonic.Artist{{ID: "ar-1", Name: "Aoi", AlbumCount: 2}},
		Albums:  []subsonic.Album{{ID: "al-1", Name: "Harbour", Artist: "Aoi"}},
		Songs: []subsonic.Song{
			{ID: "tr-2", Title: "Ballast", Album: "Harbour", Artist: "Aoi", Duration: 180},
		},
	}
	return StarredChanged{Results: found, IDs: map[string]bool{"ar-1": true, "al-1": true, "tr-2": true}}
}

// onStarred presses gs, which is how the starred screen is reached.
func onStarred(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	return next.(Model)
}

// results is what a search across the library comes back with: all three
// kinds at once, which is the shape the screen has to draw.
func results() subsonic.Results {
	return subsonic.Results{
		Artists: []subsonic.Artist{{ID: "ar-1", Name: "Aoi", AlbumCount: 2}},
		Albums: []subsonic.Album{
			{ID: "al-2", Name: "Low Water", Artist: "Aoi"},
			{ID: "al-9", Name: "Deep Water", Artist: "The Bilge Pumps"},
		},
		Songs: []subsonic.Song{
			{ID: "tr-1", Title: "Slipway", Album: "Harbour", AlbumID: "al-1", Artist: "Aoi", Duration: 180},
			{ID: "tr-7", Title: "Watermark", Album: "Bail", AlbumID: "al-3", Artist: "The Bilge Pumps", Duration: 245},
		},
	}
}

// queued is a queue with a track from more than one album in it, which is
// what a queue looks like once anything has been added to one.
// onQueue presses gq, which is how the queue screen is reached.
func onQueue(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	return next.(Model)
}

func queued() []queue.Track {
	return []queue.Track{
		{ID: "tr-1", Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second},
		{ID: "tr-2", Title: "Ballast", Artist: "Aoi", Duration: 200 * time.Second},
		{ID: "tr-9", Title: "Low Water", Artist: "The Bilge Pumps", Duration: 220 * time.Second},
	}
}

func sized(m Model) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 64, Height: 12})
	return next.(Model)
}

func send(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestGoldenScreens(t *testing.T) {
	base := sized(New())
	artists := send(t, base, ArtistsLoaded(library()))
	albums := send(t, artists, ArtistLoaded(library()[0]))
	tracks := send(t, albums, AlbumLoaded(library()[0].Albums[0]))

	for _, tc := range []struct {
		name string
		m    Model
	}{
		{"loading", base},
		{"artists", artists},
		{"artists-cursor-moved", send(t, artists, tea.KeyMsg{Type: tea.KeyDown})},
		{"albums", albums},
		{"tracks", tracks},
		// The queue arrives before the screen is opened, which is the order it
		// happens in: something is playing, and then you look at it.
		{"queue", onQueue(t, send(t, tracks,
			QueueChanged{Tracks: queued(), At: 1},
			NowPlaying{Title: "Ballast", Artist: "Aoi", Duration: 200 * time.Second},
			Progress(45*time.Second)))},
		{"queue-empty", onQueue(t, tracks)},
		{"artists-starred", send(t, artists, starredChanged())},
		{"albums-starred", send(t, albums, starredChanged())},
		{"tracks-starred", send(t, tracks, starredChanged())},
		{"starred", onStarred(t, send(t, tracks, starredChanged()))},
		{"starred-nothing", onStarred(t, tracks)},
		{"messages", onMessages(t, spoken(t, artists))},
		{"messages-nothing", onMessages(t, artists)},
		{"search", send(t, base, SearchLoaded{Query: "water", Results: results()})},
		{"search-nothing", send(t, base, SearchLoaded{Query: "zzzz", Results: subsonic.Results{}})},
		{"search-tracks-only", send(t, base, SearchLoaded{Query: "slipway",
			Results: subsonic.Results{Songs: results().Songs}})},
		{"queue-finished", onQueue(t, send(t, tracks,
			QueueChanged{Tracks: queued(), At: 3}))},
		{"tracks-playing", send(t, tracks,
			NowPlaying{Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second},
			Progress(83*time.Second),
			VolumeChanged(80))},
		{"tracks-at-the-start", send(t, tracks,
			NowPlaying{Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second})},
		{"tracks-at-the-end", send(t, tracks,
			NowPlaying{Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second},
			Progress(180*time.Second))},
		{"tracks-unknown-length", send(t, tracks,
			NowPlaying{Title: "Slipway", Artist: "Aoi"},
			Progress(83*time.Second))},
		{"tracks-paused", send(t, tracks,
			NowPlaying{Title: "Slipway", Artist: "Aoi", Duration: 180 * time.Second},
			Progress(83*time.Second),
			PausedChanged(true))},
		{"commanding", func() Model {
			c, _ := artists.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
			return c.(Model)
		}()},
		{"commanding-typed", func() Model {
			c, _ := artists.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
			m := c.(Model)
			for _, r := range "vol" {
				n, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = n.(Model)
			}
			return m
		}()},
		{"filtering", func() Model {
			f, _ := artists.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
			m := f.(Model)
			for _, c := range "bilge" {
				n, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
				m = n.(Model)
			}
			return m
		}()},
		{"filtering-no-match", func() Model {
			f, _ := artists.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
			m := f.(Model)
			for _, c := range "zzz" {
				n, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
				m = n.(Model)
			}
			return m
		}()},
		{"failed", send(t, base, Failed{Message: "the server refused; check credentials.toml"})},
		// The case that had no coverage: a failure while a list is on screen,
		// which is when most failures happen.
		{"failed-with-a-list", send(t, artists,
			Failed{Message: "mpv stopped: exit status 1\nrestart shanty to play again"})},
	} {
		t.Run(tc.name, func(t *testing.T) { golden(t, tc.name, tc.m.View()) })
	}
}

// ADR-013 as a screen test rather than a unit test: whatever the server sends,
// no frame this program prints contains a byte a terminal would obey.
func TestNoFrameEverCarriesAnEscape(t *testing.T) {
	poison := func(s string) string { return hostile + s }

	arts := library()
	for i := range arts {
		arts[i].Name = poison(arts[i].Name)
		for j := range arts[i].Albums {
			arts[i].Albums[j].Name = poison(arts[i].Albums[j].Name)
			arts[i].Albums[j].Artist = poison(arts[i].Albums[j].Artist)
			for k := range arts[i].Albums[j].Songs {
				arts[i].Albums[j].Songs[k].Title = poison(arts[i].Albums[j].Songs[k].Title)
			}
		}
	}

	base := sized(New())
	artists := send(t, base, ArtistsLoaded(arts))
	albums := send(t, artists, ArtistLoaded(arts[0]))
	tracks := send(t, albums, AlbumLoaded(arts[0].Albums[0]))
	playing := send(t, tracks, NowPlaying{Title: poison("Slipway"), Artist: poison("Aoi"), Duration: time.Minute})
	failed := send(t, base, Failed{Message: poison("the server refused")})

	for name, m := range map[string]Model{
		"artists": artists, "albums": albums, "tracks": tracks,
		"playing": playing, "failed": failed,
	} {
		frame := m.View()
		// The styles themselves emit SGR codes, so the assertion is on the
		// sequences a hostile string would introduce, not on ESC outright.
		for _, forbidden := range []string{"\x1b[2J", "\x1b[H", "\x1b[6n", "\x1b[1;1r", "\a", "\r", "\n\x9b", "\u202e"} {
			if strings.Contains(frame, forbidden) {
				t.Errorf("the %s frame carries %q", name, forbidden)
			}
		}
		for _, r := range stripSGR(frame) {
			if control(r) && r != '\n' {
				t.Errorf("the %s frame carries control character %U", name, r)
			}
		}
	}
}

// stripSGR removes the colour and attribute sequences lipgloss writes, leaving
// what a hostile string would have contributed.
func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' && s[j] != 'J' && s[j] != 'H' && s[j] != 'n' && s[j] != 'r' {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// A message with a line break in it becomes one row rather than two halves run
// together. The errors that produce these put the cause on one line and what
// to do about it on the next.
func TestAMultiLineMessageIsJoinedRatherThanConcatenated(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(library()),
		Failed{Message: "the server refused\ncheck credentials.toml"})

	frame := m.View()
	if strings.Contains(frame, "refusedcheck") {
		t.Errorf("the two lines ran together:\n%s", frame)
	}
	if !strings.Contains(frame, "the server refused · check credentials.toml") {
		t.Errorf("the message is not on one row:\n%s", frame)
	}
}
