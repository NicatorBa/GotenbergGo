package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Field is an option of a builder: a form field of a request builder, or a
// key of a stage's JSON object.
type Field struct {
	Name         string   `yaml:"name"`
	Form         string   `yaml:"form"`
	Type         string   `yaml:"type"`
	Doc          string   `yaml:"doc"`
	Required     bool     `yaml:"required"`
	Min          *float64 `yaml:"min"`
	Max          *float64 `yaml:"max"`
	MinExclusive bool     `yaml:"minExclusive"`
	Enum         string   `yaml:"enum"`
	GoType       string   `yaml:"goType"`
	Variadic     bool     `yaml:"variadic"`
	File         string   `yaml:"file"`
	Example      string   `yaml:"example"`
	// FileRef marks an option naming uploaded files, which keep their
	// filenames for merge order.
	FileRef bool `yaml:"fileRef"`
}

// Group is a set of fields shared by several builders.
type Group struct {
	Fields []Field `yaml:"fields"`
}

// Builder is a request builder.
type Builder struct {
	Type   string `yaml:"type"`
	Route  string `yaml:"route"`
	Doc    string `yaml:"doc"`
	Output string `yaml:"output"`
	Result string `yaml:"result"`
	// Entry is the call returning the builder in the generated tests,
	// such as Chromium().ConvertFromURL.
	Entry   string   `yaml:"entry"`
	Files   bool     `yaml:"files"`
	Stages  bool     `yaml:"stages"`
	Include []string `yaml:"include"`
	Fields  []Field  `yaml:"fields"`

	file string
}

// Variant is a mutually exclusive form of an op, with its own constructor.
type Variant struct {
	Name   string  `yaml:"name"`
	Doc    string  `yaml:"doc"`
	Fields []Field `yaml:"fields"`
}

// Op is a PDF operation: a single-op request builder and a stage builder.
type Op struct {
	Op             string    `yaml:"op"`
	Name           string    `yaml:"name"`
	Doc            string    `yaml:"doc"`
	DropSignatures bool      `yaml:"dropSignatures"`
	Include        []string  `yaml:"include"`
	Fields         []Field   `yaml:"fields"`
	Variants       []Variant `yaml:"variants"`

	file string
}

// Enum is an enum type.
type Enum struct {
	Name   string      `yaml:"name"`
	Doc    string      `yaml:"doc"`
	Base   string      `yaml:"base"`
	Values []EnumValue `yaml:"values"`
}

// EnumValue is a constant of an enum type.
type EnumValue struct {
	Const string `yaml:"const"`
	Value string `yaml:"value"`
	Doc   string `yaml:"doc"`
}

// Definitions holds all loaded YAML files.
type Definitions struct {
	Groups   map[string]Group
	Builders []*Builder
	Ops      []*Op
	Enums    []*Enum

	enums map[string]*Enum
}

var fieldTypes = []string{"string", "bool", "int", "float", "duration", "length", "enum", "json", "profileSet", "fileContent"}

func load(dir string) (*Definitions, error) {
	defs := &Definitions{Groups: map[string]Group{}, enums: map[string]*Enum{}}

	err := loadFile(filepath.Join(dir, "enums.yaml"), &defs.Enums)
	if err != nil {
		return nil, err
	}

	for _, e := range defs.Enums {
		if defs.enums[e.Name] != nil {
			return nil, fmt.Errorf("enums.yaml: duplicate enum %s", e.Name)
		}

		if e.Base != "" && e.Base != "int" && e.Base != "string" {
			return nil, fmt.Errorf("enums.yaml: enum %s: invalid base %q", e.Name, e.Base)
		}

		defs.enums[e.Name] = e
	}

	groupFiles, err := filepath.Glob(filepath.Join(dir, "groups", "*.yaml"))
	if err != nil {
		return nil, err
	}

	for _, path := range groupFiles {
		var g Group
		err := loadFile(path, &g)
		if err != nil {
			return nil, err
		}

		defs.Groups[strings.TrimSuffix(filepath.Base(path), ".yaml")] = g
	}

	builderFiles, err := filepath.Glob(filepath.Join(dir, "builders", "*.yaml"))
	if err != nil {
		return nil, err
	}

	for _, path := range builderFiles {
		b := &Builder{file: strings.TrimSuffix(filepath.Base(path), ".yaml")}
		err := loadFile(path, b)
		if err != nil {
			return nil, err
		}

		b.Fields, err = defs.resolve(b.Include, b.Fields)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		err = defs.checkBuilder(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		defs.Builders = append(defs.Builders, b)
	}

	opFiles, err := filepath.Glob(filepath.Join(dir, "ops", "*.yaml"))
	if err != nil {
		return nil, err
	}

	for _, path := range opFiles {
		op := &Op{file: strings.TrimSuffix(filepath.Base(path), ".yaml")}
		err := loadFile(path, op)
		if err != nil {
			return nil, err
		}

		op.Fields, err = defs.resolve(op.Include, op.Fields)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		if op.DropSignatures {
			op.Fields = append(op.Fields, Field{
				Name: "DropSignatures",
				Form: "dropSignatures",
				Type: "bool",
				Doc: "allows the operation to rewrite a signed PDF. The signatures are left out " +
					"and their visible appearance is drawn into the page; without it, the server " +
					"refuses a signed input.",
			})
		}

		err = defs.checkOp(op)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		defs.Ops = append(defs.Ops, op)
	}

	return defs, nil
}

func loadFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Load(data, out, yaml.WithKnownFields())
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	return nil
}

// resolve prepends the fields of the included groups, in include order, to
// the own fields.
func (d *Definitions) resolve(include []string, own []Field) ([]Field, error) {
	var fields []Field
	for _, name := range include {
		g, ok := d.Groups[name]
		if !ok {
			return nil, fmt.Errorf("unknown group %q", name)
		}

		fields = append(fields, g.Fields...)
	}

	return append(fields, own...), nil
}

func (d *Definitions) checkBuilder(b *Builder) error {
	if b.Type == "" || b.Route == "" || b.Entry == "" {
		return errors.New("type, route and entry are required")
	}

	switch b.Output {
	case "file":
		if b.Result != "" {
			return errors.New("result only applies to output json")
		}
	case "json":
		if b.Result == "" {
			return errors.New("output json requires result")
		}
	default:
		return fmt.Errorf("invalid output %q", b.Output)
	}

	return d.checkFields(b.Fields, true)
}

func (d *Definitions) checkOp(op *Op) error {
	if op.Op == "" || op.Name == "" {
		return errors.New("op and name are required")
	}

	all := slices.Clone(op.Fields)
	for _, v := range op.Variants {
		if v.Name == "" {
			return errors.New("variant without name")
		}

		all = append(all, v.Fields...)
	}

	return d.checkFields(all, false)
}

// checkFields validates the fields of one builder: known types, required
// keys, and no duplicate form field or method name.
func (d *Definitions) checkFields(fields []Field, request bool) error {
	forms := map[string]bool{}
	names := map[string]bool{}

	for _, f := range fields {
		if f.Name == "" || f.Type == "" {
			return fmt.Errorf("field %q: name and type are required", f.Name)
		}

		if !slices.Contains(fieldTypes, f.Type) {
			return fmt.Errorf("field %s: unknown type %q", f.Name, f.Type)
		}

		if f.Type == "fileContent" {
			if !request {
				return fmt.Errorf("field %s: fileContent only applies to request builders", f.Name)
			}

			if f.File == "" || f.Required {
				return fmt.Errorf("field %s: fileContent requires file and cannot be required", f.Name)
			}
		} else if f.Form == "" {
			return fmt.Errorf("field %s: form is required", f.Name)
		}

		if f.Type == "enum" && d.enums[f.Enum] == nil {
			return fmt.Errorf("field %s: unknown enum %q", f.Name, f.Enum)
		}

		if f.Type == "json" && f.GoType == "" {
			return fmt.Errorf("field %s: json requires goType", f.Name)
		}

		if f.Type == "json" && f.Example == "" {
			return fmt.Errorf("field %s: json requires example", f.Name)
		}

		if f.Variadic && (f.Type != "json" || !strings.HasPrefix(f.GoType, "[]")) {
			return fmt.Errorf("field %s: variadic requires a json slice", f.Name)
		}

		if f.Type == "profileSet" && !f.Required {
			return fmt.Errorf("field %s: profileSet must be required", f.Name)
		}

		if f.FileRef && f.Type != "string" && f.GoType != "[]Placement" {
			return fmt.Errorf("field %s: fileRef requires a string or []Placement", f.Name)
		}

		if (f.Min != nil || f.Max != nil) && f.Type != "int" && f.Type != "float" {
			return fmt.Errorf("field %s: min and max only apply to int and float", f.Name)
		}

		if f.Form != "" {
			if forms[f.Form] {
				return fmt.Errorf("duplicate form field %q", f.Form)
			}

			forms[f.Form] = true
		}

		for _, method := range methodNames(f) {
			if names[method] {
				return fmt.Errorf("duplicate method %s", method)
			}

			names[method] = true
		}
	}

	return nil
}

// methodNames returns the names of the methods a field generates.
func methodNames(f Field) []string {
	switch {
	case f.Type == "fileContent":
		return []string{f.Name + "FromPath", f.Name + "FromFS", f.Name + "FromReader", f.Name + "FromBytes"}
	case f.Required:
		return []string{"set" + f.Name}
	default:
		return []string{f.Name}
	}
}

// requiredFields returns the required fields, the parameters of the entry
// method or constructor.
func requiredFields(fields []Field) []Field {
	var res []Field
	for _, f := range fields {
		if f.Required {
			res = append(res, f)
		}
	}

	return res
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
