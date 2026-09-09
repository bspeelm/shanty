package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/queue"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// namedKeys are the keys the model reads by name rather than by character.
var namedKeys = map[string]tea.KeyType{
	"esc": tea.KeyEsc, "enter": tea.KeyEnter, "tab": tea.KeyTab,
	"backspace": tea.KeyBackspace, "space": tea.KeySpace,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

// press returns the model after a key, and whatever intent it emitted.
//
// A key with a name rather than a character is sent as that key. Sending it as
// runes works in normal mode, where the model reads the key's name, but in the
// filter and the command line it is typed in as text: press(m, "esc") put the
// letters e, s and c into the filter instead of leaving it.
func press(t *testing.T, m Model, key string) (Model, tea.Msg) {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if named, ok := namedKeys[key]; ok {
		msg = tea.KeyMsg{Type: named}
	}
	next, cmd := m.Update(msg)
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
	if len(Screens) != 10 {
		t.Errorf("Screens lists %d screens; there are ten, so this is a change to the interface", len(Screens))
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

	// Every screen the prefix names now exists.
	for key, want := range map[string]Screen{
		"q": ScreenQueue, "s": ScreenStarred, "p": ScreenPlaylists, "a": ScreenArtists,
	} {
		n, _ := press(t, m, "g")
		n, _ = press(t, n, key)
		if n.Screen() != want {
			t.Errorf("g%s went to %s, want %s", key, n.Screen(), want)
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

// searched is the model showing a set of results across all three kinds.
func searched(t *testing.T) Model {
	t.Helper()
	return send(t, loaded(t), SearchLoaded{Query: "water", Results: subsonic.Results{
		Artists: []subsonic.Artist{{ID: "ar-1", Name: "Aoi", AlbumCount: 2}},
		Albums:  []subsonic.Album{{ID: "al-2", Name: "Low Water", Artist: "Aoi"}},
		Songs: []subsonic.Song{
			{ID: "tr-1", Title: "Watermark", Album: "Bail", AlbumID: "al-3", Artist: "Aoi", Duration: 245},
		},
	}})
}

// TestTheCursorNeverRestsOnAHeading covers the one row a search screen has
// that nothing can be done with. Moving through the results steps over the
// group names rather than stopping on them.
func TestTheCursorNeverRestsOnAHeading(t *testing.T) {
	m := searched(t)

	// Every way of moving, including the ones that jump rather than step.
	for _, key := range []string{
		"j", "j", "j", "j", "k", "k", "k", "k",
		"down", "down", "up",
		"G", "gg", "home", "end", "pgdown", "pgup",
	} {
		switch key {
		case "gg":
			m, _ = press(t, m, "g")
			m, _ = press(t, m, "g")
		default:
			m, _ = press(t, m, key)
		}
		if at := m.Cursor(); m.found[at].kind == kindHeading {
			t.Fatalf("after %q the cursor is on the heading %q", key, m.found[at].name)
		}
	}
}

// TestASearchOpensOnTheFirstResult covers where the cursor starts. The first
// row is a heading, so it cannot be there.
func TestASearchOpensOnTheFirstResult(t *testing.T) {
	m := searched(t)

	if m.Screen() != ScreenSearch {
		t.Fatalf("a search left the interface on %s", m.Screen())
	}
	if at := m.Cursor(); m.found[at].kind == kindHeading {
		t.Errorf("a search opened on the heading %q", m.found[at].name)
	}
	if m.found[m.Cursor()].name != "Aoi" {
		t.Errorf("a search opened on %q, want the first result", m.found[m.Cursor()].name)
	}
}

// TestOpeningEachKindOfResultAsksForTheRightThing covers what enter does. The
// three kinds are in one list, so the row decides.
func TestOpeningEachKindOfResultAsksForTheRightThing(t *testing.T) {
	m := searched(t)

	_, got := press(t, m, "enter")
	if artist, ok := got.(OpenArtist); !ok || artist.ID != "ar-1" {
		t.Errorf("enter on an artist emitted %#v", got)
	}

	m, _ = press(t, m, "j") // past the ALBUMS heading, onto the album
	_, got = press(t, m, "enter")
	if album, ok := got.(OpenAlbum); !ok || album.ID != "al-2" {
		t.Errorf("enter on an album emitted %#v", got)
	}

	m, _ = press(t, m, "j") // past the TRACKS heading, onto the track
	_, got = press(t, m, "enter")
	play, ok := got.(PlayFrom)
	if !ok {
		t.Fatalf("enter on a track emitted %#v", got)
	}
	// A track found by searching is on no album the interface has loaded, so
	// it has to bring one with it.
	if len(play.Album.Songs) != 1 || play.Album.Songs[0].ID != "tr-1" {
		t.Errorf("the track was played from %+v", play.Album)
	}
	if play.Index != 0 {
		t.Errorf("the track was played at index %d", play.Index)
	}
}

// TestASearchThatFindsNothingSaysSoAndDoesNotMove covers the common case of a
// typo. There is nothing to select, and enter must not act on a row that is
// not there.
func TestASearchThatFindsNothingSaysSoAndDoesNotMove(t *testing.T) {
	m := send(t, loaded(t), SearchLoaded{Query: "zzzz", Results: subsonic.Results{}})

	if m.Screen() != ScreenSearch {
		t.Fatalf("a search that found nothing left the interface on %s", m.Screen())
	}
	if frame := m.View(); !strings.Contains(frame, "nothing matches") {
		t.Errorf("the screen does not say nothing matched:\n%s", frame)
	}
	if _, got := press(t, m, "enter"); got != nil {
		t.Errorf("enter on an empty result emitted %#v", got)
	}
	for _, key := range []string{"j", "k", "G"} {
		if n, _ := press(t, m, key); n.Cursor() != 0 {
			t.Errorf("%s moved to row %d in an empty result", key, n.Cursor())
		}
	}
}

// TestTheSearchCommandNeedsSomethingToLookFor covers the command with no
// argument, which would otherwise ask the server for everything.
func TestTheSearchCommandNeedsSomethingToLookFor(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, ":")
	for _, c := range "search" {
		m, _ = press(t, m, string(c))
	}
	m, got := press(t, m, "enter")

	if got != nil {
		t.Errorf(":search with nothing after it emitted %#v", got)
	}
	if !strings.Contains(m.Status(), "something to look for") {
		t.Errorf("it said %q", m.Status())
	}
}

// TestMovingThroughResultsLandsOnTheNextOne covers the direction the cursor
// steps over a heading in. Stepping the wrong way would leave a key looking
// dead: pressing k on the first track would step forward onto the track again
// rather than back to the album above it.
func TestMovingThroughResultsLandsOnTheNextOne(t *testing.T) {
	m := searched(t)

	// ARTISTS, Aoi, ALBUMS, Low Water, TRACKS, Watermark
	want := []string{"Aoi", "Low Water", "Watermark"}
	for i, name := range want {
		if got := m.found[m.Cursor()].name; got != name {
			t.Fatalf("moving down %d times reached %q, want %q", i, got, name)
		}
		if i < len(want)-1 {
			m, _ = press(t, m, "j")
		}
	}
	for i := len(want) - 1; i >= 0; i-- {
		if got := m.found[m.Cursor()].name; got != want[i] {
			t.Fatalf("moving back up reached %q, want %q", got, want[i])
		}
		if i > 0 {
			m, _ = press(t, m, "k")
		}
	}
}

// TestFilteringResultsDropsTheHeadings covers a filter that matches a group
// name. A heading is not a thing in the library, so narrowing the list must
// not leave one behind on its own.
func TestFilteringResultsDropsTheHeadings(t *testing.T) {
	m := searched(t)

	m, _ = press(t, m, "/")
	for _, c := range "tracks" {
		m, _ = press(t, m, string(c))
	}

	if m.rows() != 0 {
		t.Errorf("filtering for a group name left %d rows", m.rows())
	}
	if frame := m.View(); !strings.Contains(frame, "nothing matches") {
		t.Errorf("the screen does not say nothing matched:\n%s", frame)
	}

	// A filter matching a real result keeps it, without its heading.
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "/")
	for _, c := range "water" {
		m, _ = press(t, m, string(c))
	}
	if m.rows() != 2 {
		t.Fatalf("filtering for water left %d rows, want the album and the track", m.rows())
	}
	for _, i := range m.matches() {
		if m.found[i].kind == kindHeading {
			t.Errorf("a heading survived the filter: %q", m.found[i].name)
		}
	}
}

// TestAHeadingIsNeverTheLastRow is the invariant that lets end skip the
// step-over the other jumps need: a heading is written only when its group has
// something in it.
func TestAHeadingIsNeverTheLastRow(t *testing.T) {
	for _, found := range []subsonic.Results{
		{Artists: []subsonic.Artist{{Name: "Aoi"}}},
		{Albums: []subsonic.Album{{Name: "Harbour"}}},
		{Songs: []subsonic.Song{{Title: "Slipway"}}},
		{Artists: []subsonic.Artist{{Name: "Aoi"}}, Songs: []subsonic.Song{{Title: "Slipway"}}},
	} {
		rows := flatten(found)
		if len(rows) == 0 {
			t.Fatalf("%+v flattened to nothing", found)
		}
		if !rows[len(rows)-1].selectable() {
			t.Errorf("%+v ends with the heading %q", found, rows[len(rows)-1].name)
		}
	}
	if rows := flatten(subsonic.Results{}); len(rows) != 0 {
		t.Errorf("an empty result flattened to %d rows", len(rows))
	}
}

// TestStarringNamesTheRightKind covers the key on every list. A server takes
// each kind under a different parameter, so the screen has to say which it is.
func TestStarringNamesTheRightKind(t *testing.T) {
	artists := loaded(t)
	albums := send(t, artists, ArtistLoaded(library()[0]))
	tracks := send(t, albums, AlbumLoaded(library()[0].Albums[0]))

	for _, tc := range []struct {
		name string
		m    Model
		kind Kind
		id   string
	}{
		{"artists", artists, StarArtist, library()[0].ID},
		{"albums", albums, StarAlbum, library()[0].Albums[0].ID},
		{"tracks", tracks, StarSong, library()[0].Albums[0].Songs[0].ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := press(t, tc.m, "*")
			star, ok := got.(ToggleStar)
			if !ok {
				t.Fatalf("* emitted %#v", got)
			}
			if star.Kind != tc.kind {
				t.Errorf("* named it a %s, want %s", star.Kind, tc.kind)
			}
			if star.ID != tc.id {
				t.Errorf("* starred %q, want %q", star.ID, tc.id)
			}
			if !star.Starred {
				t.Error("* on something unstarred asked to unstar it")
			}
		})
	}
}

// TestStarringSomethingStarredUnstarsIt covers the other direction. The key
// turns the state over, so it has to know what the state is.
func TestStarringSomethingStarredUnstarsIt(t *testing.T) {
	m := send(t, loaded(t), StarredChanged{IDs: map[string]bool{library()[0].ID: true}})

	_, got := press(t, m, "*")
	star, ok := got.(ToggleStar)
	if !ok {
		t.Fatalf("* emitted %#v", got)
	}
	if star.Starred {
		t.Error("* on something already starred asked to star it again")
	}
}

// TestTheStarredScreenListsWhatIsStarred covers gs, and that opening a row
// there does the same as opening it anywhere else.
func TestTheStarredScreenListsWhatIsStarred(t *testing.T) {
	m := send(t, loaded(t), starredChanged())
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "s")

	if m.Screen() != ScreenStarred {
		t.Fatalf("gs went to %s", m.Screen())
	}
	if m.rows() == 0 {
		t.Fatal("the starred screen is empty with three things starred")
	}
	if at := m.Cursor(); m.grouped()[at].kind == kindHeading {
		t.Errorf("gs opened on the heading %q", m.grouped()[at].name)
	}

	_, got := press(t, m, "enter")
	if artist, ok := got.(OpenArtist); !ok || artist.ID != "ar-1" {
		t.Errorf("enter on a starred artist emitted %#v", got)
	}
}

// TestTheStarredScreenWithNothingStarredSaysSo covers the first run, when the
// screen is reached before anything has been starred.
func TestTheStarredScreenWithNothingStarredSaysSo(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "s")

	if m.Screen() != ScreenStarred {
		t.Fatalf("gs went to %s", m.Screen())
	}
	if frame := m.View(); !strings.Contains(frame, "nothing yet") {
		t.Errorf("the screen does not say nothing is starred:\n%s", frame)
	}
	if _, got := press(t, m, "*"); got != nil {
		t.Errorf("* on an empty starred screen emitted %#v", got)
	}
}

// TestEveryListMarksWhatIsStarred covers the marker the issue asks for in each
// of the three lists.
func TestEveryListMarksWhatIsStarred(t *testing.T) {
	artists := send(t, loaded(t), starredChanged())
	albums := send(t, artists, ArtistLoaded(library()[0]))
	tracks := send(t, albums, AlbumLoaded(library()[0].Albums[0]))

	for name, m := range map[string]Model{"artists": artists, "albums": albums, "tracks": tracks} {
		if frame := m.View(); !strings.Contains(frame, "★") {
			t.Errorf("the %s list shows no marker:\n%s", name, frame)
		}
	}

	// And nothing is marked when nothing is starred.
	if frame := loaded(t).View(); strings.Contains(frame, "★") {
		t.Errorf("a list with nothing starred shows a marker:\n%s", frame)
	}
}

// TestWhatWasSaidIsKept covers the point of the screen: a message is shown on
// one row until the next replaces it, so anything that went wrong while
// something else had your attention is otherwise gone.
func TestWhatWasSaidIsKept(t *testing.T) {
	m := loaded(t)

	m = send(t, m, Failed{Message: "the server refused a play"})
	m = send(t, m, Notice("playing Ballast next"))
	m = send(t, m, Failed{Message: "mpv stopped"})

	if got := m.Status(); got != "mpv stopped" {
		t.Errorf("the last row shows %q, want the newest message", got)
	}

	m, _ = press(t, m, ":")
	for _, c := range "messages" {
		m, _ = press(t, m, string(c))
	}
	m, msg := press(t, m, "enter")
	if msg != nil {
		m = send(t, m, msg)
	}

	if m.Screen() != ScreenMessages {
		t.Fatalf(":messages went to %s", m.Screen())
	}
	if m.rows() != 3 {
		t.Fatalf("the screen holds %d messages, want 3", m.rows())
	}
	// Newest first: what just happened is what somebody is looking for.
	frame := m.View()
	newest := strings.Index(frame, "mpv stopped")
	oldest := strings.Index(frame, "the server refused a play")
	if newest < 0 || oldest < 0 {
		t.Fatalf("a message is missing from the screen:\n%s", frame)
	}
	if newest > oldest {
		t.Error("the messages are oldest first, want the newest at the top")
	}
}

// TestTheMessagesKeptAreBounded covers a long session. The recent ones are
// what anybody wants, so the older ones go rather than growing for ever.
func TestTheMessagesKeptAreBounded(t *testing.T) {
	m := loaded(t)
	for i := range remembered + 50 {
		m = send(t, m, Notice(fmt.Sprintf("message %d", i)))
	}

	if len(m.said) != remembered {
		t.Errorf("kept %d messages, want the limit of %d", len(m.said), remembered)
	}
	if newest := m.said[len(m.said)-1].text; newest != fmt.Sprintf("message %d", remembered+49) {
		t.Errorf("the newest message is %q", newest)
	}
	if oldest := m.said[0].text; oldest == "message 0" {
		t.Error("the oldest message was kept over newer ones")
	}
}

// TestAnEmptyMessageIsNotKept covers the messages that clear the last row
// rather than saying anything.
func TestAnEmptyMessageIsNotKept(t *testing.T) {
	m := loaded(t)

	m = send(t, m, Notice(""))
	m = send(t, m, Failed{Message: "   "})

	if len(m.said) != 0 {
		t.Errorf("kept %d empty messages: %v", len(m.said), m.said)
	}
}

// TestTheMessagesScreenWithNothingSaidSaysSo covers the screen reached before
// anything has gone wrong.
func TestTheMessagesScreenWithNothingSaidSaysSo(t *testing.T) {
	m := send(t, loaded(t), ShowMessages{})

	if m.Screen() != ScreenMessages {
		t.Fatalf(":messages went to %s", m.Screen())
	}
	if frame := m.View(); !strings.Contains(frame, "nothing said yet") {
		t.Errorf("the screen reads:\n%s", frame)
	}
}

// TestALongMessageKeepsItsTime covers the row that will not fit. The time is
// what makes a log a log, so the message is cut rather than the time.
func TestALongMessageKeepsItsTime(t *testing.T) {
	m := sized(loaded(t))
	at := time.Date(2026, 9, 8, 15, 4, 5, 0, time.UTC)
	m.now = func() time.Time { return at }

	m = send(t, m, Failed{Message: strings.Repeat("a very long failure ", 8)})
	m = send(t, m, ShowMessages{})

	if frame := m.View(); !strings.Contains(frame, "15:04:05") {
		t.Errorf("a long message pushed its own time off the row:\n%s", frame)
	}
}

// TestRememberingDoesNotReachACopy covers the same hazard the cursor has:
// Model is a value copied on every update, and its messages are a slice. Once
// the list has been trimmed to its limit it has room to spare, and appending
// through that room would rewrite what a copy already holds.
func TestRememberingDoesNotReachACopy(t *testing.T) {
	base := loaded(t)
	// Past the limit, so the list has been trimmed and has spare room.
	for i := range remembered + 10 {
		base = send(t, base, Notice(fmt.Sprintf("message %d", i)))
	}

	first := send(t, base, Notice("what the first copy said"))
	second := send(t, base, Notice("what the second copy said"))

	if got := first.said[len(first.said)-1].text; got != "what the first copy said" {
		t.Errorf("the first copy ends with %q", got)
	}
	if got := second.said[len(second.said)-1].text; got != "what the second copy said" {
		t.Errorf("the second copy ends with %q", got)
	}
	if got := base.said[len(base.said)-1].text; !strings.HasPrefix(got, "message ") {
		t.Errorf("adding to a copy changed the model it came from, which now ends with %q", got)
	}
}

// TestReloadKeepsTheSelectionOnWhatWasSelected covers the point of the
// command: a library that has gained an album should not send the cursor back
// to the top of a list that mostly did not change.
func TestReloadKeepsTheSelectionOnWhatWasSelected(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "j") // the second artist

	was := m.artists[m.Cursor()].ID
	m = send(t, m, Reload{})
	if !m.loading {
		t.Error("a reload does not say it is under way")
	}

	// The server answers with a new artist at the front, which would push the
	// selection down a row if the cursor were left where it was.
	grown := append([]subsonic.Artist{{ID: "ar-0", Name: "A New Arrival", AlbumCount: 1}}, library()...)
	m = send(t, m, ArtistsLoaded(grown))

	if m.loading {
		t.Error("the reload never finished")
	}
	if got := m.artists[m.Cursor()].ID; got != was {
		t.Errorf("the cursor moved to %q, want it to stay on %q", got, was)
	}
	if !strings.Contains(m.Status(), "reloaded") {
		t.Errorf("a reload that returned the same list said %q", m.Status())
	}
	if !strings.Contains(m.Status(), "4 artists") {
		t.Errorf("the message does not say what came back: %q", m.Status())
	}
}

// TestReloadWhenTheSelectedRowIsGone covers a reload after the thing selected
// has been removed from the server.
func TestReloadWhenTheSelectedRowIsGone(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "j")

	m = send(t, m, Reload{})
	m = send(t, m, ArtistsLoaded(library()[:1]))

	if m.Cursor() < 0 || m.Cursor() >= m.rows() {
		t.Errorf("the cursor is on row %d of %d", m.Cursor(), m.rows())
	}
	if frame := m.View(); frame == "" {
		t.Error("the screen rendered nothing after the selected row went")
	}
}

// TestOpeningAnArtistStillStartsAtTheTop covers the navigation a reload must
// not change. Opening something is not a reload, and starts at its first row.
func TestOpeningAnArtistStillStartsAtTheTop(t *testing.T) {
	m := loaded(t)
	m = send(t, m, ArtistLoaded(library()[0]))
	m, _ = press(t, m, "j")

	// Opening the same artist again, without a reload, goes back to the top.
	m = send(t, m, ArtistLoaded(library()[0]))

	if m.Cursor() != 0 {
		t.Errorf("opening an artist left the cursor at %d", m.Cursor())
	}
	if strings.Contains(m.Status(), "reloaded") {
		t.Errorf("opening an artist reported a reload: %q", m.Status())
	}
}

// TestOpeningAfterAReloadThatShrankTheListDoesNotPanic is the failure the
// clamp above prevents: a cursor left past the end of a shorter list is an
// index opening reads past.
func TestOpeningAfterAReloadThatShrankTheListDoesNotPanic(t *testing.T) {
	m := loaded(t)
	m, _ = press(t, m, "end")

	m = send(t, m, Reload{})
	m = send(t, m, ArtistsLoaded(library()[:1]))

	// Every key that reads the selected row.
	for _, key := range []string{"enter", "*", "a", "A"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s after a reload that shrank the list panicked: %v", key, r)
				}
			}()
			_, _ = press(t, m, key)
		}()
	}
}

// TestTheWikiOpensACommandAndComesBack covers the whole way through: the list,
// one command, and back to the list before leaving.
func TestTheWikiOpensACommandAndComesBack(t *testing.T) {
	m := send(t, loaded(t), ShowWiki{})

	if m.Screen() != ScreenWiki {
		t.Fatalf(":wiki went to %s", m.Screen())
	}
	if m.rows() != len(commands) {
		t.Errorf("the wiki lists %d rows and there are %d commands", m.rows(), len(commands))
	}
	if frame := m.View(); !strings.Contains(frame, ":search") {
		t.Errorf("the list does not name the commands:\n%s", frame)
	}

	// Open one.
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "enter")
	if m.Screen() != ScreenWiki {
		t.Fatalf("opening a command went to %s", m.Screen())
	}
	frame := m.View()
	if !strings.Contains(frame, "wiki · :search") {
		t.Errorf("the heading reads:\n%s", frame)
	}
	if !strings.Contains(frame, "anywhere on your") {
		t.Errorf("the explanation is not on screen:\n%s", frame)
	}

	// esc goes back to the list, not out of the wiki.
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenWiki {
		t.Fatalf("esc left the wiki instead of going back to the list; it is on %s", m.Screen())
	}
	if frame := m.View(); !strings.Contains(frame, "11 commands") && !strings.Contains(frame, "commands") {
		t.Errorf("it did not go back to the list:\n%s", frame)
	}

	// And again leaves.
	m, _ = press(t, m, "esc")
	if m.Screen() != ScreenArtists {
		t.Errorf("esc from the list went to %s", m.Screen())
	}
}

// TestReadingAPageHasNoCursorOnIt covers the rendering. A page of prose is
// scrolled, and a highlight sitting on a sentence reads as a selection that
// does nothing.
func TestReadingAPageHasNoCursorOnIt(t *testing.T) {
	m := send(t, loaded(t), ShowWiki{})
	list := m.View()
	m, _ = press(t, m, "enter")
	page := m.View()

	if !strings.Contains(list, "\x1b[7m") {
		t.Error("the list of commands has no cursor on it")
	}
	if strings.Contains(page, "\x1b[7m") {
		t.Errorf("the page has a cursor on a sentence:\n%s", page)
	}
	if strings.Contains(page, "> ") {
		t.Errorf("the page has a selection marker on it:\n%s", page)
	}
}

// TestAWikiPageScrolls covers a description longer than the screen, which is
// most of them.
func TestAWikiPageScrolls(t *testing.T) {
	m := send(t, loaded(t), ShowWiki{})
	m, _ = press(t, m, "j") // search, which is long
	m, _ = press(t, m, "enter")

	first := m.View()
	for range 4 {
		m, _ = press(t, m, "j")
	}
	if m.View() == first {
		t.Error("the page did not scroll")
	}
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "g")
	if m.View() != first {
		t.Error("going back to the top did not show the beginning again")
	}
}

// TestAFrameWithCoverArtStillFitsTheTerminal covers the rows the art takes.
//
// The list is given whatever height is left after everything that is not the
// list, so art that is drawn but not counted makes the frame taller than the
// terminal and the top of it scrolls away.
func TestAFrameWithCoverArtStillFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{64, 24}, {64, 16}, {80, 40}} {
		w, h := size[0], size[1]

		m := New()
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = next.(Model)
		next, _ = m.Update(AlbumLoaded(subsonic.Album{
			ID: "al-1", Name: "Harbour", Artist: "Aoi",
			Songs: []subsonic.Song{{ID: "tr-1", Title: "Slipway"}, {ID: "tr-2", Title: "Ballast"}},
		}))
		m = next.(Model)

		bare := strings.Count(m.View(), "\n") + 1

		art := make([]string, 6)
		for i := range art {
			art[i] = strings.Repeat(" ", 12)
		}
		next, _ = m.Update(CoverArt{AlbumID: "al-1", Lines: art})
		m = next.(Model)

		withArt := strings.Count(m.View(), "\n") + 1
		if withArt != bare {
			t.Errorf("%dx%d: the frame is %d rows with art and %d without; art must take rows from the list",
				w, h, withArt, bare)
		}
		if withArt > h {
			t.Errorf("%dx%d: the frame is %d rows, which is taller than the terminal", w, h, withArt)
		}
	}
}

// TestTheBigCoverComingAndGoingRepaints covers what a frame comparison cannot.
//
// The renderer writes only the rows that changed. A cover filling the screen
// and the one above the track list belong to the same album, so the title row
// is identical between them and is not written again -- and that row is the one
// carrying the sequence that takes the previous picture away. Both pictures end
// up on screen, which is what a screenshot showed and what View() alone cannot
// see.
func TestTheBigCoverComingAndGoingRepaints(t *testing.T) {
	repaints := func(cmd tea.Cmd) bool {
		return cmd != nil && reflect.DeepEqual(cmd(), tea.ClearScreen())
	}

	m := New()
	next, cmd := m.Update(FullArt{"a picture filling the screen"})
	m = next.(Model)
	if !repaints(cmd) {
		t.Error("showing a cover full screen did not ask for a repaint")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !repaints(cmd) {
		t.Error("leaving a full screen cover did not ask for a repaint")
	}
	if len(next.(Model).bigArt) != 0 {
		t.Error("esc did not put the cover away")
	}

	// And esc with no cover up is ordinary navigation, not a repaint.
	m = New()
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}); repaints(cmd) {
		t.Error("esc repaints the screen when there is no cover to take away")
	}
}
