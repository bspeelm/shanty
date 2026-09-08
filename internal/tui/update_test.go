package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/queue"
)

// press returns the model after a key, and whatever intent it emitted.
func press(t *testing.T, m Model, key string) (Model, tea.Msg) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	if key == "enter" {
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}
	var out tea.Msg
	if cmd != nil {
		out = cmd()
	}
	return next.(Model), out
}

func loaded(t *testing.T) Model {
	t.Helper()
	return send(t, sized(New()), ArtistsLoaded(library()))
}

// Descending emits an intent and nothing else: this package cannot fetch the
// artist it just asked for, which is the rule the whole design rests on.
func TestOpeningEmitsAnIntentRatherThanFetching(t *testing.T) {
	m := loaded(t)

	_, msg := press(t, m, "enter")
	open, ok := msg.(OpenArtist)
	if !ok {
		t.Fatalf("enter on the artist list emitted %T, want OpenArtist", msg)
	}
	if open.ID != "ar-1" {
		t.Errorf("opened %q, want ar-1", open.ID)
	}
}

func TestBrowseDescendsAndReturns(t *testing.T) {
	m := loaded(t)
	if m.Screen() != ScreenArtists {
		t.Fatalf("started on %s", m.Screen())
	}

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Screen() != ScreenAlbums {
		t.Fatalf("after ArtistLoaded the screen is %s", m.Screen())
	}
	m = send(t, m, AlbumLoaded(library()[0].Albums[0]))
	if m.Screen() != ScreenTracks {
		t.Fatalf("after AlbumLoaded the screen is %s", m.Screen())
	}

	m, _ = press(t, m, "h")
	if m.Screen() != ScreenAlbums {
		t.Errorf("back from tracks went to %s", m.Screen())
	}
	m, _ = press(t, m, "h")
	if m.Screen() != ScreenArtists {
		t.Errorf("back from albums went to %s", m.Screen())
	}
}

// Leaving a music player by pressing left one time too many is a surprise
// nobody wants mid-album.
func TestBackAtTheTopDoesNothing(t *testing.T) {
	m := loaded(t)
	for range 3 {
		var msg tea.Msg
		m, msg = press(t, m, "h")
		if msg != nil {
			t.Fatalf("back at the top emitted %T", msg)
		}
	}
	if m.Screen() != ScreenArtists {
		t.Errorf("back at the top moved to %s", m.Screen())
	}
}

// The cursor is per screen, so going back lands where the user left.
func TestTheCursorIsRememberedPerScreen(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "j")
	if m.Cursor() != 2 {
		t.Fatalf("cursor is %d after two downs", m.Cursor())
	}

	m = send(t, m, ArtistLoaded(library()[0]))
	m, _ = press(t, m, "j")
	if m.Cursor() != 1 {
		t.Fatalf("the albums cursor is %d, want 1", m.Cursor())
	}

	m, _ = press(t, m, "h")
	if m.Cursor() != 2 {
		t.Errorf("returning to artists put the cursor at %d, want 2", m.Cursor())
	}
}

// The caller is a key that can be held down.
func TestTheCursorClamps(t *testing.T) {
	m := loaded(t)
	for range 20 {
		m, _ = press(t, m, "j")
	}
	if got, want := m.Cursor(), len(library())-1; got != want {
		t.Errorf("cursor ran to %d, want %d", got, want)
	}
	for range 20 {
		m, _ = press(t, m, "k")
	}
	if m.Cursor() != 0 {
		t.Errorf("cursor ran to %d going up, want 0", m.Cursor())
	}
}

// Update must not change the model it was called on: the previous frame is
// still being held by bubbletea when the next one is computed.
func TestUpdateDoesNotMutateItsReceiver(t *testing.T) {
	m := loaded(t)
	before := m.Cursor()

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	if m.Cursor() != before {
		t.Errorf("the receiver moved to %d without being reassigned", m.Cursor())
	}
}

// Every playback key produces an intent and touches nothing.
func TestPlaybackKeysEmitIntents(t *testing.T) {
	m := loaded(t)
	for _, tc := range []struct {
		key  string
		want tea.Msg
	}{
		{" ", TogglePause{}},
		{"n", SkipNext{}},
		{"p", SkipPrev{}},
		{"]", SeekBy{By: 10 * time.Second}},
		{"[", SeekBy{By: -10 * time.Second}},
		{"+", VolumeBy{Delta: 5}},
		{"-", VolumeBy{Delta: -5}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			_, got := press(t, m, tc.key)
			if got != tc.want {
				t.Errorf("%q emitted %#v, want %#v", tc.key, got, tc.want)
			}
		})
	}
}

// Playing carries the album by value, so the handler does not have to ask this
// package for state it already sent in.
func TestPlayingCarriesTheAlbumAndTheIndex(t *testing.T) {
	m := send(t, loaded(t), ArtistLoaded(library()[0]), AlbumLoaded(library()[0].Albums[0]))
	m, _ = press(t, m, "j")

	_, msg := press(t, m, "enter")
	play, ok := msg.(PlayFrom)
	if !ok {
		t.Fatalf("enter on the track list emitted %T, want PlayFrom", msg)
	}
	if play.Index != 1 {
		t.Errorf("playing index %d, want 1", play.Index)
	}
	if len(play.Album.Songs) != 2 || play.Album.Songs[1].ID != "tr-2" {
		t.Errorf("the album did not travel with the intent: %+v", play.Album)
	}
}

// An empty list must not be indexable, and pressing enter on nothing must not
// emit an intent naming a track that does not exist.
func TestAnEmptyLibraryIsSafeToPressKeysAt(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(nil))
	for _, key := range []string{"j", "k", "enter", "G", "g"} {
		var msg tea.Msg
		m, msg = press(t, m, key)
		if _, isOpen := msg.(OpenArtist); isOpen {
			t.Errorf("%q opened something in an empty library", key)
		}
	}
	if m.Cursor() != 0 {
		t.Errorf("the cursor moved to %d in an empty library", m.Cursor())
	}
}

// Screens is a closed set: a fourth screen added without a heading would
// render "unknown" to a user rather than failing here.
func TestEveryScreenHasAHeadingAndRenders(t *testing.T) {
	m := send(t, sized(New()), ArtistsLoaded(library()),
		ArtistLoaded(library()[0]), AlbumLoaded(library()[0].Albums[0]))

	for _, s := range Screens {
		m.screen = s
		if s.String() == "unknown" {
			t.Errorf("screen %d has no name", s)
		}
		if frame := m.View(); frame == "" {
			t.Errorf("screen %s rendered nothing", s)
		}
	}
	if len(Screens) != 4 {
		t.Errorf("Screens lists %d screens; there are four, so this is a change to the interface", len(Screens))
	}
}

// Typing / narrows the list on screen. The filter matches anywhere in the
// name, not only at the start, and ignores case.
func TestFilterNarrowsTheList(t *testing.T) {
	m := loaded(t)
	if m.rows() != 3 {
		t.Fatalf("started with %d rows, want 3", m.rows())
	}

	m, _ = press(t, m, "/")
	if !m.Filtering() {
		t.Fatal("pressing / did not enter the filter")
	}
	for _, c := range "bilge" {
		m, _ = press(t, m, string(c))
	}
	if m.rows() != 1 {
		t.Fatalf("filtering by %q left %d rows, want 1", m.Filter(), m.rows())
	}
	if got := m.View(); !strings.Contains(got, "The Bilge Pumps") {
		t.Errorf("the matching row is not on screen:\n%s", got)
	}
}

// The filter selects a row out of the full list, so opening one has to open
// what is on screen rather than the row at that index of the unfiltered list.
func TestOpeningAFilteredRowOpensWhatIsOnScreen(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "/")
	for _, c := range "bilge" {
		m, _ = press(t, m, string(c))
	}

	// The first enter accepts the filter and leaves the mode; the second opens
	// the row that is showing.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	_, msg := press(t, m, "enter")
	open, ok := msg.(OpenArtist)
	if !ok {
		t.Fatalf("enter emitted %T, want OpenArtist", msg)
	}
	if open.ID != "ar-2" {
		t.Errorf("opened %q, want ar-2, the artist that was showing", open.ID)
	}
}

func TestEscapeAbandonsTheFilter(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "/")
	for _, c := range "bilge" {
		m, _ = press(t, m, string(c))
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.Filtering() {
		t.Error("esc did not leave the filter")
	}
	if m.Filter() != "" {
		t.Errorf("esc left the filter %q applied", m.Filter())
	}
	if m.rows() != 3 {
		t.Errorf("esc left %d rows, want the whole list back", m.rows())
	}
}

// A filter matching nothing says so, rather than showing an empty screen with
// no explanation.
func TestAFilterMatchingNothingSaysSo(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "/")
	for _, c := range "zzz" {
		m, _ = press(t, m, string(c))
	}

	if m.rows() != 0 {
		t.Fatalf("a filter of zzz left %d rows", m.rows())
	}
	if got := m.View(); !strings.Contains(got, "nothing matches") {
		t.Errorf("the screen does not explain the empty list:\n%s", got)
	}
}

// A filter belongs to the list it narrowed. Going to another screen leaves it
// behind rather than narrowing the new one by a word chosen for the old.
func TestTheFilterDoesNotFollowYouToAnotherScreen(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "/")
	for _, c := range "aoi" {
		m, _ = press(t, m, string(c))
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	m = send(t, m, ArtistLoaded(library()[0]))
	if m.Filter() != "" {
		t.Errorf("the filter %q followed into the albums screen", m.Filter())
	}
	if m.rows() != 2 {
		t.Errorf("the albums screen shows %d rows, want 2", m.rows())
	}
}

// Quitting ends the session, so it is a command rather than a key. Pressing q
// says where it went, which beats doing nothing for anyone who learned q
// elsewhere.
func TestQSaysWhereQuitWent(t *testing.T) {
	m := loaded(t)

	m, msg := press(t, m, "q")
	if msg != nil {
		t.Errorf("q emitted %#v; it should not act", msg)
	}
	if !strings.Contains(m.Status(), ":q") {
		t.Errorf("q does not say where quit went: %q", m.Status())
	}
	if !strings.Contains(m.View(), ":q") {
		t.Errorf("the message is not on screen:\n%s", m.View())
	}
}

// ctrl+c still quits, because it is what people press when a program will not
// let go and it works below anything shanty decides.
func TestControlCStillQuits(t *testing.T) {
	next, cmd := loaded(t).Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_ = next
	if cmd == nil {
		t.Fatal("ctrl+c did nothing")
	}
	if _, ok := cmd().(Quit); !ok {
		t.Errorf("ctrl+c emitted %T, want Quit", cmd())
	}
}

// typeCommand enters command mode and types a line, returning the model and
// whatever running it emitted.
func typeCommand(t *testing.T, m Model, line string) (Model, tea.Msg) {
	t.Helper()
	m, _ = press(t, m, ":")
	if !m.Commanding() {
		t.Fatal("pressing : did not enter a command")
	}
	for _, c := range line {
		if c == ' ' {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
			m = next.(Model)
			continue
		}
		m, _ = press(t, m, string(c))
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

func TestColonQQuits(t *testing.T) {
	m, msg := typeCommand(t, loaded(t), "q")
	if _, ok := msg.(Quit); !ok {
		t.Errorf(":q emitted %#v, want Quit", msg)
	}
	if m.Commanding() {
		t.Error("running a command left the mode open")
	}
}

// A command taking an argument, which is the reason commands exist at all.
func TestAcommandTakesAnArgument(t *testing.T) {
	_, msg := typeCommand(t, loaded(t), "volume 40")
	if got, ok := msg.(VolumeSet); !ok || int(got) != 40 {
		t.Errorf(":volume 40 emitted %#v, want VolumeSet(40)", msg)
	}
}

func TestAnArgumentThatWillNotDoSaysWhatWasWanted(t *testing.T) {
	m, msg := typeCommand(t, loaded(t), "volume loud")
	if msg != nil {
		t.Errorf(":volume loud emitted %#v; it should not act", msg)
	}
	if !strings.Contains(m.Status(), "a number from 0 to 100") {
		t.Errorf("the message does not say what was wanted: %q", m.Status())
	}
}

func TestAnUnknownCommandSaysSo(t *testing.T) {
	m, msg := typeCommand(t, loaded(t), "dance")
	if msg != nil {
		t.Errorf(":dance emitted %#v", msg)
	}
	if !strings.Contains(m.Status(), "no such command") {
		t.Errorf("an unknown command said %q", m.Status())
	}
}

// Pressing : shows every command, which is what makes the set learnable
// without going to look for a list of them.
func TestPressingColonShowsEveryCommand(t *testing.T) {
	m, _ := press(t, loaded(t), ":")
	frame := m.View()
	for _, c := range commands {
		if !strings.Contains(frame, c.name) {
			t.Errorf("the completion row does not offer %q:\n%s", c.name, frame)
		}
	}
}

func TestTabCompletesAsFarAsTheMatchesAgree(t *testing.T) {
	m, _ := press(t, loaded(t), ":")
	m, _ = press(t, m, "v")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)

	// One match, and it takes an argument, so the cursor lands where the
	// argument goes.
	if m.Line() != "volume " {
		t.Errorf("tab completed to %q, want %q", m.Line(), "volume ")
	}
}

func TestEscapeAbandonsTheCommand(t *testing.T) {
	m, _ := press(t, loaded(t), ":")
	m, _ = press(t, m, "q")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)

	if m.Commanding() {
		t.Error("esc did not leave the command line")
	}
	if m.Line() != "" {
		t.Errorf("esc left %q on the line", m.Line())
	}
	if cmd != nil {
		t.Errorf("esc ran something: %#v", cmd())
	}
}

// Holding a volume key down stops at the end of the range. Typing a number
// outside it is a mistake, and is reported rather than quietly changed into a
// different number.
func TestAVolumeOutsideTheRangeIsReportedNotClamped(t *testing.T) {
	for _, arg := range []string{"400", "-1"} {
		m, msg := typeCommand(t, loaded(t), "volume "+arg)
		if msg != nil {
			t.Errorf(":volume %s emitted %#v; it should not act", arg, msg)
		}
		if !strings.Contains(m.Status(), "0 to 100") {
			t.Errorf(":volume %s said %q", arg, m.Status())
		}
	}
}

// g is a prefix rather than a key. gg goes to the top, and the screens that do
// not exist yet say so rather than doing nothing, so the prefix is whole.
func TestTheGPrefix(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "j")
	if m.Cursor() != 1 {
		t.Fatalf("cursor is %d", m.Cursor())
	}

	m, _ = press(t, m, "g")
	if m.Pending() != "g" {
		t.Fatalf("g did not become pending: %q", m.Pending())
	}
	m, _ = press(t, m, "g")
	if m.Pending() != "" {
		t.Error("the prefix was not consumed")
	}
	if m.Cursor() != 0 {
		t.Errorf("gg left the cursor at %d, want 0", m.Cursor())
	}

	// gq opens a screen that exists; the other two say they do not yet.
	n, _ := press(t, m, "g")
	n, _ = press(t, n, "q")
	if n.Screen() != ScreenQueue {
		t.Errorf("gq went to %s, want the queue", n.Screen())
	}

	for key, want := range map[string]string{"p": "playlists", "s": "starred"} {
		n, _ := press(t, m, "g")
		n, _ = press(t, n, key)
		if !strings.Contains(n.Status(), want) {
			t.Errorf("g%s said %q, want it to mention %s", key, n.Status(), want)
		}
	}
}

// A second key the prefix does not recognise cancels it rather than acting on
// its own, so gx is not the same as x.
func TestAnUnknownKeyAfterThePrefixDoesNothing(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "g")
	m, msg := press(t, m, "x")

	if m.Pending() != "" {
		t.Error("the prefix survived an unknown key")
	}
	if msg != nil {
		t.Errorf("gx emitted %#v", msg)
	}
}

// Digits before a movement repeat it, as they do in the editor these bindings
// borrow from.
func TestACountRepeatsAMovement(t *testing.T) {
	m := loaded(t)
	for _, c := range "2" {
		m, _ = press(t, m, string(c))
	}
	if m.Count() != "2" {
		t.Fatalf("the count is %q", m.Count())
	}
	m, _ = press(t, m, "j")

	if m.Cursor() != 2 {
		t.Errorf("2j moved to %d, want 2", m.Cursor())
	}
	if m.Count() != "" {
		t.Errorf("the count survived the movement: %q", m.Count())
	}
}

// A count is consumed by whatever follows it, used or not, so it cannot leak
// into the next key.
func TestACountIsConsumedByAnyKey(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "3")
	m, _ = press(t, m, "s") // not a movement
	if m.Count() != "" {
		t.Errorf("the count survived an unrelated key: %q", m.Count())
	}
	m, _ = press(t, m, "j")
	if m.Cursor() != 1 {
		t.Errorf("the leaked count moved the cursor to %d, want 1", m.Cursor())
	}
}

// {count}% seeks to that percentage, which is why the digits are counts rather
// than being bound to positions themselves.
func TestACountBeforePercentSeeks(t *testing.T) {
	m := loaded(t)
	for _, c := range "50" {
		m, _ = press(t, m, string(c))
	}
	_, msg := press(t, m, "%")

	if got, ok := msg.(SeekToPercent); !ok || int(got) != 50 {
		t.Errorf("50%% emitted %#v, want SeekToPercent(50)", msg)
	}
}

// A bare % has no percentage to seek to and says so.
func TestPercentWithoutACountSaysWhatIsMissing(t *testing.T) {
	m, msg := press(t, loaded(t), "%")
	if msg != nil {
		t.Errorf("a bare %% emitted %#v", msg)
	}
	if !strings.Contains(m.Status(), "percentage") {
		t.Errorf("a bare %% said %q", m.Status())
	}
}

// TestAModelDoesNotShareItsCursorWithACopy covers the one reference inside a
// value type. Model is copied on every update, and its cursor is a map, so
// writing through it would reach every copy that came before -- including
// screens already rendered.
func TestAModelDoesNotShareItsCursorWithACopy(t *testing.T) {
	base := loaded(t)
	base = send(t, base, QueueChanged{Tracks: []queue.Track{
		{ID: "a", Title: "One"}, {ID: "b", Title: "Two"}, {ID: "c", Title: "Three"},
	}, At: 0})

	// Two screens taken from the same model, moved to different rows.
	first, _ := press(t, base, "g")
	first, _ = press(t, first, "q")
	first, _ = press(t, first, "j")

	second, _ := press(t, base, "g")
	second, _ = press(t, second, "q")

	if first.Cursor() == second.Cursor() {
		t.Fatalf("both copies are on row %d; one moved and the other did not", first.Cursor())
	}
	if second.Cursor() != 0 {
		t.Errorf("moving one copy moved the other to row %d", second.Cursor())
	}
	if base.Screen() != ScreenArtists {
		t.Errorf("opening a screen on a copy changed the original to %s", base.Screen())
	}
}

// TestOpeningTheQueuePutsTheCursorOnWhatIsPlaying covers where gq lands. The
// queue is looked at to see what is coming, so it opens on the track playing
// rather than at the top.
func TestOpeningTheQueuePutsTheCursorOnWhatIsPlaying(t *testing.T) {
	m := loaded(t)
	m = send(t, m, QueueChanged{Tracks: []queue.Track{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}, At: 2})

	m, _ = press(t, m, "g")
	m, _ = press(t, m, "q")

	if m.Screen() != ScreenQueue {
		t.Fatalf("gq went to %s", m.Screen())
	}
	if m.Cursor() != 2 {
		t.Errorf("the queue opened on row %d, want the playing track at 2", m.Cursor())
	}
}

// TestTheQueueScreenSurvivesAQueueThatShrinks covers a cursor left past the
// end when tracks are played off the front of the queue.
func TestTheQueueScreenSurvivesAQueueThatShrinks(t *testing.T) {
	m := loaded(t)
	m = send(t, m, QueueChanged{Tracks: []queue.Track{{ID: "a"}, {ID: "b"}, {ID: "c"}}, At: 2})
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "q")

	m = send(t, m, QueueChanged{Tracks: []queue.Track{{ID: "a"}}, At: 0})

	if m.Cursor() != 0 {
		t.Errorf("the cursor is on row %d of a one-track queue", m.Cursor())
	}
	if frame := m.View(); frame == "" {
		t.Error("the queue screen rendered nothing after the queue shrank")
	}
}

// TestQueueingKeysAskForTheSelectedTrack covers the two keys that put a track
// in the queue. They work on the track list, which is the only screen where a
// row is one track.
func TestQueueingKeysAskForTheSelectedTrack(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "enter") // artist
	m = send(t, m, ArtistLoaded(library()[0]))
	m, _ = press(t, m, "enter") // album
	m = send(t, m, AlbumLoaded(library()[0].Albums[0]))
	m, _ = press(t, m, "j") // the second track

	_, got := press(t, m, "a")
	add, ok := got.(Enqueue)
	if !ok {
		t.Fatalf("a emitted %T, want Enqueue", got)
	}
	if add.Index != 1 {
		t.Errorf("a queued track %d, want the selected one at 1", add.Index)
	}

	_, got = press(t, m, "A")
	next, ok := got.(PlayNext)
	if !ok {
		t.Fatalf("A emitted %T, want PlayNext", got)
	}
	if next.Index != 1 {
		t.Errorf("A queued track %d, want the selected one at 1", next.Index)
	}
}

// TestQueueingSomewhereWithNoTrackSaysSo covers the screens where a row is not
// a track, so the keys have nothing to act on.
func TestQueueingSomewhereWithNoTrackSaysSo(t *testing.T) {
	for _, key := range []string{"a", "A"} {
		m := loaded(t) // the artist list

		m, got := press(t, m, key)
		if got != nil {
			t.Errorf("%s on the artist list emitted %T", key, got)
		}
		if !strings.Contains(m.Status(), "open an album") {
			t.Errorf("%s said %q", key, m.Status())
		}
	}
}
