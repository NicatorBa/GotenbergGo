package gotenberg

// PDF is the PDF module: it runs PDF operations on uploaded PDFs, either as a
// pipeline of stages ([PDF.Pipe]) or one operation per request (such as
// [PDF.Merge] or [PDF.StampText]), and reads information from PDFs (such as
// [PDF.Info]). Obtain it via [Client.PDF].
type PDF struct {
	client *Client
}

// Pipe starts a request running stages on the PDFs added via FileFrom…. At
// least one stage is required:
//
//	client.PDF().Pipe().
//		FileFromPath("in.pdf").
//		FileFromPath("logo.png").
//		Stages(gotenberg.StageStampImage("logo.png"), gotenberg.StageOptimize())
//
// Files used by a stage are uploaded on the builder too and referenced by
// filename.
func (p PDF) Pipe() PDFPipe {
	r := newRequest(p.client, "/forms/pdf/pipe")
	r.inputs = true

	return PDFPipe{r}
}

// Info starts a request reading general information about the PDFs added via
// FileFrom….
func (p PDF) Info() PDFInfo {
	return PDFInfo{newRequest(p.client, "/forms/pdf/info")}
}

// Metadata starts a request reading the metadata of the PDFs added via
// FileFrom….
func (p PDF) Metadata() PDFMetadata {
	return PDFMetadata{newRequest(p.client, "/forms/pdf/metadata")}
}

// Bookmarks starts a request reading the outline of the PDFs added via
// FileFrom….
func (p PDF) Bookmarks() PDFBookmarks {
	return PDFBookmarks{newRequest(p.client, "/forms/pdf/bookmarks")}
}

// Attachments starts a request listing the files embedded in the PDFs added
// via FileFrom….
func (p PDF) Attachments() PDFAttachments {
	return PDFAttachments{newRequest(p.client, "/forms/pdf/attachments")}
}

// Form starts a request listing the form fields of the PDFs added via
// FileFrom….
func (p PDF) Form() PDFForm {
	return PDFForm{newRequest(p.client, "/forms/pdf/form")}
}

// Validate starts a request validating the PDFs added via FileFrom… against
// profile. A PDF/A profile, or Factur-X, is paired with a PDF/UA profile via
// [PDFValidate.PairedWith]:
//
//	client.PDF().Validate(gotenberg.ProfilePDFA2b).PairedWith(gotenberg.ProfilePDFUA1)
func (p PDF) Validate(profile Profile) PDFValidate {
	b := PDFValidate{newRequest(p.client, "/forms/pdf/validate")}
	return b.setProfile(profile)
}
