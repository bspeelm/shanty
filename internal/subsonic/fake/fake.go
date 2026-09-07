// Package fake is an in-process Subsonic server for tests. Every layer runs
// against it, and nothing in `make check` touches the network.
//
// Two things make it worth more than a stub. It fails the test that sends a
// request breaking the client's own promises -- p=, a missing client name or
// User-Agent -- wherever in the tree that test lives, so §6 is enforced by
// every test without being asked. And it lies on request: the malice modes
// make truncation, stalling and path traversal one-line tests.
package fake

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
)

// APIVersion is what the fake claims to speak, matching ADR-007.
const APIVersion = "1.16.1"

// TB is an interface rather than *testing.T for one reason: the fake's claim
// is that it fails the calling test when a promise breaks, and that is only
// worth making if its own tests can substitute a recorder and check.
type TB interface {
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Helper()
	Cleanup(func())
}

// Malice selects the ways this server misbehaves. Every field is off by
// default, so a test opts into exactly the hostility it is about.
type Malice struct {
	// TruncateJSON cuts the response body in half mid-object.
	TruncateJSON bool
	// OversizeBody pads past any sane cap, so a client reading without an
	// io.LimitReader exhausts memory instead of failing a request.
	OversizeBody bool
	// Traversal rewrites every server-supplied path into one that escapes if
	// joined onto a directory unsanitised.
	Traversal bool
	// WrongContentType answers JSON requests as text/html.
	WrongContentType bool
	// RejectAuth fails every credential, however correct.
	RejectAuth bool
	// Stall never responds: the client's timeout is the only way out.
	Stall bool
}

// oversizeBytes is what OversizeBody pads with: comfortably past the 16 MiB
// this project's own client caps a metadata response at.
const oversizeBytes = 24 << 20

// Options configures a fake. With no credential it accepts nothing, which is
// the right default for a thing whose job is checking credentials.
type Options struct {
	User     string
	Password string
	APIKey   string // accepted as OpenSubsonic apiKey authentication
	// OpenSubsonic advertises the API-key extension, so a test can exercise
	// the client's downgrade to token+salt.
	OpenSubsonic bool
	Library      Library
	Malice       Malice
}

// Server is a running fake; its shutdown is registered with the test.
type Server struct {
	*httptest.Server
	t    TB
	opt  Options
	mu   sync.Mutex
	reqs []Request
}

// Request is one call as the server saw it, for tests asserting on what was
// sent. Credentials are not recorded: the server already refused a wrong one.
type Request struct {
	Endpoint string
	Query    url.Values
}

// New starts a fake. Options.Library defaults to DefaultLibrary.
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

// Requests returns what arrived, in order.
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
		// Held open until the client gives up: its timeout is the only thing
		// that ends this exchange, which is what the test proves exists.
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
		s.ok(w, response{Artists: &list})
	case "getArtist":
		a, found := s.opt.Library.findArtist(q.Get("id"))
		if !found {
			s.fail(w, 70, "Artist not found")
			return
		}
		s.ok(w, response{Artist: &a})
	case "getAlbum":
		al, found := s.opt.Library.findAlbum(q.Get("id"))
		if !found {
			s.fail(w, 70, "Album not found")
			return
		}
		al.Songs = s.maybeTraverse(al.Songs)
		s.ok(w, response{Album: &al})
	case "stream":
		sg, found := s.opt.Library.findSong(q.Get("id"))
		if !found {
			s.fail(w, 70, "Song not found")
			return
		}
		w.Header().Set("Content-Type", "audio/flac")
		// Not real FLAC: nothing in the unit suite decodes it, and the
		// integration job is where real audio meets real mpv.
		_, _ = w.Write([]byte(strings.Repeat(sg.ID+" ", 64)))
	case "scrobble":
		s.ok(w, response{})
	default:
		s.fail(w, 0, "Unknown endpoint "+endpoint)
	}
}

// hygienic checks the promises the client makes about every request. A
// violation fails the calling test, naming the mistake where it was made.
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

// maybeTraverse replaces server paths with ones that escape the tree. The
// client is expected to have one function making this harmless.
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
