//go:build integration

// Package integration tests the gotenberg package against a real Gotenberg
// server started with testcontainers-go. Docker is required: without it,
// the tests fail.
package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	gotenberg "github.com/NicatorBa/GotenbergGo"
)

const image = "gotenberg/gotenberg:9.6.0"

var client *gotenberg.Client

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	container, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("3000/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/health").WithPort("3000/tcp").WithStartupTimeout(3*time.Minute)),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start gotenberg container:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()

	endpoint, err := container.PortEndpoint(ctx, "3000/tcp", "http")
	if err != nil {
		fmt.Fprintln(os.Stderr, "container endpoint:", err)
		return 1
	}

	client, err = gotenberg.NewClient(endpoint)
	if err != nil {
		fmt.Fprintln(os.Stderr, "new client:", err)
		return 1
	}

	return m.Run()
}

const indexHTML = `<!doctype html><html><head><title>Test</title><link rel="stylesheet" href="style.css"></head>
<body><h1>Hello</h1><p>GotenbergGo</p></body></html>`

func htmlPDF(t *testing.T, opts ...func(gotenberg.ChromiumConvertFromHTML) gotenberg.ChromiumConvertFromHTML) []byte {
	t.Helper()

	b := client.Chromium().ConvertFromHTML().
		IndexFromBytes([]byte(indexHTML)).
		FileFromBytes("style.css", []byte("h1 { color: red; }"))
	for _, opt := range opts {
		b = opt(b)
	}

	res, err := b.ToBytes(t.Context())
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(res.Content, []byte("%PDF")), "result is a PDF")

	return res.Content
}

func TestSystemRoutes(t *testing.T) {
	health, err := client.Health(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "up", health.Status)

	version, err := client.Version(t.Context())
	require.NoError(t, err)
	assert.Contains(t, version, "9.6.0")

	_, err = client.Debug(t.Context())
	var apiErr *gotenberg.APIError
	if err != nil {
		require.ErrorAs(t, err, &apiErr, "debug is disabled by default")
	}
}

func TestChromiumConvertHTML(t *testing.T) {
	res, err := client.Chromium().ConvertFromHTML().
		IndexFromBytes([]byte(indexHTML)).
		HeaderFromBytes([]byte(`<html><body style="font-size:8px">header</body></html>`)).
		PaperSize(gotenberg.PaperSizeA4).
		MarginTop("1cm").
		Landscape(true).
		PrintBackground(true).
		WaitDelay(100 * time.Millisecond).
		OutputFilename("hello").
		Trace("integration-trace").
		ToBytes(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "application/pdf", res.ContentType)
	assert.Equal(t, "hello.pdf", res.Filename)
	assert.Equal(t, "integration-trace", res.Trace)
	assert.True(t, bytes.HasPrefix(res.Content, []byte("%PDF")))
}

func TestChromiumConvertMarkdownToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "md.pdf")

	info, err := client.Chromium().ConvertFromMarkdown().
		IndexFromBytes([]byte(`<html><body>{{ toHTML "doc.md" }}</body></html>`)).
		FileFromBytes("doc.md", []byte("# Title\n\nSome *markdown*.")).
		ToFile(t.Context(), path)
	require.NoError(t, err)
	assert.Equal(t, "application/pdf", info.ContentType)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(content, []byte("%PDF")))
}

func TestChromiumScreenshot(t *testing.T) {
	rr, err := client.Chromium().ScreenshotFromHTML().
		IndexFromBytes([]byte(indexHTML)).
		Width(640).
		Height(480).
		Format(gotenberg.ScreenshotFormatJPEG).
		Quality(80).
		ToReader(t.Context())
	require.NoError(t, err)
	defer func() { _ = rr.Close() }()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(rr)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", rr.ContentType)
	assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8}), "result is a JPEG")
}

func TestChromiumWithStages(t *testing.T) {
	res, err := client.Chromium().ConvertFromHTML().
		IndexFromBytes([]byte(indexHTML)).
		Stages(
			gotenberg.StageStampText("DRAFT").Position(gotenberg.StampPositionCenter).Opacity(0.3),
			gotenberg.StageSetMetadata().Title("Stamped").Author("GotenbergGo"),
		).
		ToBytes(t.Context())
	require.NoError(t, err)

	meta, err := client.PDF().Metadata().FileFromBytes("doc.pdf", res.Content).Send(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Stamped", meta["doc.pdf"].Title)
	assert.Equal(t, "GotenbergGo", meta["doc.pdf"].Author)
}

func TestLibreOfficeConvert(t *testing.T) {
	res, err := client.LibreOffice().Convert().
		FileFromBytes("notes.txt", []byte("Hello from LibreOffice")).
		Landscape(true).
		ToBytes(t.Context())
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(res.Content, []byte("%PDF")))
}

func TestMergeKeepsOrder(t *testing.T) {
	one := htmlPDF(t)
	two := htmlPDF(t, func(b gotenberg.ChromiumConvertFromHTML) gotenberg.ChromiumConvertFromHTML {
		return b.Stages(gotenberg.StageSplitRanges("1"), gotenberg.StageMerge())
	})

	res, err := client.PDF().Merge().
		FileFromBytes("z.pdf", one).
		FileFromBytes("a.pdf", two).
		FileFromBytes("m.pdf", one).
		ToBytes(t.Context())
	require.NoError(t, err)

	info, err := client.PDF().Info().FileFromBytes("merged.pdf", res.Content).Send(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, info["merged.pdf"].Pages)
}

func TestPipeAndReadRoutes(t *testing.T) {
	pdf := htmlPDF(t)

	res, err := client.PDF().Pipe().
		FileFromBytes("a.pdf", pdf).
		FileFromBytes("b.pdf", pdf).
		Stages(
			gotenberg.StageMerge(),
			gotenberg.StageRotate(90).Pages("1"),
			gotenberg.StageSetBookmarks(gotenberg.Bookmark{Title: "First", PageNumber: 1}, gotenberg.Bookmark{Title: "Second", PageNumber: 2}),
		).
		ToBytes(t.Context())
	require.NoError(t, err)

	info, err := client.PDF().Info().FileFromBytes("out.pdf", res.Content).Send(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, info["out.pdf"].Pages)

	bookmarks, err := client.PDF().Bookmarks().FileFromBytes("out.pdf", res.Content).Send(t.Context())
	require.NoError(t, err)
	require.Len(t, bookmarks["out.pdf"], 2)
	assert.Equal(t, "Second", bookmarks["out.pdf"][1].Title)

	attachments, err := client.PDF().Attachments().FileFromBytes("out.pdf", res.Content).Send(t.Context())
	require.NoError(t, err)
	assert.Empty(t, attachments["out.pdf"])

	form, err := client.PDF().Form().FileFromBytes("out.pdf", res.Content).Send(t.Context())
	require.NoError(t, err)
	assert.Empty(t, form["out.pdf"])

	report, err := client.PDF().Validate(gotenberg.ProfilePDFA2b).FileFromBytes("out.pdf", res.Content).Send(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []gotenberg.Profile{gotenberg.ProfilePDFA2b}, report["out.pdf"].Profile)
}

func TestSplitAnswersZIP(t *testing.T) {
	pdf := htmlPDF(t, func(b gotenberg.ChromiumConvertFromHTML) gotenberg.ChromiumConvertFromHTML {
		return b.Stages(gotenberg.StageMerge())
	})

	two, err := client.PDF().Merge().FileFromBytes("a.pdf", pdf).FileFromBytes("b.pdf", pdf).ToBytes(t.Context())
	require.NoError(t, err)

	res, err := client.PDF().SplitEvery(1).FileFromBytes("in.pdf", two.Content).ToBytes(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "application/zip", res.ContentType)

	zr, err := zip.NewReader(bytes.NewReader(res.Content), int64(len(res.Content)))
	require.NoError(t, err)
	assert.Len(t, zr.File, 2)
}

func TestSingleOpRoutes(t *testing.T) {
	pdf := htmlPDF(t)

	enc, err := client.PDF().Encrypt().UserPassword("secret").FileFromBytes("in.pdf", pdf).ToBytes(t.Context())
	require.NoError(t, err)

	info, err := client.PDF().Info().Password("secret").FileFromBytes("enc.pdf", enc.Content).Send(t.Context())
	require.NoError(t, err)
	assert.True(t, info["enc.pdf"].Encrypted)

	dec, err := client.PDF().Decrypt().Password("secret").DropSignatures(true).FileFromBytes("enc.pdf", enc.Content).ToBytes(t.Context())
	require.NoError(t, err)

	info, err = client.PDF().Info().FileFromBytes("dec.pdf", dec.Content).Send(t.Context())
	require.NoError(t, err)
	assert.False(t, info["dec.pdf"].Encrypted)

	text, err := client.PDF().ExtractText().FileFromBytes("in.pdf", pdf).ToBytes(t.Context())
	require.NoError(t, err)
	assert.Contains(t, string(text.Content), "Hello")
}

func TestAPIError(t *testing.T) {
	_, err := client.Chromium().ConvertFromURL("file:///etc/passwd").ToBytes(t.Context())

	var apiErr *gotenberg.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.NotEmpty(t, apiErr.Message)
	assert.False(t, errors.Is(err, gotenberg.ErrNoOutput))
}
