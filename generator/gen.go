package main

import (
	"bytes"
	"fmt"
	"go/token"
	"strconv"
	"strings"
	"unicode"
)

// gen emits the Go source of the definitions.
type gen struct {
	defs *Definitions
}

// writer accumulates Go source.
type writer struct {
	bytes.Buffer
}

func (w *writer) p(format string, args ...any) {
	fmt.Fprintf(&w.Buffer, format, args...)
	w.WriteByte('\n')
}

// doc writes text as a doc comment wrapped at 77 columns. Blank lines
// separate paragraphs; lines starting with a tab are kept as is.
func (w *writer) doc(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	for i, para := range strings.Split(text, "\n\n") {
		if i > 0 {
			w.p("//")
		}

		if strings.HasPrefix(para, "\t") {
			for _, line := range strings.Split(para, "\n") {
				w.p("//%s", line)
			}

			continue
		}

		line := "//"
		for _, word := range strings.Fields(para) {
			if len(line)+1+len(word) > 77 && line != "//" {
				w.p("%s", line)
				line = "//"
			}

			line += " " + word
		}

		w.p("%s", line)
	}
}

// sentence prefixes a doc fragment with the identifier it documents, such as
// "Landscape" + "Sets the paper…" → "Landscape sets the paper…".
func sentence(ident, doc string) string {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return ""
	}

	r := []rune(doc)
	if len(r) > 1 && unicode.IsUpper(r[0]) && unicode.IsLower(r[1]) {
		r[0] = unicode.ToLower(r[0])
	}

	return ident + " " + string(r)
}

// paramName derives a parameter name from a Go method name: URL → url,
// MIMEType → mimeType, TimestampURL → timestampURL.
func paramName(name string) string {
	r := []rune(name)

	n := 0
	for n < len(r) && unicode.IsUpper(r[n]) {
		n++
	}

	switch {
	case n == len(r):
		n = len(r)
	case n > 1:
		n--
	}

	for i := range n {
		r[i] = unicode.ToLower(r[i])
	}

	s := string(r)
	if token.IsKeyword(s) {
		s += "Value"
	}

	return s
}

// paramType returns the Go type of the parameter of a field's method.
func (g *gen) paramType(f Field) string {
	switch f.Type {
	case "string", "length":
		return "string"
	case "bool":
		return "bool"
	case "int":
		return "int"
	case "float":
		return "float64"
	case "duration":
		return "time.Duration"
	case "enum":
		return f.Enum
	case "json":
		if f.Variadic {
			return "..." + strings.TrimPrefix(f.GoType, "[]")
		}

		return f.GoType
	case "profileSet":
		return "Profile"
	}

	panic("unknown type " + f.Type)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// valueExpr returns the fieldValue expression encoding the parameter v.
func (g *gen) valueExpr(f Field, v string) string {
	switch f.Type {
	case "string", "length":
		return fmt.Sprintf("stringValue(%s)", v)
	case "bool":
		return fmt.Sprintf("boolValue(%s)", v)
	case "int":
		var checks []string
		if f.Min != nil {
			checks = append(checks, fmt.Sprintf("checkMinInt(%q, %s, %d)", f.Form, v, int(*f.Min)))
		}

		if f.Max != nil {
			checks = append(checks, fmt.Sprintf("checkMaxInt(%q, %s, %d)", f.Form, v, int(*f.Max)))
		}

		return fmt.Sprintf("intValue(%s)", strings.Join(append([]string{v}, checks...), ", "))
	case "float":
		var checks []string
		if f.Min != nil {
			checks = append(checks, fmt.Sprintf("checkMinFloat(%q, %s, %s, %t)", f.Form, v, formatFloat(*f.Min), f.MinExclusive))
		}

		if f.Max != nil {
			checks = append(checks, fmt.Sprintf("checkMaxFloat(%q, %s, %s)", f.Form, v, formatFloat(*f.Max)))
		}

		return fmt.Sprintf("floatValue(%s)", strings.Join(append([]string{v}, checks...), ", "))
	case "duration":
		return fmt.Sprintf("durationValue(%s)", v)
	case "enum":
		return fmt.Sprintf("enumValue(%q, %s)", f.Form, v)
	case "json":
		if e := g.elemEnum(f); e != nil {
			return fmt.Sprintf("jsonValue(%s, checkEnums(%q, %s))", v, f.Form, v)
		}

		return fmt.Sprintf("jsonValue(%s)", v)
	case "profileSet":
		return fmt.Sprintf("profileValue(%q, %s)", f.Form, v)
	}

	panic("unknown type " + f.Type)
}

// elemEnum returns the enum type of the elements of a json slice field, nil
// when they are not an enum.
func (g *gen) elemEnum(f Field) *Enum {
	if f.Type != "json" || !strings.HasPrefix(f.GoType, "[]") {
		return nil
	}

	return g.defs.enums[strings.TrimPrefix(f.GoType, "[]")]
}

// nilable reports whether the json parameter is a slice or a map, which is
// encoded as an empty one rather than null when nil.
func nilable(f Field) bool {
	return f.Type == "json" && (strings.HasPrefix(f.GoType, "[]") || strings.HasPrefix(f.GoType, "map["))
}

// fieldDoc returns the doc comment of a field's method.
func fieldDoc(f Field, method string, stage bool) string {
	doc := sentence(method, f.Doc)
	if doc == "" {
		doc = method + " sets the option."
	}

	kind := "form field"
	if stage {
		kind = "option"
	}

	return doc + fmt.Sprintf("\n\nIt sets the %s %q.", kind, f.Form)
}

// fieldMethod emits the method of a field on the builder type t: a public
// option method, or the unexported setter of a required field.
func (g *gen) fieldMethod(w *writer, t string, f Field, stage bool) {
	if f.Type == "fileContent" {
		g.contentMethods(w, t, f)
		return
	}

	method := f.Name
	if f.Required {
		method = "set" + f.Name
	} else {
		w.doc(fieldDoc(f, method, stage))
	}

	v := paramName(f.Name)
	w.p("func (b %s) %s(%s %s) %s {", t, method, v, g.paramType(f), t)

	if nilable(f) {
		w.p("if %s == nil {", v)
		w.p("%s = %s{}", v, f.GoType)
		w.p("}")
	}

	w.p("b.set(%q, %s)", f.Form, g.valueExpr(f, v))

	if stage && f.FileRef {
		if f.Type == "string" {
			w.p("b.ref(%q, %s)", f.Form, v)
		} else {
			w.p("b.ref(%q, placementSources(%s)...)", f.Form, v)
		}
	}

	w.p("return b")
	w.p("}")
	w.p("")

	if f.Type == "profileSet" {
		w.doc(fmt.Sprintf("PairedWith sets the second profile of a PDF/A–PDF/UA pair: "+
			"PDF/A-1 to PDF/A-3 and Factur-X pair with PDF/UA-1, PDF/A-4, PDF/A-4e and "+
			"PDF/A-4f with PDF/UA-2. The pairing rule is checked by the server. A "+
			"repeated call replaces the second profile.\n\nIt sets the %s %q.", map[bool]string{true: "option", false: "form field"}[stage], f.Form))
		w.p("func (b %s) PairedWith(profile Profile) %s {", t, t)
		w.p("b.pairProfile(%q, profile)", f.Form)
		w.p("return b")
		w.p("}")
		w.p("")
	}
}

// contentMethods emits the <Name>From… methods of a fileContent field.
func (g *gen) contentMethods(w *writer, t string, f Field) {
	doc := strings.TrimSpace(f.Doc)
	if doc != "" {
		doc = " " + doc
	}

	for _, m := range []struct{ suffix, params, call, doc string }{
		{"FromPath", "path string", "contentFromPath(%q, path)", "uploads the file at path as %s.%s The file is opened when the request is sent."},
		{"FromFS", "fsys fs.FS, name string", "contentFromFS(%q, fsys, name)", "uploads the file name of fsys as %s.%s The file is opened when the request is sent."},
		{"FromReader", "reader io.Reader", "contentFromReader(%q, reader)", "uploads the content of reader as %s.%s The reader is read completely when the method is called and its whole content is held in memory; prefer the FromPath variant for large files."},
		{"FromBytes", "data []byte", "contentFromBytes(%q, data)", "uploads data as %s.%s data must not be modified afterwards."},
	} {
		name := f.Name + m.suffix
		w.doc(name + " " + fmt.Sprintf(m.doc, f.File, doc) + " A repeated call replaces the file.")
		w.p("func (b %s) %s(%s) %s {", t, name, m.params, t)
		w.p("b."+m.call, f.File)
		w.p("return b")
		w.p("}")
		w.p("")
	}
}

// requestMethods emits the methods every request builder shares: headers,
// files, stages and the terminal methods.
func (g *gen) requestMethods(w *writer, t, output, result string, files, stages bool) {
	w.doc("Trace sets the Gotenberg-Trace header, the correlation id of the request in the server logs.")
	w.p("func (b %s) Trace(id string) %s {", t, t)
	w.p("b.setTrace(id)")
	w.p("return b")
	w.p("}")
	w.p("")

	if output == "file" {
		w.doc("OutputFilename sets the Gotenberg-Output-Filename header, the filename of the result without extension.")
		w.p("func (b %s) OutputFilename(name string) %s {", t, t)
		w.p("b.setOutputFilename(name)")
		w.p("return b")
		w.p("}")
		w.p("")
	}

	if files {
		for _, m := range []struct{ sig, call, doc string }{
			{"FileFromPath(path string)", "addFileFromPath(path)", "FileFromPath uploads the file at path under its base name. The file is opened when the request is sent."},
			{"FileFromPathAs(path, filename string)", "addFileFromPathAs(path, filename)", "FileFromPathAs uploads the file at path as filename, for instance to upload report.html as index.html. The file is opened when the request is sent."},
			{"FileFromFS(fsys fs.FS, name string)", "addFileFromFS(fsys, name)", "FileFromFS uploads the file name of fsys under its base name. The file is opened when the request is sent."},
			{"FileFromFSAs(fsys fs.FS, name, filename string)", "addFileFromFSAs(fsys, name, filename)", "FileFromFSAs uploads the file name of fsys as filename. The file is opened when the request is sent."},
			{"FileFromReader(filename string, reader io.Reader)", "addFileFromReader(filename, reader)", "FileFromReader uploads the content of reader as filename. The reader is read completely when the method is called and its whole content is held in memory; prefer FileFromPath for large files."},
			{"FileFromBytes(filename string, data []byte)", "addFileFromBytes(filename, data)", "FileFromBytes uploads data as filename. data must not be modified afterwards."},
			{"ArchiveFromPath(path string)", "addArchiveFromPath(path)", "ArchiveFromPath uploads the ZIP archive at path, which the server unpacks: every entry counts as an uploaded file. The file is opened when the request is sent."},
			{"ArchiveFromFS(fsys fs.FS, name string)", "addArchiveFromFS(fsys, name)", "ArchiveFromFS uploads the ZIP archive name of fsys, which the server unpacks: every entry counts as an uploaded file. The file is opened when the request is sent."},
			{"ArchiveFromReader(reader io.Reader)", "addArchiveFromReader(reader)", "ArchiveFromReader uploads the ZIP archive read from reader, which the server unpacks: every entry counts as an uploaded file. The reader is read completely when the method is called and its whole content is held in memory; prefer ArchiveFromPath for large files."},
			{"ArchiveFromBytes(data []byte)", "addArchiveFromBytes(data)", "ArchiveFromBytes uploads the ZIP archive data, which the server unpacks: every entry counts as an uploaded file. data must not be modified afterwards."},
			{"FileFromURL(url string, opts ...DownloadOption)", "addDownload(url, opts)", "FileFromURL makes the server download the file at url before processing the request (form field downloadFrom). The URL is not validated client-side. See WithDownloadFilename and WithDownloadHeader."},
		} {
			w.doc(m.doc)
			w.p("func (b %s) %s %s {", t, m.sig, t)
			w.p("b.%s", m.call)
			w.p("return b")
			w.p("}")
			w.p("")
		}
	}

	if stages {
		w.doc("Stages sets the PDF operations run on the result, in order; a repeated call replaces them. Files used by a stage are uploaded via FileFrom…, ArchiveFrom… or FileFromURL and referenced by filename.")
		w.p("func (b %s) Stages(stages ...Stage) %s {", t, t)
		w.p("b.setStages(stages)")
		w.p("return b")
		w.p("}")
		w.p("")
	}

	if output == "file" {
		w.doc("ToFile sends the request and writes the result to the file at path. The file is removed when writing fails or ctx is cancelled. It returns ErrNoOutput when the response carries no content.")
		w.p("func (b %s) ToFile(ctx context.Context, path string) (*ResponseInfo, error) {", t)
		w.p("return b.toFile(ctx, path)")
		w.p("}")
		w.p("")
		w.doc("ToBytes sends the request and reads the result into memory. It returns ErrNoOutput when the response carries no content.")
		w.p("func (b %s) ToBytes(ctx context.Context) (*Result, error) {", t)
		w.p("return b.toBytes(ctx)")
		w.p("}")
		w.p("")
		w.doc("ToReader sends the request and returns the streamed result, which the caller must close. It returns ErrNoOutput when the response carries no content.")
		w.p("func (b %s) ToReader(ctx context.Context) (*ResultReader, error) {", t)
		w.p("return b.toReader(ctx)")
		w.p("}")
		w.p("")

		return
	}

	w.doc("Send sends the request and returns the decoded answer, keyed by input filename.")
	w.p("func (b %s) Send(ctx context.Context) (%s, error) {", t, result)
	w.p("return sendJSON[%s](ctx, b.request)", result)
	w.p("}")
	w.p("")
}

// builder emits a request builder.
func (g *gen) builder(b *Builder) string {
	w := &writer{}

	w.doc(fmt.Sprintf("%s is the request builder of %s. %s\n\nBuilders are immutable: every method returns a modified copy.", b.Type, b.Route, b.Doc))
	w.p("type %s struct{ request }", b.Type)
	w.p("")

	for _, f := range b.Fields {
		g.fieldMethod(w, b.Type, f, false)
	}

	g.requestMethods(w, b.Type, b.Output, b.Result, b.Files, b.Stages)

	return w.String()
}

// opVariants returns the variants of an op; an op without variants has one
// unnamed variant with its fields.
func opVariants(op *Op) []Variant {
	if len(op.Variants) == 0 {
		return []Variant{{}}
	}

	return op.Variants
}

// opFields returns the shared fields of an op followed by the fields of all
// its variants.
func opFields(op *Op) []Field {
	fields := append([]Field{}, op.Fields...)
	for _, v := range op.Variants {
		fields = append(fields, v.Fields...)
	}

	return fields
}

// params returns the parameter list and the setter calls of the required
// fields.
func (g *gen) params(fields []Field) (params string, setters []string) {
	var list []string
	for _, f := range requiredFields(fields) {
		v := paramName(f.Name)
		list = append(list, v+" "+g.paramType(f))

		arg := v
		if f.Variadic {
			arg += "..."
		}

		setters = append(setters, fmt.Sprintf("set%s(%s)", f.Name, arg))
	}

	return strings.Join(list, ", "), setters
}

// op emits the single-op request builder and the stage builder of an op.
func (g *gen) op(op *Op) string {
	w := &writer{}
	route := "/forms/pdf/" + op.Op
	reqType := "PDF" + op.Name
	stageType := op.Name + "Stage"

	// Request builder.
	w.doc(fmt.Sprintf("%s is the request builder of %s, which runs the %s operation on the PDFs added via FileFrom…. %s\n\nBuilders are immutable: every method returns a modified copy.", reqType, route, op.Op, op.Doc))
	w.p("type %s struct{ request }", reqType)
	w.p("")

	for _, v := range opVariants(op) {
		fields := append(append([]Field{}, op.Fields...), v.Fields...)
		params, setters := g.params(fields)
		entry := op.Name + v.Name

		doc := fmt.Sprintf("%s starts a request running the %s operation. %s", entry, op.Op, op.Doc)
		if v.Doc != "" {
			doc += " " + v.Doc
		}

		w.doc(doc)
		w.p("func (p PDF) %s(%s) %s {", entry, params, reqType)
		w.p("r := newRequest(p.client, %q)", route)
		if op.Op == "merge" {
			w.p("r.inputs = true")
			w.p("r.merge = true")
		}

		w.p("b := %s{r}", reqType)
		for _, s := range setters {
			w.p("b = b.%s", s)
		}

		w.p("return b")
		w.p("}")
		w.p("")
	}

	for _, f := range opFields(op) {
		g.fieldMethod(w, reqType, f, false)
	}

	g.requestMethods(w, reqType, "file", "", true, false)

	// Stage builder.
	w.doc(fmt.Sprintf("%s is the stage builder of the %s operation. %s\n\nStage builders are immutable: every method returns a modified copy.", stageType, op.Op, op.Doc))
	w.p("type %s struct{ stage }", stageType)
	w.p("")

	for _, v := range opVariants(op) {
		fields := append(append([]Field{}, op.Fields...), v.Fields...)
		params, setters := g.params(fields)
		ctor := "Stage" + op.Name + v.Name

		doc := fmt.Sprintf("%s returns a %s stage. %s", ctor, op.Op, op.Doc)
		if v.Doc != "" {
			doc += " " + v.Doc
		}

		w.doc(doc)
		w.p("func %s(%s) %s {", ctor, params, stageType)
		w.p("s := %s{newStage(%q)}", stageType, op.Op)
		for _, s := range setters {
			w.p("s = s.%s", s)
		}

		w.p("return s")
		w.p("}")
		w.p("")
	}

	for _, f := range opFields(op) {
		g.fieldMethod(w, stageType, f, true)
	}

	return w.String()
}

// enums emits the enum types.
func (g *gen) enums() string {
	w := &writer{}

	for _, e := range g.defs.Enums {
		base := "string"
		if e.Base == "int" {
			base = "int"
		}

		w.doc(sentence(e.Name, e.Doc))
		w.p("type %s %s", e.Name, base)
		w.p("")
		w.p("// %s values.", e.Name)
		w.p("const (")
		for _, v := range e.Values {
			if v.Doc != "" {
				w.doc(sentence(v.Const, v.Doc))
			}

			w.p("%s %s = %s", v.Const, e.Name, g.enumLiteral(e, v.Value))
		}

		w.p(")")
		w.p("")
		w.p("func (v %s) valid() bool {", e.Name)
		w.p("switch v {")

		consts := make([]string, len(e.Values))
		for i, v := range e.Values {
			consts[i] = v.Const
		}

		w.p("case %s:", strings.Join(consts, ", "))
		w.p("return true")
		w.p("}")
		w.p("")
		w.p("return false")
		w.p("}")
		w.p("")
	}

	return w.String()
}

func (g *gen) enumLiteral(e *Enum, value string) string {
	if e.Base == "int" {
		return value
	}

	return strconv.Quote(value)
}
