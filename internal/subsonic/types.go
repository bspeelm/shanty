package subsonic

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
)

// The response format. It is declared here and again in the fake server, so
// that the two are independent.
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
	SearchResult *Results    `json:"searchResult3"`
}

type artistList struct {
	IgnoredArticles string `json:"ignoredArticles"`
	Index           []struct {
		Name    string   `json:"name"`
		Artists []Artist `json:"artist"`
	} `json:"index"`
}

// Artist, Album and Song are the types the interface displays. They carry no
// methods.
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
	// Path is the server’s path for the track. It is not used as a filename.
	Path string `json:"path"`
}

// Results is what a search found, in the three kinds a Subsonic server
// returns them.
type Results struct {
	Artists []Artist `json:"artist"`
	Albums  []Album  `json:"album"`
	Songs   []Song   `json:"song"`
}

// Empty reports whether the search found nothing at all.
func (r Results) Empty() bool {
	return len(r.Artists) == 0 && len(r.Albums) == 0 && len(r.Songs) == 0
}

// Count is how many things were found, of all three kinds.
func (r Results) Count() int { return len(r.Artists) + len(r.Albums) + len(r.Songs) }

// Extension is one OpenSubsonic capability the server reports supporting.
type Extension struct {
	Name     string `json:"name"`
	Versions []int  `json:"versions"`
}

// Ping reports whether the server responds and accepts the credential.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.get(ctx, "ping", nil)
	return err
}

// Extensions lists the OpenSubsonic capabilities the server reports. An empty
// list is a valid answer and means the server supports none of them, not that
// the request failed.
func (c *Client) Extensions(ctx context.Context) ([]Extension, error) {
	res, err := c.get(ctx, "getOpenSubsonicExtensions", nil)
	if err != nil {
		return nil, err
	}
	return res.Extensions, nil
}

// SupportsAPIKeys reports whether the server accepts API keys, so that doctor
// can suggest one when the user is authenticating with something weaker.
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

// Artists returns every artist, flattened out of the server’s alphabetical
// index and sorted by name.
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

// Artist returns one artist together with their albums.
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

// Album returns one album with its songs, ordered by track number.
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

// searchLimit is how many of each kind a search asks for. A terminal list
// nobody will scroll to the end of is not worth fetching.
const searchLimit = 20

// Search finds artists, albums and tracks whose names match the query. The
// server decides what matching means.
func (c *Client) Search(ctx context.Context, query string) (Results, error) {
	res, err := c.get(ctx, "search3", url.Values{
		"query":       {query},
		"artistCount": {strconv.Itoa(searchLimit)},
		"albumCount":  {strconv.Itoa(searchLimit)},
		"songCount":   {strconv.Itoa(searchLimit)},
	})
	if err != nil {
		return Results{}, err
	}
	// A server that found nothing may leave the result out altogether, which
	// is not an error.
	if res.SearchResult == nil {
		return Results{}, nil
	}
	return *res.SearchResult, nil
}

// Scrobble reports a play to the server. A submission of false is a "now
// playing" notification; true records the play.
func (c *Client) Scrobble(ctx context.Context, id string, submission bool) error {
	_, err := c.get(ctx, "scrobble", url.Values{
		"id":         {id},
		"submission": {fmt.Sprint(submission)},
	})
	return err
}
