package gotenberg

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	t.Run("Up", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `{"status":"up","details":{"chromium":{"status":"up","timestamp":"2026-11-21T10:00:00Z"}}}`)

		status, err := srv.client.Health(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "up", status.Status)
		assert.Equal(t, time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC), status.Details["chromium"].Timestamp)

		req := srv.last(t)
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/health", req.Path)
	})

	t.Run("Down", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusServiceUnavailable, nil, `{"status":"down","details":{"chromium":{"status":"down","timestamp":"2026-11-21T10:00:00Z","error":"dead"}}}`)

		status, err := srv.client.Health(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "down", status.Status)
		assert.Equal(t, "dead", status.Details["chromium"].Error)
	})

	t.Run("DownWithoutReport", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusServiceUnavailable, nil, "maintenance")

		_, err := srv.client.Health(t.Context())

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusUnauthorized, nil, "Unauthorized")

		_, err := srv.client.Health(t.Context())

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	})
}

func TestVersion(t *testing.T) {
	srv := newResponseServer(t, http.StatusOK, nil, "9.6.0\n")

	version, err := srv.client.Version(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "9.6.0", version)
	assert.Equal(t, "/version", srv.last(t).Path)
}

func TestDebug(t *testing.T) {
	t.Run("Enabled", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `{"version":"9.6.0"}`)

		debug, err := srv.client.Debug(t.Context())
		require.NoError(t, err)
		assert.JSONEq(t, `{"version":"9.6.0"}`, string(debug))
	})

	t.Run("Disabled", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusNotFound, nil, "Not Found")

		_, err := srv.client.Debug(t.Context())

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	})
}

func TestSystemRoutesUseClientOptions(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("9.6.0"))
	}, WithBasicAuth("u", "p"), WithHeader("X-Custom", "v"))

	_, err := srv.client.Version(t.Context())
	require.NoError(t, err)

	req := srv.last(t)
	assert.Equal(t, "v", req.Header.Get("X-Custom"))
	assert.NotEmpty(t, req.Header.Get("Authorization"))
}
