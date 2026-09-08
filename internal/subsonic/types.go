package subsonic

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"
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
	Starred      *Results    `json:"starred2"`
	PlayQueue    *PlayQueue  `json:"playQueue"`
	ScanStatus   *Scan       `json:"scanStatus"`
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

// PlayQueue is a queue a server is holding, saved by this client or another
// one.
type PlayQueue struct {
	Songs []Song `json:"entry"`
	// Current is the identifier of the track that was playing.
	Current string `json:"current"`
	// Position is how far into that track playback had reached, in
	// milliseconds, which is the unit the server keeps it in.
	Position int64 `json:"position"`
	// ChangedBy names the client that saved it, so somebody can be told where
	// it came from.
	ChangedBy string `json:"changedBy"`
}

// Empty reports whether the server is holding no queue.
func (q PlayQueue) Empty() bool { return len(q.Songs) == 0 }

// At returns how far into the current track playback had reached.
func (q PlayQueue) At() time.Duration { return time.Duration(q.Position) * time.Millisecond }

// Index is where in the queue the current track is. It is zero when the server
// named a track the queue does not hold.
func (q PlayQueue) Index() int {
	for i, s := range q.Songs {
		if s.ID == q.Current {
			return i
		}
	}
	return 0
}

// SavePlayQueue tells the server what is queued and where in it playback has
// reached, so that another machine can carry on from the same place.
func (c *Client) SavePlayQueue(ctx context.Context, ids []string, current string, at time.Duration) error {
	if len(ids) == 0 {
		return nil
	}
	params := url.Values{"id": ids}
	if current != "" {
		params.Set("current", current)
		params.Set("position", strconv.FormatInt(at.Milliseconds(), 10))
	}
	_, err := c.get(ctx, "savePlayQueue", params)
	return err
}

// PlayQueue returns the queue the server is holding. A server holding none
// leaves it out of its answer, which is not an error.
func (c *Client) PlayQueue(ctx context.Context) (PlayQueue, error) {
	res, err := c.get(ctx, "getPlayQueue", nil)
	if err != nil {
		return PlayQueue{}, err
	}
	if res.PlayQueue == nil {
		return PlayQueue{}, nil
	}
	return *res.PlayQueue, nil
}

// Scan is how a server's scan of its own music folder is going.
type Scan struct {
	Scanning bool `json:"scanning"`
	// Count is how many things the server has looked at so far. It keeps
	// counting between scans, so it is only meaningful while one runs.
	Count int64 `json:"count"`
}

// StartScan asks the server to look at its music folder again.
func (c *Client) StartScan(ctx context.Context) (Scan, error) {
	return c.scan(ctx, "startScan")
}

// ScanStatus reports how a scan is going.
func (c *Client) ScanStatus(ctx context.Context) (Scan, error) {
	return c.scan(ctx, "getScanStatus")
}

func (c *Client) scan(ctx context.Context, endpoint string) (Scan, error) {
	res, err := c.get(ctx, endpoint, nil)
	if err != nil {
		return Scan{}, err
	}
	// A server that answered without saying anything about a scan is one that
	// is not scanning.
	if res.ScanStatus == nil {
		return Scan{}, nil
	}
	return *res.ScanStatus, nil
}

// Kind is what a starrable thing is, which decides the parameter the server
// wants it under.
type Kind string

const (
	KindArtist Kind = "artistId"
	KindAlbum  Kind = "albumId"
	KindSong   Kind = "id"
)

// Star marks something as starred on the server. Unstarring is the same call
// under a different name, so one function does both.
func (c *Client) Star(ctx context.Context, kind Kind, id string, starred bool) error {
	endpoint := "star"
	if !starred {
		endpoint = "unstar"
	}
	_, err := c.get(ctx, endpoint, url.Values{string(kind): {id}})
	return err
}

// Starred returns everything the server has starred.
func (c *Client) Starred(ctx context.Context) (Results, error) {
	res, err := c.get(ctx, "getStarred2", nil)
	if err != nil {
		return Results{}, err
	}
	// A server with nothing starred may leave the list out altogether.
	if res.Starred == nil {
		return Results{}, nil
	}
	return *res.Starred, nil
}

// Scrobble reports a play to the server. A submission of false is a "now
// playing" notification; true records the play.
func (c *Client) Scrobble(ctx context.Context, id string, submission bool, at time.Time) error {
	params := url.Values{
		"id":         {id},
		"submission": {fmt.Sprint(submission)},
	}
	// A play reported late says when it happened, so a history kept while the
	// server was unreachable is not all dated to the moment it caught up. A
	// server takes this in milliseconds.
	if !at.IsZero() {
		params.Set("time", strconv.FormatInt(at.UnixMilli(), 10))
	}
	_, err := c.get(ctx, "scrobble", params)
	return err
}
