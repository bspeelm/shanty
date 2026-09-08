package tui

import (
	"fmt"
	"strings"
)

// playlistVerbs is what can follow :playlist, with what each needs after it.
// The set is closed: anything else is answered with the list.
var playlistVerbs = map[string]bool{
	"create":  true,
	"edit":    true,
	"delete":  true,
	"shuffle": true,
}

// playlistCommand reads ":playlist", ":playlist create <name>" and the rest.
//
// The name is everything after the verb, spaces and all, because a playlist is
// called what somebody called it.
func playlistCommand(arg string) (any, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return ShowPlaylists{}, nil
	}
	verb, name, _ := strings.Cut(arg, " ")
	name = strings.TrimSpace(name)

	if !playlistVerbs[verb] {
		return nil, errBadArgument{"playlist", verb, "one of " + verbList()}
	}
	if name == "" {
		return nil, errBadArgument{"playlist " + verb, "", "the name of a playlist"}
	}
	return Playlist{Verb: verb, Name: name}, nil
}

// verbList names what may follow :playlist, in a fixed order.
func verbList() string {
	return "create, delete, edit or shuffle"
}

// playlistSummary is what the completion row says :playlist does.
func playlistSummary() string {
	return fmt.Sprintf("list them, or %s one", verbList())
}
