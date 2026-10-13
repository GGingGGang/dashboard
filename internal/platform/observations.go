package platform

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"
)

// Search windows use epoch seconds. Event timestamps and opaque IDs use strings.
type SearchRequest struct {
	Expression string `json:"expression"`
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	Limit      int    `json:"limit"`
}

func (q SearchRequest) Validate(maxRows int) error {
	if strings.TrimSpace(q.Expression) == "" || len(q.Expression) > 16384 {
		return errors.New("Enter a query of at most 16 KiB")
	}
	if q.Start <= 0 || q.End <= q.Start || q.End-q.Start > 86400 || q.End > time.Now().Unix()+60 {
		return errors.New("Choose a time range of up to 24 hours ending no later than now")
	}
	if q.Limit < 1 || q.Limit > maxRows {
		return errors.New("Query limit is outside the supported range")
	}
	return nil
}

type Observation struct {
	ConnectionID string    `json:"connectionId"`
	Revision     string    `json:"revision"`
	RequestID    string    `json:"requestId"`
	ObservedAt   time.Time `json:"observedAt"`
	WindowStart  int64     `json:"windowStart"`
	WindowEnd    int64     `json:"windowEnd"`
	Outcome      string    `json:"outcome"`      // success, partial, error
	Completeness string    `json:"completeness"` // complete, unknown, truncated
	Diagnostics  []string  `json:"diagnostics"`
}

type LogSource interface {
	Provider
	SearchLogs(context.Context, SearchRequest) (LogResult, error)
}
type TraceSource interface {
	Provider
	SearchTraces(context.Context, SearchRequest) (TraceSearchResult, error)
	Trace(context.Context, string) (TraceResult, error)
}
type LogEntry struct {
	TimestampNS string            `json:"timestampNs"`
	Line        string            `json:"line"`
	Labels      map[string]string `json:"labels"`
	Fields      map[string]string `json:"fields"`
}
type LogResult struct {
	Observation Observation `json:"observation"`
	Entries     []LogEntry  `json:"entries"`
}
type TraceSummary struct {
	TraceID     string  `json:"traceId"`
	RootService string  `json:"rootService"`
	RootName    string  `json:"rootName"`
	StartNS     string  `json:"startNs"`
	DurationMS  float64 `json:"durationMs"`
}
type TraceSearchResult struct {
	Observation Observation    `json:"observation"`
	Traces      []TraceSummary `json:"traces"`
}

// Attribute.Value retains OTLP typed values, including decimal int64 strings.
type Attribute struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}
type Span struct {
	TraceID      string      `json:"traceId"`
	SpanID       string      `json:"spanId"`
	ParentSpanID string      `json:"parentSpanId"`
	Name         string      `json:"name"`
	Service      string      `json:"service"`
	StartNS      string      `json:"startNs"`
	DurationNS   string      `json:"durationNs"`
	Attributes   []Attribute `json:"attributes"`
	Resource     []Attribute `json:"resource"`
	// Preserve status, events, links, scope and future OTLP fields without coercion.
	Details json.RawMessage `json:"details"`
	Scope   json.RawMessage `json:"scope"`
}
type TraceResult struct {
	Observation Observation `json:"observation"`
	TraceID     string      `json:"traceId"`
	Spans       []Span      `json:"spans"`
}

func (o Observation) Clone() Observation { o.Diagnostics = slices.Clone(o.Diagnostics); return o }
func (r LogResult) Clone() LogResult {
	r.Observation = r.Observation.Clone()
	r.Entries = slices.Clone(r.Entries)
	for i := range r.Entries {
		r.Entries[i].Labels = maps.Clone(r.Entries[i].Labels)
		r.Entries[i].Fields = maps.Clone(r.Entries[i].Fields)
	}
	return r
}
func (r TraceSearchResult) Clone() TraceSearchResult {
	r.Observation = r.Observation.Clone()
	r.Traces = slices.Clone(r.Traces)
	return r
}
func cloneAttributes(a []Attribute) []Attribute {
	a = slices.Clone(a)
	for i := range a {
		a[i].Value = slices.Clone(a[i].Value)
	}
	return a
}
func (r TraceResult) Clone() TraceResult {
	r.Observation = r.Observation.Clone()
	r.Spans = slices.Clone(r.Spans)
	for i := range r.Spans {
		s := &r.Spans[i]
		s.Attributes = cloneAttributes(s.Attributes)
		s.Resource = cloneAttributes(s.Resource)
		s.Details = slices.Clone(s.Details)
		s.Scope = slices.Clone(s.Scope)
	}
	return r
}
