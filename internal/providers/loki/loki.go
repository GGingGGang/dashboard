package loki

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"time"

	platform "github.com/GGingGGang/dashboard/sdk"
	"github.com/GGingGGang/dashboard/sdk/httpapi"
)

type Loki struct{ api *httpapi.Client }

func Definition() platform.Definition {
	return platform.Definition{
		Info: platform.ProviderInfo{Kind: "loki", Name: "Loki", Category: "observability", Capabilities: []string{"logs"}, OnDemand: true, DefaultAuth: "none", AuthMethods: []string{"none", "basic", "bearer"}, Queries: map[string]platform.QueryInfo{"logs": {Language: "LogQL", DefaultExpression: `{service_name=~".+"}`, Modes: []string{"search"}, MaxResults: 500}}},
		Create: func(c platform.Connection, secret string) (platform.Provider, error) {
			api, err := httpapi.New(c, secret)
			if err != nil {
				return nil, err
			}
			return &Loki{api}, nil
		},
	}
}
func (p *Loki) Close() error { return p.api.Close() }

// A successful probe is not evidence that every user's query is authorized.
func (p *Loki) Check(ctx context.Context) error {
	now := time.Now().Unix()
	_, err := p.SearchLogs(ctx, platform.SearchRequest{Expression: `{service_name=~".+"}`, Start: now - 60, End: now, Limit: 1})
	return err
}
func (p *Loki) SearchLogs(ctx context.Context, q platform.SearchRequest) (out platform.LogResult, resultErr error) {
	out = platform.LogResult{Entries: []platform.LogEntry{}, Observation: platform.Observation{Completeness: "complete"}}
	if err := q.Validate(500); err != nil {
		return out, err
	}
	defer func() {
		// Apply the output bound on error paths too; valid partial rows remain usable.
		sort.SliceStable(out.Entries, func(i, j int) bool {
			a, _ := strconv.ParseInt(out.Entries[i].TimestampNS, 10, 64)
			b, _ := strconv.ParseInt(out.Entries[j].TimestampNS, 10, 64)
			return a > b
		})
		if len(out.Entries) >= q.Limit {
			out.Entries = out.Entries[:q.Limit]
			out.Observation.Completeness = "unknown"
			out.Observation.Diagnostics = append(out.Observation.Diagnostics, "Result limit reached. Narrow the time range or query; equal timestamps prevent a lossless next-page cursor.")
		}
		out = out.Clone()
	}()
	args := url.Values{"query": {q.Expression}, "start": {strconv.FormatInt(q.Start*1e9, 10)}, "end": {strconv.FormatInt(q.End*1e9, 10)}, "limit": {strconv.Itoa(q.Limit)}, "direction": {"backward"}}
	var response struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
		Data     struct {
			Type   string `json:"resultType"`
			Result []struct {
				Stream map[string]string   `json:"stream"`
				Values [][]json.RawMessage `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := p.api.Get(ctx, "/loki/api/v1/query_range", args, &response); err != nil {
		return out, err
	}
	if response.Status != "success" || response.Data.Type != "streams" {
		return out, errors.New("Use a LogQL log query; metric results are not supported by this source")
	}
	for _, w := range response.Warnings {
		out.Observation.Diagnostics = append(out.Observation.Diagnostics, p.api.Redact(w))
		out.Observation.Completeness = "unknown"
	}
	for _, stream := range response.Data.Result {
		for _, v := range stream.Values {
			if len(v) < 2 || len(v) > 3 {
				return out, errors.New("Invalid Loki log entry")
			}
			e := platform.LogEntry{Labels: stream.Stream, Fields: map[string]string{}}
			if json.Unmarshal(v[0], &e.TimestampNS) != nil || json.Unmarshal(v[1], &e.Line) != nil {
				return out, errors.New("Invalid Loki timestamp or line")
			}
			ns, err := strconv.ParseInt(e.TimestampNS, 10, 64)
			if err != nil || ns < q.Start*1e9 || ns >= q.End*1e9 {
				return out, errors.New("Loki returned an invalid or out-of-window timestamp")
			}
			e.TimestampNS = strconv.FormatInt(ns, 10)
			if len(v) == 3 && json.Unmarshal(v[2], &e.Fields) != nil {
				return out, errors.New("Invalid Loki structured metadata")
			}
			out.Entries = append(out.Entries, e)
		}
	}
	return out, nil
}
