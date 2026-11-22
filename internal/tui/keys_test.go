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

func TestOnTheKeysScreenACommandOpensItsPageAndAKeySaysItIsNotOne(t *testing.T) {
	m := send(t, loaded(t), ShowWiki{})
	if r := m.keyRows()[m.Cursor()]; r.kind != kindKey {
		t.Fatalf("the cursor starts on %+v, want the first key", r)
	}
	said, msg := press(t, m, "enter")
	if msg != nil || said.topic != "" || said.Status() == "" {
		t.Errorf("enter on a key emitted %#v and said %q", msg, said.Status())
	}

	page, _ := press(t, onCommand(t, m, "search"), "enter")
	if page.topic != "search" || !strings.Contains(page.View(), "wiki · :search") {
		t.Errorf("enter on a command did not open its page:\n%s", page.View())
	}
	back, _ := press(t, page, "esc")
	if back.Screen() != ScreenWiki || back.topic != "" || !back.keyRows()[back.Cursor()].selectable() {
		t.Errorf("esc from a page went to %s on row %d", back.Screen(), back.Cursor())
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
		"the library":           {"enter open", "esc back", "/ filter", ": command", "space pause"},
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

func TestAJumpBegunListsWhereItCanGo(t *testing.T) {
	m, _ := press(t, send(t, loaded(t), tea.WindowSizeMsg{Width: 200, Height: 12}), "g")
	rows := strings.Split(stripSGR(m.View()), "\n")
	for _, j := range jumps() {
		if !strings.Contains(rows[len(rows)-1], j.key+" "+j.short) {
			t.Errorf("the last row does not offer %s %s: %q", j.key, j.short, rows[len(rows)-1])
		}
	}
}
