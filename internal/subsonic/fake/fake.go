// Package fake is an in-process Subsonic server for tests.
//
// It fails the calling test when a request omits the client name or
// User-Agent, carries the legacy plain-password parameter, or authenticates
// incorrectly. Malice selects ways for it to answer badly.
package fake

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// APIVersion is the Subsonic API version the fake reports.
const APIVersion = "1.16.1"

// TB is the part of *testing.T the fake uses.
type TB interface {
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Helper()
	Cleanup(func())
}

// Malice selects the ways this server misbehaves. Each field is off by
// default.
type Malice struct {
	// TruncateJSON returns half a response body.
	TruncateJSON bool
	// OversizeBody returns a response larger than any client would read.
	OversizeBody bool
	// Traversal returns paths and titles that escape a directory if joined onto
	// one.
	Traversal bool
	// WrongContentType answers with text/html.
	WrongContentType bool
	// RejectAuth rejects every credential.
	RejectAuth bool
	// Stall never answers, leaving the client’s timeout to end the request.
	Stall bool
	// HostileText prefixes every displayable string with terminal escape
	// sequences.
	HostileText bool
	// RefuseScrobbles rejects every play report and answers everything else,
	// which is a server that is up and unhappy rather than one that is down.
	RefuseScrobbles bool
	// RefuseScrobblesAfter takes that many play reports and then rejects the
	// rest, which is a server that goes away part way through catching up.
	RefuseScrobblesAfter int
	// NoScanning answers the scan endpoints the way a server that does not
	// have them does, which is not the same as one that refused.
	NoScanning bool
	// ScanSteps is how many times a scan reports itself running before it
	// finishes. One means it is over by the first check.
	ScanSteps int
	// ScanAlreadyRunning answers startScan as a server does when one is
	// already under way.
	ScanAlreadyRunning bool
}

// oversizeBytes is what OversizeBody pads with.
const oversizeBytes = 24 << 20

// Hostile is the escape sequence HostileText prefixes: a screen clear, a
// cursor move, a scroll-region change and a device-status query.
const Hostile = "\x1b[2J\x1b[H\x1b[1;1r\x1b[6n\x07\r\n\x9b31m\u202e"

// Options configures a fake. With no credential set it accepts nothing.
type Options struct {
	User     string
	Password string
	APIKey   string // accepted as an API key
	// OpenSubsonic controls whether the server reports supporting API keys.
	OpenSubsonic bool
	Library      Library
	Malice       Malice
}

// Server is a running fake. It is shut down when the test ends.
type Server struct {
	*httptest.Server
	t    TB
	opt  Options
	mu   sync.Mutex
	reqs []Request
	// starred is what star and unstar have done, so that getStarred2 reports
	// what a test actually did rather than a fixture.
	starred map[string]bool
	// scrobbles counts the play reports taken, for the server that stops
	// taking them part way through.
	scrobbles int
	// queue is what savePlayQueue last stored, which getPlayQueue returns.
	queue *playQueue
	// playlists is what the playlist endpoints have made and changed, so a
	// test reads back what it actually did.
	playlists []playlist
	nextID    int
	// scanning counts down: each getScanStatus reports one step of a scan and
	// the last one reports it finished, so a test sees progress without
	// waiting for anything.
	scanning int
	scanned  int64
}

// SetPlayQueue puts a queue on the server as another client would have left
// it, which is what a test needs to offer one that this machine did not save.
func (s *Server) SetPlayQueue(ids []string, current string, position int64, by string) {
	saved := playQueue{Current: current, Position: position, ChangedBy: by}
	for _, id := range ids {
		if sg, found := s.opt.Library.findSong(id); found {
			saved.Songs = append(saved.Songs, sg)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = &saved
}

// Starred reports whether the server has the id starred. A test asserts on
// this rather than on the request log, because starring twice and unstarring
// once is a different state from starring once.
func (s *Server) Starred(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starred[id]
}

// Request is one call as the server received it. Credentials are not
// recorded.
type Request struct {
	Endpoint string
	Query    url.Values
}

// New starts a fake server. Options.Library defaults to DefaultLibrary.
func New(t TB, opt Options) *Server {
	t.Helper()
	if len(opt.Library.Artists) == 0 {
		opt.Library = DefaultLibrary()
	}
	s := &Server{t: t, opt: opt}
	s.Server = httptest.NewServer(http.HandlerFunc(s.route))
	t.Cleanup(s.Server.Close)
	return s
}

// Requests returns the calls received, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.reqs...)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	endpoint := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/"), ".view")

	s.mu.Lock()
	s.reqs = append(s.reqs, Request{Endpoint: endpoint, Query: q})
	s.mu.Unlock()

	if !s.hygienic(r, q) {
		s.fail(w, 40, "Wrong username or password")
		return
	}
	if s.opt.Malice.Stall {
		// Held open until the client gives up.
		<-r.Context().Done()
		return
	}
	if !s.authenticated(q) {
		s.fail(w, 40, "Wrong username or password")
		return
	}

	switch endpoint {
	case "ping":
		s.ok(w, response{})
	case "getOpenSubsonicExtensions":
		var ext []extension
		if s.opt.OpenSubsonic {
			ext = []extension{{Name: "apiKeyAuthentication", Versions: []int{1}}}
		}
		s.ok(w, response{Extensions: ext})
	case "getArtists":
		list := s.opt.Library.indexed()
		if s.opt.Malice.HostileText {
			for i := range list.Index {
				for j := range list.Index[i].Artists {
					list.Index[i].Artists[j].Name = Hostile + list.Index[i].Artists[j].Name
				}
			}
		}
		s.ok(w, response{Artists: &list})
	case "getArtist":
		a, found := s.opt.Library.findArtist(q.Get("id"))
		if !found {
			s.fail(w, 70, "Artist not found")
			return
		}
		if s.opt.Malice.HostileText {
			a.Name = Hostile + a.Name
			for i := range a.Albums {
				a.Albums[i].Name = Hostile + a.Albums[i].Name
			}
		}
		s.ok(w, response{Artist: &a})
	case "getAlbum":
		al, found := s.opt.Library.findAlbum(q.Get("id"))
		if !found {
			s.fail(w, 70, "Album not found")
			return
		}
		al.Songs = s.maybeTraverse(al.Songs)
		if s.opt.Malice.HostileText {
			al.Name = Hostile + al.Name
			al.Artist = Hostile + al.Artist
			for i := range al.Songs {
				al.Songs[i].Title = Hostile + al.Songs[i].Title
			}
		}
		s.ok(w, response{Album: &al})
	case "stream":
		sg, found := s.opt.Library.findSong(q.Get("id"))
		if !found {
			s.fail(w, 70, "Song not found")
			return
		}
		w.Header().Set("Content-Type", "audio/flac")
		// Not real audio. The integration test is where real audio is played.
		_, _ = w.Write([]byte(strings.Repeat(sg.ID+" ", 64)))
	case "getCoverArt":
		id := q.Get("id")
		if id == "" {
			s.fail(w, 10, "Required parameter id is missing")
			return
		}
		// A server answers a missing image with an error envelope rather than
		// a status, which is the case a client has to read the body to find.
		if id == NoArt {
			s.fail(w, 70, "Cover art not found")
			return
		}
		if s.opt.Malice.WrongContentType {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>a login page</body></html>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if s.opt.Malice.OversizeBody {
			_, _ = w.Write(append(onePixel, make([]byte, oversizeBytes)...))
			return
		}
		_, _ = w.Write(onePixel)
	case "search3":
		if q.Get("query") == "" {
			s.fail(w, 10, "Required parameter query is missing")
			return
		}
		limit := 20
		if n, err := strconv.Atoi(q.Get("songCount")); err == nil && n > 0 {
			limit = n
		}
		found := s.opt.Library.search(q.Get("query"), limit)
		if s.opt.Malice.HostileText {
			for i := range found.Artists {
				found.Artists[i].Name = Hostile + found.Artists[i].Name
			}
			for i := range found.Albums {
				found.Albums[i].Name = Hostile + found.Albums[i].Name
			}
			for i := range found.Songs {
				found.Songs[i].Title = Hostile + found.Songs[i].Title
			}
		}
		if found.Artists == nil && found.Albums == nil && found.Songs == nil {
			// A server that found nothing may leave the result out
			// altogether, and some do. The client has to read that as an
			// empty answer rather than a broken one.
			s.ok(w, response{})
			return
		}
		s.ok(w, response{SearchResult: &found})
	case "getPlaylists":
		s.mu.Lock()
		// The tracks are left out, as a server leaves them out of a listing.
		bare := make([]playlist, 0, len(s.playlists))
		for _, p := range s.playlists {
			p.Songs = nil
			bare = append(bare, p)
		}
		s.mu.Unlock()
		s.ok(w, response{Playlists: &playlists{Playlist: bare}})
	case "getPlaylist":
		p, found := s.playlistByID(q.Get("id"))
		if !found {
			s.fail(w, 70, "Playlist not found")
			return
		}
		s.ok(w, response{Playlist: &p})
	case "createPlaylist":
		name := q.Get("name")
		if name == "" {
			s.fail(w, 10, "Required parameter name is missing")
			return
		}
		s.mu.Lock()
		s.nextID++
		made := playlist{ID: fmt.Sprintf("pl-%d", s.nextID), Name: name, Owner: s.opt.User}
		s.playlists = append(s.playlists, made)
		s.mu.Unlock()
		s.ok(w, response{Playlist: &made})
	case "updatePlaylist":
		if code, why := s.changePlaylist(q); code != 0 {
			s.fail(w, code, why)
			return
		}
		s.ok(w, response{})
	case "deletePlaylist":
		s.mu.Lock()
		kept := s.playlists[:0]
		var gone bool
		for _, p := range s.playlists {
			if p.ID == q.Get("id") {
				gone = true
				continue
			}
			kept = append(kept, p)
		}
		s.playlists = kept
		s.mu.Unlock()
		if !gone {
			s.fail(w, 70, "Playlist not found")
			return
		}
		s.ok(w, response{})
	case "startScan", "getScanStatus":
		if s.opt.Malice.NoScanning {
			// The protocol has no code for an endpoint a server does not
			// have, so this is what they say instead.
			s.fail(w, 0, "Unknown endpoint "+endpoint)
			return
		}
		s.mu.Lock()
		if endpoint == "startScan" && !s.opt.Malice.ScanAlreadyRunning {
			s.scanning = max(1, s.opt.Malice.ScanSteps)
			s.scanned = 0
		}
		if endpoint == "getScanStatus" && s.scanning > 0 {
			s.scanning--
			s.scanned += 120
		}
		status := scanStatus{Scanning: s.scanning > 0, Count: s.scanned}
		s.mu.Unlock()
		s.ok(w, response{ScanStatus: &status})
	case "savePlayQueue":
		ids := q["id"]
		if len(ids) == 0 {
			s.fail(w, 10, "Required parameter id is missing")
			return
		}
		saved := playQueue{Current: q.Get("current"), ChangedBy: "shanty"}
		if n, err := strconv.ParseInt(q.Get("position"), 10, 64); err == nil {
			saved.Position = n
		}
		for _, id := range ids {
			sg, found := s.opt.Library.findSong(id)
			if !found {
				s.fail(w, 70, "Song not found")
				return
			}
			saved.Songs = append(saved.Songs, sg)
		}
		s.mu.Lock()
		s.queue = &saved
		s.mu.Unlock()
		s.ok(w, response{})
	case "getPlayQueue":
		s.mu.Lock()
		saved := s.queue
		s.mu.Unlock()
		if saved == nil {
			// A server holding no queue leaves it out altogether.
			s.ok(w, response{})
			return
		}
		s.ok(w, response{PlayQueue: saved})
	case "star", "unstar":
		id := firstOf(q, "id", "albumId", "artistId")
		if id == "" {
			s.fail(w, 10, "Required parameter id is missing")
			return
		}
		if !s.opt.Library.knows(id) {
			s.fail(w, 70, "Not found")
			return
		}
		s.mu.Lock()
		if s.starred == nil {
			s.starred = map[string]bool{}
		}
		s.starred[id] = endpoint == "star"
		s.mu.Unlock()
		s.ok(w, response{})
	case "getStarred2":
		s.mu.Lock()
		set := make(map[string]bool, len(s.starred))
		for k, v := range s.starred {
			set[k] = v
		}
		s.mu.Unlock()
		found := s.opt.Library.starredIn(set)
		if found.Artists == nil && found.Albums == nil && found.Songs == nil {
			s.ok(w, response{})
			return
		}
		s.ok(w, response{Starred: &found})
	case "scrobble":
		s.mu.Lock()
		s.scrobbles++
		taken := s.scrobbles
		s.mu.Unlock()
		refuse := s.opt.Malice.RefuseScrobbles ||
			(s.opt.Malice.RefuseScrobblesAfter > 0 && taken > s.opt.Malice.RefuseScrobblesAfter)
		if refuse {
			s.fail(w, 0, "Scrobbling is off")
			return
		}
		s.ok(w, response{})
	default:
		s.fail(w, 0, "Unknown endpoint "+endpoint)
	}
}

// hygienic checks the parameters every request must carry, and fails the
// calling test when one is missing or wrong.
func (s *Server) hygienic(r *http.Request, q url.Values) bool {
	ok := true
	if q.Has("p") {
		s.t.Errorf("fake: the legacy password parameter p= reached the server (§6 forbids it in every mode)")
		ok = false
	}
	if got := q.Get("c"); got != "shanty" {
		s.t.Errorf("fake: request carried client name %q, want %q (§9)", got, "shanty")
		ok = false
	}
	if q.Get("v") == "" {
		s.t.Errorf("fake: request carried no API version (§9)")
		ok = false
	}
	if r.Header.Get("User-Agent") == "" {
		s.t.Errorf("fake: request carried no User-Agent (§9)")
		ok = false
	}
	return ok
}

func (s *Server) authenticated(q url.Values) bool {
	if s.opt.Malice.RejectAuth {
		return false
	}
	if key := q.Get("apiKey"); key != "" {
		return s.opt.APIKey != "" && key == s.opt.APIKey
	}
	token, salt := q.Get("t"), q.Get("s")
	if token == "" || salt == "" || q.Get("u") != s.opt.User {
		return false
	}
	sum := md5.Sum([]byte(s.opt.Password + salt))
	return token == hex.EncodeToString(sum[:])
}

// maybeTraverse rewrites paths and titles to escape a directory, when the
// Traversal mode is set.
func (s *Server) maybeTraverse(in []song) []song {
	if !s.opt.Malice.Traversal {
		return in
	}
	out := make([]song, len(in))
	for i, sg := range in {
		sg.Path = "../../../../etc/" + sg.ID
		sg.Title = "../" + sg.Title
		out[i] = sg
	}
	return out
}

func (s *Server) ok(w http.ResponseWriter, body response) {
	body.Status = "ok"
	s.write(w, body)
}

func (s *Server) fail(w http.ResponseWriter, code int, msg string) {
	s.write(w, response{Status: "failed", Error: &wireError{Code: code, Message: msg}})
}

func (s *Server) write(w http.ResponseWriter, body response) {
	body.Version = APIVersion
	body.Type = "shanty-fake"
	body.ServerVersion = "0.0.0"
	body.OpenSubsonic = s.opt.OpenSubsonic

	raw, err := json.Marshal(envelope{Response: body})
	if err != nil {
		s.t.Fatalf("fake: encoding a response it built itself: %v", err)
	}
	if s.opt.Malice.OversizeBody {
		// Larger than any cap this project sets, because a padding smaller
		// than the client's limit tests the client's patience and nothing
		// else. Padded inside the body so a client that caps its reader fails
		// to decode rather than quietly succeeding on a prefix.
		raw = append(raw[:len(raw)-1], []byte(`,"pad":"`+strings.Repeat("A", oversizeBytes)+`"}`)...)
	}
	if s.opt.Malice.TruncateJSON {
		raw = raw[:len(raw)/2]
	}

	contentType := "application/json; charset=utf-8"
	if s.opt.Malice.WrongContentType {
		contentType = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(raw)
}

// firstOf returns the first of the named parameters that is present. star and
// unstar name what they act on differently depending on its kind.
func firstOf(q url.Values, names ...string) string {
	for _, name := range names {
		if v := q.Get(name); v != "" {
			return v
		}
	}
	return ""
}

// Playlists is what the server is holding, for a test to assert on.
func (s *Server) Playlists() []playlist {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]playlist(nil), s.playlists...)
}

// SetPlaylists puts playlists on the server as another client would have left
// them.
func (s *Server) SetPlaylists(names map[string][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, ids := range names {
		s.nextID++
		made := playlist{ID: fmt.Sprintf("pl-%d", s.nextID), Name: name, Owner: s.opt.User}
		for _, id := range ids {
			if sg, found := s.opt.Library.findSong(id); found {
				made.Songs = append(made.Songs, sg)
			}
		}
		made.SongCount = len(made.Songs)
		s.playlists = append(s.playlists, made)
	}
	// Left in the order they were made rather than sorted. A server has its
	// own order, and putting them in the one the screen wants would be the
	// fake doing the client's work.
	sort.Slice(s.playlists, func(i, j int) bool { return s.playlists[i].ID > s.playlists[j].ID })
}

// playlistByID returns one playlist with its tracks.
func (s *Server) playlistByID(id string) (playlist, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.playlists {
		if p.ID == id {
			p.SongCount = len(p.Songs)
			return p, true
		}
	}
	return playlist{}, false
}

// changePlaylist adds a track, removes one by position, or renames. It reports
// a protocol error code and message, or zero when the change was made.
func (s *Server) changePlaylist(q url.Values) (int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.playlists {
		if s.playlists[i].ID != q.Get("playlistId") {
			continue
		}
		if name := q.Get("name"); name != "" {
			s.playlists[i].Name = name
		}
		if add := q.Get("songIdToAdd"); add != "" {
			sg, found := s.opt.Library.findSong(add)
			if !found {
				return 70, "Song not found"
			}
			s.playlists[i].Songs = append(s.playlists[i].Songs, sg)
		}
		if at := q.Get("songIndexToRemove"); at != "" {
			n, err := strconv.Atoi(at)
			if err != nil || n < 0 || n >= len(s.playlists[i].Songs) {
				return 70, "No such track in the playlist"
			}
			s.playlists[i].Songs = append(s.playlists[i].Songs[:n], s.playlists[i].Songs[n+1:]...)
		}
		s.playlists[i].SongCount = len(s.playlists[i].Songs)
		return 0, ""
	}
	return 70, "Playlist not found"
}

// NoArt is the cover art id the server has no image for.
const NoArt = "art-missing"

// onePixel is a real one pixel PNG, written out rather than encoded so that
// there is no error to handle and no image package in the fake. A client
// checking whether the answer is an image gets the same answer it would from a
// server.
var onePixel = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
	0x10, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x62, 0xd2, 0x33, 0x71, 0x00,
	0x04, 0x00, 0x00, 0xff, 0xff, 0x01, 0x3e, 0x00, 0xa5, 0x52, 0xf9, 0x80,
	0x9e, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60,
	0x82,
}
