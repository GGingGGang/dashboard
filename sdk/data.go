package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

// DataSource supplies new domains without adding product-specific host code.
// Cells are JSON scalars. Opaque IDs and integers beyond 2^53 must be strings.
type DataSource interface {
	Provider
	ReadData(context.Context, map[string]any) (DataResult, error)
}
type DataColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type DataResult struct {
	Type     string       `json:"type"`
	Columns  []DataColumn `json:"columns,omitempty"`
	Rows     [][]any      `json:"rows,omitempty"`
	Value    any          `json:"value,omitempty"`
	Series   []Series     `json:"series,omitempty"`
	Warnings []string     `json:"warnings,omitempty"`
}

func scalar(v any, kind string) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return kind == "string" && len(x) <= 65536
	case bool:
		return kind == "boolean"
	case float64:
		return kind == "number" && !math.IsNaN(x) && !math.IsInf(x, 0) && (math.Trunc(x) != x || math.Abs(x) <= 9007199254740991)
	}
	return false
}

// CheckedCopy validates and detaches the entire result before crossing the UI boundary.
func (r DataResult) CheckedCopy() (DataResult, error) {
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > 4*1024*1024 {
		return DataResult{}, errors.New("Invalid or oversized data result")
	}
	var out DataResult
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	invalid := func() (DataResult, error) { return DataResult{}, errors.New("Invalid typed data result") }
	switch out.Type {
	case "table":
		if len(out.Columns) > 64 || len(out.Rows) > 2000 {
			return invalid()
		}
		seen := map[string]bool{}
		for _, col := range out.Columns {
			if col.Name == "" || len(col.Name) > 256 || seen[col.Name] || (col.Type != "string" && col.Type != "number" && col.Type != "boolean") {
				return invalid()
			}
			seen[col.Name] = true
		}
		for _, row := range out.Rows {
			if len(row) != len(out.Columns) {
				return invalid()
			}
			for i, v := range row {
				if !scalar(v, out.Columns[i].Type) {
					return invalid()
				}
			}
		}
	case "scalar":
		if !scalar(out.Value, "string") && !scalar(out.Value, "number") && !scalar(out.Value, "boolean") {
			return invalid()
		}
	case "timeseries":
		if len(out.Series) > 100 {
			return invalid()
		}
		count := 0
		for _, s := range out.Series {
			count += len(s.Points)
			for _, p := range s.Points {
				if math.IsNaN(p.Time) || math.IsInf(p.Time, 0) || math.Abs(p.Time) > 8640000000000 {
					return invalid()
				}
			}
		}
		if count > 20000 {
			return invalid()
		}
	default:
		return invalid()
	}
	return out, nil
}
