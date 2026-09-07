package subsonic

import (
	"net/url"
	"strings"
	"testing"
)

// There is no setting that turns this off, so the only way it fails is by
// missing a parameter. The list is asserted against what the three
// Authenticators actually emit, in both directions.
func TestRedactCoversEveryCredentialParameterWeCanEmit(t *testing.T) {
	emitted := map[string]bool{}
	for _, auth := range []Authenticator{
		APIKeyAuth("k-1"),
		TokenAuth("skipper", Token("hornpipe", "salt"), "salt"),
		PasswordAuth("skipper", "hornpipe"),
	} {
		q := url.Values{}
		auth(q)
		for k := range q {
			emitted[k] = true
		}
	}
	if len(emitted) < 3 {
		t.Fatalf("the authenticators emitted %d parameters; this test has stopped looking", len(emitted))
	}

	covered := map[string]bool{}
	for _, k := range redacted {
		covered[k] = true
	}
	for k := range emitted {
		// u= is the username, which is not a secret and is the one thing that
		// makes a redacted log still identify whose request it was.
		if k == "u" {
			continue
		}
		if !covered[k] {
			t.Errorf("an authenticator emits %q, which Redact leaves in the log", k)
		}
	}
	// p= is covered although nothing here emits it: a redactor that only
	// covers today's modes stops working the moment somebody adds one.
	if !covered["p"] {
		t.Error("Redact does not cover the legacy password parameter")
	}
}

func TestRedactReplacesValuesAndKeepsTheRest(t *testing.T) {
	u, err := url.Parse("https://music.example.org/rest/stream.view?id=tr-1&u=skipper&t=deadbeefdeadbeefdeadbeefdeadbeef&s=brackish&apiKey=k-1&c=shanty")
	if err != nil {
		t.Fatal(err)
	}

	got := Redact(u)
	for _, secret := range []string{"deadbeefdeadbeefdeadbeef", "brackish", "k-1"} {
		if strings.Contains(got, secret) {
			t.Errorf("Redact left %q in:\n%s", secret, got)
		}
	}
	for _, keep := range []string{"id=tr-1", "u=skipper", "c=shanty", "music.example.org", "stream.view"} {
		if !strings.Contains(got, keep) {
			t.Errorf("Redact dropped %q, which a debug log needs:\n%s", keep, got)
		}
	}
	// The original is not modified: a caller logging a URL must still be able
	// to send the request.
	if !strings.Contains(u.RawQuery, "brackish") {
		t.Error("Redact mutated the URL it was given")
	}
}

// Nothing here can produce the legacy password parameter, which is the point
// of there being exactly three constructors.
func TestNoAuthenticatorEmitsTheLegacyPasswordParameter(t *testing.T) {
	for name, auth := range map[string]Authenticator{
		"api key":  APIKeyAuth("k-1"),
		"token":    TokenAuth("skipper", "t", "s"),
		"password": PasswordAuth("skipper", "hornpipe"),
	} {
		q := url.Values{}
		auth(q)
		if q.Has("p") {
			t.Errorf("%s authentication emitted p=", name)
		}
		if q.Get("t") == "hornpipe" || q.Get("apiKey") == "hornpipe" {
			t.Errorf("%s authentication sent the password unhashed", name)
		}
	}
}

func TestTokenIsTheSubsonicHash(t *testing.T) {
	// The value in the specification's own worked example.
	if got, want := Token("sesame", "c19b2d"), "26719a1196d2a940705a59634eb18eab"; got != want {
		t.Errorf("Token = %q, want %q", got, want)
	}
}

func TestPasswordAuthNeverRepeatsASalt(t *testing.T) {
	auth := PasswordAuth("skipper", "hornpipe")
	seen := map[string]bool{}
	for range 100 {
		q := url.Values{}
		auth(q)
		salt := q.Get("s")
		if len(salt) < 6 {
			t.Fatalf("salt %q is shorter than the protocol's minimum", salt)
		}
		if seen[salt] {
			t.Fatalf("salt %q was minted twice in a hundred requests", salt)
		}
		seen[salt] = true
	}
}
