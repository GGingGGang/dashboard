package sdk

import (
	"encoding/json"
	"testing"
)

func TestFormSchemaDefaultsAndClosedInputs(t *testing.T) {
	s, err := ParseFormSchema(json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":5,"default":2},"enabled":{"type":"boolean"}},"required":["count"]}`))
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Values(json.RawMessage(`{}`))
	if err != nil || v["count"] != float64(2) {
		t.Fatal(v, err)
	}
	for _, raw := range []string{`{"count":6}`, `{"count":1.5}`, `{"count":"2"}`, `{"enabled":null}`, `{"secret":"oops"}`, `null`} {
		if _, err = s.Values(json.RawMessage(raw)); err == nil {
			t.Fatal("accepted invalid input", raw)
		}
	}
	for _, raw := range []string{`{"type":"object","properties":{"x":{"type":"string","pattern":".*"}}}`, `{"type":"object","properties":{"x":{"type":"object"}}}`, `{"type":"object","additionalProperties":true}`, `{"type":"object","required":["missing"]}`} {
		if _, err = ParseFormSchema(json.RawMessage(raw)); err == nil {
			t.Fatal("accepted unsupported schema", raw)
		}
	}
}
func TestTypedDataCopyAndValidation(t *testing.T) {
	original := DataResult{Type: "table", Columns: []DataColumn{{Name: "id", Type: "string"}}, Rows: [][]any{{"9223372036854775807"}}}
	copied, err := original.CheckedCopy()
	if err != nil {
		t.Fatal(err)
	}
	original.Rows[0][0] = "changed"
	if copied.Rows[0][0] != "9223372036854775807" {
		t.Fatal("result aliases provider")
	}
	for _, r := range []DataResult{{Type: "script"}, {Type: "table", Columns: []DataColumn{{Name: "n", Type: "number"}}, Rows: [][]any{{9007199254740992.0}}}, {Type: "table", Columns: []DataColumn{{Name: "n", Type: "number"}}, Rows: [][]any{{"not a number"}}}, {Type: "scalar", Value: map[string]string{"unsafe": "object"}}} {
		if _, err := r.CheckedCopy(); err == nil {
			t.Fatal("accepted invalid data", r)
		}
	}
}
