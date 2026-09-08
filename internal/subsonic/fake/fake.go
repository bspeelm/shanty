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
	"net/http"
	"net/http/httptest"
	"net/url"
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
