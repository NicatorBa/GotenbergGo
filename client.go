package gotenberg

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Client sends requests to a Gotenberg server. It is the entry point of the
// fluent API: [Client.Chromium], [Client.LibreOffice] and [Client.PDF] return
// the modules whose methods start a request builder.
//
// A Client is immutable after [NewClient] and safe for concurrent use.
type Client struct {
	baseURL    string
	httpClient *http.Client
	headers    http.Header

	basicAuth bool
	username  string
	password  string

	// errs collects the errors of the options while NewClient runs.
	errs []error
}

// ClientOption configures a [Client] in [NewClient].
type ClientOption func(*Client)

// NewClient returns a client for the Gotenberg server at baseURL, an absolute
// http or https URL such as "http://localhost:3000". A trailing "/" is
// trimmed; a path, such as "https://example.com/gotenberg", is kept as the
// prefix of every route.
//
// Request bodies are streamed and cannot be replayed, so redirects are not
// supported: a 301, 302 or 303 makes the HTTP client repeat the request as a
// GET without body, which the server rejects, and a 307 or 308 response is
// returned as an [*APIError] with that status code. A redirect in front of
// Gotenberg usually means a wrong base URL: fix the base URL, or pass an
// [http.Client] with a custom CheckRedirect via [WithHTTPClient].
func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, errorf("invalid base URL %q: %w", baseURL, err)
	}

	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errorf("invalid base URL %q: want an absolute http or https URL", baseURL)
	}

	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: http.DefaultClient,
		headers:    http.Header{},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}

	if c.httpClient == nil {
		c.errs = append(c.errs, errorf("WithHTTPClient: nil *http.Client"))
	}

	if c.basicAuth && c.headers.Get("Authorization") != "" {
		c.errs = append(c.errs, errorf("WithBasicAuth conflicts with WithHeader(\"Authorization\", ...)"))
	}

	err = errors.Join(c.errs...)
	if err != nil {
		return nil, err
	}

	c.errs = nil

	return c, nil
}

// WithHTTPClient sets the HTTP client used to send requests, for instance to
// configure a transport, a proxy, TLS or timeouts. It is used as is. The
// default is [http.DefaultClient].
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithBasicAuth sends the given credentials with every request, for a
// server with basic authentication enabled. It conflicts with
// WithHeader("Authorization", ...).
func WithBasicAuth(username, password string) ClientOption {
	return func(client *Client) {
		client.basicAuth = true
		client.username = username
		client.password = password
	}
}

// WithHeader sends the header with every request. A header of the same name
// set on a request builder, such as Gotenberg-Trace, overrides it; a
// repeated call replaces the value.
//
// For a server with OIDC authentication enabled, send the bearer token with
// WithHeader("Authorization", "Bearer "+token).
func WithHeader(name, value string) ClientOption {
	return func(client *Client) {
		if strings.TrimSpace(name) == "" {
			client.errs = append(client.errs, errorf("WithHeader: empty header name"))
			return
		}

		client.headers.Set(name, value)
	}
}

// Chromium returns the Chromium module.
func (c *Client) Chromium() Chromium {
	return Chromium{client: c}
}

// LibreOffice returns the LibreOffice module.
func (c *Client) LibreOffice() LibreOffice {
	return LibreOffice{client: c}
}

// PDF returns the PDF module.
func (c *Client) PDF() PDF {
	return PDF{client: c}
}

// newHTTPRequest returns a request for route with the client headers, the
// given headers on top, and the basic auth credentials.
func (c *Client) newHTTPRequest(ctx context.Context, method, route string, body io.Reader, headers http.Header) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+route, body)
	if err != nil {
		return nil, errorf("create request: %w", err)
	}

	for name, values := range c.headers {
		req.Header[name] = slices.Clone(values)
	}

	for name, values := range headers {
		req.Header[name] = slices.Clone(values)
	}

	if c.basicAuth {
		req.SetBasicAuth(c.username, c.password)
	}

	return req, nil
}

// do sends req with the client's HTTP client.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errorf("send request: %w", err)
	}

	return resp, nil
}
