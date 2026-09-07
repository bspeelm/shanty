package fake

import (
	"sort"
	"strings"
)

// The wire types are declared here rather than imported from the client, and
// that duplication is the point. A fake that marshals the client's own structs
// agrees with the client about every field name by construction, including the
// wrong ones, and so can never catch a decoding mistake. These are a second,
// independent statement of the same wire format; when the two disagree, a test
// fails, which is the only reason to have a fake at all.

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
	Songs     []song `json:"song,omitempty"`
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
	// Path is server-supplied and reaches the client as a candidate filename,
	// which is why the traversal malice mode rewrites exactly this field.
	Path string `json:"path"`
}

// Library is what the fake serves. A test that needs particular shapes builds
// its own; most take DefaultLibrary.
type Library struct {
	Artists []artist
}

// DefaultLibrary is two artists across three albums, deliberately small enough
// that a golden test can hold the whole thing in view, and deliberately
// containing an artist whose name sorts under a different index letter than
// its first character would suggest.
func DefaultLibrary() Library {
	return Library{Artists: []artist{
		{
			ID: "ar-1", Name: "Aoi", AlbumCount: 2,
			Albums: []album{
				{
					ID: "al-1", Name: "Harbour", Artist: "Aoi", ArtistID: "ar-1",
					SongCount: 2, Duration: 360,
					Songs: []song{
						{ID: "tr-1", Title: "Slipway", Album: "Harbour", AlbumID: "al-1", Artist: "Aoi", ArtistID: "ar-1", Track: 1, Duration: 180, Suffix: "flac", Path: "Aoi/Harbour/01 Slipway.flac"},
						{ID: "tr-2", Title: "Ballast", Album: "Harbour", AlbumID: "al-1", Artist: "Aoi", ArtistID: "ar-1", Track: 2, Duration: 180, Suffix: "flac", Path: "Aoi/Harbour/02 Ballast.flac"},
					},
				},
				{
					ID: "al-2", Name: "Low Water", Artist: "Aoi", ArtistID: "ar-1",
					SongCount: 1, Duration: 200,
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
					SongCount: 1, Duration: 240,
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

// indexed groups artists the way getArtists does: by the first letter of the
// name with articles ignored, which is why "The Bilge Pumps" files under B.
func (l Library) indexed() artistList {
	byLetter := map[string][]artist{}
	var order []string
	for _, a := range l.Artists {
		letter := indexLetter(a.Name)
		if _, seen := byLetter[letter]; !seen {
			order = append(order, letter)
		}
		// The index carries the artist without its albums; getArtist is what
		// expands one.
		byLetter[letter] = append(byLetter[letter], artist{ID: a.ID, Name: a.Name, AlbumCount: a.AlbumCount})
	}
	sort.Strings(order)
	out := artistList{IgnoredArticles: ignoredArticles}
	for _, letter := range order {
		out.Index = append(out.Index, index{Name: letter, Artists: byLetter[letter]})
	}
	return out
}

// ignoredArticles is what Navidrome reports by default. The client is expected
// to take the server's word for it rather than carry its own list.
const ignoredArticles = "The El La Los Las Le Les"

// indexLetter files a name the way a Subsonic server does: leading article
// stripped, first rune upper-cased, anything not a letter under "#".
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
