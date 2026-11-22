# GotenbergGo

A Go client for [Gotenberg](https://gotenberg.dev) 9.x, modelled on the
official PHP client [gotenberg-php](https://github.com/gotenberg/gotenberg-php).

- Supports Gotenberg 9.x; tested against **9.6.0**.
- Standard library only; immutable, fluent request builders.
- Chromium (convert and screenshot), LibreOffice, the PDF module (pipe, all
  single-operation routes, read routes) and the system routes.

## Installation

```sh
go get github.com/NicatorBa/GotenbergGo
```

```go
import gotenberg "github.com/NicatorBa/GotenbergGo"
```

## Usage

```go
client, err := gotenberg.NewClient("http://localhost:3000")
if err != nil {
	return err
}
```

### Chromium

```go
_, err = client.Chromium().ConvertFromHTML().
	IndexFromPath("report.html").
	FileFromPath("style.css").
	PaperSize(gotenberg.PaperSizeA4).
	MarginTop("1cm").
	ToFile(ctx, "report.pdf")

png, err := client.Chromium().ScreenshotFromURL("https://example.com").
	Width(1280).
	Height(720).
	ToBytes(ctx)
```

### LibreOffice

```go
// The documents are merged in the order they were added.
_, err = client.LibreOffice().Convert().
	FileFromPath("cover.docx").
	FileFromPath("figures.xlsx").
	Stages(gotenberg.StageMerge()).
	ToFile(ctx, "merged.pdf")
```

### PDF module

```go
// One operation per request.
_, err = client.PDF().Merge().
	FileFromPath("a.pdf").
	FileFromPath("b.pdf").
	ToFile(ctx, "merged.pdf")

// A pipeline of stages.
rr, err := client.PDF().Pipe().
	FileFromPath("in.pdf").
	Stages(
		gotenberg.StageStampText("DRAFT").Position(gotenberg.StampPositionCenter).Opacity(0.3),
		gotenberg.StageOptimize(),
	).
	ToReader(ctx)
defer rr.Close()

// Read routes answer with typed results, keyed by input filename.
info, err := client.PDF().Info().FileFromPath("in.pdf").Send(ctx)
fmt.Println(info["in.pdf"].Pages)
```

### Stages and their files

Files used by a stage (the image of `StageStampImage`, the source of
`StageInsert`, the certificate of `StageSign`, …) are referenced **by
filename only**. Upload them on the request builder as well:

```go
client.PDF().Pipe().
	FileFromPath("in.pdf").
	FileFromPath("logo.png").
	Stages(gotenberg.StageStampImage("logo.png"))
```

### Errors

Invalid builder arguments are returned by the terminal method, and the
request is not sent. Non-2xx answers are returned as `*gotenberg.APIError`:

```go
var apiErr *gotenberg.APIError
if errors.As(err, &apiErr) {
	log.Println(apiErr.StatusCode, apiErr.Message, apiErr.Trace)
}
```

### Authentication

```go
// Basic auth
client, err := gotenberg.NewClient(url, gotenberg.WithBasicAuth("user", "password"))

// OIDC bearer token
client, err := gotenberg.NewClient(url, gotenberg.WithHeader("Authorization", "Bearer "+token))
```

### Redirects

Request bodies are streamed and cannot be replayed, so redirects are not
supported: a redirect answer ends as an `*APIError`. A redirect in front of
Gotenberg usually means a wrong base URL; fix the base URL, or pass an
`*http.Client` with a custom `CheckRedirect` via `WithHTTPClient`.

## Development

```sh
go generate ./...          # regenerate the builders from generator/**/*.yaml
go test ./...              # unit tests
cd integration && go test -tags integration ./...   # needs Docker
```

The builders are generated from the YAML definitions in `generator/`; the
generated `*_gen.go` files are committed.

## License

MIT, see [LICENSE](LICENSE).
