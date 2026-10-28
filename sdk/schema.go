package sdk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
)

// FormSchema is the contract-v1 JSON Schema subset: flat objects and scalar fields.
// Unknown schema keywords are rejected, never silently ignored.
type FormSchema struct {
	Type                 string               `json:"type"`
	Properties           map[string]FormField `json:"properties"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties bool                 `json:"additionalProperties,omitempty"`
}
type FormField struct {
	Type        string   `json:"type"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Enum        []any    `json:"enum,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	MaxLength   *int     `json:"maxLength,omitempty"`
}

func ParseFormSchema(raw json.RawMessage) (FormSchema, error) {
	s := FormSchema{Type: "object", Properties: map[string]FormField{}}
	if len(raw) == 0 || string(raw) == "null" {
		return s, nil
	}
	if len(raw) > 65536 {
		return s, errors.New("Schema exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, err
	}
	if s.Type != "object" || s.AdditionalProperties || len(s.Properties) > 40 {
		return s, errors.New("Only closed flat object schemas are supported")
	}
	for name, f := range s.Properties {
		if name == "" || len(name) > 128 || !slices.Contains([]string{"string", "number", "integer", "boolean"}, f.Type) {
			return s, errors.New("Invalid schema field")
		}
		if (f.Minimum != nil || f.Maximum != nil) && f.Type != "number" && f.Type != "integer" {
			return s, errors.New("Numeric bounds need a numeric field")
		}
		if f.Minimum != nil && f.Maximum != nil && *f.Minimum > *f.Maximum {
			return s, errors.New("Invalid bounds")
		}
		if f.MaxLength != nil && (f.Type != "string" || *f.MaxLength < 0 || *f.MaxLength > 16384) {
			return s, errors.New("Invalid maxLength")
		}
		if len(f.Enum) > 100 {
			return s, errors.New("Enum exceeds 100 choices")
		}
		for _, v := range f.Enum {
			if err := f.validate(v, false); err != nil {
				return s, err
			}
		}
		if f.Default != nil {
			if err := f.validate(f.Default, true); err != nil {
				return s, err
			}
		}
	}
	seen := map[string]bool{}
	for _, name := range s.Required {
		if _, ok := s.Properties[name]; !ok || seen[name] {
			return s, errors.New("Invalid required field")
		}
		seen[name] = true
	}
	return s, nil
}
func (f FormField) validate(v any, enum bool) error {
	valid := false
	switch f.Type {
	case "string":
		x, ok := v.(string)
		max := 16384
		if f.MaxLength != nil {
			max = *f.MaxLength
		}
		valid = ok && len([]rune(x)) <= max
	case "boolean":
		_, valid = v.(bool)
	case "number", "integer":
		x, ok := v.(float64)
		valid = ok && !math.IsNaN(x) && !math.IsInf(x, 0) && math.Abs(x) <= 9007199254740991 && (f.Type != "integer" || math.Trunc(x) == x) && (f.Minimum == nil || x >= *f.Minimum) && (f.Maximum == nil || x <= *f.Maximum)
	}
	if !valid {
		return errors.New("Field type or bounds mismatch")
	}
	if enum && len(f.Enum) > 0 {
		for _, item := range f.Enum {
			if item == v {
				return nil
			}
		}
		return errors.New("Value is not an enum choice")
	}
	return nil
}
func (s FormSchema) Values(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if len(raw) > 65536 {
		return nil, errors.New("Input exceeds 64 KiB")
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return nil, errors.New("Input must be an object")
	}
	for name, v := range values {
		f, ok := s.Properties[name]
		if !ok {
			return nil, fmt.Errorf("Unknown input: %s", name)
		}
		if err := f.validate(v, true); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	for name, f := range s.Properties {
		if _, ok := values[name]; !ok && f.Default != nil {
			values[name] = f.Default
		}
	}
	for _, name := range s.Required {
		v, ok := values[name]
		if !ok || v == "" {
			return nil, fmt.Errorf("Required input: %s", name)
		}
	}
	return values, nil
}
