package gotenberg

import (
	"encoding/json"
	"errors"
	"maps"
)

// Stage is a PDF operation run after a conversion, or on uploaded PDFs via
// [PDF.Pipe]. Stages are created with the Stage… constructors, such as
// [StageMerge] or [StageStampText], and passed to the Stages method of a
// request builder. All stage builders are immutable value types.
//
// Files used by a stage, such as the image of [StageStampImage], are
// referenced by filename only: upload them on the request builder via
// FileFrom…, ArchiveFrom… or FileFromURL.
type Stage interface {
	stageState() stage
}

// stage is the shared state of the stage builders.
type stage struct {
	op     string
	fields map[string]json.RawMessage
	// refs holds, per option, the filenames of the uploaded files the
	// option references.
	refs map[string][]string
	err  error
}

func newStage(op string) stage {
	return stage{op: op, fields: map[string]json.RawMessage{}, refs: map[string][]string{}}
}

func (s stage) stageState() stage {
	return s
}

// MarshalJSON encodes the stage as the JSON object the server expects, such
// as {"op":"stamp-text","text":"DRAFT"}.
func (s stage) MarshalJSON() ([]byte, error) {
	if s.op == "" {
		return nil, errors.New("gotenberg: stage not created by a Stage constructor")
	}

	object := maps.Clone(s.fields)
	if object == nil {
		object = map[string]json.RawMessage{}
	}

	object["op"] = mustMarshal(s.op)

	return json.Marshal(object)
}

// set sets the option name, or records the deferred error of v.
func (s *stage) set(name string, v fieldValue) {
	if v.err != nil {
		s.err = errors.Join(s.err, v.err)
		return
	}

	s.fields = maps.Clone(s.fields)
	if s.fields == nil {
		s.fields = map[string]json.RawMessage{}
	}

	s.fields[name] = v.json
}

// ref records the filenames the option name references.
func (s *stage) ref(name string, filenames ...string) {
	s.refs = maps.Clone(s.refs)
	if s.refs == nil {
		s.refs = map[string][]string{}
	}

	s.refs[name] = filenames
}

// pairProfile completes the profile of the option name with the second
// profile of a pair, replacing a previous one.
func (s *stage) pairProfile(name string, p Profile) {
	raw, ok := s.fields[name]
	if !ok {
		// The first profile was invalid; its error is recorded.
		return
	}

	var profiles []Profile
	if json.Unmarshal(raw, &profiles) != nil {
		var one Profile
		_ = json.Unmarshal(raw, &one)
		profiles = []Profile{one}
	}

	s.set(name, profileValue(name, profiles[0], p))
}

// Bookmark is an entry of a document outline, returned by [PDF.Bookmarks]
// and set by [StageSetBookmarks].
type Bookmark struct {
	Title string `json:"title"`
	// PageNumber is the 1-based page the bookmark leads to.
	PageNumber int        `json:"pageNumber"`
	Children   []Bookmark `json:"children,omitempty"`
}

// Placement draws a page of one of the current documents on a page of the
// document [StageCompose] builds.
type Placement struct {
	// Source is the filename of the current document the page is taken
	// from.
	Source string `json:"source,omitempty"`
	// SourcePage is the 1-based page of Source to draw.
	SourcePage int `json:"sourcePage,omitempty"`
	// Page is the 1-based page of the composed document to draw on.
	Page int `json:"page,omitempty"`
	// X and Y are the offset, in points, from the bottom-left corner.
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`
	// Scale scales the drawn page; the server default applies when 0.
	Scale    float64 `json:"scale,omitempty"`
	Rotation float64 `json:"rotation,omitempty"`
}

// RedactRegion is a rectangle [StageRedact] removes the content of, in
// points from the bottom-left corner of a 1-based page.
type RedactRegion struct {
	Page   int     `json:"page"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Permissions lists what a document encrypted by [StageEncrypt] allows. All
// permissions are sent: a permission left false is denied. Without
// [EncryptStage.Permissions], the server allows everything.
type Permissions struct {
	Print             bool `json:"print"`
	PrintHighQuality  bool `json:"printHighQuality"`
	ModifyContent     bool `json:"modifyContent"`
	ExtractText       bool `json:"extractText"`
	ModifyAnnotations bool `json:"modifyAnnotations"`
	FillForms         bool `json:"fillForms"`
	Assemble          bool `json:"assemble"`
}

// placementSources returns the filenames the placements reference, which
// must keep their names for merge order.
func placementSources(placements []Placement) []string {
	var names []string
	for _, p := range placements {
		if p.Source != "" {
			names = append(names, p.Source)
		}
	}

	return names
}
