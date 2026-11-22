package gotenberg

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// HealthStatus is the answer of [Client.Health].
type HealthStatus struct {
	// Status is "up" or "down".
	Status string `json:"status"`
	// Details holds the status of each module, such as "chromium".
	Details map[string]HealthDetail `json:"details"`
}

// HealthDetail is the health of one module.
type HealthDetail struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
}

// Health returns the health report of the server (GET /health). A server
// that is down answers 503 with a report: it is returned with Status "down"
// and no error. Other non-2xx answers return an [*APIError].
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	resp, err := c.get(ctx, "/health")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if !isSuccess(resp.StatusCode) && resp.StatusCode != http.StatusServiceUnavailable {
		return nil, newAPIError(resp)
	}

	var status HealthStatus
	err = json.NewDecoder(resp.Body).Decode(&status)
	if err != nil {
		if resp.StatusCode == http.StatusServiceUnavailable {
			// Not a health report, such as a proxy's answer.
			return nil, &APIError{StatusCode: resp.StatusCode, Trace: resp.Header.Get(headerTrace)}
		}

		return nil, errorf("decode health report: %w", err)
	}

	return &status, nil
}

// Version returns the version of the server, such as "9.6.0"
// (GET /version).
func (c *Client) Version(ctx context.Context) (string, error) {
	resp, err := c.get(ctx, "/version")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if !isSuccess(resp.StatusCode) {
		return "", newAPIError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errorf("read version: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}

// Debug returns the debug information of the server as raw JSON
// (GET /debug). The route is only available when enabled server-side;
// otherwise the server answers 404, returned as an [*APIError].
func (c *Client) Debug(ctx context.Context) (json.RawMessage, error) {
	resp, err := c.get(ctx, "/debug")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if !isSuccess(resp.StatusCode) {
		return nil, newAPIError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errorf("read debug information: %w", err)
	}

	if !json.Valid(body) {
		return nil, errorf("decode debug information: invalid JSON")
	}

	return json.RawMessage(body), nil
}

// get sends a GET request for route. The caller closes the response body.
func (c *Client) get(ctx context.Context, route string) (*http.Response, error) {
	if c == nil {
		return nil, errNoClient
	}

	req, err := c.newHTTPRequest(ctx, http.MethodGet, route, nil, nil)
	if err != nil {
		return nil, err
	}

	return c.do(req)
}

// isSuccess reports whether code is a 2xx status code.
func isSuccess(code int) bool {
	return code >= 200 && code < 300
}
