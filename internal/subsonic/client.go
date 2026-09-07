// Package subsonic is the only package in shanty that imports net/http, which
// `make budgets` asserts: "what does this program say on the network" has a
// one-package answer, and this is it.
package subsonic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// §9: the network can delay forever, so every client here has a deadline. The
// stream has no client at all -- its URL goes to mpv, which owns that
// connection for as long as the track plays.
const (
	MetadataTimeout = 10 * time.Second
	ArtTimeout      = 30 * time.Second
)

// A hostile server should buy a failed request, not the machine's memory. A
// library large enough to exceed this is a bug report worth having.
const maxMetadataBytes = 16 << 20

// Client talks to exactly one server (§9).
type Client struct {
	base *url.URL
	auth Authenticator

	metadata *http.Client

	agent string
	log   *slog.Logger
}

// Options are the knobs that are not credentials. No TLS setting: ADR-004.
type Options struct {
	// UserAgent defaults to shanty/<APIVersion>.
	UserAgent string
	// Logger, when set, records every request URL through Redact.
	Logger *slog.Logger
}

// New builds a client for one server: the base URL a user typed, no /rest.
func New(server string, auth Authenticator, opt Options) (*Client, error) {
	if auth == nil {
		return nil, errors.New("no credential was supplied")
	}
	base, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil {
		return nil, fmt.Errorf("server %q is not a URL: %w", server, err)
	}
	if base.Host == "" {
		return nil, fmt.Errorf("server %q names no host", server)
	}
	agent := opt.UserAgent
	if agent == "" {
		agent = ClientName + "/" + APIVersion
	}
	return &Client{
		base:     base,
		auth:     auth,
		metadata: &http.Client{Timeout: MetadataTimeout},
		agent:    agent,
		log:      opt.Logger,
	}, nil
}

// URL builds a request URL with the credential attached, exported for the one
// thing needing a URL rather than a response: the stream handed to mpv over
// IPC, never over argv (§7).
func (c *Client) URL(endpoint string, params url.Values) *url.URL {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/rest/" + endpoint + ".view"

	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	q.Set("v", APIVersion)
	q.Set("c", ClientName)
	q.Set("f", "json")
	c.auth(q)
	u.RawQuery = q.Encode()
	return &u
}

// StreamURL is what mpv is told to load; every byte after this is mpv's.
func (c *Client) StreamURL(id string) string {
	return c.URL("stream", url.Values{"id": {id}}).String()
}

// get performs one metadata request. A nil payload field on an "ok" answer is
// a server bug, and the caller reports it as one.
func (c *Client) get(ctx context.Context, endpoint string, params url.Values) (*response, error) {
	u := c.URL(endpoint, params)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.agent)

	if c.log != nil {
		c.log.Debug("subsonic request", "url", Redact(u))
	}

	resp, err := c.metadata.Do(req)
	if err != nil {
		// The URL is in the transport's error, credential and all, so it is
		// rebuilt from the redacted form rather than wrapped.
		return nil, fmt.Errorf("%s: %w", Redact(u), unwrapURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", Redact(u), resp.Status)
	}

	// An HTML answer is almost never the music server: it is a reverse proxy,
	// a captive portal or a login page in front of it, and "the server's
	// answer is not valid JSON" would send the user looking in the wrong
	// place. Other content types are accepted -- ADR-007 makes servers other
	// than Navidrome best-effort, and refusing an honest text/plain would
	// break one for nothing.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return nil, fmt.Errorf("%s answered with an HTML page rather than JSON.\nSomething is in front of the server -- a reverse proxy, or a login page. Open that URL in a browser to see what", Redact(u))
	}

	// Capped before it is read, not after: a body large enough to matter is
	// one we must not have finished reading to find out about.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return nil, err
	}
	if len(body) == maxMetadataBytes {
		return nil, fmt.Errorf("%s answered with more than %d bytes, which is past the cap a metadata response is read under", Redact(u), maxMetadataBytes)
	}
	return decode(body)
}

// decode is the trust boundary: every byte was the server's, and FuzzDecode is
// pointed here.
func decode(body []byte) (*response, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("the server's answer is not valid JSON: %w", err)
	}
	if env.Response.Status == "" {
		return nil, errors.New("the server's answer carries no subsonic-response status")
	}
	if env.Response.Status != "ok" {
		if env.Response.Error != nil {
			return nil, env.Response.Error
		}
		return nil, fmt.Errorf("the server answered %q without saying why", env.Response.Status)
	}
	return &env.Response, nil
}

// unwrapURLError strips net/url's wrapper, whose Error prints the whole URL,
// credential included.
func unwrapURLError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}
