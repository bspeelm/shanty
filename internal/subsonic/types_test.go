package subsonic_test

import (
	"testing"

	"github.com/bspeelm/shanty/internal/subsonic/fake"
)

// albumRequests counts the getAlbumList2 calls the server received.
func albumRequests(srv *fake.Server) int {
	n := 0
	for _, r := range srv.Requests() {
		if r.Endpoint == "getAlbumList2" {
			n++
		}
	}
	return n
}

// TestEveryAlbumComesBackAcrossPages seeds more albums than two pages hold, so
// the paging is exercised rather than assumed.
func TestEveryAlbumComesBackAcrossPages(t *testing.T) {
	c, srv := dial(t, fake.Options{User: user, Password: pass})
	// The default library's three albums plus these make 1004, which is three
	// pages of 500.
	srv.SetAlbums(1001)

	got, err := c.Albums(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1004 {
		t.Fatalf("the client collected %d albums, want 1004", len(got))
	}
	if n := albumRequests(srv); n != 3 {
		t.Errorf("1004 albums took %d requests, want 3", n)
	}

	// Every album arrives once and the pages are joined in order.
	seen := map[string]bool{}
	for i, al := range got {
		if seen[al.ID] {
			t.Fatalf("album %s came back twice, at position %d", al.ID, i)
		}
		seen[al.ID] = true
		if al.Name == "" {
			t.Errorf("album %s came back with no name", al.ID)
		}
	}
	for _, id := range []string{"al-1", "al-2", "al-3", "al-seeded-0000", "al-seeded-1000"} {
		if !seen[id] {
			t.Errorf("album %s is missing", id)
		}
	}
	if got[0].Name > got[len(got)-1].Name {
		t.Errorf("the pages were joined out of order: %q then %q", got[0].Name, got[len(got)-1].Name)
	}
}

// TestAShortPageEndsTheListing is the small library: one request, because the
// first page already came back short.
func TestAShortPageEndsTheListing(t *testing.T) {
	c, srv := dial(t, fake.Options{User: user, Password: pass})

	got, err := c.Albums(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("the default library returned %d albums, want 3", len(got))
	}
	if n := albumRequests(srv); n != 1 {
		t.Errorf("three albums took %d requests, want 1", n)
	}
}

// TestAServerThatNeverRunsOutOfAlbumsIsAbandoned covers the request cap: a
// server answering every offset with a full page must not be followed for
// ever.
func TestAServerThatNeverRunsOutOfAlbumsIsAbandoned(t *testing.T) {
	c, srv := dial(t, fake.Options{User: user, Password: pass,
		Malice: fake.Malice{AlbumsNeverEnd: true}})

	got, err := c.Albums(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	n := albumRequests(srv)
	if n == 0 {
		t.Fatal("the client asked for no albums at all")
	}
	if n > 100 {
		t.Fatalf("the client made %d requests against a server that never ends", n)
	}
	if len(got) != n*500 {
		t.Errorf("%d requests of 500 returned %d albums", n, len(got))
	}
}

// TestAnErrorListingAlbumsIsReported checks the paging does not swallow a
// refusal and report an empty library.
func TestAnErrorListingAlbumsIsReported(t *testing.T) {
	c, srv := dial(t, fake.Options{User: user, Password: pass,
		Malice: fake.Malice{RejectAuth: true}})

	got, err := c.Albums(t.Context())
	if err == nil {
		t.Fatalf("a refused listing returned %d albums and no error", len(got))
	}
	if got != nil {
		t.Errorf("a failed listing also returned %d albums", len(got))
	}
	if n := albumRequests(srv); n != 1 {
		t.Errorf("a refusal cost %d requests, want 1", n)
	}
}
