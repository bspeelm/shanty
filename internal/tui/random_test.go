package tui

import (
	"math/rand/v2"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// actor stands in for the program around the interface. It answers each
// intent the way the app does, and holds the answers until they are
// delivered: some late, some twice, some as failures, some with shorter or
// empty lists.
type actor struct {
	r       *rand.Rand
	waiting []tea.Msg
}

// twoLists is the playlists the actor's server holds.
var twoLists = []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}

func (a *actor) artist(id string) ArtistLoaded {
	return ArtistLoaded{ID: id, Name: "Aoi", Albums: library()[0].Albums[:a.r.IntN(3)]}
}

func (a *actor) album(id string) AlbumLoaded {
	album := tenTracks()
	album.ID, album.Songs = id, album.Songs[:a.r.IntN(len(album.Songs)+1)]
	return AlbumLoaded(album)
}

func (a *actor) playlist(id string) subsonic.Playlist {
	p := eveningOf(a.r.IntN(4))
	p.ID = id
	return p
}

func (a *actor) starred() StarredChanged {
	s := starredChanged()
	s.Results.Artists = s.Results.Artists[:a.r.IntN(2)]
	s.Results.Albums = s.Results.Albums[:a.r.IntN(2)]
	s.Results.Songs = s.Results.Songs[:a.r.IntN(2)]
	return s
}

func (a *actor) found(query string) SearchLoaded {
	found := results()
	found.Artists = found.Artists[:a.r.IntN(2)]
	found.Albums = found.Albums[:a.r.IntN(3)]
	found.Songs = found.Songs[:a.r.IntN(3)]
	return SearchLoaded{Query: query, Results: found}
}

func (a *actor) queue() QueueChanged {
	tracks := queued()[:a.r.IntN(4)]
	return QueueChanged{Tracks: tracks, At: a.r.IntN(len(tracks) + 1)}
}

// opened holds the answer to an open: usually what was asked for, sometimes
// twice, and sometimes a failure.
func (a *actor) opened(ask int, loaded tea.Msg) {
	switch a.r.IntN(8) {
	case 0:
		a.waiting = append(a.waiting, Failed{Message: "the server did not answer", Ask: ask})
	case 1:
		a.waiting = append(a.waiting, Opened{Ask: ask, Loaded: loaded}, Opened{Ask: ask, Loaded: loaded})
	default:
		a.waiting = append(a.waiting, Opened{Ask: ask, Loaded: loaded})
	}
}

// reload holds the answer to a reload of the screen showing.
func (a *actor) reload(m Model) {
	var loaded tea.Msg
	switch m.Screen() {
	case ScreenLibrary, ScreenArtists:
		loaded = ArtistsLoaded(library()[:a.r.IntN(4)])
	case ScreenAlbums:
		loaded = a.artist(m.artist.ID)
	case ScreenEveryAlbum:
		loaded = AlbumsLoaded(everyAlbum()[:a.r.IntN(4)])
	case ScreenTracks:
		loaded = a.album(m.album.ID)
	case ScreenStarred:
		loaded = a.starred()
	default:
		a.waiting = append(a.waiting, Failed{Message: "there is nothing here to fetch again"})
		return
	}
	a.waiting = append(a.waiting, Reloaded{Loaded: loaded})
	if a.r.IntN(4) == 0 {
		a.waiting = append(a.waiting, Reloaded{Loaded: loaded})
	}
}

// answer holds what the app would send back for an intent. An intent the app
// passes straight back to the interface is returned to be sent at once.
func (a *actor) answer(m Model, intent tea.Msg) tea.Msg {
	switch intent := intent.(type) {
	case OpenArtist:
		a.opened(intent.Ask, a.artist(intent.ID))
	case OpenAlbum:
		a.opened(intent.Ask, a.album(intent.ID))
	case OpenPlaylist:
		a.opened(intent.Ask, PlaylistLoaded(a.playlist(intent.ID)))
	case FetchPlaylists:
		a.waiting = append(a.waiting, PlaylistsLoaded(twoLists[:a.r.IntN(3)]))
	case FetchEveryAlbum:
		a.waiting = append(a.waiting, AlbumsLoaded(everyAlbum()[:a.r.IntN(4)]))
	case Reload:
		a.reload(m)
		return intent
	case Search:
		a.waiting = append(a.waiting, a.found(string(intent)))
	case EditPlaylist:
		a.waiting = append(a.waiting, PlaylistChanged(a.playlist(intent.ID)))
	case RenamePlaylist:
		a.waiting = append(a.waiting, PlaylistChanged(a.playlist(intent.ID)))
	case ReorderPlaylist:
		if a.r.IntN(3) == 0 {
			a.waiting = append(a.waiting, Failed{Message: "it changed on the server; nothing was moved"})
		} else {
			a.waiting = append(a.waiting, PlaylistChanged(a.playlist(intent.ID)))
		}
	case DeletePlaylist:
		return Confirm{Question: "delete it?", Do: Notice("deleted")}
	case Playlist:
		if intent.Verb == "edit" {
			a.waiting = append(a.waiting, EditingPlaylist(a.playlist("pl-1")))
		}
	case ToggleStar:
		a.waiting = append(a.waiting, a.starred())
	case PlayFrom, PlayNext, Enqueue, JumpTo, Shuffle, ShuffleThis, ShuffleSongs, SkipNext, SkipPrev:
		a.waiting = append(a.waiting, a.queue())
	case ShowArt:
		return FullArt{"a picture filling the screen"}
	case ShowWiki, ShowMessages, ShowVisualiser, Notice:
		return intent
	}
	return nil
}

// run is one seeded run of the interface with the actor answering it.
type run struct {
	t     *testing.T
	seed  uint64
	r     *rand.Rand
	m     Model
	app   actor
	steps int
	// keys counts the keys each screen received with no overlay over it and
	// no line being typed, and reorders the times a reorder was entered.
	keys     map[Screen]int
	reorders int
}

// send applies one message, draws the frame, checks every cursor, and has the
// actor answer whatever was emitted. The visualiser's own clock is not run.
func (r *run) send(msg tea.Msg) {
	for depth := 0; msg != nil && depth < 4; depth++ {
		was := r.m.Reordering()
		next, cmd := r.m.Update(msg)
		r.m = next.(Model)
		r.steps++
		if !was && r.m.Reordering() {
			r.reorders++
		}
		_ = r.m.View()
		if err := cursorsWithinTheirLists(r.m); err != nil {
			r.t.Fatalf("seed %d, step %d: %v, showing the %s screen, after %#v",
				r.seed, r.steps, err, r.m.Screen(), msg)
		}
		msg = nil
		if cmd != nil && !r.m.ShowingSky() {
			msg = r.app.answer(r.m, cmd())
		}
	}
}

// press sends keys one at a time, counting those that reach a list.
func (r *run) press(keys ...string) {
	for _, key := range keys {
		typing := r.m.Filtering() || r.m.Commanding() || r.m.Confirming()
		if !typing && !r.m.ShowingSky() && !r.m.ShowingArt() {
			r.keys[r.m.Screen()]++
		}
		if key == " " {
			r.send(tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		r.send(keyOf(key))
	}
}

// command types a line on the command line and runs it.
func (r *run) command(line string) {
	r.send(keyOf(":"))
	for _, c := range line {
		if c == ' ' {
			r.send(tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		r.send(keyOf(string(c)))
	}
	r.send(keyOf("enter"))
}

// pick is one of the choices, at random.
func pick[T any](r *rand.Rand, choices ...T) T { return choices[r.IntN(len(choices))] }

// step does one thing a person or the server could do next.
func (r *run) step() {
	// An overlay is left soon after it opens.
	if (r.m.ShowingSky() || r.m.ShowingArt()) && r.r.IntN(3) > 0 {
		r.press("esc")
		return
	}
	switch n := r.r.IntN(100); {
	case n < 40 && len(r.app.waiting) > 0:
		i := r.r.IntN(len(r.app.waiting))
		msg := r.app.waiting[i]
		r.app.waiting = append(r.app.waiting[:i:i], r.app.waiting[i+1:]...)
		r.send(msg)
	case n < 50:
		// An open answered before anything else is pressed.
		r.press("enter")
		for len(r.app.waiting) > 0 {
			msg := r.app.waiting[0]
			r.app.waiting = r.app.waiting[1:]
			r.send(msg)
		}
	case n < 56:
		r.press("g", pick(r.r, "l", "a", "b", "p", "q", "s", "g"))
	case n < 63:
		line := pick(r.r, "reload", "search water", "playlist", "messages", "wiki", "playlist edit Evening",
			"playlist delete", "nosuchthing")
		if r.m.Screen() == ScreenPlaylist && r.r.IntN(2) == 0 {
			line = "playlist reorder"
		}
		r.command(line)
	case n < 66:
		r.press("/")
		r.press(pick(r.r, "a", "w", "zz", "b"))
		r.press(pick(r.r, "enter", "esc", "backspace"))
	case n < 68:
		r.send(tea.WindowSizeMsg{Width: pick(r.r, 24, 40, 80, 200), Height: 4 + r.r.IntN(30)})
	case n < 71:
		r.send(pick[tea.Msg](r.r, r.app.queue(), r.app.starred(), Notice("playing Ballast next"),
			Failed{Message: "nothing is playing to seek in"}, ArtistsLoaded(library()[:r.r.IntN(4)])))
	case n < 72:
		r.send(pick[tea.Msg](r.r, ShowVisualiser{}, FullArt{"a picture filling the screen"}))
	default:
		r.press(pick(r.r, "j", "j", "j", "k", "k", "enter", "enter", "enter", "h", "G", "home",
			"end", "pgup", "pgdown", "S", "*", "a", "A", "r", "e", "J", "K", "?", "y", "5", "%", " ", "n", "x"))
	}
}

// TestNoRunOfKeysAndAnswersLeavesACursorOffItsList drives the interface with
// keys, typed commands and resizes drawn from a seeded generator, with an
// actor answering what it emits. After every message the frame is drawn and
// the cursor of every screen is checked against its list.
func TestNoRunOfKeysAndAnswersLeavesACursorOffItsList(t *testing.T) {
	seeds, steps := uint64(4), 250
	keys, reorders := map[Screen]int{}, 0
	for seed := uint64(1); seed <= seeds; seed++ {
		r := &run{t: t, seed: seed, r: rand.New(rand.NewPCG(seed, 11)), keys: keys}
		r.app.r = r.r
		r.m = sized(New())
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("seed %d, step %d: %v on the %s screen", seed, r.steps, p, r.m.Screen())
				}
			}()
			r.send(ArtistsLoaded(library()))
			for r.steps < steps {
				r.step()
			}
		}()
		reorders += r.reorders
	}

	for _, s := range Screens {
		t.Logf("%-12s %4d keys", s, keys[s])
		if keys[s] == 0 {
			t.Errorf("no key was pressed on the %s screen in %d runs", s, seeds)
		}
	}
	t.Logf("%d reorders entered", reorders)
	if reorders == 0 {
		t.Errorf("a reorder was never entered in %d runs", seeds)
	}
}
