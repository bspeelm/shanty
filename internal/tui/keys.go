package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// spelling is how a key that shanty names differently is written for reading.
var spelling = map[string]string{
	"up": "↑", "down": "↓", "left": "←", "right": "→", " ": "space",
}

// spell is a key as it is written for reading.
func spell(key string) string {
	if as, differs := spelling[key]; differs {
		return as
	}
	return key
}

// jump is one key pressed after the prefix that begins a jump: the key, one
// word for where it goes, what it does, and the doing of it.
type jump struct {
	key, short, what string
	do               func(Model) (tea.Model, tea.Cmd)
}

// jumps is every jump, in the order they are listed.
func jumps() []jump {
	to := func(s Screen, fetch tea.Msg) func(Model) (tea.Model, tea.Cmd) {
		return func(m Model) (tea.Model, tea.Cmd) {
			if fetch == nil {
				return m.show(s), nil
			}
			return m.show(s), emit(fetch)
		}
	}
	return []jump{
		{"g", "top", "jump to the first row", func(m Model) (tea.Model, tea.Cmd) { return m.moveTo(0).ontoARow(), nil }},
		{"l", "library", "go to the library", to(ScreenLibrary, nil)},
		{"a", "artists", "go to the artists", to(ScreenArtists, nil)},
		{"b", "albums", "go to every album", to(ScreenEveryAlbum, FetchEveryAlbum{})},
		{"p", "playlists", "go to the playlists", to(ScreenPlaylists, FetchPlaylists{})},
		{"q", "queue", "go to the queue, on the track playing", func(m Model) (tea.Model, tea.Cmd) {
			m = m.show(ScreenQueue)
			return m.selecting(ScreenQueue, max(0, min(len(m.queued)-1, m.queuedAt))).settled(ScreenQueue), nil
		}},
		{"s", "starred", "go to what is starred", to(ScreenStarred, nil)},
		{"/", "search", "go back to the last search", func(m Model) (tea.Model, tea.Cmd) {
			if m.query == "" {
				return m.say("nothing has been searched for yet — type :search <words>"), nil
			}
			return m.show(ScreenSearch), nil
		}},
		{"v", "visualiser", "fill the screen with the visualiser", (Model).openSky},
	}
}

// goTo does the jump the key after the prefix names. A key that names none
// does nothing.
func (m Model) goTo(key string) (tea.Model, tea.Cmd) {
	for _, j := range jumps() {
		if j.key == key {
			return j.do(m)
		}
	}
	return m, nil
}

// modeKey is one key that acts while a filter, a command or an answer is
// being typed.
type modeKey struct {
	mode      mode
	key, what string
	do        func(Model) (tea.Model, tea.Cmd)
}

// modeKeys is every key that acts inside a mode. Any other key is typed, or
// at a question answers no.
func modeKeys() []modeKey {
	quit := func(m Model) (tea.Model, tea.Cmd) { return m, emit(Quit{}) }
	moved := func(by func(Model) Model) func(Model) (tea.Model, tea.Cmd) {
		return func(m Model) (tea.Model, tea.Cmd) { return by(m), nil }
	}
	yes := func(m Model) (tea.Model, tea.Cmd) { return m.answer(true) }
	return []modeKey{
		{modeFilter, "esc", "drop the filter and show the whole list", moved(func(m Model) Model {
			m.mode, m.filter = modeNormal, ""
			return m.moveTo(0).ontoARow()
		})},
		{modeFilter, "enter", "keep the filter", moved(func(m Model) Model {
			m.mode = modeNormal
			return m
		})},
		{modeFilter, "backspace", "take back the last character", moved(func(m Model) Model {
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
			}
			return m.moveTo(0).ontoARow()
		})},
		{modeFilter, "up", "move the selection up one row", moved(func(m Model) Model { return m.move(-1) })},
		{modeFilter, "down", "move the selection down one row", moved(func(m Model) Model { return m.move(1) })},
		{modeFilter, "pgup", "move up by one screenful", moved(func(m Model) Model { return m.move(-m.page()) })},
		{modeFilter, "pgdown", "move down by one screenful", moved(func(m Model) Model { return m.move(m.page()) })},
		{modeFilter, "home", "jump to the first row", moved(func(m Model) Model { return m.moveTo(0).ontoARow() })},
		{modeFilter, "end", "jump to the last row", moved(func(m Model) Model { return m.moveTo(m.rows() - 1) })},
		{modeFilter, "ctrl+c", "quit shanty", quit},
		{modeCommand, "esc", "abandon the line", moved(func(m Model) Model {
			m.mode, m.line = modeNormal, ""
			return m
		})},
		{modeCommand, "tab", "complete the command as far as the matches agree", moved(func(m Model) Model {
			m.line = complete(m.line)
			return m
		})},
		{modeCommand, "enter", "run the command", (Model).runCommand},
		{modeCommand, "backspace", "take back the last character", moved(func(m Model) Model {
			if m.line != "" {
				m.line = m.line[:len(m.line)-1]
			}
			return m
		})},
		{modeCommand, "ctrl+c", "quit shanty", quit},
		{modeConfirm, "y", "answer yes", yes},
		{modeConfirm, "Y", "answer yes", yes},
	}
}

// modeKey handles a key while a filter, a command or an answer is being
// typed. A key the mode has no use for is typed into the filter or the line,
// and at a question it answers no.
func (m Model) modeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	for _, k := range modeKeys() {
		if k.mode == m.mode && k.key == msg.String() {
			return k.do(m)
		}
	}
	text := string(msg.Runes)
	if msg.Type == tea.KeySpace {
		text = " "
	}
	switch {
	case m.mode == modeConfirm:
		return m.answer(false)
	case msg.Type != tea.KeyRunes && msg.Type != tea.KeySpace:
		return m, nil
	case m.mode == modeFilter:
		m.filter += text
		return m.moveTo(0).ontoARow(), nil
	}
	m.line += text
	return m, nil
}

// keysFor is the keys bound to an action now, written for reading. The keys
// it comes with are first, in their own order.
func (m Model) keysFor(name string) []string {
	rank := map[string]int{}
	for _, a := range Actions() {
		if a.Name == name {
			for i, key := range a.Keys {
				rank[key] = i - len(a.Keys)
			}
		}
	}
	var keys []string
	for key, a := range m.bound() {
		if a.Name == name {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if rank[keys[i]] != rank[keys[j]] {
			return rank[keys[i]] < rank[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for i := range keys {
		keys[i] = spell(keys[i])
	}
	return keys
}

// first is the first key bound to an action now, or a note that it has none.
func (m Model) first(action string) string {
	if keys := m.keysFor(action); len(keys) > 0 {
		return keys[0]
	}
	return "(no key)"
}

// jumpKeys is a jump as it is pressed and written: the prefix bound now, and
// the key after it.
func (m Model) jumpKeys(j jump) string {
	if prefix := m.first("go-prefix"); len(prefix) > 1 {
		return prefix + " " + j.key
	}
	return m.first("go-prefix") + j.key
}

// keyGroups is the headings the actions are listed under, in order, and
// groupOf is the heading an action belongs under.
var keyGroups = []string{"moving", "going places", "lists", "the queue", "playlists", "playback"}

func groupOf(action string) string {
	switch {
	case strings.HasPrefix(action, "move-"):
		return "moving"
	case strings.HasPrefix(action, "queue-"):
		return "the queue"
	case strings.HasPrefix(action, "playlist-"):
		return "playlists"
	case action == "filter", action == "star", action == "shuffle-selected":
		return "lists"
	case action == "pause", action == "next", action == "previous",
		strings.HasPrefix(action, "seek-"), strings.HasPrefix(action, "volume-"):
		return "playback"
	}
	return "going places"
}

// keyRows is the screen that lists every key and every command: each action
// with the keys bound to it now, each jump, the keys inside each mode, and
// the commands.
func (m Model) keyRows() []result {
	var rows []result
	in := ""
	head := func(name string) {
		in = ""
		rows = append(rows, result{kind: kindHeading, name: strings.ToUpper(name)})
	}
	add := func(keys, what string) { rows = append(rows, result{kind: kindKey, name: keys, detail: what + in}) }
	keys := func(action string) string {
		if bound := m.keysFor(action); len(bound) > 0 {
			return strings.Join(bound, " ")
		}
		return "(no key)"
	}

	for _, group := range keyGroups {
		head(group)
		for _, a := range Actions() {
			if groupOf(a.Name) == group {
				add(keys(a.Name), a.Summary)
			}
		}
		if group == "going places" {
			for _, j := range jumps() {
				add(m.jumpKeys(j), j.what)
			}
		}
	}
	head("counts")
	add("0-9", "type a number before one of the keys below")
	for _, a := range Actions() {
		if a.Count != "" {
			add("{n} "+keys(a.Name), a.Count)
		}
	}
	for _, mode := range []struct {
		mode mode
		name string
	}{{modeFilter, "typing a filter"}, {modeCommand, "typing a command"}, {modeConfirm, "a yes or no question"}} {
		head(mode.name)
		in = ", " + mode.name
		for _, k := range modeKeys() {
			if k.mode == mode.mode {
				add(spell(k.key), k.what)
			}
		}
	}
	add("any other key", "answer no")
	head("reordering a playlist")
	in = ", reordering a playlist"
	add(keys("playlist-move-up")+" "+keys("playlist-move-down"), "move the selected track up or down")
	add(keys("open"), "save the arrangement")
	add(keys("back"), "discard it")
	head("adding to a playlist")
	in = ", adding to a playlist"
	add(keys("queue-append"), "add the selected track")
	add(keys("playlist-remove"), "take it out")
	add(keys("playlist-edit"), "stop adding")
	head("under the visualiser or a cover")
	in = ", under the visualiser or a cover"
	for _, a := range Actions() {
		if overCover[a.Name] {
			add(keys(a.Name), a.Summary)
		}
	}
	add("0-9", "type a number before the key that seeks to a percentage")
	add("any other key", "take a cover away; do nothing under the visualiser")
	head("commands")
	for _, c := range commands {
		rows = append(rows, result{kind: kindCommand, id: c.name, name: ":" + spelled(c), detail: c.summary})
	}
	return rows
}

// reorderHint, editingHint, skyHint and coverHint are the last row while a
// playlist is being reordered or added to, and under the visualiser or a
// cover, with the keys bound now.
func (m Model) reorderHint() string {
	return "reordering " + Sanitise(m.playlist.Name) + " — " + m.first("playlist-move-up") + "/" +
		m.first("playlist-move-down") + " move, " + m.first("open") + " saves, " + m.first("back") + " discards"
}

func (m Model) editingHint(name string) string {
	return "adding to " + Sanitise(name) + " — " + m.first("queue-append") + " adds, " +
		m.first("playlist-remove") + " removes, " + m.first("playlist-edit") + " leaves"
}

func (m Model) skyHint() string {
	if m.paused {
		return "paused — " + m.first("pause") + " resumes · " + m.first("back") + " leaves"
	}
	return m.first("back") + " leaves · " + m.first("pause") + " pauses"
}

func (m Model) coverHint() string {
	return m.first("back") + " leaves · " + m.first("pause") + " pauses · any other key takes the cover away"
}

// hints is the actions named first on the last row of each screen, each with
// the word for what it does there.
var hints = map[Screen]string{
	ScreenLibrary:    "open:open",
	ScreenArtists:    "open:open queue-append:queue queue-next:next shuffle-selected:shuffle star:star",
	ScreenAlbums:     "open:open queue-append:queue queue-next:next shuffle-selected:shuffle star:star",
	ScreenEveryAlbum: "open:open queue-append:queue queue-next:next shuffle-selected:shuffle star:star",
	ScreenTracks:     "open:play queue-append:queue queue-next:next shuffle-selected:shuffle star:star",
	ScreenQueue:      "open:play playlist-remove:remove playlist-move-up:up playlist-move-down:down shuffle-selected:shuffle",
	ScreenSearch:     "open:open queue-append:queue queue-next:next shuffle-selected:shuffle star:star",
	ScreenStarred:    "open:open queue-append:queue queue-next:next shuffle-selected:shuffle star:star",
	ScreenPlaylists:  "open:open queue-append:queue queue-next:next shuffle-selected:shuffle",
	ScreenPlaylist:   "open:play queue-append:queue queue-next:next shuffle-selected:shuffle playlist-remove:remove playlist-edit:add",
	ScreenWiki:       "open:read",
}

// hintLine is the key help for the screen showing: its own actions, then the
// ones every screen has, with the keys bound to them now. As many as fit in w
// are written, with an ellipsis where some were left out, and the key that
// lists them all is always last.
func (m Model) hintLine(w int) string {
	var items []string
	own := hints[m.screen]
	if m.reading() {
		own = ""
	}
	// The library with nowhere to go back to does not offer back.
	if m.screen != ScreenLibrary {
		own += " back:back"
	}
	for _, pair := range strings.Fields(own + " filter:filter command:command pause:pause") {
		action, word, _ := strings.Cut(pair, ":")
		if keys := m.keysFor(action); len(keys) > 0 {
			items = append(items, keys[0]+" "+word)
		}
	}
	if up, down := m.keysFor("move-up"), m.keysFor("move-down"); len(up) > 0 && len(down) > 0 {
		items = append([]string{up[0] + down[0] + " move"}, items...)
	}
	last := "keys"
	if keys := m.keysFor("keys"); len(keys) > 0 {
		last = keys[0] + " keys"
	}
	line := ""
	for i, item := range items {
		next := line + item + " · "
		if lipgloss.Width(next+"… · "+last) > w && i < len(items) {
			return line + "… · " + last
		}
		line = next
	}
	return line + last
}
