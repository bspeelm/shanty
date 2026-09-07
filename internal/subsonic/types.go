package subsonic

import (
	"context"
	"fmt"
	"net/url"
	"sort"
)

// The wire format, declared here and again in the fake: two independent
// statements are what make a decoding mistake fail rather than agree with
// itself.
type envelope struct {
	Response response `json:"subsonic-response"`
}

type response struct {
	Status       string      `json:"status"`
	Version      string      `json:"version"`
	Type         string      `json:"type"`
	OpenSubsonic bool        `json:"openSubsonic"`
	Error        *Error      `json:"error"`
	Artists      *artistList `json:"artists"`
	Artist       *Artist     `json:"artist"`
	Album        *Album      `json:"album"`
	Extensions   []Extension `json:"openSubsonicExtensions"`
}

type artistList struct {
	IgnoredArticles string `json:"ignoredArticles"`
	Index           []struct {
		Name    string   `json:"name"`
		Artists []Artist `json:"artist"`
	} `json:"index"`
}

// Artist, Album and Song are what the TUI renders, and carry no behaviour:
// giving it a type that could fetch something hands it the I/O rule to break.
type Artist struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	AlbumCount int     `json:"albumCount"`
	Albums     []Album `json:"album"`
}

type Album struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	ArtistID  string `json:"artistId"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"`
	Songs     []Song `json:"song"`
}

type Song struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Album    string `json:"album"`
	AlbumID  string `json:"albumId"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId"`
	Track    int    `json:"track"`
	Duration int    `json:"duration"`
	Suffix   string `json:"suffix"`
	// Path is the server's, and it is not a filename. Nothing in shanty joins
	// it onto a directory; when cover art arrives and something has to, it
	// goes through one sanitiser with its own tests (§2).
	Path string `json:"path"`
}

// Extension is one OpenSubsonic capability the server advertises.
type Extension struct {
	Name     string `json:"name"`
	Versions []int  `json:"versions"`
}

// Ping asks whether the server answers and the credential works.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.get(ctx, "ping", nil)
	return err
}

// Extensions lists what the server says it supports. An empty list is an
// answer: it means token and salt, not that the server is broken.
func (c *Client) Extensions(ctx context.Context) ([]Extension, error) {
	res, err := c.get(ctx, "getOpenSubsonicExtensions", nil)
	if err != nil {
		return nil, err
	}
	return res.Extensions, nil
}

// SupportsAPIKeys reports whether the server offers the credential ADR-001
// prefers, so doctor can name it when the user is on something weaker.
func (c *Client) SupportsAPIKeys(ctx context.Context) (bool, error) {
	exts, err := c.Extensions(ctx)
	if err != nil {
		return false, err
	}
	for _, e := range exts {
		if e.Name == "apiKeyAuthentication" {
			return true, nil
		}
	}
	return false, nil
}

// Artists flattens the server's index. The index letters are dropped rather
// than reproduced: shanty carries no article list, because a client that
// disagreed with its server about where "The Bilge Pumps" files would be wrong
// in a way nobody could fix.
func (c *Client) Artists(ctx context.Context) ([]Artist, error) {
	res, err := c.get(ctx, "getArtists", nil)
	if err != nil {
		return nil, err
	}
	if res.Artists == nil {
		return nil, fmt.Errorf("the server answered ok to getArtists with no artist list")
	}
	var out []Artist
	for _, idx := range res.Artists.Index {
		out = append(out, idx.Artists...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Artist returns one artist with its albums.
func (c *Client) Artist(ctx context.Context, id string) (Artist, error) {
	res, err := c.get(ctx, "getArtist", url.Values{"id": {id}})
	if err != nil {
		return Artist{}, err
	}
	if res.Artist == nil {
		return Artist{}, fmt.Errorf("the server answered ok to getArtist(%s) with no artist", id)
	}
	return *res.Artist, nil
}

// Album returns one album with its songs, in track order.
func (c *Client) Album(ctx context.Context, id string) (Album, error) {
	res, err := c.get(ctx, "getAlbum", url.Values{"id": {id}})
	if err != nil {
		return Album{}, err
	}
	if res.Album == nil {
		return Album{}, fmt.Errorf("the server answered ok to getAlbum(%s) with no album", id)
	}
	album := *res.Album
	sort.SliceStable(album.Songs, func(i, j int) bool { return album.Songs[i].Track < album.Songs[j].Track })
	return album, nil
}

// Scrobble tells the server what was played. submission false is a "now
// playing" notice; true is the play itself. Where it goes from there is the
// server's business and deliberately not ours (ADR-005).
func (c *Client) Scrobble(ctx context.Context, id string, submission bool) error {
	_, err := c.get(ctx, "scrobble", url.Values{
		"id":         {id},
		"submission": {fmt.Sprint(submission)},
	})
	return err
}
