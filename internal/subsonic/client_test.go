package subsonic_test

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
)

const (
	user = "skipper"
	pass = "hornpipe"
)

// An md5 token as it appears in a Subsonic query: 32 hex characters.
var md5Hex = regexp.MustCompile(`\b[0-9a-f]{32}\b`)

// An external test package on purpose: it lets these tests import the fake,
// which imports this package, without a cycle -- and it means every test here
// reaches the client only through what it exports.
func dial(t *testing.T, opt fake.Options) (*subsonic.Client, *fake.Server) {
	t.Helper()
	if opt.User == "" && opt.APIKey == "" {
		opt.User, opt.Password = user, pass
	}
	srv := fake.New(t, opt)
	auth := subsonic.PasswordAuth(opt.User, opt.Password)
	if opt.APIKey != "" {
		auth = subsonic.APIKeyAuth(opt.APIKey)
	}
	c, err := subsonic.New(srv.URL, auth, subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

// §5's named gate. Reflection over the constructed client rather than a list
// of the clients we remember building: an http.Client added later with no
// timeout is exactly the one nobody would think to add to a list.
func TestClientTimeouts(t *testing.T) {
	c, _ := dial(t, fake.Options{})

	v := reflect.ValueOf(c).Elem()
	var found int
	for i := range v.NumField() {
		f := v.Field(i)
		// Compared by type name rather than by importing net/http: this
		// package is the only one allowed that import, and its own test
		// should not be the exception that makes the budget arguable.
		if f.Kind() != reflect.Ptr || f.IsNil() || f.Type().String() != "*http.Client" {
			continue
		}
		found++
		// .Int() rather than .Interface(): the field is unexported, and
		// reflect refuses to hand out an interface for one.
		if f.Elem().FieldByName("Timeout").Int() == 0 {
			t.Errorf("field %q is an http.Client with no timeout; §9 gives every one of them a deadline",
				v.Type().Field(i).Name)
		}
	}
	if found == 0 {
		t.Fatal("reflection found no *http.Client on the client; this test has stopped looking at anything")
	}
}

func TestPingAuthenticates(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  fake.Options
	}{
		{name: "password, salted fresh each request", opt: fake.Options{}},
		{name: "an api key", opt: fake.Options{APIKey: "k-1", OpenSubsonic: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := dial(t, tc.opt)
			if err := c.Ping(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A stored token and a password-derived one are different things: the stored
// pair repeats, and the derived one must not.
func TestPasswordAuthSaltsEveryRequest(t *testing.T) {
	c, srv := dial(t, fake.Options{})
	for range 3 {
		if err := c.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	for _, r := range srv.Requests() {
		seen[r.Query.Get("s")+"/"+r.Query.Get("t")] = true
	}
	if len(seen) != 3 {
		t.Errorf("three requests produced %d distinct salt/token pairs, want 3", len(seen))
	}
}

func TestStoredTokenAuthIsAccepted(t *testing.T) {
	srv := fake.New(t, fake.Options{User: user, Password: pass})
	salt := "brackish"
	c, err := subsonic.New(srv.URL, subsonic.TokenAuth(user, subsonic.Token(pass, salt), salt), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseWalksTheLibrary(t *testing.T) {
	c, _ := dial(t, fake.Options{})
	ctx := context.Background()

	artists, err := c.Artists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 2 {
		t.Fatalf("got %d artists, want 2", len(artists))
	}
	// Flattened out of the server's index and sorted by name, not by the
	// letter the server filed them under.
	if artists[0].Name != "Aoi" || artists[1].Name != "The Bilge Pumps" {
		t.Errorf("artists came back as %q and %q", artists[0].Name, artists[1].Name)
	}

	one, err := c.Artist(ctx, artists[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Albums) != 2 {
		t.Fatalf("got %d albums, want 2", len(one.Albums))
	}

	album, err := c.Album(ctx, one.Albums[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(album.Songs) != 2 {
		t.Fatalf("got %d songs, want 2", len(album.Songs))
	}
	if album.Songs[0].Track != 1 || album.Songs[1].Track != 2 {
		t.Errorf("songs are not in track order: %+v", album.Songs)
	}
}

func TestScrobbleReachesTheServer(t *testing.T) {
	c, srv := dial(t, fake.Options{})
	if err := c.Scrobble(context.Background(), "tr-1", true); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	last := reqs[len(reqs)-1]
	if last.Endpoint != "scrobble" || last.Query.Get("id") != "tr-1" || last.Query.Get("submission") != "true" {
		t.Errorf("scrobble arrived as %s %v", last.Endpoint, last.Query)
	}
}

// The stream URL is built rather than fetched: it goes to mpv over IPC, and
// nothing in this process reads a byte of it (§7).
func TestStreamURLCarriesTheCredentialAndNothingElse(t *testing.T) {
	c, _ := dial(t, fake.Options{})

	raw := c.StreamURL("tr-1")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for _, want := range []string{"id", "c", "v", "t", "s", "u"} {
		if !q.Has(want) {
			t.Errorf("the stream URL carries no %s=", want)
		}
	}
	if q.Has("p") {
		t.Error("the stream URL carries the legacy password parameter")
	}
	if !strings.HasSuffix(u.Path, "/rest/stream.view") {
		t.Errorf("stream path = %q", u.Path)
	}
}

func TestServerErrorsCarryTheirFix(t *testing.T) {
	srv := fake.New(t, fake.Options{User: user, Password: pass, Malice: fake.Malice{RejectAuth: true}})
	c, err := subsonic.New(srv.URL, subsonic.PasswordAuth(user, pass), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}

	err = c.Ping(context.Background())
	var serr *subsonic.Error
	if !errors.As(err, &serr) {
		t.Fatalf("err = %v, want a *subsonic.Error", err)
	}
	if !serr.Unauthorized() {
		t.Errorf("code %d did not report itself as an auth failure", serr.Code)
	}
	if !strings.Contains(err.Error(), "credentials.toml") {
		t.Errorf("the refusal does not say what to check:\n%s", err)
	}
}

// The three hostile-server answers ADR-001 names, each costing a failed
// request and nothing else.
func TestAHostileServerCostsOneFailedRequest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		malice fake.Malice
		want   string
	}{
		{name: "truncated JSON", malice: fake.Malice{TruncateJSON: true}, want: "not valid JSON"},
		{name: "an oversized body", malice: fake.Malice{OversizeBody: true}, want: "past the cap"},
		{name: "an HTML page in front of it", malice: fake.Malice{WrongContentType: true}, want: "reverse proxy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := dial(t, fake.Options{Malice: tc.malice})
			_, err := c.Artists(context.Background())
			if err == nil {
				t.Fatal("a hostile answer was accepted")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A server that never answers must cost the context, not the process.
func TestAStallEndsAtTheContext(t *testing.T) {
	c, _ := dial(t, fake.Options{Malice: fake.Malice{Stall: true}})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.Ping(ctx)
	if err == nil {
		t.Fatal("a stalling server was waited on forever")
	}
	if elapsed := time.Since(start); elapsed > subsonic.MetadataTimeout {
		t.Errorf("the request took %v, past even the client timeout", elapsed)
	}
}

// Whatever the transport says went wrong, it must not say it with the
// credential in it: net/url's own error prints the whole URL.
func TestTransportErrorsDoNotLeakTheCredential(t *testing.T) {
	c, err := subsonic.New("http://127.0.0.1:1", subsonic.PasswordAuth(user, pass), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}

	err = c.Ping(context.Background())
	if err == nil {
		t.Fatal("connecting to a closed port succeeded")
	}
	if strings.Contains(err.Error(), pass) {
		t.Fatalf("the password is in the error:\n%s", err)
	}
	// Asserted on the values rather than the parameter names, which survive
	// redaction by design: t=REDACTED still contains "t=".
	if md5Hex.MatchString(err.Error()) {
		t.Errorf("the error carries something shaped like an auth token:\n%s", err)
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("the error was not redacted at all:\n%s", err)
	}
}
