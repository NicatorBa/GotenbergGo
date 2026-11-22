package gotenberg

import (
	"errors"
	"maps"
	"slices"
	"strings"
)

// download is one entry of the downloadFrom form field.
type download struct {
	URL              string            `json:"url"`
	ExtraHTTPHeaders map[string]string `json:"extraHttpHeaders,omitempty"`
	Filename         string            `json:"filename,omitempty"`

	err error
}

// DownloadOption configures a file the server downloads itself, added via
// FileFromURL.
type DownloadOption func(*download)

// WithDownloadFilename sets the filename of the downloaded file. Without it,
// the server names the file.
func WithDownloadFilename(filename string) DownloadOption {
	return func(d *download) {
		if filename == "" {
			d.err = errors.Join(d.err, errors.New("WithDownloadFilename: empty filename"))
			return
		}

		d.Filename = filename
	}
}

// WithDownloadHeader adds an HTTP header the server sends when it downloads
// the file. It may be given several times.
func WithDownloadHeader(name, value string) DownloadOption {
	return func(d *download) {
		if strings.TrimSpace(name) == "" {
			d.err = errors.Join(d.err, errors.New("WithDownloadHeader: empty header name"))
			return
		}

		d.ExtraHTTPHeaders = maps.Clone(d.ExtraHTTPHeaders)
		if d.ExtraHTTPHeaders == nil {
			d.ExtraHTTPHeaders = map[string]string{}
		}

		d.ExtraHTTPHeaders[name] = value
	}
}

// addDownload adds a file the server downloads from url. The URL is not
// validated: the server is authoritative.
func (r *request) addDownload(url string, opts []DownloadOption) {
	d := download{URL: url}
	for _, opt := range opts {
		if opt != nil {
			opt(&d)
		}
	}

	if d.err != nil {
		r.addErr(d.err)
		return
	}

	r.downloads = append(slices.Clip(r.downloads), d)
}
