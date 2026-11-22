package gotenberg

import (
	"encoding/json"
)

// The result types of the read routes are named Document… because the
// builders use the PDF… names, such as [PDFInfo].

// DocumentInfo is the answer of [PDF.Info] for one document.
type DocumentInfo struct {
	Pages      int    `json:"pages"`
	Version    string `json:"version"`
	Encrypted  bool   `json:"encrypted"`
	Tagged     bool   `json:"tagged"`
	Linearized bool   `json:"linearized"`
	FormFields bool   `json:"formFields"`
	Signed     bool   `json:"signed"`
}

// DocumentMetadata is the answer of [PDF.Metadata] for one document. A field
// the document does not carry is empty. The dates are PDF date strings, such
// as "D:20240102150405Z".
type DocumentMetadata struct {
	Title        string `json:"title,omitempty"`
	Author       string `json:"author,omitempty"`
	Subject      string `json:"subject,omitempty"`
	Keywords     string `json:"keywords,omitempty"`
	Creator      string `json:"creator,omitempty"`
	Producer     string `json:"producer,omitempty"`
	CreationDate string `json:"creationDate,omitempty"`
	ModDate      string `json:"modDate,omitempty"`
}

// Attachment is a file embedded in a document, returned by
// [PDF.Attachments].
type Attachment struct {
	Name         string `json:"name"`
	MIMEType     string `json:"mimeType,omitempty"`
	Relationship string `json:"relationship,omitempty"`
}

// FormField is an interactive form field, returned by [PDF.Form]. Its name
// and value feed [StageSetForm] as they are.
type FormField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Value is a string, or a []any of strings for a list that selects
	// several.
	Value       any      `json:"value"`
	Pages       []int    `json:"pages"`
	Options     []string `json:"options,omitempty"`
	States      []string `json:"states,omitempty"`
	ReadOnly    bool     `json:"readOnly,omitempty"`
	Required    bool     `json:"required,omitempty"`
	MultiSelect bool     `json:"multiSelect,omitempty"`
	Editable    bool     `json:"editable,omitempty"`
	MaxLength   int      `json:"maxLength,omitempty"`
}

// ValidationReport is the answer of [PDF.Validate] for one document.
type ValidationReport struct {
	// Profile is the validated profile, or the two profiles of a pair.
	Profile    []Profile   `json:"profile"`
	Compliant  bool        `json:"compliant"`
	Violations []Violation `json:"violations"`
}

// UnmarshalJSON decodes a report whose profile is a string, or an array for
// a pair.
func (r *ValidationReport) UnmarshalJSON(data []byte) error {
	var raw struct {
		Profile    json.RawMessage `json:"profile"`
		Compliant  bool            `json:"compliant"`
		Violations []Violation     `json:"violations"`
	}

	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	r.Compliant = raw.Compliant
	r.Violations = raw.Violations
	r.Profile = nil

	if len(raw.Profile) == 0 || string(raw.Profile) == "null" {
		return nil
	}

	var one Profile
	if json.Unmarshal(raw.Profile, &one) == nil {
		r.Profile = []Profile{one}
		return nil
	}

	return json.Unmarshal(raw.Profile, &r.Profile)
}

// Violation is one conformance failure of a [ValidationReport].
type Violation struct {
	Specification string `json:"specification,omitempty"`
	Clause        string `json:"clause,omitempty"`
	TestNumber    int    `json:"testNumber,omitempty"`
	Description   string `json:"description"`
	Context       string `json:"context,omitempty"`
	Severity      string `json:"severity,omitempty"`
}
