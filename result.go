package gotenberg

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
)

// Response headers.
const (
	headerVersion               = "Gotenberg-Version"
	headerRemediationResidual   = "Gotenberg-Remediation-Residual"
	headerRemediationRolledBack = "Gotenberg-Remediation-Rolled-Back"
)

// ResponseInfo describes the file a request answered with, read from the
// response headers. The file is a single document, such as a PDF, or a ZIP
// (application/zip) when there are several results.
type ResponseInfo struct {
	// Filename is the filename parameter of the Content-Disposition header.
	Filename string
	// ContentType is the Content-Type header.
	ContentType string
	// ContentLength is the length of the content, -1 when unknown.
	ContentLength int64
	// Trace is the Gotenberg-Trace header.
	Trace string
	// Version is the Gotenberg-Version header.
	Version string

	// RemediationResidual and RemediationRolledBack are set after a
	// remediate stage; -1 when the header is absent.
	RemediationResidual   int
	RemediationRolledBack int
}

// Result is a response read into memory by ToBytes.
type Result struct {
	ResponseInfo
	Content []byte
}

// ResultReader is a streamed response returned by ToReader. The caller must
// close it. It is not safe for concurrent use.
type ResultReader struct {
	ResponseInfo
	body   io.ReadCloser
	reader io.Reader
	closed bool
}

// Read implements [io.Reader].
func (r *ResultReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, errors.New("gotenberg: read on closed ResultReader")
	}

	return r.reader.Read(p)
}

// Close closes the response body. It is idempotent.
func (r *ResultReader) Close() error {
	if r.closed {
		return nil
	}

	r.closed = true

	return r.body.Close()
}

func newResponseInfo(resp *http.Response) ResponseInfo {
	info := ResponseInfo{
		ContentType:           resp.Header.Get("Content-Type"),
		ContentLength:         resp.ContentLength,
		Trace:                 resp.Header.Get(headerTrace),
		Version:               resp.Header.Get(headerVersion),
		RemediationResidual:   intHeader(resp.Header, headerRemediationResidual),
		RemediationRolledBack: intHeader(resp.Header, headerRemediationRolledBack),
	}

	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err == nil {
		info.Filename = params["filename"]
	}

	return info
}

// intHeader returns the integer value of the header, -1 when it is absent or
// not an integer.
func intHeader(h http.Header, name string) int {
	n, err := strconv.Atoi(h.Get(name))
	if err != nil {
		return -1
	}

	return n
}

// output sends the request and returns the response with a body that is
// known to carry content, or [ErrNoOutput].
func (r request) output(ctx context.Context) (*http.Response, io.Reader, error) {
	resp, err := r.do(ctx)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode == http.StatusNoContent || resp.ContentLength == 0 {
		_ = resp.Body.Close()
		return nil, nil, ErrNoOutput
	}

	body := bufio.NewReader(resp.Body)

	_, err = body.Peek(1)
	if errors.Is(err, io.EOF) {
		_ = resp.Body.Close()
		return nil, nil, ErrNoOutput
	}

	if err != nil {
		_ = resp.Body.Close()
		return nil, nil, errorf("read response: %w", err)
	}

	return resp, body, nil
}

func (r request) toBytes(ctx context.Context) (*Result, error) {
	resp, body, err := r.output(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	content, err := io.ReadAll(body)
	if err != nil {
		return nil, errorf("read response: %w", err)
	}

	return &Result{ResponseInfo: newResponseInfo(resp), Content: content}, nil
}

func (r request) toReader(ctx context.Context) (*ResultReader, error) {
	resp, body, err := r.output(ctx)
	if err != nil {
		return nil, err
	}

	return &ResultReader{ResponseInfo: newResponseInfo(resp), body: resp.Body, reader: body}, nil
}

// toFile writes the response to the file at path. The file is removed when
// writing fails or the context is cancelled, so no partial file is left.
func (r request) toFile(ctx context.Context, path string) (*ResponseInfo, error) {
	if path == "" {
		return nil, errorf("ToFile: empty path")
	}

	resp, body, err := r.output(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	f, err := os.Create(path)
	if err != nil {
		return nil, errorf("create %s: %w", path, err)
	}

	_, err = io.Copy(f, body)
	if err == nil {
		err = ctx.Err()
	}

	closeErr := f.Close()
	if err == nil && closeErr != nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(path)
		return nil, errorf("write %s: %w", path, err)
	}

	info := newResponseInfo(resp)

	return &info, nil
}

// sendJSON sends the request and decodes its JSON answer.
func sendJSON[T any](ctx context.Context, r request) (T, error) {
	var res T

	resp, err := r.do(ctx)
	if err != nil {
		return res, err
	}
	defer func() { _ = resp.Body.Close() }()

	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return res, errorf("decode response: %w", err)
	}

	return res, nil
}
