package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/subsonic"
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
		{Name: "playlist-edit", Summary: "add to the playlist you are looking at, or leave the one you are adding to",
			Keys: []string{"e"}, do: modelCmd(func(m Model) (tea.Model, tea.Cmd) {
				if m.editing.ID != "" {
					name := m.editing.Name
					m.editing, m.inPlaylist = subsonic.Playlist{}, nil
					m.status = "finished with " + Sanitise(name)
					return m.remember(m.status), nil
				}
				// Looking at a playlist is knowing which one to add to, so the
				// key that leaves is the key that starts. `:playlist edit` is
				// for the one you are not looking at.
				if m.screen == ScreenPlaylist && m.playlist.ID != "" {
					return m.editPlaylist(m.playlist)
				}
				return m, nil
			})},
		{Name: "playlist-remove", Summary: "take the selected track out of the playlist you are looking at, or the one you are adding to",
			Keys: []string{"r"}, do: modelCmd(func(m Model) (tea.Model, tea.Cmd) {
				if m.editing.ID != "" {
					return m.editSelected(false)
				}
				// A playlist on screen is a list of the tracks in it, so a
				// track can be taken out of it without editing it first.
				if m.screen == ScreenPlaylist {
					return m.removeFromPlaylist()
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

// Bind returns the key to action map, with the given actions bound to the
// given keys instead of the ones they come with.
//
// Everything wrong with what was asked for is reported rather than the first
// thing: somebody fixing a configuration wants the whole list.
func Bind(want map[string][]string) (map[string]Action, []string) {
	actions := Actions()
	known := make(map[string]Action, len(actions))
	for _, a := range actions {
		known[a.Name] = a
	}

	var wrong []string
	for _, name := range sorted(want) {
		if _, ok := known[name]; !ok {
			wrong = append(wrong, fmt.Sprintf("%q is not something shanty does; %s", name, nearest(name, actions)))
			continue
		}
		if len(want[name]) == 0 {
			wrong = append(wrong, fmt.Sprintf("%q is bound to no keys; remove the line to keep the usual ones", name))
			continue
		}
		bound := known[name]
		bound.Keys = want[name]
		known[name] = bound
	}

	out := make(map[string]Action, len(known)*2)
	for _, name := range sortedActions(known) {
		for _, key := range known[name].Keys {
			if taken, already := out[key]; already {
				wrong = append(wrong, fmt.Sprintf("%q is bound to both %s and %s", key, taken.Name, name))
				continue
			}
			out[key] = known[name]
		}
	}
	return out, wrong
}

// nearest names an action close to what was asked for, so a typo is answered
// with the word that was meant.
func nearest(name string, actions []Action) string {
	for _, a := range actions {
		if strings.HasPrefix(a.Name, name) || strings.HasPrefix(name, a.Name) {
			return "did you mean " + a.Name + "?"
		}
	}
	return "run `shanty doctor` for the list"
}

// Names is every action, for reporting what may be bound.
func Names() []string {
	out := make([]string, 0, len(Actions()))
	for _, a := range Actions() {
		out = append(out, a.Name)
	}
	return out
}

func sorted(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedActions(m map[string]Action) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WithKeys returns a model using the given bindings.
func (m Model) WithKeys(keys map[string]Action) Model {
	m.keys = keys
	return m
}
