package fake

import (
	"sort"
	"strings"
)

// The response types. They are declared here and again in the client, so that
// the two are independent.

// envelope is the "subsonic-response" wrapper every endpoint returns.
type envelope struct {
	Response response `json:"subsonic-response"`
}

type response struct {
	Status        string      `json:"status"`
	Version       string      `json:"version"`
	Type          string      `json:"type,omitempty"`
	ServerVersion string      `json:"serverVersion,omitempty"`
	OpenSubsonic  bool        `json:"openSubsonic,omitempty"`
	Error         *wireError  `json:"error,omitempty"`
	Artists       *artistList `json:"artists,omitempty"`
	Artist        *artist     `json:"artist,omitempty"`
	Album         *album      `json:"album,omitempty"`
	Extensions    []extension `json:"openSubsonicExtensions,omitempty"`
	SearchResult  *results    `json:"searchResult3,omitempty"`
	Starred       *results    `json:"starred2,omitempty"`
	PlayQueue     *playQueue  `json:"playQueue,omitempty"`
	ScanStatus    *scanStatus `json:"scanStatus,omitempty"`
	Playlists     *playlists  `json:"playlists,omitempty"`
	Playlist      *playlist   `json:"playlist,omitempty"`
}

type playlists struct {
	Playlist []playlist `json:"playlist"`
}

// playlist is a list somebody made. Its tracks are sent only when it is asked
// for by itself.
type playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"`
	Songs     []song `json:"entry,omitempty"`
}

// scanStatus is how a scan of the server's own music folder is going.
type scanStatus struct {
	Scanning bool  `json:"scanning"`
	Count    int64 `json:"count"`
}

// playQueue is what savePlayQueue stored and getPlayQueue returns.
type playQueue struct {
	Songs     []song `json:"entry,omitempty"`
	Current   string `json:"current,omitempty"`
	Position  int64  `json:"position,omitempty"`
	ChangedBy string `json:"changedBy,omitempty"`
}

// results is what search3 returns.
type results struct {
	Artists []artist `json:"artist,omitempty"`
	Albums  []album  `json:"album,omitempty"`
	Songs   []song   `json:"song,omitempty"`
}

type wireError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type extension struct {
	Name     string `json:"name"`
	Versions []int  `json:"versions"`
}

type artistList struct {
	IgnoredArticles string  `json:"ignoredArticles"`
	Index           []index `json:"index"`
}

type index struct {
	Name    string   `json:"name"`
	Artists []artist `json:"artist"`
}

type artist struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	AlbumCount int     `json:"albumCount"`
	Albums     []album `json:"album,omitempty"`
}

type album struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	ArtistID  string `json:"artistId"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"`
	Year      int    `json:"year,omitempty"`
	Songs     []song `json:"song,omitempty"`
	// CoverArt is what getCoverArt is asked for. A real server's is opaque,
	// so this one is too rather than being the album id again.
	CoverArt string `json:"coverArt,omitempty"`
}

type song struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Album    string `json:"album"`
	AlbumID  string `json:"albumId"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId"`
	Track    int    `json:"track"`
	Duration int    `json:"duration"`
	Suffix   string `json:"suffix"`
	// Path is the server’s path for the track.
	Path string `json:"path"`
}

// Library is what the fake serves. A test that needs particular shapes builds
// its own; most take DefaultLibrary.
type Library struct {
	Artists []artist
}

// DefaultLibrary is two artists across three albums.
func DefaultLibrary() Library {
	return Library{Artists: []artist{
		{
			ID: "ar-1", Name: "Aoi", AlbumCount: 2,
			Albums: []album{
				{
					ID: "al-1", Name: "Harbour", Artist: "Aoi", ArtistID: "ar-1",
					SongCount: 2, Duration: 360, Year: 1973, CoverArt: "mf-al-1",
					Songs: []song{
						{ID: "tr-1", Title: "Slipway", Album: "Harbour", AlbumID: "al-1", Artist: "Aoi", ArtistID: "ar-1", Track: 1, Duration: 180, Suffix: "flac", Path: "Aoi/Harbour/01 Slipway.flac"},
						{ID: "tr-2", Title: "Ballast", Album: "Harbour", AlbumID: "al-1", Artist: "Aoi", ArtistID: "ar-1", Track: 2, Duration: 180, Suffix: "flac", Path: "Aoi/Harbour/02 Ballast.flac"},
					},
				},
				{
					ID: "al-2", Name: "Low Water", Artist: "Aoi", ArtistID: "ar-1",
					SongCount: 1, Duration: 200, Year: 1975, CoverArt: "mf-al-2",
					Songs: []song{
						{ID: "tr-3", Title: "Neap", Album: "Low Water", AlbumID: "al-2", Artist: "Aoi", ArtistID: "ar-1", Track: 1, Duration: 200, Suffix: "flac", Path: "Aoi/Low Water/01 Neap.flac"},
					},
				},
			},
		},
		{
			ID: "ar-2", Name: "The Bilge Pumps", AlbumCount: 1,
			Albums: []album{
				{
					ID: "al-3", Name: "Bail", Artist: "The Bilge Pumps", ArtistID: "ar-2",
					SongCount: 1, Duration: 240, CoverArt: "mf-al-3",
					Songs: []song{
						{ID: "tr-4", Title: "Scupper", Album: "Bail", AlbumID: "al-3", Artist: "The Bilge Pumps", ArtistID: "ar-2", Track: 1, Duration: 240, Suffix: "flac", Path: "The Bilge Pumps/Bail/01 Scupper.flac"},
					},
				},
			},
		},
	}}
}

func (l Library) findArtist(id string) (artist, bool) {
	for _, a := range l.Artists {
		if a.ID == id {
			return a, true
		}
	}
	return artist{}, false
}

func (l Library) findAlbum(id string) (album, bool) {
	for _, a := range l.Artists {
		for _, al := range a.Albums {
			if al.ID == id {
				return al, true
			}
		}
	}
	return album{}, false
}

func (l Library) findSong(id string) (song, bool) {
	for _, a := range l.Artists {
		for _, al := range a.Albums {
			for _, s := range al.Songs {
				if s.ID == id {
					return s, true
				}
			}
		}
	}
	return song{}, false
}

// indexed groups the artists by index letter, the way getArtists does.
func (l Library) indexed() artistList {
	byLetter := map[string][]artist{}
	var order []string
	for _, a := range l.Artists {
		letter := indexLetter(a.Name)
		if _, seen := byLetter[letter]; !seen {
			order = append(order, letter)
		}
		// The index omits albums; getArtist returns them.
		byLetter[letter] = append(byLetter[letter], artist{ID: a.ID, Name: a.Name, AlbumCount: a.AlbumCount})
	}
	sort.Strings(order)
	out := artistList{IgnoredArticles: ignoredArticles}
	for _, letter := range order {
		out.Index = append(out.Index, index{Name: letter, Artists: byLetter[letter]})
	}
	return out
}

// ignoredArticles is the list of leading words the index skips.
const ignoredArticles = "The El La Los Las Le Les"

// indexLetter returns the index letter for a name: leading article removed,
// first character upper-cased, anything not a letter under "#".
func indexLetter(name string) string {
	for _, article := range strings.Fields(ignoredArticles) {
		if prefix := article + " "; strings.HasPrefix(name, prefix) {
			name = name[len(prefix):]
			break
		}
	}
	if name == "" {
		return "#"
	}
	first := strings.ToUpper(name[:1])
	if first < "A" || first > "Z" {
		return "#"
	}
	return first
}

// search matches artists, albums and songs whose names contain the query,
// ignoring case. A real server does more than this; what matters here is that
// the three kinds come back together, that each carries what the interface
// needs to open it, and that a query matching nothing is answered rather than
// refused.
//
// Nested children are left out, as a real server leaves them out: a search
// result names a thing, and opening it is a second request.
func (l Library) search(query string, limit int) results {
	q := strings.ToLower(strings.TrimSpace(query))
	var out results
	if q == "" {
		return out
	}
	matches := func(name string) bool { return strings.Contains(strings.ToLower(name), q) }

	for _, a := range l.Artists {
		if matches(a.Name) && len(out.Artists) < limit {
			out.Artists = append(out.Artists, artist{ID: a.ID, Name: a.Name, AlbumCount: a.AlbumCount})
		}
		for _, al := range a.Albums {
			if matches(al.Name) && len(out.Albums) < limit {
				bare := al
				bare.Songs = nil
				out.Albums = append(out.Albums, bare)
			}
			for _, sg := range al.Songs {
				if matches(sg.Title) && len(out.Songs) < limit {
					out.Songs = append(out.Songs, sg)
				}
			}
		}
	}
	return out
}

// starredIn returns everything in the library whose id is in the set, in the
// three kinds a server reports them.
func (l Library) starredIn(set map[string]bool) results {
	var out results
	for _, a := range l.Artists {
		if set[a.ID] {
			out.Artists = append(out.Artists, artist{ID: a.ID, Name: a.Name, AlbumCount: a.AlbumCount})
		}
		for _, al := range a.Albums {
			if set[al.ID] {
				bare := al
				bare.Songs = nil
				out.Albums = append(out.Albums, bare)
			}
			for _, sg := range al.Songs {
				if set[sg.ID] {
					out.Songs = append(out.Songs, sg)
				}
			}
		}
	}
	return out
}

// knows reports whether the id belongs to anything in the library, which is
// what lets the fake refuse a star for something that does not exist.
func (l Library) knows(id string) bool {
	for _, a := range l.Artists {
		if a.ID == id {
			return true
		}
		for _, al := range a.Albums {
			if al.ID == id {
				return true
			}
			for _, sg := range al.Songs {
				if sg.ID == id {
					return true
				}
			}
		}
	}
	return false
}
