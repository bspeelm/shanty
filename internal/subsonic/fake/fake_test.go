package fake

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	testUser = "skipper"
	testPass = "hornpipe"
)

// recorder stands in for *testing.T so a test can assert that the fake would
// have failed its caller, without failing this one.
type recorder struct{ errs []string }

func (r *recorder) Errorf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatalf(f string, a ...any) { r.Errorf(f, a...) }
func (r *recorder) Helper()                   {}
func (r *recorder) Cleanup(func())            {}

func (r *recorder) sawErrorContaining(sub string) bool {
	for _, e := range r.errs {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

// wellFormed is the query a compliant client sends: client name, API version,
// JSON format, and token authentication. Tests that are about a missing piece
// remove it from this rather than building a broken query by hand, so what the
// test is about is the one line that differs.
func wellFormed() url.Values {
	salt := "brackish"
	sum := md5.Sum([]byte(testPass + salt))
	return url.Values{
		"u": {testUser},
		"t": {hex.EncodeToString(sum[:])},
		"s": {salt},
		"v": {APIVersion},
		"c": {"shanty"},
		"f": {"json"},
	}
}

func get(t *testing.T, s *Server, endpoint string, q url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, s.URL+"/rest/"+endpoint+".view?"+q.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "shanty/test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode(t *testing.T, resp *http.Response) response {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decoding the fake's response: %v", err)
	}
	return env.Response
}

func TestTokenAuthenticationIsAccepted(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass})
	if got := decode(t, get(t, s, "ping", wellFormed())).Status; got != "ok" {
		t.Errorf("ping status = %q, want ok", got)
	}
}

func TestAPIKeyAuthenticationIsAccepted(t *testing.T) {
	s := New(t, Options{APIKey: "k-1", OpenSubsonic: true})
	q := wellFormed()
	q.Del("u")
	q.Del("t")
	q.Del("s")
	q.Set("apiKey", "k-1")
	if got := decode(t, get(t, s, "ping", q)).Status; got != "ok" {
		t.Errorf("ping status = %q, want ok", got)
	}
}

func TestAWrongTokenIsRefused(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass})
	q := wellFormed()
	q.Set("t", strings.Repeat("0", 32))

	got := decode(t, get(t, s, "ping", q))
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.Error == nil || got.Error.Code != 40 {
		t.Errorf("error = %+v, want code 40", got.Error)
	}
}

// The three tests below are the fake's reason to exist. Each drives a request
// that breaks a promise the client makes in §6 or §9, and asserts the fake
// reported it against the test that made it -- because a fake that answers
// such a request politely lets the mistake ship.

func TestTheLegacyPasswordParameterFailsTheCallingTest(t *testing.T) {
	rec := &recorder{}
	s := New(rec, Options{User: testUser, Password: testPass})
	defer s.Close()

	q := wellFormed()
	q.Set("p", testPass)
	get(t, s, "ping", q)

	if !rec.sawErrorContaining("legacy password parameter") {
		t.Errorf("sending p= did not fail the calling test; reported: %v", rec.errs)
	}
}

func TestAMissingClientNameFailsTheCallingTest(t *testing.T) {
	rec := &recorder{}
	s := New(rec, Options{User: testUser, Password: testPass})
	defer s.Close()

	q := wellFormed()
	q.Del("c")
	get(t, s, "ping", q)

	if !rec.sawErrorContaining("client name") {
		t.Errorf("omitting c= did not fail the calling test; reported: %v", rec.errs)
	}
}

func TestAMissingUserAgentFailsTheCallingTest(t *testing.T) {
	rec := &recorder{}
	s := New(rec, Options{User: testUser, Password: testPass})
	defer s.Close()

	// net/http sets a User-Agent unless it is explicitly emptied, so this is
	// built by hand rather than through get.
	req, err := http.NewRequest(http.MethodGet, s.URL+"/rest/ping.view?"+wellFormed().Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if !rec.sawErrorContaining("User-Agent") {
		t.Errorf("omitting the User-Agent did not fail the calling test; reported: %v", rec.errs)
	}
}

func TestBrowseWalksArtistsToSongs(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass})

	artists := decode(t, get(t, s, "getArtists", wellFormed())).Artists
	if artists == nil {
		t.Fatal("getArtists returned no artist list")
	}
	var total int
	for _, idx := range artists.Index {
		total += len(idx.Artists)
	}
	if total != 2 {
		t.Fatalf("getArtists returned %d artists, want 2", total)
	}

	q := wellFormed()
	q.Set("id", "ar-1")
	one := decode(t, get(t, s, "getArtist", q)).Artist
	if one == nil || len(one.Albums) != 2 {
		t.Fatalf("getArtist(ar-1) = %+v, want 2 albums", one)
	}

	q.Set("id", one.Albums[0].ID)
	al := decode(t, get(t, s, "getAlbum", q)).Album
	if al == nil || len(al.Songs) != 2 {
		t.Fatalf("getAlbum(%s) = %+v, want 2 songs", one.Albums[0].ID, al)
	}
	if al.Songs[0].Title != "Slipway" {
		t.Errorf("first song = %q, want Slipway", al.Songs[0].Title)
	}
}

// The index is the server's, not the client's: "The Bilge Pumps" files under B
// because the server says so, and a client carrying its own article list would
// disagree with the server it is browsing.
func TestArticlesAreIgnoredWhenIndexing(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass})
	artists := decode(t, get(t, s, "getArtists", wellFormed())).Artists

	found := map[string]string{}
	for _, idx := range artists.Index {
		for _, a := range idx.Artists {
			found[a.Name] = idx.Name
		}
	}
	if got := found["The Bilge Pumps"]; got != "B" {
		t.Errorf("The Bilge Pumps indexed under %q, want B", got)
	}
	if got := found["Aoi"]; got != "A" {
		t.Errorf("Aoi indexed under %q, want A", got)
	}
	if artists.IgnoredArticles == "" {
		t.Error("the server reported no ignoredArticles, so a client cannot take its word for it")
	}
}

func TestTruncatedJSONDoesNotDecode(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass, Malice: Malice{TruncateJSON: true}})
	var env envelope
	err := json.NewDecoder(get(t, s, "getArtists", wellFormed()).Body).Decode(&env)
	if err == nil {
		t.Fatal("a truncated body decoded cleanly; the malice mode is not biting")
	}
}

func TestAnOversizeBodyExceedsAnySaneCap(t *testing.T) {
	const cap = 1 << 16
	s := New(t, Options{User: testUser, Password: testPass, Malice: Malice{OversizeBody: true}})

	n, err := io.Copy(io.Discard, get(t, s, "getArtists", wellFormed()).Body)
	if err != nil {
		t.Fatal(err)
	}
	if n <= cap {
		t.Fatalf("oversize body was %d bytes, which fits under a %d cap", n, cap)
	}
}

func TestTraversalRewritesEveryServerSuppliedPath(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass, Malice: Malice{Traversal: true}})
	q := wellFormed()
	q.Set("id", "al-1")

	al := decode(t, get(t, s, "getAlbum", q)).Album
	if al == nil || len(al.Songs) == 0 {
		t.Fatal("getAlbum returned no songs to rewrite")
	}
	for _, sg := range al.Songs {
		if !strings.HasPrefix(sg.Path, "../") {
			t.Errorf("song %s path = %q, which does not escape anything", sg.ID, sg.Path)
		}
	}
}

func TestAStallEndsOnlyWhenTheClientGivesUp(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass, Malice: Malice{Stall: true}})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL+"/rest/ping.view?"+wellFormed().Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "shanty/test")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("a stalling server answered; nothing here would prove a timeout exists")
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("the request failed after %v, so it did not reach the stall", time.Since(start))
	}
}

func TestStreamServesTheRequestedSong(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass})
	q := wellFormed()
	q.Set("id", "tr-1")

	resp := get(t, s, "stream", q)
	if got := resp.Header.Get("Content-Type"); got != "audio/flac" {
		t.Errorf("stream Content-Type = %q, want audio/flac", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "tr-1") {
		t.Error("the stream body does not identify the song it was asked for")
	}
}

func TestTheServerRecordsWhatArrived(t *testing.T) {
	s := New(t, Options{User: testUser, Password: testPass})
	q := wellFormed()
	q.Set("id", "tr-1")
	q.Set("submission", "true")
	get(t, s, "scrobble", q)

	reqs := s.Requests()
	if len(reqs) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(reqs))
	}
	if reqs[0].Endpoint != "scrobble" {
		t.Errorf("endpoint = %q, want scrobble", reqs[0].Endpoint)
	}
	if reqs[0].Query.Get("id") != "tr-1" || reqs[0].Query.Get("submission") != "true" {
		t.Errorf("query = %v, want the song id and a submission flag", reqs[0].Query)
	}
}

func TestOpenSubsonicSupportIsAdvertisedOnlyWhenConfigured(t *testing.T) {
	for _, tc := range []struct {
		name string
		on   bool
		want int
	}{
		{name: "a server offering API keys", on: true, want: 1},
		{name: "a server that does not", on: false, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(t, Options{User: testUser, Password: testPass, OpenSubsonic: tc.on})
			got := decode(t, get(t, s, "getOpenSubsonicExtensions", wellFormed()))
			if len(got.Extensions) != tc.want {
				t.Errorf("advertised %d extensions, want %d", len(got.Extensions), tc.want)
			}
		})
	}
}
