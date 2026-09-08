package subsonic

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"net/url"
)

const (
	// APIVersion is the Subsonic API version sent with every request.
	APIVersion = "1.16.1"
	// ClientName is sent with every request as the c= parameter.
	ClientName = "shanty"
)

// Authenticator adds one credential to a request’s query parameters.
type Authenticator func(url.Values)

// APIKeyAuth authenticates with an API key.
func APIKeyAuth(key string) Authenticator {
	return func(q url.Values) { q.Set("apiKey", key) }
}

// TokenAuth authenticates with a stored token and salt.
func TokenAuth(username, token, salt string) Authenticator {
	return func(q url.Values) {
		q.Set("u", username)
		q.Set("t", token)
		q.Set("s", salt)
	}
}

// PasswordAuth authenticates with a password, generating a new salt for each
// request.
func PasswordAuth(username, password string) Authenticator {
	return func(q url.Values) {
		salt := Salt()
		q.Set("u", username)
		q.Set("s", salt)
		q.Set("t", Token(password, salt))
	}
}

// Salt returns a new random salt.
func Salt() string { return rand.Text() }

// Token returns the MD5 of password and salt that Subsonic servers accept in
// place of a password.
func Token(password, salt string) string {
	sum := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(sum[:])
}

// redacted lists the query parameters Redact replaces. It includes the legacy
// plain-password parameter, which shanty never sends.
var redacted = []string{"t", "s", "apiKey", "p"}

// Redact returns the URL with credential parameters replaced, for logging. The
// URL passed in is not modified.
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
