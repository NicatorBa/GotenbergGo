package gotenberg

// Chromium is the Chromium module: it converts URLs, HTML and Markdown to PDF
// and takes screenshots of them. Obtain it via [Client.Chromium].
type Chromium struct {
	client *Client
}

// ConvertFromURL starts a request converting the page at url, an absolute
// http or https URL, to PDF.
func (c Chromium) ConvertFromURL(url string) ChromiumConvertFromURL {
	b := ChromiumConvertFromURL{newRequest(c.client, "/forms/chromium/convert/url")}
	return b.setURL(url)
}

// ConvertFromHTML starts a request converting an HTML document to PDF. The
// document is added via the Index… methods, such as
// [ChromiumConvertFromHTML.IndexFromPath]; further files, such as images,
// stylesheets or fonts, via FileFrom….
func (c Chromium) ConvertFromHTML() ChromiumConvertFromHTML {
	return ChromiumConvertFromHTML{newRequest(c.client, "/forms/chromium/convert/html")}
}

// ConvertFromMarkdown starts a request converting Markdown files to PDF. The
// HTML template, added via the Index… methods, calls {{ toHTML "file.md" }}
// for each Markdown file added via FileFrom….
func (c Chromium) ConvertFromMarkdown() ChromiumConvertFromMarkdown {
	return ChromiumConvertFromMarkdown{newRequest(c.client, "/forms/chromium/convert/markdown")}
}

// ScreenshotFromURL starts a request taking a screenshot of the page at url,
// an absolute http or https URL.
func (c Chromium) ScreenshotFromURL(url string) ChromiumScreenshotFromURL {
	b := ChromiumScreenshotFromURL{newRequest(c.client, "/forms/chromium/screenshot/url")}
	return b.setURL(url)
}

// ScreenshotFromHTML starts a request taking a screenshot of an HTML
// document, added via the Index… methods.
func (c Chromium) ScreenshotFromHTML() ChromiumScreenshotFromHTML {
	return ChromiumScreenshotFromHTML{newRequest(c.client, "/forms/chromium/screenshot/html")}
}

// ScreenshotFromMarkdown starts a request taking a screenshot of Markdown
// files rendered into the HTML template added via the Index… methods.
func (c Chromium) ScreenshotFromMarkdown() ChromiumScreenshotFromMarkdown {
	return ChromiumScreenshotFromMarkdown{newRequest(c.client, "/forms/chromium/screenshot/markdown")}
}

// Cookie is a cookie Chromium sets before loading the page.
type Cookie struct {
	Name     string         `json:"name"`
	Value    string         `json:"value"`
	Domain   string         `json:"domain"`
	Path     string         `json:"path,omitempty"`
	Secure   bool           `json:"secure,omitempty"`
	HTTPOnly bool           `json:"httpOnly,omitempty"`
	SameSite CookieSameSite `json:"sameSite,omitempty"`
}
