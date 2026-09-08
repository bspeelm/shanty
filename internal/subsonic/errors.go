package subsonic

import "fmt"

// Error is what the server said went wrong. Subsonic answers HTTP 200 with a
// failure inside the body, so this is the only place most failures appear.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// advice maps a protocol error code to what the user should do about it. A
// code that is not listed is reported with the server’s own message.
var advice = map[int]string{
	0:  "the server did not say what went wrong; its own log will",
	10: "shanty sent a malformed request, which is a bug in shanty; please report it",
	20: "this server is too new for shanty's API version; upgrade shanty",
	30: "this server is too old for shanty; upgrade the server, or see ADR-007 for the versions supported",
	40: "check the credential in credentials.toml, and that username in config.toml matches it",
	41: "this server will not accept token authentication for your account; set an api_key in credentials.toml instead",
	50: "the account is not allowed to do this; an administrator changes that on the server",
	60: "the server's trial has expired; that is settled on the server, not here",
	70: "the server does not have it; it may have been removed since the library was listed",
}

func (e *Error) Error() string {
	if fix, known := advice[e.Code]; known {
		return fmt.Sprintf("%s (server error %d)\n%s", e.messageOr("the server refused"), e.Code, fix)
	}
	return fmt.Sprintf("%s (server error %d)", e.messageOr("the server refused"), e.Code)
}

func (e *Error) messageOr(fallback string) string {
	if e.Message == "" {
		return fallback
	}
	return e.Message
}

// NotFound is code 70, the one a caller acts on rather than reports: an album
// that vanished between listing and opening is a refresh, not a failure.
func (e *Error) NotFound() bool { return e.Code == 70 }

// Unauthorized groups the credential failures: the user's next action is the
// same for all of them.
func (e *Error) Unauthorized() bool { return e.Code == 40 || e.Code == 41 || e.Code == 50 }
