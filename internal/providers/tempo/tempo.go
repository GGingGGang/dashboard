package tempo

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers/internal/httpapi"
)

type Tempo struct{ api *httpapi.Client }

func Definition() platform.Definition {
	return platform.Definition{
		Info: platform.ProviderInfo{Kind: "tempo", Name: "Tempo", Category: "observability", Capabilities: []string{"traces"}, OnDemand: true, DefaultAuth: "none", AuthMethods: []string{"none", "basic", "bearer"}, Queries: map[string]platform.QueryInfo{"traces": {Language: "TraceQL", DefaultExpression: `{ true }`, Modes: []string{"search", "detail"}, MaxResults: 100}}},
		Create: func(c platform.Connection, secret string) (platform.Provider, error) {
			api, err := httpapi.New(c, secret)
			if err != nil {
				return nil, err
			}
			return &Tempo{api}, nil
		},
	}
}
func (p *Tempo) Close() error { return p.api.Close() }
func (p *Tempo) Check(ctx context.Context) error {
	now := time.Now().Unix()
	_, err := p.SearchTraces(ctx, platform.SearchRequest{Expression: `{ true }`, Start: now - 60, End: now, Limit: 1})
	return err
}
func (p *Tempo) SearchTraces(ctx context.Context, q platform.SearchRequest) (platform.TraceSearchResult, error) {
	out := platform.TraceSearchResult{Traces: []platform.TraceSummary{}, Observation: platform.Observation{Completeness: "unknown", Diagnostics: []string{"Search returns matching trace summaries, not a complete span set or a stable ordered archive."}}}
	if err := q.Validate(100); err != nil {
		return out, err
	}
	var response struct {
		Traces []struct {
			ID       string  `json:"traceID"`
			Service  string  `json:"rootServiceName"`
			Name     string  `json:"rootTraceName"`
			Start    string  `json:"startTimeUnixNano"`
			Duration float64 `json:"durationMs"`
		} `json:"traces"`
	}
	args := url.Values{"q": {q.Expression}, "start": {strconv.FormatInt(q.Start, 10)}, "end": {strconv.FormatInt(q.End, 10)}, "limit": {strconv.Itoa(q.Limit)}}
	if err := p.api.Get(ctx, "/api/search", args, &response); err != nil {
		return out, err
	}
	for _, t := range response.Traces {
		id, err := traceID(t.ID)
		if err != nil {
			return out, err
		}
		if _, err = strconv.ParseUint(t.Start, 10, 64); err != nil || t.Duration < 0 {
			return out, errors.New("Invalid Tempo trace timestamp or duration")
		}
		out.Traces = append(out.Traces, platform.TraceSummary{TraceID: id, RootService: t.Service, RootName: t.Name, StartNS: t.Start, DurationMS: t.Duration})
		if len(out.Traces) == q.Limit {
			out.Observation.Diagnostics = append(out.Observation.Diagnostics, "Result limit reached; narrow your query or time range.")
			break
		}
	}
	return out, nil
}

func traceID(raw string) (string, error) {
	if len(raw) < 1 || len(raw) > 32 {
		return "", errors.New("Enter a hexadecimal trace ID of up to 32 characters")
	}
	for _, c := range raw {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return "", errors.New("Invalid trace ID")
		}
	}
	return strings.Repeat("0", 32-len(raw)) + strings.ToLower(raw), nil
}
func wireID(raw string, bytes int) (string, error) {
	if len(raw) == bytes*2 {
		if _, err := hex.DecodeString(raw); err == nil {
			return strings.ToLower(raw), nil
		}
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(b) != bytes {
		return "", errors.New("Invalid Tempo span identifier")
	}
	return hex.EncodeToString(b), nil
}

type scopeSpans struct {
	Scope json.RawMessage   `json:"scope"`
	Spans []json.RawMessage `json:"spans"`
}
type batch struct {
	Resource struct {
		Attributes []platform.Attribute `json:"attributes"`
	} `json:"resource"`
	Scopes       []scopeSpans `json:"scopeSpans"`
	LegacyScopes []struct {
		Scope json.RawMessage   `json:"instrumentationLibrary"`
		Spans []json.RawMessage `json:"spans"`
	} `json:"instrumentationLibrarySpans"`
}

func (p *Tempo) Trace(ctx context.Context, rawID string) (platform.TraceResult, error) {
	out := platform.TraceResult{Spans: []platform.Span{}, Observation: platform.Observation{Completeness: "unknown", Diagnostics: []string{"Returned spans may be sampled or incomplete. A missing parent is not proof of a root span."}}}
	id, err := traceID(rawID)
	if err != nil {
		return out, err
	}
	out.TraceID = id
	var response struct {
		Batches   []batch `json:"batches"`
		Resources []batch `json:"resourceSpans"`
	}
	// Do not apply the search window: it could exclude other blocks of this trace.
	if err = p.api.Get(ctx, "/api/traces/"+id, nil, &response); err != nil {
		return out, err
	}
	for _, b := range append(response.Batches, response.Resources...) {
		service := ""
		for _, a := range b.Resource.Attributes {
			if a.Key == "service.name" {
				var v struct {
					Value string `json:"stringValue"`
				}
				_ = json.Unmarshal(a.Value, &v)
				service = v.Value
			}
		}
		for _, s := range b.LegacyScopes {
			b.Scopes = append(b.Scopes, scopeSpans{Scope: s.Scope, Spans: s.Spans})
		}
		for _, scope := range b.Scopes {
			for _, raw := range scope.Spans {
				if len(out.Spans) >= 2000 {
					out.Observation.Completeness = "truncated"
					out.Observation.Diagnostics = append(out.Observation.Diagnostics, "Showing the first 2000 spans.")
					return out.Clone(), nil
				}
				var s struct {
					TraceID    string               `json:"traceId"`
					SpanID     string               `json:"spanId"`
					ParentID   string               `json:"parentSpanId"`
					Name       string               `json:"name"`
					Start      string               `json:"startTimeUnixNano"`
					End        string               `json:"endTimeUnixNano"`
					Attributes []platform.Attribute `json:"attributes"`
				}
				if json.Unmarshal(raw, &s) != nil {
					return out, errors.New("Invalid Tempo span")
				}
				tid, err := wireID(s.TraceID, 16)
				if err != nil {
					return out, err
				}
				if tid != id {
					return out, errors.New("Tempo returned a span from another trace")
				}
				sid, err := wireID(s.SpanID, 8)
				if err != nil {
					return out, err
				}
				parent := ""
				if s.ParentID != "" {
					parent, err = wireID(s.ParentID, 8)
					if err != nil {
						return out, err
					}
				}
				start, e1 := strconv.ParseUint(s.Start, 10, 64)
				end, e2 := strconv.ParseUint(s.End, 10, 64)
				if e1 != nil || e2 != nil || end < start {
					return out, errors.New("Invalid Tempo span timing")
				}
				out.Spans = append(out.Spans, platform.Span{TraceID: tid, SpanID: sid, ParentSpanID: parent, Name: s.Name, Service: service, StartNS: s.Start, DurationNS: strconv.FormatUint(end-start, 10), Attributes: s.Attributes, Resource: b.Resource.Attributes, Details: raw, Scope: scope.Scope})
			}
		}
	}
	return out.Clone(), nil
}
