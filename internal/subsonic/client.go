// Package subsonic is a client for the Subsonic API. It is the only package in
// shanty that imports net/http.
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

// Request timeouts. The audio stream has no client here: its URL is given to
// mpv, which makes that connection itself.
const (
	MetadataTimeout = 10 * time.Second
	ArtTimeout      = 30 * time.Second
)

// maxMetadataBytes is the most of a metadata response that will be read.
const maxMetadataBytes = 16 << 20

// Client talks to one Subsonic server.
type Client struct {
	base *url.URL
	auth Authenticator

	metadata *http.Client

	agent string
	log   *slog.Logger
}

// Options are the settings that are not credentials.
type Options struct {
	// UserAgent defaults to shanty and the API version.
	UserAgent string
	// Logger, when set, records each request URL with credentials redacted.
	Logger *slog.Logger
}

// New returns a client for one server. The server argument is the base URL,
// without the /rest path.
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

// URL returns the request URL for an endpoint, with the credential and the
// standard parameters attached.
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

// StreamURL returns the URL that plays a track.
func (c *Client) StreamURL(id string) string {
	return c.URL("stream", url.Values{"id": {id}}).String()
}

// get performs one metadata request and returns the decoded response.
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
		// Rebuilt from the redacted URL, because the transport’s error contains the
		// full one.
		return nil, fmt.Errorf("%s: %w", Redact(u), unwrapURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", Redact(u), resp.Status)
	}

	// An HTML response means something other than the music server answered.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return nil, fmt.Errorf("%s answered with an HTML page rather than JSON.\nSomething is in front of the server -- a reverse proxy, or a login page. Open that URL in a browser to see what", Redact(u))
	}

	// The body is read up to maxMetadataBytes.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return nil, err
	}
	if len(body) == maxMetadataBytes {
		return nil, fmt.Errorf("%s answered with more than %d bytes, which is past the cap a metadata response is read under", Redact(u), maxMetadataBytes)
	}
	return decode(body)
}

// decode parses a response body and returns an error unless the server
// reported success.
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

// unwrapURLError removes net/url’s wrapper, whose message contains the whole
// URL including the credential.
func unwrapURLError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}
