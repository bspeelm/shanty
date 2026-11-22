package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// onCommand puts the cursor of the keys screen on the row of one command.
func onCommand(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, r := range m.keyRows() {
		if r.kind == kindCommand && r.id == name {
			return m.selecting(ScreenWiki, i)
		}
	}
	t.Fatalf("the keys screen lists no command called %s", name)
	return m
}

// listedKeys is every key the keys screen names in the rows under one
// heading, or under every heading when none is given.
func listedKeys(m Model, heading string) map[string]bool {
	out, under := map[string]bool{}, ""
	for _, r := range m.keyRows() {
		switch {
		case r.kind == kindHeading:
			under = r.name
		case r.kind == kindKey && (heading == "" || under == heading):
			for _, key := range strings.Fields(r.name) {
				out[key] = true
			}
		}
	}
	return out
}

// seen is what a key can change that somebody could see, with what it asked
// for.
func seen(m Model, msg tea.Msg) []any {
	return []any{m.screen, m.cursor, m.mode, m.filter, m.line, m.status, m.showSky, m.pending, msg}
}

// TestTheKeysScreenListsEveryKeyTheProgramActsOn walks the three tables the
// keys are dispatched from and holds each key against the screen ? opens.
func TestTheKeysScreenListsEveryKeyTheProgramActsOn(t *testing.T) {
	m, msg := press(t, loaded(t), "?")
	m = send(t, m, msg)
	if m.Screen() != ScreenWiki {
		t.Fatalf("? went to %s", m.Screen())
	}
	listed := listedKeys(m, "")

	for key, action := range m.bound() {
		if !listed[spell(key)] {
			t.Errorf("%q is bound to %s and the keys screen does not list it", key, action.Name)
		}
	}
	for _, j := range jumps() {
		if !listed["g"+j.key] {
			t.Errorf("g%s is a jump and the keys screen does not list it", j.key)
		}
	}
	for heading, mode := range map[string]mode{
		"TYPING A FILTER": modeFilter, "TYPING A COMMAND": modeCommand, "A YES OR NO QUESTION": modeConfirm,
	} {
		under := listedKeys(m, heading)
		for _, k := range modeKeys() {
			if k.mode == mode && !under[spell(k.key)] {
				t.Errorf("%q acts under %s and the keys screen does not list it there", k.key, heading)
			}
		}
	}
	for _, c := range commands {
		if !strings.Contains(m.rowsText(), ":"+c.name) {
			t.Errorf(":%s is a command and the keys screen does not list it", c.name)
		}
	}
}

// rowsText is every row of the keys screen as one string.
func (m Model) rowsText() string {
	var b strings.Builder
	for _, r := range m.keyRows() {
		b.WriteString(r.name + "  " + r.detail + "\n")
	}
	return b.String()
}

// TestEveryJumpTheKeysScreenListsGoesSomewhere presses each jump from a state
// in which every one of them has somewhere to go.
func TestEveryJumpTheKeysScreenListsGoesSomewhere(t *testing.T) {
	m := send(t, searched(t), Notice("one"), Notice("two"), QueueChanged{Tracks: queued(), At: 1},
		ShowMessages{}, keyOf("j"))
	idle, nothing := press(t, m, "g")
	idle, nothing = press(t, idle, "Z")

	for _, j := range jumps() {
		got, _ := press(t, m, "g")
		got, msg := press(t, got, j.key)
		if j.key == "v" {
			msg = nil
		}
		if reflect.DeepEqual(seen(got, msg), seen(idle, nothing)) {
			t.Errorf("g%s is listed as %q and changed nothing", j.key, j.what)
		}
	}
	if !reflect.DeepEqual(seen(idle, nothing), seen(m, nil)) {
		t.Errorf("g and then a key that is no jump did something")
	}
}

// TestEveryKeyInsideAModeDoesSomething presses each key the mode tables hold,
// in its mode, from a state in which it has something to do.
func TestEveryKeyInsideAModeDoesSomething(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 64, Height: 10})
	filtering, _ := press(t, send(t, next.(Model), AlbumLoaded(tenTracks())), "/")
	filtering = typed(t, filtering, "track")
	for range 5 {
		filtering, _ = press(t, filtering, "down")
	}
	commanding, _ := press(t, loaded(t), ":")
	commanding = typed(t, commanding, "pl")
	asking := send(t, loaded(t), Confirm{Question: "delete it?", Do: Notice("deleted")})
	in := map[mode]Model{modeFilter: filtering, modeCommand: commanding, modeConfirm: asking}

	for _, k := range modeKeys() {
		m := in[k.mode]
		if m.mode != k.mode {
			t.Fatalf("the fixture for %q is not in its mode", k.key)
		}
		after, cmd := m.Update(keyOf(k.key))
		if k.key == "ctrl+c" {
			after, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		}
		var msg tea.Msg
		if cmd != nil {
			msg = cmd()
		}
		if reflect.DeepEqual(seen(after.(Model), msg), seen(m, nil)) {
			t.Errorf("%q is listed as %q in its mode and changed nothing", k.key, k.what)
		}
	}

	// At a question, any key the table does not hold answers no.
	said, msg := press(t, asking, "x")
	if msg != nil || said.Confirming() || !strings.Contains(said.Status(), "nothing was changed") {
		t.Errorf("x at a question emitted %#v and said %q", msg, said.Status())
	}
	if _, msg := press(t, asking, "y"); msg != Notice("deleted") {
		t.Errorf("y at a question emitted %#v", msg)
	}
}

func TestRebindingAnActionChangesWhatTheKeysScreenAndTheLastRowShow(t *testing.T) {
	keys, wrong := Bind(map[string][]string{"pause": {"x"}, "keys": {"F1"}})
	if len(wrong) > 0 {
		t.Fatal(wrong)
	}
	m := send(t, loaded(t).WithKeys(keys), tea.WindowSizeMsg{Width: 200, Height: 12})

	if last := m.hintLine(200); !strings.Contains(last, "x pause") || !strings.HasSuffix(last, "F1 keys") || strings.Contains(last, "space") {
		t.Errorf("the last row does not show the keys in force: %q", last)
	}
	text := m.rowsText()
	if !strings.Contains(text, "x  pause, or resume") || strings.Contains(text, "space  pause") {
		t.Errorf("the keys screen does not show pause on the key it is bound to:\n%s", text)
	}
}

func TestTheKeysScreenIsNarrowedByWhatAKeyDoes(t *testing.T) {
	m := narrowed(t, send(t, loaded(t), ShowWiki{}), "shuffle")
	frame := m.View()
	if m.rows() < 2 || !strings.Contains(frame, "S  ") || !strings.Contains(frame, ":shuffle") {
		t.Errorf("a filter on what a key does left %d rows:\n%s", m.rows(), frame)
	}
	if strings.Contains(frame, "MOVING") {
		t.Errorf("a heading is still listed in the narrowed list:\n%s", frame)
	}
}

func TestOnTheKeysScreenEnterOpensAPageForACommandOrAKey(t *testing.T) {
	m := send(t, loaded(t), ShowWiki{})
	page, _ := press(t, onCommand(t, m, "search"), "enter")
	if page.topic != "search" || !strings.Contains(page.View(), "wiki · :search") {
		t.Errorf("enter on a command did not open its page:\n%s", page.View())
	}
	back, _ := press(t, page, "esc")
	if back.Screen() != ScreenWiki || back.topic != "" || back.keyRows()[back.Cursor()].id != "search" {
		t.Errorf("esc from a page went to %s on row %d", back.Screen(), back.Cursor())
	}
}

// TestEveryKeyRowCanBeReadInFullAtAnyWidth opens the page of each key row at
// a narrow and a wide terminal, and holds the page to every word of the row.
func TestEveryKeyRowCanBeReadInFullAtAnyWidth(t *testing.T) {
	for _, w := range []int{64, 200} {
		m := send(t, loaded(t), tea.WindowSizeMsg{Width: w, Height: 12}, ShowWiki{})
		cut := 0
		for i, r := range m.keyRows() {
			if r.kind != kindKey {
				continue
			}
			at := m.selecting(ScreenWiki, i)
			if strings.Contains(stripSGR(at.View()), "…\n") {
				cut++
			}
			page, msg := press(t, at, "enter")
			if msg != nil || !page.reading() {
				t.Fatalf("at %d columns enter on %q emitted %#v and opened nothing", w, r.name, msg)
			}
			text := strings.Join(strings.Fields(strings.Join(page.pageLines(), " ")), " ")
			if text != strings.Join(strings.Fields(r.detail), " ") || !strings.Contains(page.View(), "wiki · "+r.name) {
				t.Errorf("at %d columns the page for %q reads %q, want %q", w, r.name, text, r.detail)
			}
			if back, _ := press(t, page, "esc"); back.Cursor() != i || back.reading() {
				t.Errorf("at %d columns esc from the page for %q went to row %d", w, r.name, back.Cursor())
			}
		}
		if w == 64 && cut == 0 {
			t.Error("no row is cut at 64 columns, so the narrow case measures nothing")
		}
	}
}

func TestAPageOpenedFromANarrowedListIsWholeAndTheListComesBackNarrowed(t *testing.T) {
	m := narrowed(t, send(t, loaded(t), ShowWiki{}), "headless")
	at := m.Cursor()
	page, _ := press(t, m, "enter")
	if page.Filter() != "" || page.rows() < 3 {
		t.Fatalf("the page opened narrowed by %q to %d rows", page.Filter(), page.rows())
	}
	back, _ := press(t, page, "esc")
	if back.Filter() != "headless" || back.Cursor() != at || back.reading() {
		t.Errorf("esc came back with the filter %q on row %d", back.Filter(), back.Cursor())
	}
}

func TestEveryRowOfTheKeysScreenSaysWhatItIsWithoutItsHeading(t *testing.T) {
	m := send(t, loaded(t), ShowWiki{})
	seenRows := map[string]bool{}
	under := ""
	for _, r := range m.keyRows() {
		if r.kind == kindHeading {
			under = strings.ToLower(r.name)
			continue
		}
		row := r.name + " | " + r.detail
		if seenRows[row] {
			t.Errorf("two rows read %q", row)
		}
		seenRows[row] = true
		moded := strings.Contains(under, "typing") || strings.Contains(under, "question") ||
			strings.Contains(under, "reordering") || strings.Contains(under, "adding") || strings.Contains(under, "visualiser")
		if moded && !strings.HasPrefix(r.detail, under+": ") {
			t.Errorf("the row %q under %q does not say which mode it is about", row, under)
		}
	}
}

// TestACountIsListedForEveryKeyThatTakesOne walks the actions that use a
// number typed before them, and holds each to being listed and to doing
// something different with the number.
func TestACountIsListedForEveryKeyThatTakesOne(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 64, Height: 10})
	base := send(t, next.(Model), AlbumLoaded(tenTracks()), keyOf("j"), keyOf("j"), keyOf("j"), keyOf("j"), keyOf("j"))
	text := base.rowsText()
	if !strings.Contains(text, "0-9  type a number") {
		t.Errorf("the keys screen does not list the digits:\n%s", text)
	}
	counted := 0
	for _, a := range Actions() {
		if a.Count == "" {
			continue
		}
		counted++
		if !strings.Contains(text, "{n} "+strings.Join(base.keysFor(a.Name), " ")+"  "+a.Count) {
			t.Errorf("%s takes a count and the keys screen does not list it", a.Name)
		}
		key := keyOf(a.Keys[0])
		if a.Keys[0] == "ctrl+u" || a.Keys[0] == "ctrl+d" {
			key = tea.KeyMsg{Type: map[string]tea.KeyType{"ctrl+u": tea.KeyCtrlU, "ctrl+d": tea.KeyCtrlD}[a.Keys[0]]}
		}
		from := base
		if a.Name == "playlist-remove" {
			from = onQueue(t, send(t, loaded(t), QueueChanged{Tracks: queued()}))
		}
		plain, cmd := from.Update(key)
		with := send(t, from, keyOf("2"))
		numbered, counting := with.Update(key)
		var one, two tea.Msg
		if cmd != nil {
			one = cmd()
		}
		if counting != nil {
			two = counting()
		}
		if reflect.DeepEqual(seen(plain.(Model), one), seen(numbered.(Model), two)) {
			t.Errorf("a number before %s changes nothing, and it is listed as taking one", a.Name)
		}
	}
	if counted < 7 {
		t.Errorf("only %d actions say what a count does", counted)
	}
}

// TestTheLastRowNamesTheKeysOfTheScreenShowingFirst holds the hint line of
// each screen to starting with what that screen's own keys do there.
func TestTheLastRowNamesTheKeysOfTheScreenShowingFirst(t *testing.T) {
	rows := everywhere(t)
	for name, wants := range map[string][]string{
		"a track on an album":   {"enter play", "a queue", "A next", "S shuffle", "* star"},
		"an artist":             {"enter open", "a queue", "A next", "S shuffle", "* star"},
		"a track in the queue":  {"enter play", "r remove", "K up", "J down", "S shuffle"},
		"a track in a playlist": {"enter play", "a queue", "r remove", "e add"},
		"the library":           {"enter open", "/ filter", ": command", "space pause"},
		"the messages":          {"esc back", "/ filter"},
	} {
		line := rows[name].hintLine(400)
		at := 0
		for _, want := range wants {
			i := strings.Index(line[at:], want)
			if i < 0 {
				t.Errorf("%s: %q is missing, or out of order, in %q", name, want, line)
				break
			}
			at += i
		}
		if !strings.HasPrefix(line, "↑↓ move · ") || !strings.HasSuffix(line, " · ? keys") {
			t.Errorf("%s: the line does not begin with moving and end with the keys key: %q", name, line)
		}
	}
	if strings.Contains(rows["the messages"].hintLine(400), "enter") {
		t.Errorf("the messages screen names enter, which does nothing there")
	}
}

func TestAJumpBegunListsEveryPlaceItCanGoAtAnyWidth(t *testing.T) {
	for _, w := range []int{40, 64, 80, 200} {
		m, _ := press(t, send(t, loaded(t), tea.WindowSizeMsg{Width: w, Height: 14}), "g")
		frame := stripSGR(m.View())
		for _, j := range jumps() {
			if !strings.Contains(frame, j.key+" "+j.short) {
				t.Errorf("at %d columns the frame does not offer %s %s:\n%s", w, j.key, j.short, frame)
			}
		}
		rows := strings.Split(frame, "\n")
		if len(rows) != 14 || !strings.HasPrefix(rows[len(rows)-1], "g …") {
			t.Errorf("at %d columns the frame is %d rows ending %q", w, len(rows), rows[len(rows)-1])
		}
	}
}

// lastRow is the last row of the frame, without its styling.
func lastRow(m Model) string {
	rows := strings.Split(stripSGR(m.View()), "\n")
	return rows[len(rows)-1]
}

func TestUnderACoverTheLastRowSaysWhatWorksThere(t *testing.T) {
	m := send(t, deepIn(t), FullArt{"a picture filling the screen"})
	if got := lastRow(m); got != "esc leaves · space pauses · any other key takes the cover away" {
		t.Errorf("under a cover the last row reads %q", got)
	}
}

func TestTheLibraryDoesNotOfferBackAndOtherScreensDo(t *testing.T) {
	if line := sized(New()).hintLine(400); strings.Contains(line, "back") {
		t.Errorf("the library offers back, which does nothing there: %q", line)
	}
	if line := loaded(t).hintLine(400); !strings.Contains(line, "esc back") {
		t.Errorf("the artists do not offer back: %q", line)
	}
}

// TestTheLastRowAndTheKeysScreenFollowEveryRebinding binds the keys the
// hints name to other keys, and holds each hint to the key bound now.
func TestTheLastRowAndTheKeysScreenFollowEveryRebinding(t *testing.T) {
	keys, wrong := Bind(map[string][]string{
		"back": {"x"}, "pause": {"z"}, "open": {"o"}, "go-prefix": {";"},
		"queue-append": {"i"}, "playlist-remove": {"d"}, "playlist-edit": {"w"},
		"playlist-move-up": {"U"}, "playlist-move-down": {"D"},
	})
	if len(wrong) > 0 {
		t.Fatal(wrong)
	}
	bound := func(m Model) Model { return send(t, m.WithKeys(keys), tea.WindowSizeMsg{Width: 200, Height: 14}) }

	if got := lastRow(reordering(t, bound(evening3(t)))); got != "reordering Evening — U/D move, o saves, x discards" {
		t.Errorf("reordering: %q", got)
	}
	editing := send(t, bound(loaded(t)), EditingPlaylist(eveningOf(1)), keyOf("j"))
	if got := lastRow(editing); got != "adding to Evening — i adds, d removes, w leaves" {
		t.Errorf("adding to a playlist: %q", got)
	}
	sky := send(t, bound(loaded(t)), ShowVisualiser{})
	if got := lastRow(sky); got != "x leaves · z pauses" {
		t.Errorf("the visualiser: %q", got)
	}
	if got := lastRow(send(t, sky, PausedChanged(true))); got != "paused — z resumes · x leaves" {
		t.Errorf("the visualiser, paused: %q", got)
	}
	if got := lastRow(send(t, bound(loaded(t)), FullArt{"a picture"})); !strings.HasPrefix(got, "x leaves · z pauses") {
		t.Errorf("a cover: %q", got)
	}

	jumping, _ := press(t, bound(loaded(t)), ";")
	if got := lastRow(jumping); got != "; …" || jumping.Pending() == "" {
		t.Errorf("a jump begun with the key bound to it: %q", got)
	}
	if text := bound(loaded(t)).rowsText(); !strings.Contains(text, ";l  go to the library") || strings.Contains(text, "gl  ") {
		t.Errorf("the keys screen does not list the jumps with the prefix bound now:\n%s", text)
	}

	long, _ := Bind(map[string][]string{"go-prefix": {"ctrl+g"}})
	if text := loaded(t).WithKeys(long).rowsText(); !strings.Contains(text, "ctrl+g l  go to the library") {
		t.Errorf("a prefix of more than one character runs into the key after it:\n%s", text)
	}
}

func TestAnActionLeftWithNoKeyIsStillListed(t *testing.T) {
	keys, wrong := Bind(map[string][]string{"pause": {"n"}})
	if len(wrong) != 1 {
		t.Fatalf("binding pause to the key next holds reported %v", wrong)
	}
	text := loaded(t).WithKeys(keys).rowsText()
	if !strings.Contains(text, "(no key)  pause, or resume if already paused") {
		t.Errorf("pause has no key and the keys screen does not list it as such:\n%s", text)
	}
}
