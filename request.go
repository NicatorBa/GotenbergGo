package gotenberg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"slices"
)

// Request headers.
const (
	headerTrace          = "Gotenberg-Trace"
	headerOutputFilename = "Gotenberg-Output-Filename"
)

// Multipart field names.
const (
	filesField        = "files"
	archivesField     = "archives"
	downloadFromField = "downloadFrom"
	stagesField       = "stages"
)

// request is the shared state of all request builders. Builders are value
// types: every helper that modifies a reference-typed field copies it first
// (copy-on-write), so chains branching off the same builder never affect
// each other.
type request struct {
	client *Client
	route  string

	headers   http.Header
	fields    map[string][]string
	files     []file
	downloads []download
	stages    []Stage

	// inputs reports whether the uploaded files are the input documents of
	// the route, which merge order applies to (see uploads); merge whether
	// the route merges them without a merge stage.
	inputs bool
	merge  bool

	// err holds the deferred errors raised while building.
	err error
}

func newRequest(c *Client, route string) request {
	return request{
		client:    c,
		route:     route,
		headers:   http.Header{},
		fields:    map[string][]string{},
		files:     []file{},
		downloads: []download{},
		stages:    []Stage{},
	}
}

// addErr records a deferred error.
func (r *request) addErr(err error) {
	r.err = errors.Join(r.err, fmt.Errorf("gotenberg: %w", err))
}

func (r *request) setHeader(name, value string) {
	r.headers = r.headers.Clone()
	r.headers.Set(name, value)
}

func (r *request) setTrace(id string) {
	r.setHeader(headerTrace, id)
}

func (r *request) setOutputFilename(name string) {
	if name == "" {
		r.addErr(errors.New("OutputFilename: empty filename"))
		return
	}

	r.setHeader(headerOutputFilename, name)
}

// set sets the form field name, or records the deferred error of v.
func (r *request) set(name string, v fieldValue) {
	if v.err != nil {
		r.addErr(v.err)
		return
	}

	r.fields = maps.Clone(r.fields)
	r.fields[name] = v.form
}

// pairProfile completes the profile of the form field name with the second
// profile of a pair, replacing a previous one.
func (r *request) pairProfile(name string, p Profile) {
	current := r.fields[name]
	if len(current) == 0 {
		// The first profile was invalid; its error is recorded.
		return
	}

	r.set(name, profileValue(name, Profile(current[0]), p))
}

// setStages replaces the stages of the request.
func (r *request) setStages(stages []Stage) {
	for i, s := range stages {
		if s == nil {
			r.addErr(fmt.Errorf("stage %d: nil stage", i))
			continue
		}

		st := s.stageState()
		if st.op == "" {
			r.addErr(fmt.Errorf("stage %d: not created by a Stage constructor", i))
			continue
		}

		if st.err != nil {
			r.addErr(fmt.Errorf("stage %d (%s): %w", i, st.op, st.err))
		}
	}

	r.stages = slices.Clone(stages)
}

// send streams the request to the server and returns its response, whatever
// its status. The caller closes the response body.
func (r request) send(ctx context.Context) (*http.Response, error) {
	if r.client == nil {
		return nil, errNoClient
	}

	if r.err != nil {
		return nil, r.err
	}

	files := r.uploads()

	readers, err := openFiles(files)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	contentType := mw.FormDataContentType()

	done := make(chan struct{})
	go func() {
		defer close(done)

		err := r.writeBody(mw, files, readers)
		closeAll(readers)
		_ = pw.CloseWithError(err)
	}()

	// finish stops the body writer, if the transport did not consume the
	// whole body, and waits for it to close the files.
	finish := func() {
		_ = pr.CloseWithError(errors.New("gotenberg: request body abandoned"))
		<-done
	}

	req, err := r.client.newHTTPRequest(ctx, http.MethodPost, r.route, pr, r.headers)
	if err != nil {
		finish()
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := r.client.do(req)
	finish()

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// do sends the request and returns its response when the status is 2xx, or
// an [*APIError] otherwise. The caller closes the response body.
func (r request) do(ctx context.Context) (*http.Response, error) {
	resp, err := r.send(ctx)
	if err != nil {
		return nil, err
	}

	if !isSuccess(resp.StatusCode) {
		return nil, newAPIError(resp)
	}

	return resp, nil
}

// writeBody writes the multipart body: the form fields in name order, the
// downloads, the stages and the files.
func (r request) writeBody(mw *multipart.Writer, files []file, readers []io.ReadCloser) error {
	for _, name := range slices.Sorted(maps.Keys(r.fields)) {
		for _, value := range r.fields[name] {
			err := mw.WriteField(name, value)
			if err != nil {
				return err
			}
		}
	}

	if len(r.downloads) > 0 {
		b, err := json.Marshal(r.downloads)
		if err != nil {
			return err
		}

		err = mw.WriteField(downloadFromField, string(b))
		if err != nil {
			return err
		}
	}

	if len(r.stages) > 0 {
		b, err := json.Marshal(r.stages)
		if err != nil {
			return err
		}

		err = mw.WriteField(stagesField, string(b))
		if err != nil {
			return err
		}
	}

	for i, f := range files {
		w, err := mw.CreateFormFile(f.field, f.filename)
		if err != nil {
			return err
		}

		err = f.write(w, readers[i])
		if err != nil {
			return err
		}
	}

	return mw.Close()
}
