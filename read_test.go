package gotenberg

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadRoutesDecode(t *testing.T) {
	t.Run("Info", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `{"a.pdf":{"pages":3,"version":"1.7","encrypted":false,"tagged":true,"linearized":false,"formFields":true,"signed":false}}`)

		res, err := srv.client.PDF().Info().FileFromBytes("a.pdf", []byte("%PDF")).Send(t.Context())
		require.NoError(t, err)
		assert.Equal(t, map[string]DocumentInfo{"a.pdf": {Pages: 3, Version: "1.7", Tagged: true, FormFields: true}}, res)
	})

	t.Run("Bookmarks", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `{"a.pdf":[{"title":"Intro","pageNumber":1,"children":[{"title":"Sub","pageNumber":2}]}]}`)

		res, err := srv.client.PDF().Bookmarks().FileFromBytes("a.pdf", nil).Send(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []Bookmark{{Title: "Intro", PageNumber: 1, Children: []Bookmark{{Title: "Sub", PageNumber: 2}}}}, res["a.pdf"])
	})

	t.Run("Form", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `{"a.pdf":[{"name":"n","type":"text","value":"v","pages":[1]},{"name":"l","type":"choice","value":["a","b"],"pages":[2],"multiSelect":true}]}`)

		res, err := srv.client.PDF().Form().Pages("1-2").FileFromBytes("a.pdf", nil).Send(t.Context())
		require.NoError(t, err)
		require.Len(t, res["a.pdf"], 2)
		assert.Equal(t, "v", res["a.pdf"][0].Value)
		assert.Equal(t, []any{"a", "b"}, res["a.pdf"][1].Value)
		assert.Equal(t, []string{"1-2"}, srv.last(t).Fields["pages"])
	})

	t.Run("ValidateOneProfile", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `{"a.pdf":{"profile":"PDF/A-2b","compliant":false,"violations":[{"specification":"ISO 19005-2:2011","clause":"6.1.2","testNumber":1,"description":"d","severity":"fatal"}]}}`)

		res, err := srv.client.PDF().Validate(ProfilePDFA2b).FileFromBytes("a.pdf", nil).Send(t.Context())
		require.NoError(t, err)
		assert.Equal(t, ValidationReport{
			Profile:    []Profile{ProfilePDFA2b},
			Violations: []Violation{{Specification: "ISO 19005-2:2011", Clause: "6.1.2", TestNumber: 1, Description: "d", Severity: "fatal"}},
		}, res["a.pdf"])
	})

	t.Run("ValidatePair", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `{"a.pdf":{"profile":["PDF/A-2b","PDF/UA-1"],"compliant":true,"violations":[]}}`)

		res, err := srv.client.PDF().Validate(ProfilePDFA2b).PairedWith(ProfilePDFUA1).FileFromBytes("a.pdf", nil).Send(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []Profile{ProfilePDFA2b, ProfilePDFUA1}, res["a.pdf"].Profile)
		assert.True(t, res["a.pdf"].Compliant)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusOK, nil, `not json`)

		_, err := srv.client.PDF().Metadata().FileFromBytes("a.pdf", nil).Send(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gotenberg: decode response")
	})

	t.Run("APIError", func(t *testing.T) {
		srv := newResponseServer(t, http.StatusBadRequest, nil, `Invalid form data`)

		_, err := srv.client.PDF().Attachments().FileFromBytes("a.pdf", nil).Send(t.Context())

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "Invalid form data", apiErr.Message)
	})
}
