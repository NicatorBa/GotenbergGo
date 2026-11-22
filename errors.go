package gotenberg

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrNoOutput is returned by ToFile, ToBytes and ToReader when the server
// answers with a success status but without any content.
var ErrNoOutput = errors.New("gotenberg: response carries no output")

// errNoClient is returned by the terminal methods of a builder or module that
// was not obtained from a [Client], such as a zero value.
var errNoClient = errors.New("gotenberg: no client; obtain builders from a Client")

// maxErrorBody bounds the part of an error response read into
// [APIError.Message].
const maxErrorBody = 64 << 10

// APIError is returned when the server answers with a non-2xx status.
// Callers inspect it with [errors.As]:
//
//	var apiErr *gotenberg.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest {
//		// ...
//	}
type APIError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Message is the response body, Gotenberg's error message, read from
	// at most the first 64 KiB and trimmed.
	Message string
	// Trace is the Gotenberg-Trace response header.
	Trace string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	msg := fmt.Sprintf("gotenberg: server answered %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	if e.Message != "" {
		msg += ": " + e.Message
	}

	return msg
}

// newAPIError reads the error answer of resp and closes its body.
func newAPIError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(body)),
		Trace:      resp.Header.Get(headerTrace),
	}
}

// errorf returns an error prefixed with "gotenberg: ".
func errorf(format string, args ...any) error {
	return fmt.Errorf("gotenberg: "+format, args...)
}
