package gotenberg

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// capturedFile is a file part received by the test server.
type capturedFile struct {
	Field    string
	Filename string
	Content  []byte
}

// capturedRequest is a request received by the test server.
type capturedRequest struct {
	Method string
	Path   string
	Header http.Header
	Fields map[string][]string
	Files  []capturedFile
}

// hasFile reports whether the request carries a file under field whose
// filename ends with suffix.
func (r capturedRequest) hasFile(field, suffix string) bool {
	for _, f := range r.Files {
		if f.Field == field && strings.HasSuffix(f.Filename, suffix) {
			return true
		}
	}

	return false
}

// filenames returns the filenames of the files under field, in order.
func (r capturedRequest) filenames(field string) []string {
	var names []string
	for _, f := range r.Files {
		if f.Field == field {
			names = append(names, f.Filename)
		}
	}

	return names
}

// testServer records the requests it receives and answers with a fixed
// response.
type testServer struct {
	client *Client
	server *httptest.Server

	mu       sync.Mutex
	requests []capturedRequest
}

// newTestServer starts a server answering every request with handler, after
// recording it. The client of the server is returned with it.
func newTestServer(t *testing.T, handler http.HandlerFunc, opts ...ClientOption) *testServer {
	t.Helper()

	s := &testServer{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, err := capture(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		s.requests = append(s.requests, captured)
		s.mu.Unlock()

		handler(w, r)
	}))
	t.Cleanup(s.server.Close)

	client, err := NewClient(s.server.URL, opts...)
	require.NoError(t, err)
	s.client = client

	return s
}

// newFileServer answers with a PDF.
func newFileServer(t *testing.T) *testServer {
	return newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="result.pdf"`)
		_, _ = io.WriteString(w, "%PDF-1.7")
	})
}

// newJSONServer answers with an empty JSON object.
func newJSONServer(t *testing.T) *testServer {
	return newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{}")
	})
}

func (s *testServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.requests)
}

func (s *testServer) last(t *testing.T) capturedRequest {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	require.NotEmpty(t, s.requests, "no request received")

	return s.requests[len(s.requests)-1]
}

// capture reads the request, including a multipart body.
func capture(r *http.Request) (capturedRequest, error) {
	c := capturedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Header: r.Header.Clone(),
		Fields: map[string][]string{},
	}

	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return c, nil
	}

	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return c, nil
		}

		if err != nil {
			return c, err
		}

		content, err := io.ReadAll(part)
		if err != nil {
			return c, err
		}

		if part.FileName() != "" {
			c.Files = append(c.Files, capturedFile{Field: part.FormName(), Filename: part.FileName(), Content: content})
			continue
		}

		c.Fields[part.FormName()] = append(c.Fields[part.FormName()], string(content))
	}
}

// mustJSON encodes v.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return string(b)
}

// stageObject encodes the stage and decodes it as a JSON object.
func stageObject(t *testing.T, s Stage) map[string]json.RawMessage {
	t.Helper()

	b, err := json.Marshal(s)
	require.NoError(t, err)

	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &object))

	return object
}
