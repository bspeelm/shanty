package tui

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// actor stands in for the program around the interface. It answers each
// intent the way the app does, and holds the answers until they are
// delivered: some late, some twice, some as failures, some with shorter or
// empty lists.
type actor struct {
	r       *rand.Rand
	waiting []tea.Msg
	// q is the queue, kept with the functions the program keeps it with.
	// selected is the track the cursor was on when a key that edits the queue
	// was pressed, and broke is the first thing found wrong with an edit.
	q        queue.Queue
	selected string
	broke    string
	// did names an edit of the queue just made, to be counted.
	did string
}

// queued is the queue as the interface is sent it.
func (a *actor) queued() QueueChanged {
	return QueueChanged{Tracks: a.q.Tracks(), At: a.q.At()}
}

// tracksOf is the songs of an album as tracks of the queue.
func tracksOf(album subsonic.Album) []queue.Track {
	var out []queue.Track
	for _, s := range album.Songs {
		out = append(out, queue.Track{ID: s.ID, Title: s.Title})
	}
	return out
}

// edited takes an edit of the queue and holds it against what an edit of that
// kind should leave current: the same track, or the one that followed it when
// it was the one taken out.
func (a *actor) edited(next queue.Queue, removed, from, to int) {
	before, at := a.q.Tracks(), a.q.At()
	want := ""
	switch {
	case removed == at && at+1 < len(before):
		want = before[at+1].ID
	case removed != at && at < len(before):
		want = before[at].ID
	}
	touched := max(removed, from)
	if now, _ := next.Current(); now.ID != want && a.broke == "" {
		a.broke = fmt.Sprintf("with %q current, an edit of row %d left %q current, want %q", idOf(before, at), touched, now.ID, want)
	}
	if id := idOf(before, touched); a.selected != "" && id != a.selected && a.broke == "" {
		a.broke = fmt.Sprintf("the cursor was on %q and the edit named row %d, which held %q", a.selected, touched, id)
	}
	if from >= 0 && to >= 0 && to < len(before) && idOf(next.Tracks(), to) != idOf(before, from) && a.broke == "" {
		a.broke = fmt.Sprintf("moving row %d to %d left %q there", from, to, idOf(next.Tracks(), to))
	}
	a.q = next
}

// idOf is the identifier of the track at a place, or nothing where there is
// none.
func idOf(tracks []queue.Track, i int) string {
	if i < 0 || i >= len(tracks) {
		return ""
	}
	return tracks[i].ID
}

// twoLists is the playlists the actor's server holds.
var twoLists = []subsonic.Playlist{{ID: "pl-1", Name: "Evening"}, {ID: "pl-2", Name: "Morning"}}

func (a *actor) artist(id string) ArtistLoaded {
	albums := append([]subsonic.Album(nil), library()[0].Albums[:min(2, a.r.IntN(5))]...)
	for i := range albums {
		albums[i].ID = id + "/" + albums[i].ID
	}
	return ArtistLoaded{ID: id, Name: "artist " + id, Albums: albums}
}

func (a *actor) album(id string) AlbumLoaded {
	album := tenTracks()
	album.ID, album.Songs = id, album.Songs[:a.r.IntN(len(album.Songs)+1)]
	return AlbumLoaded(album)
}

func (a *actor) playlist(id string) subsonic.Playlist {
	p := eveningOf(1 + a.r.IntN(3))
	p.ID = id
	if a.r.IntN(3) == 0 {
		p.Songs = p.Songs[:a.r.IntN(len(p.Songs)+1)]
	}
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
	a.q = queue.New(tracks...).Jump(a.r.IntN(len(tracks) + 1))
	return a.queued()
}

// opened holds the answer to an open: usually what was asked for, sometimes
// twice, and sometimes a failure.
func (a *actor) opened(ask int, loaded tea.Msg) {
	switch a.r.IntN(8) {
	case 0, 2:
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
		a.waiting = append(a.waiting, PlaylistsLoaded(twoLists[:min(2, a.r.IntN(6))]))
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
	case PlayFrom:
		a.q = queue.New(tracksOf(intent.Album)...).Jump(intent.Index)
		return a.queued()
	case Enqueue:
		a.q = a.q.AppendAll(tracksOf(intent.Album)[intent.Index])
		return a.queued()
	case PlayNext:
		a.q = a.q.InsertAllNext(tracksOf(intent.Album)[intent.Index])
		return a.queued()
	case QueueThis:
		if intent.Next {
			a.q = a.q.InsertAllNext(queued()...)
		} else {
			a.q = a.q.AppendAll(queued()...)
		}
		return a.queued()
	case RemoveQueued:
		a.did = "a track taken out of the queue"
		if int(intent) == a.q.At() {
			a.did = "the playing track taken out of the queue"
		}
		a.edited(a.q.Remove(int(intent)), int(intent), -1, -1)
		return a.queued()
	case MoveQueued:
		a.did = "a track moved in the queue"
		a.edited(a.q.Move(intent.From, intent.To), -1, intent.From, intent.To)
		return a.queued()
	case JumpTo:
		a.q = a.q.Jump(int(intent))
		return a.queued()
	case SkipNext:
		a.q = a.q.Next()
		return a.queued()
	case SkipPrev:
		a.q = a.q.Previous()
		return a.queued()
	case ShuffleThis:
		if intent.Scope == ScopeQueue {
			a.q = a.q.ShuffleRest(a.r)
			return a.queued()
		}
		return a.queue()
	case Shuffle, ShuffleSongs:
		return a.queue()
	case ShowArt:
		return FullArt{"a picture filling the screen"}
	case SeekToPercent:
		return Failed{Message: "nothing is playing to seek in"}
	case ShowWiki, ShowMessages, ShowVisualiser, Notice:
		return intent
	}
	return nil
}

// held is what an answer that is dropped must leave as it was.
type held struct {
	screen                  Screen
	cursor                  map[Screen]int
	artist, album, playlist string
	albums, songs, tracks   int
	waiting                 int
}

func holding2(m Model) held {
	return held{m.screen, m.cursor, m.artist.ID, m.album.ID, m.playlist.ID,
		len(m.artist.Albums), len(m.album.Songs), len(m.playlist.Songs), m.waiting}
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
	// no line being typed. kinds counts what was delivered and done, by name.
	keys  map[Screen]int
	kinds map[string]int
}

// failf stops the run, saying where it had reached.
func (r *run) failf(msg tea.Msg, format string, args ...any) {
	r.t.Helper()
	r.t.Fatalf("seed %d, message %d: %s, showing the %s screen, after %#v",
		r.seed, r.steps, fmt.Sprintf(format, args...), r.m.Screen(), msg)
}

// check holds what a message did against what that kind of message may do.
func (r *run) check(was Model, msg tea.Msg) {
	r.t.Helper()
	before, after := holding2(was), holding2(r.m)
	switch msg := msg.(type) {
	case Opened:
		if msg.Ask == 0 || msg.Ask != was.waiting {
			r.kinds["an open's answer that was not waited for"]++
			if !reflect.DeepEqual(before, after) {
				r.failf(msg, "an answer nobody was waiting for changed %+v to %+v", before, after)
			}
			return
		}
		r.kinds["an open's answer that was waited for"]++
		want, id := ScreenAlbums, after.artist
		switch loaded := msg.Loaded.(type) {
		case ArtistLoaded:
			want, id = ScreenAlbums, loaded.ID
			if after.artist != id {
				r.failf(msg, "the artist held is %q, want %q", after.artist, id)
			}
		case AlbumLoaded:
			want, id = ScreenTracks, loaded.ID
			if after.album != id {
				r.failf(msg, "the album held is %q, want %q", after.album, id)
			}
		case PlaylistLoaded:
			want, id = ScreenPlaylist, loaded.ID
			if after.playlist != id {
				r.failf(msg, "the playlist held is %q, want %q", after.playlist, id)
			}
		}
		if after.screen != want || r.m.loading() || after.waiting != 0 {
			r.failf(msg, "the open waited for left the screen on %s, loading %v, waiting for %d",
				after.screen, r.m.loading(), after.waiting)
		}
	case Reloaded:
		r.kinds["a reload's answer"]++
		if after.screen != before.screen {
			r.failf(msg, "a reload's answer moved the screen from %s", before.screen)
		}
	case PlaylistChanged:
		r.kinds["a changed playlist"]++
		if was.reordering && was.playlist.ID == msg.ID {
			r.kinds["a changed playlist during a reorder of it"]++
		}
		if was.editing.ID == msg.ID {
			r.kinds["a changed playlist while adding to it"]++
		}
		if after.screen != before.screen {
			r.failf(msg, "a changed playlist moved the screen from %s", before.screen)
		}
	case Failed:
		switch {
		case msg.Ask == 0:
			r.kinds["a failure with no number"]++
			if after.waiting != before.waiting {
				r.failf(msg, "a failure about something else ended the wait for open %d", before.waiting)
			}
			if before.waiting != 0 {
				r.kinds["a failure with no number while an open was waited for"]++
			}
		case msg.Ask == before.waiting:
			r.kinds["the failure of the open waited for"]++
			if after.waiting != 0 {
				r.failf(msg, "the open that failed is still waited for")
			}
		default:
			r.kinds["the failure of an open not waited for"]++
			if !reflect.DeepEqual(before, after) {
				r.failf(msg, "the failure of an open nobody was waiting for changed %+v to %+v", before, after)
			}
		}
	}
}

// send applies one message, draws the frame, checks every cursor and what the
// message was allowed to do, and has the actor answer whatever was emitted.
// The command that starts the visualiser's clock is not run.
func (r *run) send(msg tea.Msg) {
	for depth := 0; msg != nil && depth < 4; depth++ {
		was := r.m
		next, cmd := r.m.Update(msg)
		r.m = next.(Model)
		r.steps++
		if !was.Reordering() && r.m.Reordering() {
			r.kinds["a reorder entered"]++
		}
		_ = r.m.View()
		if err := cursorsWithinTheirLists(r.m); err != nil {
			r.failf(msg, "%v", err)
		}
		r.check(was, msg)
		if r.m.screen == ScreenWiki && r.m.topic == "" && r.m.filter == "" && was.screen != ScreenWiki {
			listed := listedKeys(r.m, "")
			for _, j := range jumps() {
				if !listed["g"+j.key] {
					r.failf(msg, "g%s is a jump and the keys screen does not list it", j.key)
				}
			}
		}
		emitted := msg
		msg = nil
		if cmd != nil && (was.ShowingSky() || !r.m.ShowingSky()) {
			msg = r.app.answer(r.m, cmd())
			if r.app.did != "" {
				r.kinds[r.app.did]++
				if r.app.did == "the playing track taken out of the queue" {
					r.kinds["a track taken out of the queue"]++
				}
				r.app.did = ""
			}
			if r.app.broke != "" {
				r.failf(emitted, "%s", r.app.broke)
			}
		}
	}
}

// deliver sends one of the answers the actor is holding, not always the
// oldest. When none is left, nothing may still be drawn as loading.
func (r *run) deliver() {
	i := r.r.IntN(len(r.app.waiting))
	msg := r.app.waiting[i]
	r.app.waiting = append(r.app.waiting[:i:i], r.app.waiting[i+1:]...)
	r.send(msg)
	if len(r.app.waiting) == 0 && r.m.loading() {
		r.failf(msg, "every answer has arrived and the screen is still loading, waiting for open %d, reload %q",
			r.m.waiting, r.m.keep)
	}
}

// press sends keys one at a time, counting where each one landed.
func (r *run) press(keys ...string) {
	for _, key := range keys {
		typing := r.m.Filtering() || r.m.Commanding()
		switch {
		case r.m.Confirming():
			r.kinds["a question answered"]++
		case typing && key == "ctrl+c":
			r.kinds["ctrl+c while typing"]++
		case typing && key == "tab":
			r.kinds["tab while typing"]++
		case typing:
		case r.m.ShowingSky():
			r.kinds["a key under the visualiser"]++
		case r.m.ShowingArt():
			r.kinds["a key under a cover"]++
		default:
			r.keys[r.m.Screen()]++
		}
		plain := !typing && !r.m.Confirming() && !r.m.ShowingSky() && !r.m.ShowingArt()
		r.app.selected = ""
		if plain && r.m.screen == ScreenQueue && r.m.editing.ID == "" {
			r.app.selected = r.m.under().id
		}
		// Back with nothing else for it to leave returns to the place that
		// was left last, holding what it held.
		was, goesBack := r.m, plain && (key == "esc" || key == "h") && !r.m.reordering &&
			r.m.filter == "" && r.m.topic == "" && len(r.m.trail) > 0
		switch key {
		case " ":
			r.send(tea.KeyMsg{Type: tea.KeySpace})
		case "ctrl+c":
			r.send(tea.KeyMsg{Type: tea.KeyCtrlC})
		case "ctrl+d":
			r.send(tea.KeyMsg{Type: tea.KeyCtrlD})
		case "ctrl+u":
			r.send(tea.KeyMsg{Type: tea.KeyCtrlU})
		default:
			r.send(keyOf(key))
		}
		if goesBack {
			r.kinds["a back to a place that was left"]++
			p := was.trail[len(was.trail)-1]
			switch {
			case r.m.screen != p.screen:
				r.failf(keyOf(key), "back went to %s, want %s", r.m.screen, p.screen)
			case p.screen == ScreenAlbums && r.m.artist.ID != p.artist.ID:
				r.failf(keyOf(key), "back shows the albums of %q, and those of %q were left", r.m.artist.ID, p.artist.ID)
			case p.screen == ScreenTracks && r.m.album.ID != p.album.ID:
				r.failf(keyOf(key), "back shows the tracks of %q, and those of %q were left", r.m.album.ID, p.album.ID)
			}
			if p.screen == ScreenAlbums && p.artist.ID != was.artist.ID {
				r.kinds["a back to an artist other than the one held"]++
			}
		}
		if r.m.screen == ScreenAlbums {
			for _, album := range r.m.artist.Albums {
				if !strings.HasPrefix(album.ID, r.m.artist.ID+"/") || r.m.heading() != "artist "+r.m.artist.ID {
					r.failf(keyOf(key), "the albums of %q are shown under the heading of another artist", album.ID)
				}
			}
		}
	}
}

// reachesTheLibrary reports whether back, pressed often enough, ends on the
// library from here. Nothing answers what the presses ask for.
func reachesTheLibrary(m Model) bool {
	for range trailKept + 12 {
		key := keyOf("esc")
		if m.Confirming() {
			key = keyOf("n")
		}
		next, _ := m.Update(key)
		m = next.(Model)
		if m.screen == ScreenLibrary && m.mode == modeNormal && !m.showSky && len(m.bigArt) == 0 {
			return true
		}
	}
	return false
}

// command types a line on the command line and runs it.
func (r *run) command(line string) {
	r.press(":")
	for _, c := range line {
		r.press(string(c))
	}
	r.press("enter")
}

// pick is one of the choices, at random.
func pick[T any](r *rand.Rand, choices ...T) T { return choices[r.IntN(len(choices))] }

// step does one thing a person or the server could do next.
func (r *run) step() {
	// An overlay is left soon after it opens, and a question is answered.
	if (r.m.ShowingSky() || r.m.ShowingArt()) && r.r.IntN(4) > 0 {
		r.press(pick(r.r, "esc", "esc", "j", " ", "g", "5"))
		return
	}
	if r.m.Confirming() {
		r.press(pick(r.r, "y", "n"))
		return
	}
	// A playlist being reordered is moved about, saved, or changed by somebody
	// else, and one on screen is sometimes reordered.
	if r.m.Reordering() && r.r.IntN(3) > 0 {
		if r.r.IntN(3) == 0 {
			r.send(PlaylistChanged(r.app.playlist(r.m.playlist.ID)))
			return
		}
		r.press(pick(r.r, "J", "K", "j", "enter"))
		return
	}
	if r.m.Screen() == ScreenPlaylist && !r.m.Reordering() && r.r.IntN(4) == 0 {
		r.command("playlist reorder")
		return
	}
	// The playlist being added to is looked for in the list of playlists.
	if r.m.Editing().ID != "" && r.m.Screen() != ScreenPlaylists && r.r.IntN(6) == 0 {
		r.press("g", "p")
		return
	}
	// A page of the wiki is read to its end and the window is resized.
	if r.m.reading() && r.r.IntN(3) == 0 {
		r.press("G")
		r.kinds["a resize"]++
		r.send(tea.WindowSizeMsg{Width: pick(r.r, 24, 40, 80, 200), Height: 4 + r.r.IntN(30)})
		return
	}
	// An artist, and then one of their albums, is opened and waited for.
	if (r.m.screen == ScreenArtists || r.m.screen == ScreenAlbums) && r.r.IntN(4) == 0 {
		r.press(pick(r.r, "j", "k"), "enter")
		for len(r.app.waiting) > 0 {
			r.deliver()
		}
		return
	}
	// From a screen opened from a row, another artist is opened from the
	// starred list and the way back is walked.
	if deep(r.m.screen) && r.m.screen != ScreenPlaylist && r.m.artist.ID != "ar-1" && r.r.IntN(3) == 0 {
		r.send(starredChanged())
		r.press("g", "s", "enter")
		for len(r.app.waiting) > 0 {
			r.deliver()
		}
		r.press("esc", "esc", "esc")
		return
	}
	// The track playing, and others, are taken out of the queue and moved.
	if r.m.screen == ScreenQueue && r.r.IntN(3) == 0 {
		r.press(pick(r.r, "r", "r", "K", "J", "j", "k"))
		return
	}
	// A seek with nothing playing fails while something is being opened.
	if r.m.waiting != 0 && r.r.IntN(5) == 0 {
		r.press("5", "%")
		return
	}
	switch n := r.r.IntN(100); {
	case n < 22 && len(r.app.waiting) > 0:
		r.deliver()
	case n < 30:
		if r.r.IntN(30) == 0 {
			r.kinds["gv pressed"]++
			r.press("g", "v")
			return
		}
		r.press("g", pick(r.r, "l", "a", "b", "p", "p", "q", "q", "s", "g", "/"))
	case n < 40:
		line := pick(r.r, "reload", "reload", "search water", "playlist", "messages", "wiki", "wiki",
			"playlist edit Evening", "playlist delete", "playlist rename Dusk", "art", "nosuchthing")
		if r.m.Screen() == ScreenPlaylist && r.r.IntN(2) == 0 {
			line = "playlist reorder"
		}
		r.command(line)
	case n < 43:
		r.press(pick(r.r, "/", ":"))
		r.press(pick(r.r, "a", "w", "zz", "b", "pl"))
		r.press(pick(r.r, "enter", "esc", "backspace", "tab", "ctrl+c"))
		if r.m.Filtering() || r.m.Commanding() {
			r.press("esc")
		}
	case n < 48:
		r.kinds["a resize"]++
		r.send(tea.WindowSizeMsg{Width: pick(r.r, 24, 40, 80, 200), Height: 4 + r.r.IntN(30)})
	case n < 52:
		// Something the server sends without being asked, as another client
		// changing a playlist does.
		changed := r.app.playlist(pick(r.r, "pl-1", "pl-2"))
		if r.m.playlist.ID != "" && r.r.IntN(2) == 0 {
			changed.ID = r.m.playlist.ID
		}
		switch r.r.IntN(8) {
		case 0:
			r.send(r.app.queue())
		case 1:
			r.send(r.app.starred())
		case 2:
			r.send(Notice("playing Ballast next"))
		case 3:
			r.send(Failed{Message: "mpv stopped"})
		case 4:
			r.send(ArtistsLoaded(library()[:r.r.IntN(4)]))
		case 5:
			r.send(Confirm{Question: "delete it?", Do: Notice("deleted")})
		default:
			r.send(PlaylistChanged(changed))
		}
	case n < 53:
		r.send(pick[tea.Msg](r.r, ShowVisualiser{}, FullArt{"a picture filling the screen"}))
	default:
		keys := []string{"j", "j", "j", "k", "k", "enter", "enter", "enter", "h", "G", "home",
			"end", "pgup", "pgdown", "ctrl+d", "ctrl+u", "S", "*", "a", "A", "r", "e", "J", "K", "?", "5", "%", " ", "n", "x"}
		switch r.m.Screen() {
		case ScreenWiki, ScreenPlaylists, ScreenArtists, ScreenAlbums:
			keys = append(keys, "enter", "enter", "enter", "G")
		}
		key := pick(r.r, keys...)
		r.press(key)
		// An open is often answered before the next key, and often not.
		if key == "enter" && len(r.app.waiting) > 0 && r.r.IntN(5) < 3 {
			r.deliver()
		}
	}
}

// kindsWanted is everything a set of runs has to have delivered or done for
// the checks above to have been reached.
var kindsWanted = []string{
	"an open's answer that was waited for",
	"an open's answer that was not waited for",
	"the failure of the open waited for",
	"the failure of an open not waited for",
	"a failure with no number",
	"a failure with no number while an open was waited for",
	"a reload's answer",
	"a changed playlist",
	"a changed playlist during a reorder of it",
	"a changed playlist while adding to it",
	"a reorder entered",
	"a back to a place that was left",
	"a back to an artist other than the one held",
	"a track taken out of the queue",
	"the playing track taken out of the queue",
	"a track moved in the queue",
	"a question answered",
	"a resize",
	"a key under the visualiser",
	"a key under a cover",
	"gv pressed",
	"tab while typing",
	"ctrl+c while typing",
}

// randomRuns is how many seeded runs the test makes and how many messages
// each one sends.
const (
	randomSeeds    = 6
	randomMessages = 400
)

// TestNoRunOfKeysAndAnswersBreaksWhatEachMessageMayDo drives the interface
// with keys, typed commands and resizes drawn from a seeded generator, with
// an actor answering what it emits. After every message the frame is drawn,
// the cursor of every screen is checked against its list, and what the
// message did is held against what a message of its kind may do.
func TestNoRunOfKeysAndAnswersBreaksWhatEachMessageMayDo(t *testing.T) {
	keys, kinds := map[Screen]int{}, map[string]int{}
	for seed := uint64(1); seed <= randomSeeds; seed++ {
		r := &run{t: t, seed: seed, r: rand.New(rand.NewPCG(seed, 11)), keys: keys, kinds: kinds}
		r.app.r = r.r
		r.m = sized(New())
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("seed %d, message %d: %v on the %s screen", seed, r.steps, p, r.m.Screen())
				}
			}()
			r.send(ArtistsLoaded(library()))
			for r.steps < randomMessages {
				r.step()
				if r.steps%20 == 0 && !reachesTheLibrary(r.m) {
					r.failf(nil, "back does not reach the library from here, with %d places remembered", len(r.m.trail))
				}
			}
		}()
	}

	for _, s := range Screens {
		t.Logf("%-12s %4d keys", s, keys[s])
		if keys[s] == 0 {
			t.Errorf("no key was pressed on the %s screen in %d runs", s, randomSeeds)
		}
	}
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Logf("%4d  %s", kinds[name], name)
	}
	for _, name := range kindsWanted {
		if kinds[name] == 0 {
			t.Errorf("%d runs never reached: %s", randomSeeds, name)
		}
	}
}
