package gotenberg

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// fieldValue is an option value encoded both as a multipart form field
// (form, one entry per repetition) and as a JSON value of a stage (json). err
// is the deferred validation error of the value.
type fieldValue struct {
	form []string
	json json.RawMessage
	err  error
}

// enum is implemented by the generated enum types.
type enum interface {
	comparable
	valid() bool
}

func stringValue(v string) fieldValue {
	return fieldValue{form: []string{v}, json: mustMarshal(v)}
}

func boolValue(v bool) fieldValue {
	return fieldValue{form: []string{strconv.FormatBool(v)}, json: mustMarshal(v)}
}

func intValue(v int, errs ...error) fieldValue {
	return fieldValue{form: []string{strconv.Itoa(v)}, json: mustMarshal(v), err: errors.Join(errs...)}
}

func floatValue(v float64, errs ...error) fieldValue {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fieldValue{err: fmt.Errorf("invalid number %g", v)}
	}

	s := strconv.FormatFloat(v, 'f', -1, 64)

	return fieldValue{form: []string{s}, json: json.RawMessage(s), err: errors.Join(errs...)}
}

func durationValue(v time.Duration) fieldValue {
	return stringValue(v.String())
}

func enumValue[E enum](name string, v E) fieldValue {
	if !v.valid() {
		return fieldValue{err: fmt.Errorf("invalid %s %v", name, v)}
	}

	return fieldValue{form: []string{fmt.Sprint(v)}, json: mustMarshal(v)}
}

func jsonValue(v any, errs ...error) fieldValue {
	err := errors.Join(errs...)
	if err != nil {
		return fieldValue{err: err}
	}

	b, err := json.Marshal(v)
	if err != nil {
		return fieldValue{err: err}
	}

	return fieldValue{form: []string{string(b)}, json: b}
}

// profileValue encodes one profile, or a pair: the form field is repeated
// for a pair, and the JSON value is a string, or an array of the two.
func profileValue(name string, profiles ...Profile) fieldValue {
	form := make([]string, len(profiles))
	for i, p := range profiles {
		if !p.valid() {
			return fieldValue{err: fmt.Errorf("invalid %s %v", name, p)}
		}

		form[i] = string(p)
	}

	if len(profiles) == 1 {
		return fieldValue{form: form, json: mustMarshal(profiles[0])}
	}

	return fieldValue{form: form, json: mustMarshal(profiles)}
}

func checkMinInt(name string, v, min int) error {
	if v < min {
		return fmt.Errorf("%s must be at least %d, got %d", name, min, v)
	}

	return nil
}

func checkMaxInt(name string, v, max int) error {
	if v > max {
		return fmt.Errorf("%s must be at most %d, got %d", name, max, v)
	}

	return nil
}

func checkMinFloat(name string, v, min float64, exclusive bool) error {
	if exclusive && v <= min {
		return fmt.Errorf("%s must be greater than %g, got %g", name, min, v)
	}

	if v < min {
		return fmt.Errorf("%s must be at least %g, got %g", name, min, v)
	}

	return nil
}

func checkMaxFloat(name string, v, max float64) error {
	if v > max {
		return fmt.Errorf("%s must be at most %g, got %g", name, max, v)
	}

	return nil
}

// checkEnums validates the enum elements of a JSON-encoded slice.
func checkEnums[E enum](name string, values []E) error {
	for i, v := range values {
		if !v.valid() {
			return fmt.Errorf("invalid %s entry %d: %v", name, i, v)
		}
	}

	return nil
}

// mustMarshal encodes values that cannot fail to encode: strings, numbers,
// booleans and slices of them.
func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return b
}
