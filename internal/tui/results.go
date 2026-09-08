package tui

import (
	"fmt"
	"time"

	"github.com/bspeelm/shanty/internal/subsonic"
)

// kind is what one row of a search result is.
type kind int

const (
	// kindHeading names the group that follows it. It is a row nothing can be
	// done with, and the cursor steps over it.
	kindHeading kind = iota
	kindArtist
	kindAlbum
	kindSong
)

// result is one row of a search result.
type result struct {
	kind kind
	name string
	// detail is what is shown on the right of the row.
	detail string
	// id is what opening the row asks for.
	id string
	// album is carried by a track, so that playing it has something to play
	// from.
	album subsonic.Album
	index int
}

// selectable reports whether the cursor may rest on this row.
func (r result) selectable() bool { return r.kind != kindHeading }

// flatten turns what a search found into rows, with a heading before each
// group. A group with nothing in it gets no heading.
func flatten(found subsonic.Results) []result {
	var out []result

	if len(found.Artists) > 0 {
		out = append(out, result{kind: kindHeading, name: "ARTISTS"})
		for _, a := range found.Artists {
			out = append(out, result{
				kind: kindArtist, name: a.Name, id: a.ID,
				detail: plural(a.AlbumCount, "album"),
			})
		}
	}
	if len(found.Albums) > 0 {
		out = append(out, result{kind: kindHeading, name: "ALBUMS"})
		for _, a := range found.Albums {
			out = append(out, result{
				kind: kindAlbum, name: a.Name, id: a.ID,
				detail: a.Artist,
			})
		}
	}
	if len(found.Songs) > 0 {
		out = append(out, result{kind: kindHeading, name: "TRACKS"})
		for _, s := range found.Songs {
			// A track found by searching is not on any album the interface has
			// loaded, so it carries one of its own to be played from.
			out = append(out, result{
				kind: kindSong, name: s.Title, id: s.ID,
				detail: s.Artist + " · " + clock(time.Duration(s.Duration)*time.Second),
				album:  subsonic.Album{ID: s.AlbumID, Name: s.Album, Artist: s.Artist, Songs: []subsonic.Song{s}},
			})
		}
	}
	return out
}

// firstSelectable is the row the cursor lands on when a search returns.
func firstSelectable(rows []result) int {
	for i, r := range rows {
		if r.selectable() {
			return i
		}
	}
	return 0
}

// stepOver returns the first selectable row from i, moving in the direction
// given. It is what makes a heading a row the cursor passes rather than rests
// on.
func stepOver(rows []result, i, dir int) int {
	if dir == 0 {
		dir = 1
	}
	for j := i; j >= 0 && j < len(rows); j += dir {
		if rows[j].selectable() {
			return j
		}
	}
	// Nothing selectable that way; look back the other way rather than
	// leaving the cursor on a heading.
	for j := i; j >= 0 && j < len(rows); j -= dir {
		if rows[j].selectable() {
			return j
		}
	}
	return i
}

// searchHeading is the title bar for a set of results.
func searchHeading(query string, rows []result) string {
	found := 0
	for _, r := range rows {
		if r.selectable() {
			found++
		}
	}
	if found == 0 {
		return fmt.Sprintf("search · nothing matches %q", Sanitise(query))
	}
	return fmt.Sprintf("search · %d for %q", found, Sanitise(query))
}
