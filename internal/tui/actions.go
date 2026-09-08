package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// An Action is one thing a key does, under a name.
//
// The name is what a key is bound to, so that a binding can be written down,
// checked against the documentation, and changed in the configuration without
// anything knowing which key it came from.
type Action struct {
	// Name identifies the action in the configuration and in the wiki.
	Name string
	// Summary is what it does, in the words the wiki uses.
	Summary string
	// Keys are the keys bound to it out of the box.
	Keys []string
	// do performs it. repeat is the count typed before the key, and counted
	// says whether one was typed at all.
	do func(m Model, repeat int, counted bool) (tea.Model, tea.Cmd)
}

// Actions is every named action, in the order the wiki lists them.
//
// A function rather than a variable, so nothing can change the table it is
// dispatching through.
func Actions() []Action {
	return []Action{
		{Name: "move-up", Summary: "move the selection up one row", Keys: []string{"up", "k"},
			do: func(m Model, repeat int, _ bool) (tea.Model, tea.Cmd) { return m.move(-repeat), nil }},
		{Name: "move-down", Summary: "move the selection down one row", Keys: []string{"down", "j"},
			do: func(m Model, repeat int, _ bool) (tea.Model, tea.Cmd) { return m.move(repeat), nil }},
		{Name: "move-top", Summary: "jump to the first row", Keys: []string{"home"},
			do: func(m Model, _ int, _ bool) (tea.Model, tea.Cmd) { return m.moveTo(0).ontoARow(), nil }},
		{Name: "move-bottom", Summary: "jump to the last row", Keys: []string{"end", "G"},
			do: func(m Model, _ int, _ bool) (tea.Model, tea.Cmd) { return m.moveTo(m.rows() - 1), nil }},
		{Name: "move-page-up", Summary: "move up by one screenful", Keys: []string{"pgup"},
			do: func(m Model, repeat int, _ bool) (tea.Model, tea.Cmd) { return m.move(-m.page() * repeat), nil }},
		{Name: "move-page-down", Summary: "move down by one screenful", Keys: []string{"pgdown"},
			do: func(m Model, repeat int, _ bool) (tea.Model, tea.Cmd) { return m.move(m.page() * repeat), nil }},

		{Name: "open", Summary: "open the selected artist or album, or play the selected track",
			Keys: []string{"enter", "l", "right"}, do: modelCmd((Model).open)},
		{Name: "back", Summary: "go back to the previous screen",
			Keys: []string{"esc", "backspace", "h", "left"}, do: modelCmd((Model).back)},
		{Name: "filter", Summary: "narrow the list you are looking at", Keys: []string{"/"},
			do: func(m Model, _ int, _ bool) (tea.Model, tea.Cmd) {
				m.mode, m.filter = modeFilter, ""
				return m.moveTo(0), nil
			}},
		{Name: "command", Summary: "type a command", Keys: []string{":"},
			do: func(m Model, _ int, _ bool) (tea.Model, tea.Cmd) {
				m.mode, m.line, m.status = modeCommand, "", ""
				return m, nil
			}},
		{Name: "go-prefix", Summary: "begin a two-key jump, as in gg or gq", Keys: []string{"g"},
			do: func(m Model, _ int, _ bool) (tea.Model, tea.Cmd) {
				m.pending = "g"
				return m, nil
			}},

		{Name: "queue-append", Summary: "add the selected track to the end of the queue, or add it to the playlist being edited",
			Keys: []string{"a"}, do: modelCmd(func(m Model) (tea.Model, tea.Cmd) {
				if m.editing.ID != "" {
					return m.editSelected(true)
				}
				return m.queueSelected(false)
			})},
		{Name: "queue-next", Summary: "play the selected track after the one playing", Keys: []string{"A"},
			do: modelCmd(func(m Model) (tea.Model, tea.Cmd) { return m.queueSelected(true) })},
		{Name: "playlist-remove", Summary: "take the selected track out of the playlist being edited",
			Keys: []string{"r"}, do: modelCmd(func(m Model) (tea.Model, tea.Cmd) {
				if m.editing.ID != "" {
					return m.editSelected(false)
				}
				return m, nil
			})},
		{Name: "star", Summary: "star the selected artist, album or track, or unstar it",
			Keys: []string{"*"}, do: modelCmd((Model).starSelected)},

		{Name: "pause", Summary: "pause, or resume if already paused", Keys: []string{" "},
			do: sends(func(int) tea.Msg { return TogglePause{} })},
		{Name: "next", Summary: "skip to the next track", Keys: []string{"n"},
			do: sends(func(int) tea.Msg { return SkipNext{} })},
		{Name: "previous", Summary: "go back to the previous track", Keys: []string{"p"},
			do: sends(func(int) tea.Msg { return SkipPrev{} })},
		{Name: "seek-forward", Summary: "seek forward ten seconds", Keys: []string{"]"},
			do: sends(func(int) tea.Msg { return SeekBy{By: 10 * time.Second} })},
		{Name: "seek-back", Summary: "seek backward ten seconds", Keys: []string{"["},
			do: sends(func(int) tea.Msg { return SeekBy{By: -10 * time.Second} })},
		{Name: "volume-up", Summary: "increase the volume by five percent", Keys: []string{"+", "="},
			do: sends(func(int) tea.Msg { return VolumeBy{Delta: 5} })},
		{Name: "volume-down", Summary: "decrease the volume by five percent", Keys: []string{"-", "_"},
			do: sends(func(int) tea.Msg { return VolumeBy{Delta: -5} })},
		{Name: "seek-percent", Summary: "seek to a percentage of the track, as in 50%", Keys: []string{"%"},
			do: func(m Model, repeat int, counted bool) (tea.Model, tea.Cmd) {
				if !counted {
					m.status = "type a percentage first, as in 50%"
					return m, nil
				}
				return m, emit(SeekToPercent(min(100, repeat)))
			}},

		{Name: "quit", Summary: "quit shanty", Keys: []string{"ctrl+c"},
			do: sends(func(int) tea.Msg { return Quit{} })},
		{Name: "quit-hint", Summary: "say how to quit", Keys: []string{"q"},
			do: func(m Model, _ int, _ bool) (tea.Model, tea.Cmd) {
				// Quitting ends the session and is a command rather than a key
				// (ADR-016). Saying so beats doing nothing for anyone who
				// learned q somewhere else.
				m.status = "type :q to quit"
				return m, nil
			}},
	}
}

// modelCmd adapts an action that needs neither the count nor whether one was
// typed.
func modelCmd(do func(Model) (tea.Model, tea.Cmd)) func(Model, int, bool) (tea.Model, tea.Cmd) {
	return func(m Model, _ int, _ bool) (tea.Model, tea.Cmd) { return do(m) }
}

// sends adapts an action that only asks the caller for something.
func sends(what func(int) tea.Msg) func(Model, int, bool) (tea.Model, tea.Cmd) {
	return func(m Model, repeat int, _ bool) (tea.Model, tea.Cmd) {
		return m, emit(what(repeat))
	}
}

// binding maps every key to the action it performs.
func binding(actions []Action) map[string]Action {
	out := make(map[string]Action, len(actions)*2)
	for _, a := range actions {
		for _, key := range a.Keys {
			out[key] = a
		}
	}
	return out
}
