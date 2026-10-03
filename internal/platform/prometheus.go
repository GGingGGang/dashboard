package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Prometheus struct{ *api }

func (p *Prometheus) Check(ctx context.Context) error {
	_, err := p.Query(ctx, Query{Expression: "vector(1)"})
	return err
}
func (p *Prometheus) Discover(context.Context) ([]Target, error) { return []Target{}, nil }

func (p *Prometheus) Query(ctx context.Context, q Query) (QueryResult, error) {
	out := QueryResult{Series: []Series{}, Warnings: []string{}}
	if strings.TrimSpace(q.Expression) == "" || len(q.Expression) > 16384 {
		return out, errors.New("Enter a query of at most 16 KiB")
	}
	path := "/api/v1/query"
	args := url.Values{"query": {q.Expression}, "timeout": {"5s"}, "limit": {"101"}}
	if q.Start != 0 {
		if q.End <= q.Start || q.End-q.Start > 86400 || q.End > time.Now().Unix()+60 {
			return out, errors.New("Choose a time range of up to 24 hours ending no later than now")
		}
		path = "/api/v1/query_range"
		step := max(int64(15), (q.End-q.Start+598)/599)
		args.Set("start", strconv.FormatInt(q.Start, 10))
		args.Set("end", strconv.FormatInt(q.End, 10))
		args.Set("step", strconv.FormatInt(step, 10))
	}
	var response struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
		Infos    []string `json:"infos"`
		Data     struct {
			Type   string          `json:"resultType"`
			Result json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := p.get(ctx, path, args, &response); err != nil {
		return out, err
	}
	if response.Status != "success" {
		return out, errors.New("Prometheus rejected the query")
	}
	out.Type = response.Data.Type
	// Upstream warnings can echo expressions; secrets are never part of requests' query text.
	for _, w := range append(response.Warnings, response.Infos...) {
		if p.secret != "" {
			w = strings.ReplaceAll(w, p.secret, "[redacted]")
		}
		out.Warnings = append(out.Warnings, w)
	}
	switch out.Type {
	case "vector", "matrix":
		var rows []struct {
			Metric     map[string]string   `json:"metric"`
			Value      []json.RawMessage   `json:"value"`
			Values     [][]json.RawMessage `json:"values"`
			Histogram  json.RawMessage     `json:"histogram"`
			Histograms json.RawMessage     `json:"histograms"`
		}
		if err := json.Unmarshal(response.Data.Result, &rows); err != nil {
			return out, errors.New("Invalid Prometheus series")
		}
		if len(rows) > 100 {
			rows = rows[:100]
			out.Truncated = true
			out.Warnings = append(out.Warnings, "Showing the first 100 series; narrow your query")
		}
		for _, row := range rows {
			series := Series{Labels: row.Metric, Points: []Point{}}
			values := row.Values
			if out.Type == "vector" && len(row.Value) > 0 {
				values = [][]json.RawMessage{row.Value}
			}
			for _, pair := range values {
				point, err := decodePoint(pair)
				if err != nil {
					return out, err
				}
				series.Points = append(series.Points, point)
			}
			if len(row.Histogram) > 0 || len(row.Histograms) > 0 {
				out.Warnings = append(out.Warnings, "Native histogram samples are not plotted; use histogram_avg or histogram_quantile")
			}
			out.Series = append(out.Series, series)
		}
	case "scalar":
		var pair []json.RawMessage
		if err := json.Unmarshal(response.Data.Result, &pair); err != nil {
			return out, errors.New("Invalid scalar")
		}
		point, err := decodePoint(pair)
		if err != nil {
			return out, err
		}
		out.Series = append(out.Series, Series{Labels: map[string]string{}, Points: []Point{point}})
	case "string":
		out.Warnings = append(out.Warnings, "String results cannot be plotted; use a numeric expression")
	default:
		return out, fmt.Errorf("Unsupported query result type: %s", out.Type)
	}
	return out, nil
}

func decodePoint(pair []json.RawMessage) (Point, error) {
	var p Point
	if len(pair) != 2 {
		return p, errors.New("Invalid sample")
	}
	if err := json.Unmarshal(pair[0], &p.Time); err != nil {
		return p, errors.New("Invalid sample timestamp")
	}
	var raw string
	if err := json.Unmarshal(pair[1], &raw); err != nil {
		return p, errors.New("Invalid sample value")
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return p, errors.New("Invalid numeric sample")
	}
	if !math.IsInf(v, 0) && !math.IsNaN(v) {
		p.Value = &v
	}
	return p, nil
}
