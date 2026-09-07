package subsonic

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"net/url"
)

const (
	APIVersion = "1.16.1" // what shanty declares it speaks (ADR-007)
	// ClientName is the c= every request carries. Servers log it and users
	// filter play history by it, which is half of why ADR-002 settled the
	// name before any request was sent.
	ClientName = "shanty"
)

// Authenticator adds one credential to a request's query. A function rather
// than a struct, so a password-backed credential can mint a fresh salt per
// request while a stored token cannot, and so nothing assembles a credential
// except the three constructors below -- none of which can produce p=.
type Authenticator func(url.Values)

// APIKeyAuth is the strongest mode: one revocable string, killed server-side.
func APIKeyAuth(key string) Authenticator {
	return func(q url.Values) { q.Set("apiKey", key) }
}

// TokenAuth replays a stored pair. It is fixed, so a server logging query
// strings sees the same one every time: the cost of storing a token rather
// than a password, and still not the password.
func TokenAuth(username, token, salt string) Authenticator {
	return func(q url.Values) {
		q.Set("u", username)
		q.Set("t", token)
		q.Set("s", salt)
	}
}

// PasswordAuth mints a fresh salt for every request, so no two requests carry
// the same token and a single logged URL replays nothing.
func PasswordAuth(username, password string) Authenticator {
	return func(q url.Values) {
		salt := rand.Text()
		q.Set("u", username)
		q.Set("s", salt)
		q.Set("t", Token(password, salt))
	}
}

// Token is the Subsonic hash: md5 of password+salt. MD5 is the protocol's
// choice, and is why §2 treats a token as replayable-here, not as protection.
func Token(password, salt string) string {
	sum := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(sum[:])
}

// redacted includes the legacy p= this client does not send: a redactor
// covering only today's modes stops working the moment somebody adds one.
var redacted = []string{"t", "s", "apiKey", "p"}

// Redact rewrites a URL for logging, with no setting to turn it off: a knob
// that exists gets turned, and this one by whoever pastes a log into an issue.
func Redact(u *url.URL) string {
	c := *u
	q := c.Query()
	for _, key := range redacted {
		if q.Has(key) {
			q.Set(key, "REDACTED")
		}
	}
	c.RawQuery = q.Encode()
	return c.String()
}
