# AGENTS.md

Guidance for people and coding agents changing this repository. It covers
the design the code follows, which files are generated, how to add or change
an option, the checks to run, and the traps found while building it. What
the library offers is described in the README.

## Project overview

- GotenbergGo is a Go client for [Gotenberg](https://gotenberg.dev/docs) 9.x,
  tested against 9.6.0. Its reference implementation is the official PHP
  client [gotenberg-php v3.5.0](https://github.com/gotenberg/gotenberg-php/tree/v3.5.0).
- The maintainer sets the design described under "Design". Don't change a
  design rule on your own: propose the change, and once it is agreed, update
  this file in the same change as the code. Point out every rule you add that
  this file does not cover yet.
- The library uses the standard library only. testify is a test-only
  dependency, and testcontainers-go is confined to `integration/`. Don't add
  other dependencies to the root `go.mod`.
- Everything in the repository is in English: code, comments, docs and
  commit messages.

## Layout

The repository holds three modules:

| Path | Module | Content |
|---|---|---|
| `/` | `github.com/NicatorBa/GotenbergGo` | the `gotenberg` package, flat layout |
| `generator/` | `…/generator` | code generator and the YAML definitions |
| `integration/` | `…/integration` | integration tests |

- The root package is a single package with a flat layout, split into files
  by concern.
- `generator/` and `integration/` are nested modules so that their
  dependencies (`go.yaml.in/yaml/v4` v4.0.0, testcontainers-go) stay out of
  the library's `go.mod`.
- `integration/go.mod` contains `replace github.com/NicatorBa/GotenbergGo => ../`,
  so the tests always run against the working tree.
- All three modules use the same `go` directive: the oldest Go release still
  supported by the Go team, currently `go 1.26.0`. Raising it in one module
  means raising it in all three.

### Hand-written files in the root

| File | Content |
|---|---|
| `client.go` | `Client`, `NewClient`, the `With…` options, the module accessors |
| `request.go` | the shared `request` state, copy-on-write helpers, multipart streaming |
| `files.go` | file sources, archives, `fileContent` helpers, merge-order renaming |
| `download.go` | `FileFromURL` options (`WithDownload…`) |
| `values.go` | `fieldValue` encoding (form and JSON), range and enum checks |
| `stage.go` | the `Stage` interface, the shared `stage` state, JSON helper types |
| `result.go` | `ResponseInfo`, `Result`, `ResultReader`, the terminal method logic |
| `read.go` | result types of the read routes |
| `system.go` | `Health`, `Version`, `Debug` |
| `errors.go` | `APIError`, `ErrNoOutput` |
| `chromium.go`, `libreoffice.go`, `pdf.go` | module types and the entry methods of the non-op builders |
| `generate.go` | the `//go:generate` directive |
| `doc.go`, `example_test.go` | package documentation and runnable examples |

## Design

### Scope

The client covers:

- **Chromium:** convert from URL, HTML and Markdown, and screenshot from URL,
  HTML and Markdown.
- **LibreOffice:** convert.
- **PDF module:** `/forms/pdf/pipe` with stages, one builder for each of the
  29 single-op routes `/forms/pdf/{op}`, and the 6 read routes with typed
  results.
- **System routes:** `/health`, `/version` and `/debug`.

Deliberately not implemented:

- Webhooks (the asynchronous mode with the `Gotenberg-Webhook-*` headers).
- A dedicated option for OIDC bearer tokens. Callers use
  `WithHeader("Authorization", "Bearer …")`, which the `WithHeader` doc
  comment explains.

### Fluent API

The API is modelled on the Gotenberg route structure:

- `*Client` is the entry point. It has one accessor per Gotenberg module:
  `Chromium()`, `LibreOffice()` and `PDF()`.
- The accessors return module types (`Chromium`, `LibreOffice`, `PDF`) **by
  value**. A module only holds `client *Client`.
- Each module has one method per route, called an entry method. It returns
  a dedicated builder for that route.
- Every builder embeds the unexported `request` type, which holds the shared
  request state.
- A builder or module obtained as a zero value, such as `gotenberg.Chromium{}`
  or a nil `*Client`, never panics: its terminal method returns an error.

Entry methods and the builders they return share a name:

| Entry method | Builder |
|---|---|
| `Chromium().ConvertFromURL(url)` / `ConvertFromHTML()` / `ConvertFromMarkdown()` | `ChromiumConvertFromURL` / `…HTML` / `…Markdown` |
| `Chromium().ScreenshotFromURL(url)` / `ScreenshotFromHTML()` / `ScreenshotFromMarkdown()` | `ChromiumScreenshotFromURL` / `…HTML` / `…Markdown` |
| `LibreOffice().Convert()` | `LibreOfficeConvert` |
| `PDF().Pipe()` | `PDFPipe` |
| `PDF().<Op>(…)`, such as `Merge()`, `StampText(text)`, `ExtractAttachments()` | `PDF<Op>` |
| `PDF().SplitEvery(n)` / `SplitRanges(ranges...)` | both `PDFSplit` |
| `PDF().Info()`, `Metadata()`, `Bookmarks()`, `Attachments()`, `Form()` | `PDFInfo`, `PDFMetadata`, … |
| `PDF().Validate(profile)` | `PDFValidate` |

The required fields of a route are parameters of its entry method, such as
`ConvertFromURL(url string)`. Optional fields are methods on the builder.

### Immutable builders

Builders are value types, and every option method works copy-on-write:

1. The method takes the builder by value.
2. It copies the state it modifies (headers, form fields, files, …).
3. It applies the change to the copy and returns the builder.

So two chains branching off the same builder never affect each other, and a
builder can be sent more than once. The details:

- `newRequest` initialises every reference-typed field to an empty, non-nil
  value. `http.Header(nil).Clone()` returns nil, and a following `Set`
  would panic.
- The helpers on `request` are unexported. Every public method is defined
  on the concrete builder type and returns that type, so a chain never
  loses its builder type.
- Bool methods always take a `bool` parameter (`Landscape(true)`,
  `SkipNetworkIdleEvent(false)`), also for options whose default is true.
  A field is only sent when its method was called.
- A repeated call replaces the previous value. This also holds for
  `Stages(...)` and for the fixed files `Index…`, `Header…` and `Footer…`.
- The header methods are generated for every request builder:
  - `Trace(id)` sets `Gotenberg-Trace`.
  - `OutputFilename(name)` sets `Gotenberg-Output-Filename` and exists only
    on builders that answer with a file.

  Both wrap the hand-written helpers `setTrace` and `setOutputFilename`.

### Enums and JSON form fields

- Fields with a fixed set of values use Go types: string-based, or int-based
  for numeric sets such as `NativeMaxImageResolution`. Constants carry the
  type name as prefix, such as `PaperSizeA4` or `ProfilePDFA2b`.
- Every enum type has an unexported `valid() bool`, which the client-side
  validation uses.
- Form fields whose value is JSON take typed Go parameters, and the builder
  encodes them:
  - `Cookies(...Cookie)`
  - `ExtraHTTPHeaders(map[string]string)`
  - `FailOnHTTPStatusCodes(...int)`
  - `FailOnConsoleMessageTypes(...ConsoleMessageType)`
  - `EmulatedMediaFeatures(map[string]string)`
- `ExtraHTTPHeaders` takes a plain map. A header limited to matching URLs
  carries the server syntax `;scope=<regex>` in its value.
- A nil slice or map is sent as `[]` or `{}`, never as `null`.

### File inputs

Builders that accept files offer six methods to add files:

```go
FileFromPath(path string)
FileFromPathAs(path, filename string)
FileFromFS(fsys fs.FS, name string)
FileFromFSAs(fsys fs.FS, name, filename string)
FileFromReader(filename string, reader io.Reader)
FileFromBytes(filename string, data []byte)
```

- The sent filename is `filepath.Base(path)` or `path.Base(name)`. The
  `…As` variants send the given filename instead, for instance to upload
  `report.html` as `index.html`.
- The path and FS variants only store the reference. The file is opened
  when the request is sent.
- Every `…FromReader` method reads the reader completely when it is called
  and keeps the content in memory, so the request no longer depends on the
  reader. Its doc comment says so and recommends the `…FromPath` variant
  for large files.
- `FileFromBytes` keeps the caller's slice, which must not be modified
  afterwards.

ZIP archives that the server unpacks use the parallel `ArchiveFrom…`
family: `ArchiveFromPath`, `ArchiveFromFS`, `ArchiveFromReader(reader)` and
`ArchiveFromBytes(data)`.

- There are no `…As` variants, because the archive's name is irrelevant to
  the server.
- Archives from memory are uploaded as `archive1.zip`, `archive2.zip`, ….
- A ZIP added via `FileFrom…` stays a ZIP.

The multipart field names:

- `FileFrom…`, `IndexFrom…`, `HeaderFrom…` and `FooterFrom…` upload under
  `files`.
- `ArchiveFrom…` uploads under `archives`.

Fixed files have their own methods, one family each, as `type: fileContent`
fields:

- `IndexFromPath/FS/Reader/Bytes` uploads `index.html`, which the Chromium
  HTML and Markdown routes require. It is not an entry method parameter; a
  missing index is reported by the server (400).
- `HeaderFrom…` and `FooterFrom…` upload `header.html` and `footer.html` on
  the Chromium convert routes.

### Server-side downloads

`FileFromURL(url string, opts ...DownloadOption)` makes the server download
the file itself:

- Each call adds one entry `{url, extraHttpHeaders, filename}`. All entries
  are sent as a single JSON-encoded form field `downloadFrom`.
- `WithDownloadFilename(filename)` sets the filename.
  `WithDownloadHeader(name, value)` adds an extra header and may be given
  several times.
- `DownloadOption` is a functional option (`func(*download)`).
- Download options use the prefix `WithDownload…`. The plain `With…` prefix
  is reserved for client options.
- The URL is not validated client-side. The server answers 400 for a
  non-absolute or non-http(s) URL.

### Merge order

The server orders input files alphanumerically by filename, not by upload
order. The client therefore renames the input documents when a request
merges them, so that the merge follows the caller's order:

- **When:** the request has a `merge` stage in `stages`, or is the
  `PDFMerge` builder.
- **Which builders:** only those whose uploaded files are the input
  documents, which are `LibreOfficeConvert`, `PDFPipe` and `PDFMerge` (the
  `request.inputs` flag). On the Chromium builders the uploaded files are
  `index.html` and its assets, and are never renamed.
- **How:** every input document added via `FileFrom…` gets a zero-padded
  index prefix in the order it was added, such as `0001_a.pdf`. The width is
  at least 4 digits and grows with the number of files. This is the approach
  of gotenberg-php.
- **Not renamed:**
  - Files whose filename appears in any stage option of the request, such
    as `image`, `source`, `file`, `certificate`, `key`, `xml` and
    `compose.placements[].source`. Generated code marks these options with
    `fileRef`.
  - Fixed files such as `index.html`.
- **What the order guarantee covers:** only files added via `FileFrom…`.
  Files from `ArchiveFrom…` are named by the archive entries, and files from
  `FileFromURL` are named by the server unless `WithDownloadFilename` is
  given; neither can be prefixed.

The doc comments of `StageMerge` and `PDF().Merge()` document these limits,
including the corner case of `merge` combined with `compose`.

### Stages

PDF operations are stages. Each one has a fluent, immutable stage builder:

```go
gotenberg.StageMerge()
gotenberg.StageSplitEvery(2)
gotenberg.StageStampText("DRAFT").Position(gotenberg.StampPositionCenter).Opacity(0.3)
gotenberg.StageEncrypt().UserPassword("secret").Algorithm(gotenberg.EncryptionAlgorithmAES256)
```

How stages are named and passed:

- All stage builders implement the interface `Stage`. Its method is
  unexported, so only this package can implement it.
- A stage is passed via `Stages(stages ...Stage)` to the Chromium convert
  builders, `LibreOfficeConvert` and `PDFPipe`. The stages are encoded as a
  JSON array into the form field `stages`.
- The constructor is the function `Stage<Op>(…)`, which returns the exported
  type `<Op>Stage`. A Go function and a type cannot share a name; for
  example, `StageStampText("DRAFT")` returns `StampTextStage`.

Which options become parameters:

- Required options are constructor parameters, as in `StageStampText(text)`,
  `StageRotate(angle)`, `StageStampImage(image)` and
  `StageRemediate(profile)`. Optional options are methods.
- The same rule applies to the entry methods of the single-op builders, such
  as `PDF().Rotate(angle)`.
- Mutually exclusive variants get separate constructors that share one type:
  `StageSplitEvery(n)` and `StageSplitRanges(ranges...)` both return
  `SplitStage`.
- `StageEncrypt()` has no required parameter, although at least one of
  `UserPassword` and `OwnerPassword` is needed. The server checks that.
- Operations marked *drop* in the stage table (see "Gotenberg server API")
  get a `DropSignatures(bool)` method on both builders.

How stage options map to the routes:

- The options of a stage and the form fields of its single-op route are
  identical. Both builders are generated from one op YAML.
- In the stage's JSON object, options use native JSON values:
  - `bool` becomes `true`/`false`, `int` and `float` become numbers;
  - `json` is embedded unquoted;
  - `duration`, `length`, `enum` and `string` become strings.

Files used by a stage are referenced **by filename only**:

- This applies to files such as `stamp-image.image`, `insert.source`,
  `stamp-pdf.source`, `attach.file`, `sign.certificate`/`key` and
  `remediate.xml`, and to the single-op routes as well.
- The caller uploads the file separately on the request builder via
  `FileFrom…`, `ArchiveFrom…` or `FileFromURL`. The package documentation
  and the doc comments of the affected methods describe this.
- The client does not check that a referenced file was uploaded, since it
  may come from an archive or a download. The server is authoritative.

```go
client.PDF().Pipe().
	FileFromPath("in.pdf").
	FileFromPath("logo.png").
	Stages(gotenberg.StageStampImage("logo.png"))
```

Profiles and helper types:

- **Profile pairs:** the `profile` option of `remediate` and of the validate
  route takes one profile as the constructor or entry parameter. A
  generated `PairedWith(profile)` method sets the second profile of a
  PDF/A–PDF/UA pair, and a repeated call replaces it.
  - In a form, the field is sent once, or twice for a pair. In stage JSON,
    it is a string, or an array of the two.
  - Both values are validated against `Profile`. The pairing rule is checked
    by the server only.
- **Helper types:** `Bookmark` (also returned by the bookmarks read route),
  `Placement`, `RedactRegion` and `Permissions` (encrypt) are hand-written
  structs matching the server's JSON.
  - `Permissions` always sends all seven keys, so a permission left false
    is denied. Without `Permissions(…)`, the server allows everything.
  - `set-form` takes `map[string]any`, whose values are a string or a
    `[]string`.

### Terminal methods and results

Builders that answer with a file:

```go
ToFile(ctx context.Context, path string) (*ResponseInfo, error)
ToBytes(ctx context.Context) (*Result, error)
ToReader(ctx context.Context) (*ResultReader, error)
```

- The answer is a single file, such as `application/pdf`, or a ZIP
  (`application/zip`) when there are several results.
- `ResponseInfo` is filled from the response headers:

  | Field | Source |
  |---|---|
  | `Filename` | the `filename` parameter of `Content-Disposition`, parsed with `mime.ParseMediaType` |
  | `ContentType` | `Content-Type` |
  | `ContentLength` | the content length; `-1` if unknown, as in `http.Response` |
  | `Trace` | `Gotenberg-Trace` |
  | `Version` | `Gotenberg-Version` |
  | `RemediationResidual` | `Gotenberg-Remediation-Residual`; `-1` when absent |
  | `RemediationRolledBack` | `Gotenberg-Remediation-Rolled-Back`; `-1` when absent |

- `Result` embeds `ResponseInfo` and adds `Content []byte`.
- `ResultReader` embeds `ResponseInfo` and implements `io.ReadCloser`.
  `Close` is idempotent, the caller must close the reader, and it is not
  safe for concurrent use.
- `ToFile` deletes the target file when writing fails or the context is
  cancelled, so no partial file is left behind.
- A 2xx response without content returns the sentinel `ErrNoOutput`.

The read routes answer with JSON and have `Send(ctx)` instead:

```go
func (b PDFInfo) Send(ctx context.Context) (map[string]DocumentInfo, error)
func (b PDFMetadata) Send(ctx context.Context) (map[string]DocumentMetadata, error)
func (b PDFBookmarks) Send(ctx context.Context) (map[string][]Bookmark, error)
func (b PDFAttachments) Send(ctx context.Context) (map[string][]Attachment, error)
func (b PDFForm) Send(ctx context.Context) (map[string][]FormField, error)
func (b PDFValidate) Send(ctx context.Context) (map[string]ValidationReport, error)
```

- The result is keyed by input filename, and `Send` returns only the decoded
  map, without `ResponseInfo`.
- The read builders accept PDFs via `FileFrom…` and offer
  `Password(string)`. `PDFForm` adds `Pages(string)`, and `PDFValidate`
  takes the profile as entry parameter (`Validate(profile).PairedWith(…)`).
- The result types are named `Document…` (`DocumentInfo`,
  `DocumentMetadata`) because the builders already use the `PDF<Op>` names.
- `ValidationReport.Profile` is a `[]Profile`. The server sends a string or
  an array, and `UnmarshalJSON` normalises it.

### System routes

The system routes are plain methods on `*Client`, without a builder:

```go
func (c *Client) Health(ctx context.Context) (*HealthStatus, error)
func (c *Client) Version(ctx context.Context) (string, error)
func (c *Client) Debug(ctx context.Context) (json.RawMessage, error)
```

- `Health` sends `GET /health`. A 503 that carries a health report is **not**
  an error: it is decoded and returned with `Status: "down"`. Other non-2xx
  answers, and a 503 without a report, return `*APIError`.
- `Version` sends `GET /version` and returns the plain text body, trimmed.
- `Debug` sends `GET /debug` and returns the raw JSON, whose structure is not
  stable enough for a Go type. When the route is disabled server-side, the
  answer is 404, returned as `*APIError`.
- The client options (`WithBasicAuth`, `WithHeader`) apply to these requests
  too.

### Client configuration

```go
func NewClient(baseURL string, opts ...ClientOption) (*Client, error)

type ClientOption func(*Client)

func WithHTTPClient(c *http.Client) ClientOption // default: http.DefaultClient
func WithBasicAuth(username, password string) ClientOption
func WithHeader(name, value string) ClientOption // sent with every request
```

`NewClient` checks its arguments:

- `baseURL` must be an absolute http or https URL; otherwise `NewClient`
  returns an error. A trailing `/` is trimmed, and a path is kept as a
  prefix of every route.
- `WithBasicAuth` combined with `WithHeader("Authorization", …)` is a
  conflict, and `NewClient` returns an error.
- `WithHTTPClient(nil)` and an empty header name are errors too.

How the options behave:

- A header set on a builder, such as `Gotenberg-Trace`, overrides a header
  of the same name from `WithHeader`. A repeated `WithHeader` replaces the
  value.
- A caller-supplied `*http.Client` is stored and used as is.
- `*Client` is immutable after `NewClient` and safe for concurrent use.

### Errors

- A non-2xx response is returned as `*APIError`, except for the 503 health
  report:

  ```go
  type APIError struct {
  	StatusCode int
  	Message    string // response body, Gotenberg's error message
  	Trace      string // Gotenberg-Trace header
  }
  ```

  Callers inspect it with `errors.As`. `Message` is read from at most the
  first 64 KiB of the body.
- Errors from building the request, I/O and transport are wrapped with `%w`,
  so `errors.Is(err, context.Canceled)` and similar checks work.
- Every error message starts with `gotenberg: `.

### Deferred errors and validation

- Option methods cannot return an error without breaking the fluent chain.
  An error raised while building is stored in the builder instead, and the
  terminal method returns it without sending the request. Examples are a
  failing read in `FileFromReader` and an empty filename.
- The general rule is that the server is authoritative, so the client does
  not validate values. The exceptions:
  - Enums, and the numeric ranges the server documents (`min`/`max`), are
    validated. A violation is a deferred error.
  - Invalid builder arguments are deferred errors: an empty path or
    filename, a nil reader or `fs.FS`, an invalid FS name, NaN/Inf, or an
    empty `OutputFilename`.
  - `NewClient` validates its arguments.
- The PDF/A–PDF/UA pairing rule, the stage ordering rules and the server's
  cross-field rules are checked by the server only.

### Transport

- The multipart body is streamed with `io.Pipe` and `mime/multipart.Writer`;
  the request is never buffered in memory.
- Every file reference (the path and FS variants of all `…From…` methods) is
  opened before the request starts. If one cannot be opened, nothing is sent
  and the error is returned.
- The streamed body cannot be replayed, and `Request.GetBody` is not set.
  Redirects are therefore not supported. This is documented on `NewClient`
  and in the package docs, not worked around:
  - On 301/302/303, Go's `http.Client` follows with a GET without body, and
    the server answers with an error, returned as `*APIError`.
  - On 307/308, Go does not follow, because the body cannot be replayed. The
    3xx response is returned as `*APIError`.
  - The docs recommend fixing the base URL, or passing an `*http.Client`
    with a custom `CheckRedirect` via `WithHTTPClient`.
- There are no automatic retries and no library-level timeouts. Callers use
  the context of the terminal method or `http.Client.Timeout`.

## Generated code

- Every `*_gen.go` and `*_gen_test.go` file is generated. Never edit one by
  hand: change the YAML or the generator, then run `go generate ./...` from
  the repository root, and commit the regenerated files together with the
  change. CI fails if the committed files differ from the generator output.
- `generate.go` invokes the generator with
  `//go:generate go -C generator run . -out ..`. A plain
  `go run ./generator` does not work across module boundaries.
- The generator deletes every `*_gen*.go` file in the root before it writes
  new ones. Never give a hand-written file a name containing `_gen`.
- Every generated file starts with `// Code generated by … DO NOT EDIT.`.

What is generated and what is written by hand:

- **Generated:** the builder types with all their option, file, header and
  terminal methods; the unexported setters `set<Name>` of the required
  fields; the op entry methods `PDF().<Op>()` and stage constructors
  `Stage<Op>()`; the enum types; and one table test file per definition.
- **Hand-written:** `Client`, `request`, `stage`, the module types, and the
  entry methods of the non-op builders, such as `Chromium().ConvertFromURL`
  and `PDF().Validate`.

### Definition files

| Location | Content | Generates |
|---|---|---|
| `generator/groups/*.yaml` | fields shared by several builders | nothing on its own |
| `generator/builders/*.yaml` | one request builder (Chromium, LibreOffice, `PDFPipe`, read routes) | `<file>_gen.go` + `<file>_gen_test.go` |
| `generator/ops/*.yaml` | one PDF operation | `PDF<Op>`, `<Op>Stage` and `Stage<Op>` in `pdf_<file>_gen.go` + `pdf_<file>_gen_test.go` |
| `generator/enums.yaml` | all enum types | `enums_gen.go` + `enums_gen_test.go` |

Output files are named after the YAML file, flat in the root. For example,
`builders/chromium_convert_url.yaml` produces `chromium_convert_url_gen.go`,
and `ops/stamp_text.yaml` produces `pdf_stamp_text_gen.go`.

Groups are pulled in with `include`. This is a convention of the generator,
not a YAML feature:

- The generated methods follow the definition order: the included groups in
  `include` order, then the file's own fields.
- A group must not include another group.
- Builder and op files can both use `include`.

Builder file:

```yaml
type: ChromiumConvertFromURL      # Go type name
route: /forms/chromium/convert/url
entry: Chromium().ConvertFromURL  # call returning the builder in generated tests
doc: It converts a URL to PDF with Chromium.
output: file                      # file → ToFile/ToBytes/ToReader; json → Send
files: true                       # FileFrom…/ArchiveFrom…/FileFromURL methods
stages: true                      # Stages(...Stage) method
include: [chromium_common, paper, chromium_pdf]
fields:
  - name: URL                     # Go method name
    form: url                     # form field name
    type: string
    required: true                # → entry method parameter, setter setURL
    example: https://example.com
```

With `output: json`, the builder file also names the result type, as in
`result: map[string]DocumentInfo`.

Op file:

```yaml
op: stamp-text                    # route /forms/pdf/stamp-text, JSON "op"
name: StampText                   # → PDFStampText, StampTextStage, StageStampText(), PDF().StampText()
doc: It draws a line of text over the pages.
dropSignatures: false             # true → DropSignatures(bool) on both builders
include: [stamp]
fields:
  - { name: Text, form: text, type: string, required: true, example: DRAFT }
  - name: FontSize
    form: fontSize
    type: float
    min: 0
    minExclusive: true
```

Mutually exclusive variants are listed as `variants`. Each variant has its
own constructor and entry method, and its own required fields:

```yaml
op: split
name: Split
dropSignatures: true
variants:
  - name: Every                   # → StageSplitEvery(every int), PDF().SplitEvery(every int)
    fields:
      - { name: Every, form: every, type: int, required: true, min: 1 }
  - name: Ranges                  # → StageSplitRanges(ranges ...string)
    fields:
      - { name: Ranges, form: ranges, type: json, goType: "[]string", required: true, variadic: true, example: '[]string{"1-3"}' }
```

Enum file:

```yaml
- name: PaperSize
  doc: Is a named paper size, setting both the paper width and height.
  values:
    - { const: PaperSizeA4, value: A4 }
- name: NativeMaxImageResolution
  base: int                       # int-based enum
  values:
    - { const: NativeMaxImageResolution75, value: 75 }
```

### Field types

| `type` | Go parameter | Form value | Notes |
|---|---|---|---|
| `string` | `string` | as is | |
| `bool` | `bool` | `true` / `false` | the method always takes a `bool` |
| `int` | `int` | decimal | `min`/`max` validated client-side |
| `float` | `float64` | `strconv.FormatFloat(v, 'f', -1, 64)` | `min`/`max`/`minExclusive` |
| `duration` | `time.Duration` | `d.String()`, such as `1.5s` | |
| `length` | `string` | as is | such as `"8.5in"` or `"20mm"`; the server parses it |
| `enum` | the enum type (`enum:`) | its value | validated client-side |
| `json` | `goType`, such as `[]int` or `map[string]string` | `json.Marshal` | `variadic: true` → `...T` for slices; enum slice elements are validated |
| `profileSet` | `Profile` | sent once, or repeated for a pair | always `required`; adds `PairedWith` |
| `fileContent` | file source | multipart file named `file:` | generates `<Name>FromPath/FS/Reader/Bytes`; request builders only |

Field keys:

- Common keys: `name`, `form`, `type`, `doc`, `required`, `min`, `max`,
  `minExclusive`, `enum`, `goType`, `variadic`, `file` (`fileContent` only)
  and `example`.
- `example` is the value used by the generated test. When it is omitted, it
  is derived from the type; for `json` fields it is required, written as a
  Go expression.
- `fileRef: true` marks an option naming uploaded files, which merge order
  must not rename. It applies to a `string`, or to `[]Placement`.

Rules the generator enforces:

- A duplicate form field or method name within a builder is an error, also
  when it comes from two groups.
- A `json` field needs both `goType` and `example`.
- A `profileSet` field must be `required`.
- `min` and `max` apply only to `int` and `float`.
- The YAML files are loaded with known-field checking, so a misspelt key is
  an error, not a silently ignored option.

The generated tests are table tests against `httptest.Server`. Each field
gets an entry that:

- calls the method with `example`,
- sends the request,
- and asserts the encoded value of the form field (or, for stages, the JSON
  key).

Fields with `min`/`max`/`enum` also get cases asserting the deferred error
and that no request was sent.

## Changing the API

- **Add or change an option:** edit the field in the builder, op or group
  YAML, then run `go generate ./...`.
  - The `doc` text becomes the method comment, prefixed with the method
    name: `doc: Sets the scale…` turns into `// Scale sets the scale…`.
  - Use `min`/`max`/`minExclusive` only for ranges the server documents.
    Otherwise leave validation to the server.
- **Add an enum:** add it to `enums.yaml`, with constants prefixed by the
  type name.
- **Add a PDF operation:** add `ops/<op>.yaml`.
  - Set `dropSignatures: true` if the server marks the operation *drop*.
  - Use `variants` for mutually exclusive required options.
  - Mark options naming uploaded files with `fileRef: true`.
  - Update the operation count under "Scope" and the CHANGELOG.
- **Add a request builder that is not an op:** add `builders/<name>.yaml`
  and write the entry method by hand in the module file (`chromium.go`,
  `pdf.go`, …).
  - The entry method calls the generated setter `set<Name>` for each
    required field.
  - Set `r.inputs = true` when the uploaded files are the input documents.
- **Change behaviour all builders share** (files, headers, terminal
  methods): change the hand-written helper on `request`. Touch the generator
  only if the generated wrapper itself must change.
- **Change a JSON helper type** (`Cookie`, `Placement`, `RedactRegion`,
  `Permissions`, `Bookmark`): edit the Go struct by hand. Match the JSON
  names and `omitempty` to the server's struct (see "Server source").

## Conventions

- **Copy before you modify.** Every helper that changes a map or slice
  copies it first:
  - a map: `maps.Clone`;
  - appending to a slice: `append(slices.Clip(s), x)`;
  - changing slice elements: `slices.Clone`.

  `TestBuildersAreImmutable` and `TestStagesAreImmutable` catch violations.
  Extend them when you add state.
- **Record errors, don't return them.** An option method never panics and
  never returns an error. It records the error (`request.addErr`,
  `stage.set`), and the terminal method returns it without sending the
  request.
- **Error prefix.** The messages of the field checks carry no prefix;
  `addErr` adds `gotenberg: `. Every other error returned to the caller
  starts with `gotenberg: ` itself.
- **Stage files are named by filename only.** Never upload a file implicitly
  from a stage builder.

## Checks before sending a change

Run these from the repository root:

```sh
go generate ./...
gofmt -l .                # must print nothing
go vet ./...
(cd generator && go vet ./...)
(cd integration && go vet -tags integration ./...)
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
(cd generator && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...)
go test ./...
(cd integration && go test -tags integration -count=1 ./...)   # needs Docker
```

- Integration tests fail, not skip, when Docker is unavailable. That is
  intended; don't add a skip.
- CI runs `go test -race` on Go 1.26 and 1.27. `-race` needs cgo, so it
  fails to build on a machine without a C compiler (such as Windows without
  gcc). Run without `-race` locally then, and rely on CI.
- Pin staticcheck to a release that supports the Go version in use. Older
  releases fail with "export data version … is greater than maximum
  supported".

## Tests

- **Assertions:** use [testify](https://github.com/stretchr/testify),
  `require` for preconditions and `assert` otherwise. Compare JSON with
  `assert.JSONEq`, not string equality.
- **Unit tests** live in the root module next to the code, with no build tag
  and no Docker.
  - They check requests against an `httptest.Server`: method, path, headers,
    multipart fields and files. The test server simulates both success and
    error responses.
  - Use the helpers in `helpers_test.go` (`newFileServer`, `newJSONServer`,
    `newTestServer`) instead of writing new servers.
  - The hand-written tests are `client_test.go`, `request_test.go`,
    `result_test.go`, `system_test.go` and `read_test.go`. Put a new test
    into the file of the code it covers.
- **Generated tests** cover every field (see "Generated code"). Give
  `example` a realistic value wherever the type's default would be
  meaningless.
- **Table tests:** use one for three or more similar cases.
- **Integration tests:** they live in `integration/integration_test.go`.
  - They are guarded by the build tag `integration` and run with
    `go test -tags integration ./...` inside `integration/`. The tag is kept
    on purpose in addition to the nested module, so a plain `go test ./...`
    there does not start containers.
  - They start `gotenberg/gotenberg:9.6.0`, pinned to the exact tag, via
    testcontainers-go.
  - When Docker is unavailable, they **fail**; never add a skip.
  - A new route or a change of the wire format deserves one integration
    case; a new option method does not.

## Versioning, CI and documentation

- **Versioning:** SemVer, starting with `v0.x.y` tags on `main`. The library
  version is independent of the Gotenberg version. The README states the
  supported Gotenberg version (9.x, tested against 9.6.0).
- **Go versions:** CI tests the two Go releases the Go team supports, 1.26
  and 1.27.
- **CI** runs GitHub Actions, `.github/workflows/ci.yml`, with three jobs:
  - `checks`: gofmt, go vet, staticcheck (pinned), and a check that the
    generated files are up to date (`go generate ./...`, then
    `git diff --exit-code`);
  - `unit`: unit tests with `-race` on both Go versions;
  - `integration`: integration tests in `integration/` (needs Docker).
- **README:** installation, a short example per module, notes on stages and
  their files, redirects, and bearer tokens via `WithHeader`. Update it when
  one of these changes.
- **CHANGELOG:** `CHANGELOG.md` follows
  [Keep a Changelog](https://keepachangelog.com/). Record user-facing
  changes under `## [Unreleased]`.
- **Examples:** runnable `Example…` functions in `example_test.go`, which
  pkg.go.dev renders.

## Traps

- **YAML flow mappings:** `{ name: X, doc: a, b }` cuts the doc at the
  comma. Quote a `doc` containing a comma, or use the block form.
- **The `y` key:** quote it (`form: "y"`), so that no YAML reader takes it
  for a boolean.
- **Generator imports:** the generator derives a file's imports from the
  package names used in its code lines, not in its comments. A generated
  string literal mentioning `time.` or `json.` adds an unused import.
- **Request lifecycle:** `send` waits for the body writer to finish, so no
  file handle outlives the call. On Windows an open handle blocks the
  removal of `t.TempDir()`.
- **Server-side checks:** keep cross-field and ordering rules in the server.
  Duplicating them in the client goes stale when the server changes them.

## Gotenberg server API

This section documents Gotenberg 9.6.0 for reference. The design above
decides what the client implements. The option lists of the routes live in
the YAML definitions, with the server defaults in their `doc` texts.

### General behaviour

- All conversion and processing routes take `POST` with
  `multipart/form-data`, under the path prefix `/forms`.
- **Files:**
  - Every multipart file part is an input file, named by its filename. The
    conventional field name is `files`.
  - Parts in the field `archives` are ZIPs the server unpacks; each entry
    counts as a file under `files`. A ZIP under `files` stays a ZIP.
- **Downloads:** the form field `downloadFrom` holds a JSON array of
  `{url, extraHttpHeaders, filename}`. The server downloads the files before
  processing. It can be disabled server-side.
- **Input order:** where files are processed in sequence (such as merge),
  the server sorts them alphanumerically by filename, and breaks ties by
  upload order.
- **Unknown form fields** are rejected with 400.
- **Request headers:**
  - `Gotenberg-Output-Filename`: the output filename without extension.
  - `Gotenberg-Trace`: the correlation id. The header name is configurable
    server-side; the default is `Gotenberg-Trace`.
  - `Gotenberg-Webhook-Url`, `-Events-Url`, `-Method` and
    `-Extra-Http-Headers` select the asynchronous mode (not implemented).
    These are not allowed on routes that answer with JSON.
- **Response headers:**
  - always: `Content-Disposition`, `Content-Type`, `Gotenberg-Trace` and
    `Gotenberg-Version`;
  - after a `remediate` stage: `Gotenberg-Remediation-Residual` and
    `Gotenberg-Remediation-Rolled-Back`, both integer counts.
- **Response body:**
  - one output file as is,
  - several outputs as `application/zip`,
  - or JSON on the read routes.

  Errors are non-2xx answers with a `text/plain` message.
- **Authentication:** HTTP Basic auth or OIDC (`Authorization: Bearer
  <token>`). The server enables at most one of them.

### Routes

| Route | Required input | Notes |
|---|---|---|
| `GET /health` (also `HEAD`) | – | JSON report `{"status":"up",…}`, 503 when down |
| `GET /version` | – | plain text, such as `9.6.0` |
| `GET /debug` | – | JSON, only when enabled server-side |
| `/forms/chromium/convert/url` | form field `url` (absolute http/https) | `stages` allowed |
| `/forms/chromium/convert/html` | file `index.html` | `stages` allowed |
| `/forms/chromium/convert/markdown` | `index.html` (template calling `{{ toHTML "file.md" }}`) + `.md` files | `stages` allowed |
| `/forms/chromium/screenshot/url` | form field `url` | no `stages` |
| `/forms/chromium/screenshot/html` | file `index.html` | no `stages` |
| `/forms/chromium/screenshot/markdown` | `index.html` + `.md` files | no `stages` |
| `/forms/libreoffice/convert` | one or more office documents | `stages` allowed |
| `/forms/pdf/pipe` | PDFs + `stages` (required) | |
| `/forms/pdf/{op}` | PDFs + the op's required options | one route per stage |
| `/forms/pdf/{info,metadata,bookmarks,attachments,form,validate}` | PDFs | JSON answer |

Chromium:

- Further uploaded files on the Chromium routes are assets, such as images,
  CSS and fonts.
- `paperSize` is case-insensitive. It sets both width and height, and
  `paperWidth`/`paperHeight` then override one side at a time.
- Lengths are a number of inches, or carry a unit (`pt`, `px`, `in`, `mm`,
  `cm`, `pc`).
- `failOnHttpStatusCodes` defaults to `[499,599]`, where x99 stands for the
  whole class.

LibreOffice:

- Each document becomes its own PDF, and several are answered as a ZIP
  unless a `merge` stage is used.
- Files referenced by a stage are not converted.
- `nativePdfua` accepts only `PDF/UA-1`, and not together with `PDF/A-4`.

### Stages

Stages are JSON objects `{"op": "<name>", …options}` with typed JSON values.
They run in one of three ways:

1. on a conversion (Chromium convert, LibreOffice), via the optional form
   field `stages`;
2. on uploaded PDFs via `/forms/pdf/pipe`, where `stages` is required;
3. one stage per request via `/forms/pdf/{op}`, with the options as flat
   form fields under their JSON names.

On the single-op routes of option 3, option values are encoded like this:

- bools as `true`/`false`, numbers as text;
- lists and objects as JSON;
- a `profile` pair as the field repeated.

Files a stage refers to are uploaded like any other file and referenced by
filename. They are not input documents.

Options marked *drop* accept `dropSignatures` (bool), which is required to
rewrite a signed PDF.

| op | Options | Notes |
|---|---|---|
| `merge` | *drop* | all documents into one |
| `split` | `every` int ≥ 1 **or** `ranges` []string; *drop* | |
| `select` | `pages` (req.); *drop* | keeps pages in the given order |
| `delete` | `pages` (req.); *drop* | |
| `insert` | `source` file (req.), `at` int ≥ 0, `sourcePages`; *drop* | |
| `rotate` | `angle` int (req., multiple of 90), `pages` | |
| `stamp-text` | `text` (req.), `pages`, `x`, `y`, `position`, `fontSize` > 0, `font`, `color` `#rrggbb`, `opacity` 0–1, `rotation` | |
| `stamp-image` | `image` PNG/JPEG file (req.), `pages`, `x`, `y`, `position`, `width`, `height`, `opacity`, `rotation` | |
| `stamp-pdf` | `source` file (req.), `sourcePage` int ≥ 0, `pages`, `x`, `y`, `position`, `scale` > 0, `opacity`, `rotation` | |
| `compose` | `paperSize` / `paperWidth` / `paperHeight`, `placements` [] (`source`, `sourcePage`, `page`, `x`, `y`, `scale`, `rotation`); *drop* | n-up / imposition |
| `set-metadata` | `title`, `author`, `subject`, `keywords`, `creator`, `producer`, `creationDate`, `modDate` (PDF dates) | |
| `set-bookmarks` | `bookmarks` [] `{title, pageNumber, children}` (req.) | |
| `set-form` | `fields` map name → string or []string (req.) | fills the form |
| `flatten-form` | *drop* | |
| `flatten-transparency` | *drop* | |
| `convert-colors` | `target` (req.: `cmyk`, `rgb`, `gray`, `output-intent`); *drop* | |
| `redact` | `regions` [] `{page, x, y, width, height}` (req.); *drop* | |
| `autotag` | – | structure tree for untagged PDFs |
| `attach` | `file` (req.), `name`, `mimeType`, `relationship` (`Source`, `Data`, `Alternative`, `Supplement`, `Unspecified`) | |
| `detach` | `name` (req.) | |
| `optimize` | `objectStreams`, `dedup`, `recompressImages`, `subsetFonts` (bool, default true); *drop* | lossless |
| `encrypt` | `userPassword` and/or `ownerPassword`, `algorithm` (`aes256` default, `aes128`, `rc4-128`, `rc4-40`), `permissions` {`print`, `printHighQuality`, `modifyContent`, `extractText`, `modifyAnnotations`, `fillForms`, `assemble`} (default true); *drop* | |
| `decrypt` | `password`; *drop* | must come first to open encrypted inputs |
| `remediate` | `profile` (req.; one profile, or a pair PDF/A + PDF/UA), `language`, `aggressive`, `figureAltText`, `xml` file, `level`, `documentType`; *drop* | see below |
| `sign` | `certificate` file (req.; PKCS#12, or PEM with a `key` file), `key`, `certificatePassword`, `reason`, `location`, `contactInfo`, `name`, `pades`, `timestampUrl` | |
| `timestamp` | `timestampUrl` (req.) | RFC 3161 |
| `render` | `pages`, `dpi` ≥ 1, `format` (`png`/`jpeg`), `quality` 1–100 (jpeg only) | adds page images to the output |
| `extract-attachments` | `name` | outputs the embedded files |
| `extract-text` | `pages`, `invisible`, `artifacts`, `annotations` | outputs `.txt` |

Values used by several options:

- `position`: `center`, `top-left`, `top`, `top-right`, `left`, `right`,
  `bottom-left`, `bottom`, `bottom-right`.
- `font`: the 14 standard PDF fonts (Helvetica, Times and Courier with their
  variants, Symbol and ZapfDingbats).
- `pages`: a page selection, such as `1-3,5`, `odd` or `last`.

`remediate` profiles and options:

- **Profiles:** `PDF/A-1a/1b/2a/2b/2u/3a/3b/3u/4/4e/4f`, `PDF/UA-1`,
  `PDF/UA-2` and `Factur-X`.
- **Pairs:** PDF/A-1 to PDF/A-3 and Factur-X pair with PDF/UA-1; PDF/A-4,
  PDF/A-4e and PDF/A-4f pair with PDF/UA-2.
- **`level`:** `MINIMUM`, `BASIC WL`, `BASIC`, `EN 16931`, `EXTENDED`,
  `XRECHNUNG`.
- **`documentType`:** `INVOICE`, `ORDER`, `ORDER_RESPONSE`, `ORDER_CHANGE`.
- `xml`, `level` and `documentType` apply only to Factur-X, and `level` and
  `documentType` need `xml`.

Ordering rules, which the server validates:

- `decrypt` must be the first stage.
- `render`, `extract-attachments` and `extract-text` must be the last stage,
  so a request has at most one of them.
- A request has at most one `remediate`, `encrypt`, `sign` and `timestamp`.
- Stages allowed after another stage, besides the final read stages:

  | After | Allowed |
  |---|---|
  | `sign` / `timestamp` | `sign`, `timestamp` |
  | `encrypt` | `optimize` |
  | `remediate` | `encrypt`, `optimize`, `sign`, `timestamp` |

- These combinations are not allowed in one request:
  - `encrypt` with `sign` or `timestamp`;
  - `decrypt` with `sign` or `timestamp`;
  - `encrypt` with a `remediate` whose profile includes PDF/A or Factur-X.

### Read routes

`/forms/pdf/{name}` takes one or more PDFs and an optional `password`. The
answer is a JSON object keyed by input filename.

| Route | Extra fields | Value per file |
|---|---|---|
| `info` | – | `{pages, version, encrypted, tagged, linearized, formFields, signed}` |
| `metadata` | – | `{title, author, subject, keywords, creator, producer, creationDate, modDate}` |
| `bookmarks` | – | `[{title, pageNumber, children}]` |
| `attachments` | – | `[{name, mimeType, relationship}]` |
| `form` | `pages` | `[{name, type, value, pages, options, states, readOnly, required, multiSelect, editable, maxLength}]` |
| `validate` | `profile` (req.; repeated for a pair) | `{profile, compliant, violations: [{specification, clause, testNumber, description, context, severity}]}` |

### Server source

When an option's name, type or default is unclear, read the server source at
tag `v9.6.0` rather than guessing:

| Topic | Source |
|---|---|
| stage structs and their JSON names | `pkg/modules/pdf/stage_types.go` |
| how single-op routes map form fields onto those structs | `pkg/modules/pdf/formdata.go` |
| pipe, single-op and read routes | `pkg/modules/pdf/routes.go` |
| form fields of the conversion routes | `pkg/modules/chromium/routes.go`, `pkg/modules/libreoffice/routes.go` |
| files, archives, `downloadFrom`, headers | `pkg/modules/api/{context,formdata,middlewares,api}.go` |
| defaults | `pkg/modules/chromium/chromium.go`, `pkg/modules/libreoffice/api/api.go` |
| read-route models | `github.com/gotenberg/litho` v0.0.13, `pkg/models` |

Other references:

- The docs at <https://gotenberg.dev/docs>.
- The client sources of gotenberg-php v3.5.0, in `src/`.

A full clone fails on Windows because of long test file names. Use a sparse
checkout instead:

```sh
git clone --depth 1 --branch v9.6.0 --no-checkout https://github.com/gotenberg/gotenberg.git
cd gotenberg
git sparse-checkout set --no-cone '/pkg/**/*.go' '!/pkg/**/*_test.go'
git checkout v9.6.0
```

## Commits

- Commit the YAML change, the generator change and the regenerated files
  together, in one change.
- Keep comments short and factual. The doc comment of a generated method
  comes from its YAML `doc`, so write it there.
- Keep private references out of code, commits and docs: local paths,
  session links, chat excerpts.
