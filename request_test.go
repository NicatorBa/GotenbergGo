package gotenberg

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildersAreImmutable(t *testing.T) {
	srv := newFileServer(t)

	base := srv.client.Chromium().ConvertFromHTML().IndexFromBytes([]byte("<html></html>")).Landscape(true)
	a := base.Trace("a").Scale(1.5).FileFromBytes("a.css", []byte("a"))
	b := base.Trace("b").PaperSize(PaperSizeA4)

	_, err := a.ToBytes(t.Context())
	require.NoError(t, err)
	reqA := srv.last(t)

	_, err = b.ToBytes(t.Context())
	require.NoError(t, err)
	reqB := srv.last(t)

	_, err = base.ToBytes(t.Context())
	require.NoError(t, err)
	reqBase := srv.last(t)

	assert.Equal(t, "a", reqA.Header.Get("Gotenberg-Trace"))
	assert.Equal(t, []string{"1.5"}, reqA.Fields["scale"])
	assert.NotContains(t, reqA.Fields, "paperSize")
	assert.Equal(t, []string{"index.html", "a.css"}, reqA.filenames("files"))

	assert.Equal(t, "b", reqB.Header.Get("Gotenberg-Trace"))
	assert.Equal(t, []string{"A4"}, reqB.Fields["paperSize"])
	assert.NotContains(t, reqB.Fields, "scale")
	assert.Equal(t, []string{"index.html"}, reqB.filenames("files"))

	assert.Empty(t, reqBase.Header.Get("Gotenberg-Trace"))
	assert.Equal(t, []string{"true"}, reqBase.Fields["landscape"])
	assert.Len(t, reqBase.Fields, 1)
}

func TestFileSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.html")
	require.NoError(t, os.WriteFile(path, []byte("from path"), 0o600))

	fsys := fstest.MapFS{"sub/style.css": {Data: []byte("from fs")}}

	srv := newFileServer(t)
	_, err := srv.client.Chromium().ConvertFromHTML().
		IndexFromPath(path).
		FileFromPathAs(path, "copy.html").
		FileFromFS(fsys, "sub/style.css").
		FileFromFSAs(fsys, "sub/style.css", "other.css").
		FileFromReader("reader.txt", strings.NewReader("from reader")).
		FileFromBytes("bytes.txt", []byte("from bytes")).
		HeaderFromFS(fsys, "sub/style.css").
		FooterFromReader(strings.NewReader("footer")).
		ToBytes(t.Context())
	require.NoError(t, err)

	got := map[string]string{}
	for _, f := range srv.last(t).Files {
		assert.Equal(t, "files", f.Field)
		got[f.Filename] = string(f.Content)
	}

	assert.Equal(t, map[string]string{
		"index.html":  "from path",
		"copy.html":   "from path",
		"style.css":   "from fs",
		"other.css":   "from fs",
		"reader.txt":  "from reader",
		"bytes.txt":   "from bytes",
		"header.html": "from fs",
		"footer.html": "footer",
	}, got)
}

func TestFileContentReplaces(t *testing.T) {
	srv := newFileServer(t)
	_, err := srv.client.Chromium().ConvertFromHTML().
		IndexFromBytes([]byte("first")).
		IndexFromBytes([]byte("second")).
		ToBytes(t.Context())
	require.NoError(t, err)

	files := srv.last(t).Files
	require.Len(t, files, 1)
	assert.Equal(t, "second", string(files[0].Content))
}

func TestReaderIsReadImmediately(t *testing.T) {
	srv := newFileServer(t)

	b := srv.client.PDF().Merge().FileFromReader("a.pdf", strings.NewReader("%PDF-a"))

	// Sent twice: the content does not depend on the reader any more.
	for range 2 {
		_, err := b.ToBytes(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "%PDF-a", string(srv.last(t).Files[0].Content))
	}
}

func TestDeferredErrors(t *testing.T) {
	readErr := errors.New("boom")

	tests := []struct {
		name  string
		build func(c *Client) PDFMerge
		want  string
	}{
		{"EmptyPath", func(c *Client) PDFMerge { return c.PDF().Merge().FileFromPath("") }, "empty path"},
		{"EmptyFilenameAs", func(c *Client) PDFMerge { return c.PDF().Merge().FileFromPathAs("a.pdf", "") }, "empty filename"},
		{"NilFS", func(c *Client) PDFMerge { return c.PDF().Merge().FileFromFS(nil, "a.pdf") }, "nil fs.FS"},
		{"InvalidFSName", func(c *Client) PDFMerge { return c.PDF().Merge().FileFromFS(fstest.MapFS{}, "../a.pdf") }, "invalid name"},
		{"NilReader", func(c *Client) PDFMerge { return c.PDF().Merge().FileFromReader("a.pdf", nil) }, "nil reader"},
		{"ReaderError", func(c *Client) PDFMerge {
			return c.PDF().Merge().FileFromReader("a.pdf", iotest.ErrReader(readErr))
		}, "boom"},
		{"EmptyOutputFilename", func(c *Client) PDFMerge { return c.PDF().Merge().OutputFilename("") }, "empty filename"},
		{"EmptyDownloadFilename", func(c *Client) PDFMerge {
			return c.PDF().Merge().FileFromURL("https://example.com/a.pdf", WithDownloadFilename(""))
		}, "empty filename"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFileServer(t)
			_, err := tt.build(srv.client).ToBytes(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.True(t, strings.HasPrefix(err.Error(), "gotenberg: "), err.Error())
			assert.Zero(t, srv.count(), "request must not be sent")
		})
	}

	t.Run("Wrapped", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.PDF().Merge().FileFromReader("a.pdf", iotest.ErrReader(readErr)).ToBytes(t.Context())
		require.ErrorIs(t, err, readErr)
	})

	t.Run("Several", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.Chromium().ConvertFromURL("https://example.com").
			Scale(-1).
			FailOnConsoleMessageTypes("bogus").
			PaperSize("A99").
			ToBytes(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "paperSize")
	})
}

func TestMissingFileIsNotSent(t *testing.T) {
	srv := newFileServer(t)
	_, err := srv.client.PDF().Merge().
		FileFromBytes("a.pdf", []byte("%PDF")).
		FileFromPath(filepath.Join(t.TempDir(), "missing.pdf")).
		ToBytes(t.Context())
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Zero(t, srv.count())
}

func TestArchives(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docs.zip")
	require.NoError(t, os.WriteFile(path, []byte("PK"), 0o600))

	srv := newFileServer(t)
	_, err := srv.client.PDF().Merge().
		ArchiveFromPath(path).
		ArchiveFromBytes([]byte("PK")).
		ArchiveFromReader(strings.NewReader("PK")).
		ArchiveFromFS(fstest.MapFS{"x/more.zip": {Data: []byte("PK")}}, "x/more.zip").
		ToBytes(t.Context())
	require.NoError(t, err)

	req := srv.last(t)
	assert.Equal(t, []string{"docs.zip", "archive2.zip", "archive3.zip", "more.zip"}, req.filenames("archives"))
	assert.Empty(t, req.filenames("files"))
}

func TestDownloads(t *testing.T) {
	srv := newFileServer(t)
	_, err := srv.client.PDF().Merge().
		FileFromURL("https://example.com/a.pdf").
		FileFromURL("https://example.com/b", WithDownloadFilename("b.pdf"), WithDownloadHeader("Authorization", "Bearer x"), WithDownloadHeader("X-A", "1")).
		ToBytes(t.Context())
	require.NoError(t, err)

	assert.JSONEq(t,
		`[{"url":"https://example.com/a.pdf"},{"url":"https://example.com/b","filename":"b.pdf","extraHttpHeaders":{"Authorization":"Bearer x","X-A":"1"}}]`,
		srv.last(t).Fields["downloadFrom"][0])
}

func TestMergeOrder(t *testing.T) {
	t.Run("MergeBuilder", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.PDF().Merge().
			FileFromBytes("z.pdf", []byte("%PDF")).
			FileFromBytes("a.pdf", []byte("%PDF")).
			FileFromBytes("m.pdf", []byte("%PDF")).
			ToBytes(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"0001_z.pdf", "0002_a.pdf", "0003_m.pdf"}, srv.last(t).filenames("files"))
	})

	t.Run("MergeStageKeepsReferencedFiles", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.PDF().Pipe().
			FileFromBytes("b.pdf", []byte("%PDF")).
			FileFromBytes("logo.png", []byte("png")).
			FileFromBytes("a.pdf", []byte("%PDF")).
			Stages(StageMerge(), StageStampImage("logo.png")).
			ToBytes(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"0001_b.pdf", "logo.png", "0002_a.pdf"}, srv.last(t).filenames("files"))
	})

	t.Run("ComposePlacements", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.PDF().Pipe().
			FileFromBytes("b.pdf", []byte("%PDF")).
			FileFromBytes("a.pdf", []byte("%PDF")).
			Stages(StageMerge(), StageCompose(Placement{Source: "a.pdf"}).PaperSize(PaperSizeA4)).
			ToBytes(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"0001_b.pdf", "a.pdf"}, srv.last(t).filenames("files"))
	})

	t.Run("LibreOfficeWithoutMerge", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.LibreOffice().Convert().
			FileFromBytes("b.docx", []byte("x")).
			FileFromBytes("a.docx", []byte("x")).
			Stages(StageOptimize()).
			ToBytes(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"b.docx", "a.docx"}, srv.last(t).filenames("files"))
	})

	t.Run("LibreOfficeWithMerge", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.LibreOffice().Convert().
			FileFromBytes("b.docx", []byte("x")).
			FileFromBytes("a.docx", []byte("x")).
			Stages(StageMerge()).
			ToBytes(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"0001_b.docx", "0002_a.docx"}, srv.last(t).filenames("files"))
	})

	t.Run("ChromiumNeverRenames", func(t *testing.T) {
		srv := newFileServer(t)
		_, err := srv.client.Chromium().ConvertFromHTML().
			IndexFromBytes([]byte("<html></html>")).
			FileFromBytes("b.css", []byte("x")).
			Stages(StageMerge()).
			ToBytes(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"index.html", "b.css"}, srv.last(t).filenames("files"))
	})

	t.Run("WidthGrows", func(t *testing.T) {
		b := newRequest(nil, "/")
		b.inputs, b.merge = true, true
		for range 10000 {
			b.addFileFromBytes("a.pdf", nil)
		}

		files := b.uploads()
		assert.Equal(t, "00001_a.pdf", files[0].filename)
		assert.Equal(t, "10000_a.pdf", files[9999].filename)
		assert.Equal(t, "a.pdf", b.files[0].filename, "the builder state is not modified")
	})
}

func TestStages(t *testing.T) {
	srv := newFileServer(t)
	_, err := srv.client.PDF().Pipe().
		FileFromBytes("a.pdf", []byte("%PDF")).
		Stages(
			StageRotate(90).Pages("1"),
			StageStampText("DRAFT").Position(StampPositionCenter).Opacity(0.3),
			StageRemediate(ProfilePDFA2b).PairedWith(ProfilePDFUA1),
			StageEncrypt().UserPassword("secret").Permissions(Permissions{Print: true}),
		).
		ToBytes(t.Context())
	require.NoError(t, err)

	assert.JSONEq(t, `[
		{"op":"rotate","angle":90,"pages":"1"},
		{"op":"stamp-text","text":"DRAFT","position":"center","opacity":0.3},
		{"op":"remediate","profile":["PDF/A-2b","PDF/UA-1"]},
		{"op":"encrypt","userPassword":"secret","permissions":{"print":true,"printHighQuality":false,"modifyContent":false,"extractText":false,"modifyAnnotations":false,"fillForms":false,"assemble":false}}
	]`, srv.last(t).Fields["stages"][0])
}

func TestStageErrors(t *testing.T) {
	tests := []struct {
		name   string
		stages []Stage
		want   string
	}{
		{"Nil", []Stage{nil}, "stage 0: nil stage"},
		{"ZeroValue", []Stage{StageMerge(), MergeStage{}}, "stage 1: not created"},
		{"Invalid", []Stage{StageStampText("x").Opacity(2)}, "stage 0 (stamp-text): opacity must be at most 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFileServer(t)
			_, err := srv.client.PDF().Pipe().FileFromBytes("a.pdf", nil).Stages(tt.stages...).ToBytes(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Zero(t, srv.count())
		})
	}
}

func TestStagesAreImmutable(t *testing.T) {
	base := StageStampText("DRAFT")
	a := base.Opacity(0.5)
	b := base.Font(StampFontCourier)

	assert.JSONEq(t, `{"op":"stamp-text","text":"DRAFT"}`, mustJSON(base))
	assert.JSONEq(t, `{"op":"stamp-text","text":"DRAFT","opacity":0.5}`, mustJSON(a))
	assert.JSONEq(t, `{"op":"stamp-text","text":"DRAFT","font":"Courier"}`, mustJSON(b))
}

func TestPairedWithReplaces(t *testing.T) {
	s := StageRemediate(ProfilePDFA4).PairedWith(ProfilePDFUA1).PairedWith(ProfilePDFUA2)
	assert.JSONEq(t, `{"op":"remediate","profile":["PDF/A-4","PDF/UA-2"]}`, mustJSON(s))

	srv := newJSONServer(t)
	_, err := srv.client.PDF().Validate(ProfilePDFA2b).PairedWith(ProfilePDFUA2).PairedWith(ProfilePDFUA1).
		FileFromBytes("a.pdf", nil).Send(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"PDF/A-2b", "PDF/UA-1"}, srv.last(t).Fields["profile"])
}

func TestNilSliceIsEmptyJSON(t *testing.T) {
	srv := newFileServer(t)
	_, err := srv.client.Chromium().ConvertFromURL("https://example.com").
		FailOnHTTPStatusCodes().
		ExtraHTTPHeaders(nil).
		ToBytes(t.Context())
	require.NoError(t, err)

	req := srv.last(t)
	assert.Equal(t, []string{"[]"}, req.Fields["failOnHttpStatusCodes"])
	assert.Equal(t, []string{"{}"}, req.Fields["extraHttpHeaders"])
}
