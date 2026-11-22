// Package gotenberg is a client for Gotenberg 9.x (https://gotenberg.dev), a
// Docker-based API converting documents to PDF and processing PDFs.
//
// The API is fluent and modelled on the Gotenberg routes. A [Client] exposes
// one module per Gotenberg module, [Client.Chromium], [Client.LibreOffice] and
// [Client.PDF]; each module method starts an immutable request builder, and
// a terminal method sends the request:
//
//	client, err := gotenberg.NewClient("http://localhost:3000")
//	if err != nil {
//		return err
//	}
//
//	_, err = client.Chromium().ConvertFromURL("https://example.com").
//		PaperSize(gotenberg.PaperSizeA4).
//		Landscape(true).
//		ToFile(ctx, "example.pdf")
//
// # Builders
//
// Builders are value types: every method returns a modified copy, so chains
// branching off the same builder never affect each other, and a builder can
// be sent several times. Invalid arguments, such as an empty filename or an
// enum or numeric value out of range, do not break the chain: they are
// returned by the terminal method, and the request is not sent. Otherwise the
// server is authoritative and validates the request.
//
// Builders answering with a file offer ToFile, ToBytes and ToReader; the read
// routes of the PDF module, such as [PDF.Info], answer with JSON decoded by
// Send. A non-2xx answer is returned as an [*APIError].
//
// # Files
//
// Files are added via FileFromPath, FileFromFS, FileFromReader and
// FileFromBytes (FileFromPathAs and FileFromFSAs upload under another name),
// ZIP archives the server unpacks via ArchiveFrom…, and files the server
// downloads itself via FileFromURL. Path and fs.FS files are opened when the
// request is sent, and the request body is streamed.
//
// # Stages
//
// PDF operations are stages, created by the Stage… constructors such as
// [StageMerge] or [StageStampText]. They run after a conversion (Stages of the
// Chromium convert and LibreOffice builders), as a pipeline on uploaded PDFs
// ([PDF.Pipe]), or one per request via the single-operation builders such as
// [PDF.Merge] or [PDF.StampText].
//
// Files used by a stage, such as the image of [StageStampImage], are
// referenced by filename only: upload them on the request builder too.
//
//	client.PDF().Pipe().
//		FileFromPath("in.pdf").
//		FileFromPath("logo.png").
//		Stages(gotenberg.StageStampImage("logo.png"))
//
// When a request merges its input documents, the client prefixes their
// filenames with their position so that the merge follows the order the
// files were added in; see [StageMerge].
//
// # Redirects
//
// The streamed request body cannot be replayed, so redirects are not
// supported; see [NewClient].
package gotenberg
