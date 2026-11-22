package gotenberg

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newResponseServer(t *testing.T, status int, headers map[string]string, body string) *testServer {
	return newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}

		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

func mergeRequest(c *Client) PDFMerge {
	return c.PDF().Merge().FileFromBytes("a.pdf", []byte("%PDF"))
}

func TestToBytes(t *testing.T) {
	srv := newResponseServer(t, http.StatusOK, map[string]string{
		"Content-Type":                      "application/pdf",
		"Content-Disposition":               `attachment; filename="out file.pdf"`,
		"Gotenberg-Trace":                   "trace",
		"Gotenberg-Version":                 "9.6.0",
		"Gotenberg-Remediation-Residual":    "2",
		"Gotenberg-Remediation-Rolled-Back": "0",
	}, "%PDF-1.7")

	res, err := mergeRequest(srv.client).ToBytes(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []byte("%PDF-1.7"), res.Content)
	assert.Equal(t, ResponseInfo{
		Filename:              "out file.pdf",
		ContentType:           "application/pdf",
		ContentLength:         8,
		Trace:                 "trace",
		Version:               "9.6.0",
		RemediationResidual:   2,
		RemediationRolledBack: 0,
	}, res.ResponseInfo)
}

func TestResponseInfoDefaults(t *testing.T) {
	srv := newResponseServer(t, http.StatusOK, nil, "%PDF")

	res, err := mergeRequest(srv.client).ToBytes(t.Context())
	require.NoError(t, err)
	assert.Empty(t, res.Filename)
	assert.Equal(t, -1, res.RemediationResidual)
	assert.Equal(t, -1, res.RemediationRolledBack)
}

func TestToReader(t *testing.T) {
	srv := newResponseServer(t, http.StatusOK, map[string]string{"Content-Type": "application/zip"}, "PKzip")

	rr, err := mergeRequest(srv.client).ToReader(t.Context())
	require.NoError(t, err)

	content, err := io.ReadAll(rr)
	require.NoError(t, err)
	assert.Equal(t, "PKzip", string(content))
	assert.Equal(t, "application/zip", rr.ContentType)

	require.NoError(t, rr.Close())
	require.NoError(t, rr.Close(), "Close is idempotent")

	_, err = rr.Read(make([]byte, 1))
	require.Error(t, err)
}

func TestToFile(t *testing.T) {
	srv := newResponseServer(t, http.StatusOK, map[string]string{"Content-Disposition": `attachment; filename="a.pdf"`}, "%PDF-1.7")
	path := filepath.Join(t.TempDir(), "out.pdf")

	info, err := mergeRequest(srv.client).ToFile(t.Context(), path)
	require.NoError(t, err)
	assert.Equal(t, "a.pdf", info.Filename)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "%PDF-1.7", string(content))
}

func TestToFileRemovesPartialFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "%PDF-partial")
		w.(http.Flusher).Flush()
		cancel()
		<-ctx.Done()
	})
	path := filepath.Join(t.TempDir(), "out.pdf")

	_, err := mergeRequest(srv.client).ToFile(ctx, path)
	require.ErrorIs(t, err, context.Canceled)

	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestNoOutput(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := newResponseServer(t, status, nil, "")
			path := filepath.Join(t.TempDir(), "out.pdf")

			_, err := mergeRequest(srv.client).ToBytes(t.Context())
			require.ErrorIs(t, err, ErrNoOutput)

			_, err = mergeRequest(srv.client).ToReader(t.Context())
			require.ErrorIs(t, err, ErrNoOutput)

			_, err = mergeRequest(srv.client).ToFile(t.Context(), path)
			require.ErrorIs(t, err, ErrNoOutput)

			_, statErr := os.Stat(path)
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}

	t.Run("Chunked", func(t *testing.T) {
		srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.(http.Flusher).Flush() // no Content-Length
		})

		_, err := mergeRequest(srv.client).ToBytes(t.Context())
		require.ErrorIs(t, err, ErrNoOutput)
	})
}

func TestAPIError(t *testing.T) {
	srv := newResponseServer(t, http.StatusBadRequest, map[string]string{"Gotenberg-Trace": "trace"},
		"Invalid form data: form field 'scale' is invalid\n")

	_, err := mergeRequest(srv.client).ToBytes(t.Context())

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "Invalid form data: form field 'scale' is invalid", apiErr.Message)
	assert.Equal(t, "trace", apiErr.Trace)
	assert.Equal(t, "gotenberg: server answered 400 Bad Request: Invalid form data: form field 'scale' is invalid", apiErr.Error())
}

func TestAPIErrorBodyIsLimited(t *testing.T) {
	srv := newResponseServer(t, http.StatusInternalServerError, nil, strings.Repeat("x", 2*maxErrorBody))

	_, err := mergeRequest(srv.client).ToBytes(t.Context())

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Len(t, apiErr.Message, maxErrorBody)
}

func TestRedirectIsAnError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
	})

	_, err := mergeRequest(srv.client).ToBytes(t.Context())

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusTemporaryRedirect, apiErr.StatusCode)
}

func TestTransportError(t *testing.T) {
	c, err := NewClient("http://127.0.0.1:1")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = mergeRequest(c).ToBytes(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, strings.HasPrefix(err.Error(), "gotenberg: "))
	assert.False(t, errors.As(err, new(*APIError)))
}
