package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"slices"
	"sort"
	"time"
)

type ContributionRequest struct {
	PluginID string          `json:"pluginId"`
	Kind     string          `json:"kind"`
	ID       string          `json:"id"`
	Input    json.RawMessage `json:"input"`
}

// ExecuteContribution resolves declarations before acquiring a connection-scoped source.
func (a *Service) ExecuteContribution(connectionID, sourceID string, request ContributionRequest) (result sdk.DataResult, err error) {
	ctx, done, c, err := a.begin(connectionID, 15*time.Second)
	if err != nil {
		return result, err
	}
	defer done()
	capability, renderer := "", ""
	var pluginRevision uint64
	var raw json.RawMessage
	for _, p := range a.registry.Plugins() {
		if p.Manifest.ID != request.PluginID || !p.Enabled {
			continue
		}
		pluginRevision = p.Revision
		if request.Kind == "card" {
			for _, v := range p.Manifest.Cards {
				if v.ID == request.ID {
					capability, renderer, raw = v.Capability, v.Renderer, v.ConfigSchema
				}
			}
		}
		if request.Kind == "query" {
			for _, v := range p.Manifest.Queries {
				if v.ID == request.ID {
					capability, renderer, raw = v.Capability, v.Renderer, v.InputSchema
				}
			}
		}
	}
	if capability == "" {
		return result, errors.New("Plugin contribution is unavailable")
	}
	defer func() {
		for _, plugin := range a.registry.Plugins() {
			if plugin.Manifest.ID == request.PluginID && plugin.Enabled && plugin.Revision == pluginRevision {
				return
			}
		}
		result = sdk.DataResult{}
		err = errors.New("Plugin changed during the query; retry")
	}()
	schema, err := sdk.ParseFormSchema(raw)
	if err != nil {
		return result, err
	}
	input, err := schema.Values(request.Input)
	if err != nil {
		return result, err
	}
	sources, err := a.sourceScope(ctx, c)
	if err != nil {
		return result, err
	}
	supported := false
	targetID := ""
	for _, s := range sources.List() {
		if s.ID == sourceID {
			supported = slices.Contains(s.Capabilities, capability)
			targetID = s.TargetID
		}
	}
	if !supported {
		return result, errors.New("Source does not support this contribution")
	}
	callCtx, p, _, release, err := sources.Acquire(ctx, sourceID)
	if err != nil {
		return result, err
	}
	defer release()
	// A registration may have changed between listing it and acquiring its lease.
	implemented := false
	switch capability {
	case "data":
		_, implemented = p.(sdk.DataSource)
	case "builds":
		_, implemented = p.(sdk.BuildSource)
	case "queue":
		_, implemented = p.(sdk.QueueSource)
	case "deployments":
		_, implemented = p.(sdk.DeploymentSource)
	case "metrics":
		_, implemented = p.(sdk.MetricSource)
	case "logs":
		_, implemented = p.(sdk.LogSource)
	case "traces":
		_, implemented = p.(sdk.TraceSource)
	}
	if !implemented {
		return result, errors.New("Source capability changed; retry the query")
	}

	defer func() {
		if callCtx.Err() != nil {
			result = sdk.DataResult{}
			err = callCtx.Err()
		}
	}()
	text := func(key string) string { v, _ := input[key].(string); return v }
	if targetID == "" {
		targetID = text("targetId")
	}
	end := time.Now().Unix()
	period := 3600.0
	if v, ok := input["period"].(float64); ok {
		period = v
	}
	if capability != "data" && (period < 1 || period > 86400) {
		return result, errors.New("Period must be between 1 and 86400 seconds")
	}
	q := sdk.Query{Expression: text("expression"), Start: end - int64(period), End: end}
	var records any
	switch capability {
	case "data":
		result, err = p.(sdk.DataSource).ReadData(callCtx, input)
	case "builds":
		allowed := false
		for _, t := range c.Connection.Targets {
			if t.ID == targetID && (len(t.Capabilities) == 0 || slices.Contains(t.Capabilities, "builds")) {
				allowed = true
			}
		}
		if !allowed {
			return result, errors.New("Select a configured build target")
		}
		var more bool
		records, more, err = p.(sdk.BuildSource).Builds(callCtx, targetID, 0)
		if more {
			result.Warnings = []string{"Additional builds are available in history"}
		}
	case "queue":
		records, err = p.(sdk.QueueSource).Queue(callCtx)
	case "deployments":
		records, err = p.(sdk.DeploymentSource).Deployments(callCtx)
	case "metrics":
		if q.Expression == "" || len(q.Expression) > 16384 {
			return result, errors.New("Enter a query")
		}
		var v sdk.QueryResult
		v, err = p.(sdk.MetricSource).Query(callCtx, q)
		result = sdk.DataResult{Type: "timeseries", Series: v.Series, Warnings: v.Warnings}
		if v.Truncated {
			result.Warnings = append(result.Warnings, "Truncated result")
		}
	case "logs", "traces":
		search := sdk.SearchRequest{Expression: q.Expression, Start: q.Start, End: q.End, Limit: 100}
		if err = search.Validate(100); err != nil {
			return result, err
		}
		if capability == "logs" {
			var v sdk.LogResult
			v, err = p.(sdk.LogSource).SearchLogs(callCtx, search)
			records = v.Entries
			result.Warnings = v.Observation.Diagnostics
		} else {
			var v sdk.TraceSearchResult
			v, err = p.(sdk.TraceSource).SearchTraces(callCtx, search)
			records = v.Traces
			result.Warnings = v.Observation.Diagnostics
		}
	}
	if err != nil {
		return sdk.DataResult{}, err
	}
	if records != nil {
		warnings := result.Warnings
		result, err = tableResult(records)
		result.Warnings = warnings
		if err != nil {
			return result, err
		}
	}
	if result.Type == "" {
		result.Type = "table"
	}
	if result.Type != renderer {
		return sdk.DataResult{}, errors.New("Result does not match declared renderer")
	}
	return result.CheckedCopy()
}

// Existing records become string columns so int64 IDs retain their precision.
func tableResult(records any) (sdk.DataResult, error) {
	result := sdk.DataResult{Type: "table"}
	raw, err := json.Marshal(records)
	if err != nil || len(raw) > 4*1024*1024 {
		return result, errors.New("Oversized result")
	}
	var rows []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&rows); err != nil {
		return result, err
	}
	keys := map[string]bool{}
	for _, row := range rows {
		for k := range row {
			keys[k] = true
		}
	}
	names := []string{}
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Columns = append(result.Columns, sdk.DataColumn{Name: name, Type: "string"})
	}
	for _, row := range rows {
		values := make([]any, len(names))
		for i, name := range names {
			v := row[name]
			switch v.(type) {
			case nil:
				values[i] = nil
			case map[string]any, []any:
				b, _ := json.Marshal(v)
				values[i] = string(b)
			default:
				values[i] = fmt.Sprint(v)
			}
		}
		result.Rows = append(result.Rows, values)
	}
	return result, nil
}
