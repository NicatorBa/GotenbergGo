package gotenberg

// LibreOffice is the LibreOffice module: it converts office documents to PDF.
// Obtain it via [Client.LibreOffice].
type LibreOffice struct {
	client *Client
}

// Convert starts a request converting the office documents added via
// FileFrom… to PDF. Each document is converted to its own PDF; several are
// answered as a ZIP unless a merge stage combines them, in the order the
// files were added (see [StageMerge]).
func (l LibreOffice) Convert() LibreOfficeConvert {
	r := newRequest(l.client, "/forms/libreoffice/convert")
	r.inputs = true

	return LibreOfficeConvert{r}
}
