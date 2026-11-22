package gotenberg

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		opts    []ClientOption
		want    string
		wantErr bool
	}{
		{name: "HTTP", baseURL: "http://localhost:3000", want: "http://localhost:3000"},
		{name: "HTTPSWithPath", baseURL: "https://example.com/gotenberg/", want: "https://example.com/gotenberg"},
		{name: "Relative", baseURL: "localhost:3000", wantErr: true},
		{name: "NoHost", baseURL: "http://", wantErr: true},
		{name: "OtherScheme", baseURL: "ftp://example.com", wantErr: true},
		{name: "Invalid", baseURL: "http://[::1", wantErr: true},
		{name: "NilHTTPClient", baseURL: "http://localhost", opts: []ClientOption{WithHTTPClient(nil)}, wantErr: true},
		{name: "EmptyHeaderName", baseURL: "http://localhost", opts: []ClientOption{WithHeader(" ", "v")}, wantErr: true},
		{
			name:    "BasicAuthConflict",
			baseURL: "http://localhost",
			opts:    []ClientOption{WithBasicAuth("u", "p"), WithHeader("Authorization", "Bearer x")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.baseURL, tt.opts...)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "gotenberg: ")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, c.baseURL)
		})
	}
}

func TestClientOptions(t *testing.T) {
	httpClient := &http.Client{}
	srv := newFileServer(t)

	c, err := NewClient(srv.server.URL,
		WithHTTPClient(httpClient),
		WithBasicAuth("user", "pass"),
		WithHeader("X-Custom", "a"),
		WithHeader("Gotenberg-Trace", "client"),
	)
	require.NoError(t, err)
	assert.Same(t, httpClient, c.httpClient)

	_, err = c.PDF().Merge().FileFromBytes("a.pdf", []byte("%PDF")).Trace("builder").ToBytes(t.Context())
	require.NoError(t, err)

	req := srv.last(t)
	assert.Equal(t, "a", req.Header.Get("X-Custom"))
	assert.Equal(t, []string{"builder"}, req.Header.Values("Gotenberg-Trace"), "builder headers override client headers")

	user, pass, ok := (&http.Request{Header: req.Header}).BasicAuth()
	require.True(t, ok)
	assert.Equal(t, "user", user)
	assert.Equal(t, "pass", pass)
}

func TestZeroValueModules(t *testing.T) {
	_, err := Chromium{}.ConvertFromURL("https://example.com").ToBytes(t.Context())
	require.ErrorIs(t, err, errNoClient)

	_, err = PDFInfo{}.Send(t.Context())
	require.ErrorIs(t, err, errNoClient)

	_, err = (*Client)(nil).Version(t.Context())
	require.ErrorIs(t, err, errNoClient)
}
