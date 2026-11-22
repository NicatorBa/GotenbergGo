package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// example is the example value of a field in the generated tests.
type example struct {
	// arg is the Go expression passed to the method ("x..." when
	// variadic).
	arg string
	// form is the Go expression of the expected form values ([]string).
	form string
	// json is the Go expression of the expected JSON value (string).
	json string
}

func quoteJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return strconv.Quote(string(b))
}

func formOf(values ...string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}

	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

func (g *gen) enumValueOf(e *Enum, constName string) EnumValue {
	for _, v := range e.Values {
		if v.Const == constName || constName == "" {
			return v
		}
	}

	panic(fmt.Sprintf("enum %s has no constant %s", e.Name, constName))
}

func (g *gen) example(f Field) example {
	switch f.Type {
	case "string", "length":
		ex := f.Example
		if ex == "" {
			ex = "example"
			if f.Type == "length" {
				ex = "8.5in"
			}
		}

		return example{arg: strconv.Quote(ex), form: formOf(ex), json: quoteJSON(ex)}
	case "bool":
		return example{arg: "true", form: formOf("true"), json: strconv.Quote("true")}
	case "int":
		n := 1
		switch {
		case f.Example != "":
			var err error
			n, err = strconv.Atoi(f.Example)
			if err != nil {
				panic(fmt.Sprintf("field %s: invalid example: %v", f.Name, err))
			}
		case f.Min != nil:
			n = int(*f.Min)
		}

		s := strconv.Itoa(n)

		return example{arg: s, form: formOf(s), json: strconv.Quote(s)}
	case "float":
		x := 1.5
		switch {
		case f.Example != "":
			var err error
			x, err = strconv.ParseFloat(f.Example, 64)
			if err != nil {
				panic(fmt.Sprintf("field %s: invalid example: %v", f.Name, err))
			}
		case f.Min != nil && f.Max != nil:
			x = (*f.Min + *f.Max) / 2
		case f.Min != nil:
			x = *f.Min + 1.5
		case f.Max != nil:
			x = *f.Max - 1.5
		}

		s := formatFloat(x)

		return example{arg: s, form: formOf(s), json: strconv.Quote(s)}
	case "duration":
		ex := f.Example
		if ex == "" {
			ex = "1500ms"
		}

		d, err := time.ParseDuration(ex)
		if err != nil {
			panic(fmt.Sprintf("field %s: invalid example: %v", f.Name, err))
		}

		return example{arg: fmt.Sprintf("time.Duration(%d)", d), form: formOf(d.String()), json: quoteJSON(d.String())}
	case "enum":
		e := g.defs.enums[f.Enum]
		v := g.enumValueOf(e, f.Example)

		js := quoteJSON(v.Value)
		if e.Base == "int" {
			js = strconv.Quote(v.Value)
		}

		return example{arg: v.Const, form: formOf(v.Value), json: js}
	case "json":
		arg := f.Example
		if f.Variadic {
			arg += "..."
		}

		return example{arg: arg, form: "[]string{mustJSON(" + f.Example + ")}", json: "mustJSON(" + f.Example + ")"}
	case "profileSet":
		constName := f.Example
		if constName == "" {
			constName = "ProfilePDFA2b"
		}

		v := g.enumValueOf(g.defs.enums["Profile"], constName)

		return example{arg: v.Const, form: formOf(v.Value), json: quoteJSON(v.Value)}
	}

	panic("no example for type " + f.Type)
}

// invalid is an argument the client rejects with a deferred error.
type invalid struct {
	label string
	arg   string
}

func (g *gen) invalids(f Field) []invalid {
	var res []invalid

	switch f.Type {
	case "enum", "profileSet":
		name := f.Enum
		if f.Type == "profileSet" {
			name = "Profile"
		}

		res = append(res, invalid{"Invalid", g.invalidEnum(g.defs.enums[name])})
	case "int":
		if f.Min != nil {
			res = append(res, invalid{"BelowMin", strconv.Itoa(int(*f.Min) - 1)})
		}

		if f.Max != nil {
			res = append(res, invalid{"AboveMax", strconv.Itoa(int(*f.Max) + 1)})
		}
	case "float":
		if f.Min != nil {
			v := *f.Min - 1
			if f.MinExclusive {
				v = *f.Min
			}

			res = append(res, invalid{"BelowMin", formatFloat(v)})
		}

		if f.Max != nil {
			res = append(res, invalid{"AboveMax", formatFloat(*f.Max + 1)})
		}
	case "json":
		if e := g.elemEnum(f); e != nil {
			arg := f.GoType + "{" + g.invalidEnum(e) + "}"
			if f.Variadic {
				arg += "..."
			}

			res = append(res, invalid{"InvalidEntry", arg})
		}
	}

	return res
}

func (g *gen) invalidEnum(e *Enum) string {
	if e.Base == "int" {
		return e.Name + "(-1)"
	}

	return e.Name + `("invalid")`
}

// entryArgs returns the arguments of an entry method or constructor: the
// examples of the required fields, with override replacing the one of the
// field named overrideField.
func (g *gen) entryArgs(fields []Field, overrideField, override string) string {
	var args []string
	for _, f := range requiredFields(fields) {
		if f.Name == overrideField {
			args = append(args, override)
			continue
		}

		args = append(args, g.example(f).arg)
	}

	return strings.Join(args, ", ")
}

// requestCase is one row of the table test of a request builder.
type requestCase struct {
	name    string
	build   string // expression of type T built from c
	field   string // form field to check
	want    string // expected form values or header values
	header  string // header to check instead of a form field
	file    string // filename suffix of an expected file under field
	wantErr bool
}

// requestTest emits the table test of a request builder. entry returns the
// expression building the builder from c for the given required field
// override.
func (g *gen) requestTest(w *writer, testName, t, route, output string, fields, required []Field, entry func(overrideField, override string) string, files, stages bool) {
	var cases []requestCase

	base := entry("", "")
	if len(required) == 0 {
		cases = append(cases, requestCase{name: "Entry", build: base})
	}

	for _, f := range required {
		ex := g.example(f)
		cases = append(cases, requestCase{name: f.Name, build: base, field: f.Form, want: ex.form})

		for _, inv := range g.invalids(f) {
			cases = append(cases, requestCase{name: f.Name + inv.label, build: entry(f.Name, inv.arg), wantErr: true})
		}

		if f.Type == "profileSet" {
			p := g.enumValueOf(g.defs.enums["Profile"], "ProfilePDFUA1")
			cases = append(cases,
				requestCase{name: "PairedWith", build: base + ".PairedWith(" + p.Const + ")", field: f.Form, want: strings.TrimSuffix(ex.form, "}") + ", " + strconv.Quote(p.Value) + "}"},
				requestCase{name: "PairedWithInvalid", build: base + `.PairedWith(Profile("invalid"))`, wantErr: true},
			)
		}
	}

	for _, f := range fields {
		if f.Required {
			continue
		}

		if f.Type == "fileContent" {
			cases = append(cases,
				requestCase{name: f.Name + "FromBytes", build: base + "." + f.Name + `FromBytes([]byte("content"))`, field: "files", file: f.File},
				requestCase{name: f.Name + "FromReader", build: base + "." + f.Name + `FromReader(strings.NewReader("content"))`, field: "files", file: f.File},
			)

			continue
		}

		ex := g.example(f)
		cases = append(cases, requestCase{name: f.Name, build: base + "." + f.Name + "(" + ex.arg + ")", field: f.Form, want: ex.form})

		for _, inv := range g.invalids(f) {
			cases = append(cases, requestCase{name: f.Name + inv.label, build: base + "." + f.Name + "(" + inv.arg + ")", wantErr: true})
		}
	}

	cases = append(cases, requestCase{name: "Trace", build: base + `.Trace("trace-id")`, header: "Gotenberg-Trace", want: formOf("trace-id")})

	if output == "file" {
		cases = append(cases, requestCase{name: "OutputFilename", build: base + `.OutputFilename("out")`, header: "Gotenberg-Output-Filename", want: formOf("out")})
	}

	if files {
		cases = append(cases,
			requestCase{name: "FileFromBytes", build: base + `.FileFromBytes("in.pdf", []byte("%PDF"))`, field: "files", file: "in.pdf"},
			requestCase{name: "FileFromBytesEmptyFilename", build: base + `.FileFromBytes("", []byte("%PDF"))`, wantErr: true},
			requestCase{name: "ArchiveFromBytes", build: base + `.ArchiveFromBytes([]byte("PK"))`, field: "archives", file: "archive1.zip"},
			requestCase{name: "FileFromURL", build: base + `.FileFromURL("https://example.com/a.pdf", WithDownloadFilename("a.pdf"))`, field: "downloadFrom", want: formOf(`[{"url":"https://example.com/a.pdf","filename":"a.pdf"}]`)},
		)
	}

	if stages {
		cases = append(cases,
			requestCase{name: "Stages", build: base + ".Stages(StageMerge())", field: "stages", want: formOf(`[{"op":"merge"}]`)},
			requestCase{name: "StagesNil", build: base + ".Stages(nil)", wantErr: true},
		)
	}

	server, send := "newFileServer", "_, err := b.ToBytes(t.Context())"
	if output == "json" {
		server, send = "newJSONServer", "_, err := b.Send(t.Context())"
	}

	w.p("func %s(t *testing.T) {", testName)
	w.p("tests := []struct {")
	w.p("name string")
	w.p("build func(c *Client) %s", t)
	w.p("field string")
	w.p("want []string")
	w.p("header string")
	w.p("file string")
	w.p("wantErr bool")
	w.p("}{")

	for _, c := range cases {
		want := c.want
		if want == "" {
			want = "nil"
		}

		w.p("{")
		w.p("name: %q,", c.name)
		w.p("build: func(c *Client) %s { return %s },", t, c.build)
		if c.field != "" {
			w.p("field: %q,", c.field)
		}

		if want != "nil" {
			w.p("want: %s,", want)
		}

		if c.header != "" {
			w.p("header: %q,", c.header)
		}

		if c.file != "" {
			w.p("file: %q,", c.file)
		}

		if c.wantErr {
			w.p("wantErr: true,")
		}

		w.p("},")
	}

	w.p("}")
	w.p("")
	w.p("for _, tt := range tests {")
	w.p("t.Run(tt.name, func(t *testing.T) {")
	w.p("srv := %s(t)", server)
	w.p("b := tt.build(srv.client)")
	w.p("%s", send)
	w.p("if tt.wantErr {")
	w.p("require.Error(t, err)")
	w.p("assert.Zero(t, srv.count())")
	w.p("return")
	w.p("}")
	w.p("")
	w.p("require.NoError(t, err)")
	w.p("req := srv.last(t)")
	w.p("assert.Equal(t, http.MethodPost, req.Method)")
	w.p("assert.Equal(t, %q, req.Path)", route)
	w.p("switch {")
	w.p("case tt.header != \"\":")
	w.p("assert.Equal(t, tt.want, req.Header.Values(tt.header))")
	w.p("case tt.file != \"\":")
	w.p("assert.True(t, req.hasFile(tt.field, tt.file), \"file %%s under %%s\", tt.file, tt.field)")
	w.p("case tt.field != \"\":")
	w.p("assert.Equal(t, tt.want, req.Fields[tt.field])")
	w.p("}")
	w.p("})")
	w.p("}")
	w.p("}")
	w.p("")
}

func (g *gen) builderTest(b *Builder) string {
	w := &writer{}

	entry := func(overrideField, override string) string {
		return "c." + b.Entry + "(" + g.entryArgs(b.Fields, overrideField, override) + ")"
	}

	g.requestTest(w, "Test"+b.Type+"Generated", b.Type, b.Route, b.Output, b.Fields, requiredFields(b.Fields), entry, b.Files, b.Stages)

	return g.withStrings(w.String())
}

// withStrings adds a reference to the strings package, which the generated
// tests of fileContent fields use, so that writeGo imports it.
func (g *gen) withStrings(body string) string {
	if strings.Contains(body, "strings.NewReader") {
		return "import \"strings\"\n\n" + body
	}

	return body
}

// stageCase is one row of the table test of a stage builder.
type stageCase struct {
	name    string
	stage   string
	key     string
	want    string
	wantErr bool
}

func (g *gen) opTest(op *Op) string {
	w := &writer{}
	reqType := "PDF" + op.Name
	stageType := op.Name + "Stage"

	// Request builder: every variant, the shared fields on the first one.
	for i, v := range opVariants(op) {
		fields := append(append([]Field{}, op.Fields...), v.Fields...)
		entry := func(overrideField, override string) string {
			return "c.PDF()." + op.Name + v.Name + "(" + g.entryArgs(fields, overrideField, override) + ")"
		}

		testFields := v.Fields
		if i == 0 {
			testFields = fields
		}

		g.requestTest(w, "Test"+reqType+v.Name+"Generated", reqType, "/forms/pdf/"+op.Op, "file", testFields, requiredFields(fields), entry, i == 0, false)
	}

	// Stage builder.
	var cases []stageCase
	for i, v := range opVariants(op) {
		fields := append(append([]Field{}, op.Fields...), v.Fields...)
		ctor := func(overrideField, override string) string {
			return "Stage" + op.Name + v.Name + "(" + g.entryArgs(fields, overrideField, override) + ")"
		}

		base := ctor("", "")
		required := requiredFields(fields)
		if len(required) == 0 {
			cases = append(cases, stageCase{name: "Constructor" + v.Name, stage: base})
		}

		for _, f := range required {
			ex := g.example(f)
			cases = append(cases, stageCase{name: f.Name, stage: base, key: f.Form, want: ex.json})

			for _, inv := range g.invalids(f) {
				cases = append(cases, stageCase{name: f.Name + inv.label, stage: ctor(f.Name, inv.arg), wantErr: true})
			}

			if f.Type == "profileSet" {
				p := g.enumValueOf(g.defs.enums["Profile"], "ProfilePDFUA1")
				var first string
				_ = json.Unmarshal([]byte(mustUnquote(ex.json)), &first)
				cases = append(cases,
					stageCase{name: "PairedWith", stage: base + ".PairedWith(" + p.Const + ")", key: f.Form, want: quoteJSON([]string{first, p.Value})},
					stageCase{name: "PairedWithInvalid", stage: base + `.PairedWith(Profile("invalid"))`, wantErr: true},
				)
			}
		}

		testFields := v.Fields
		if i == 0 {
			testFields = fields
		}

		for _, f := range testFields {
			if f.Required {
				continue
			}

			ex := g.example(f)
			cases = append(cases, stageCase{name: f.Name, stage: base + "." + f.Name + "(" + ex.arg + ")", key: f.Form, want: ex.json})

			for _, inv := range g.invalids(f) {
				cases = append(cases, stageCase{name: f.Name + inv.label, stage: base + "." + f.Name + "(" + inv.arg + ")", wantErr: true})
			}
		}
	}

	w.p("func Test%sGenerated(t *testing.T) {", stageType)
	w.p("tests := []struct {")
	w.p("name string")
	w.p("stage Stage")
	w.p("key string")
	w.p("want string")
	w.p("wantErr bool")
	w.p("}{")

	for _, c := range cases {
		w.p("{")
		w.p("name: %q,", c.name)
		w.p("stage: %s,", c.stage)
		if c.key != "" {
			w.p("key: %q,", c.key)
			w.p("want: %s,", c.want)
		}

		if c.wantErr {
			w.p("wantErr: true,")
		}

		w.p("},")
	}

	w.p("}")
	w.p("")
	w.p("for _, tt := range tests {")
	w.p("t.Run(tt.name, func(t *testing.T) {")
	w.p("st := tt.stage.stageState()")
	w.p("if tt.wantErr {")
	w.p("assert.Error(t, st.err)")
	w.p("return")
	w.p("}")
	w.p("")
	w.p("require.NoError(t, st.err)")
	w.p("object := stageObject(t, tt.stage)")
	w.p("assert.JSONEq(t, %s, string(object[\"op\"]))", quoteJSON(op.Op))
	w.p("if tt.key != \"\" {")
	w.p("assert.JSONEq(t, tt.want, string(object[tt.key]))")
	w.p("}")
	w.p("})")
	w.p("}")
	w.p("}")
	w.p("")

	return g.withStrings(w.String())
}

func mustUnquote(s string) string {
	u, err := strconv.Unquote(s)
	if err != nil {
		panic(err)
	}

	return u
}

func (g *gen) enumsTest() string {
	w := &writer{}

	w.p("func TestEnumsGenerated(t *testing.T) {")
	for _, e := range g.defs.Enums {
		w.p("t.Run(%q, func(t *testing.T) {", e.Name)
		for _, v := range e.Values {
			w.p("assert.True(t, %s.valid())", v.Const)
		}

		w.p("assert.False(t, %s.valid())", g.invalidEnum(e))
		w.p("})")
	}

	w.p("}")

	return w.String()
}
