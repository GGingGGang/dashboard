package jsonapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"net/url"
	"sort"
	"strings"
)

type Provider struct{ url string }

func (p *Provider) Check(ctx context.Context) error {
	_, err := p.ReadJSON(ctx, sdk.JSONRequest{Source: "http", URL: p.url, Filter: "."})
	return err
}
func (p *Provider) ReadJSON(ctx context.Context, q sdk.JSONRequest) (sdk.JSONResult, error) {
	return (Reader{}).Read(ctx, q)
}
func (p *Provider) PublishSources(ctx context.Context, host sdk.SourceRegistrar) error {
	return host.Update(ctx, sdk.SourceDescriptor{ID: "default", Name: "HTTP JSON", Capabilities: []string{"data", "json"}}, p)
}
func (p *Provider) ReadData(ctx context.Context, input map[string]any) (sdk.DataResult, error) {
	filter, _ := input["filter"].(string)
	path, _ := input["path"].(string)
	mode, _ := input["mode"].(string)
	relative, err := url.Parse(path)
	if err != nil || relative.IsAbs() || relative.Host != "" || relative.User != nil || relative.Fragment != "" {
		return sdk.DataResult{}, errors.New("Use a relative API path without credentials or fragments")
	}
	address := strings.TrimRight(p.url, "/")
	if path != "" {
		address += "/" + strings.TrimLeft(path, "/")
	}
	result, err := p.ReadJSON(ctx, sdk.JSONRequest{Source: "http", URL: address, Filter: filter})
	if err != nil {
		return sdk.DataResult{}, err
	}
	if mode == "scalar" {
		if !result.Scalar {
			return sdk.DataResult{}, errors.New("Choose a jq expression returning one scalar")
		}
		return sdk.DataResult{Type: "scalar", Value: result.Text}.CheckedCopy()
	}
	decoder := json.NewDecoder(bytes.NewBufferString(result.JSON))
	decoder.UseNumber()
	var value any
	if err = decoder.Decode(&value); err != nil {
		return sdk.DataResult{}, err
	}
	rows, ok := value.([]any)
	if !ok {
		rows = []any{value}
	}
	if len(rows) > 200 {
		return sdk.DataResult{}, errors.New("Narrow the jq query to at most 200 rows")
	}
	names := map[string]bool{}
	records := []map[string]any{}
	for _, row := range rows {
		record, ok := row.(map[string]any)
		if !ok {
			record = map[string]any{"value": row}
		}
		records = append(records, record)
		for key := range record {
			names[key] = true
		}
	}
	keys := []string{}
	for key := range names {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := sdk.DataResult{Type: "table"}
	for _, key := range keys {
		out.Columns = append(out.Columns, sdk.DataColumn{Name: key, Type: "string"})
	}
	for _, record := range records {
		row := []any{}
		for _, key := range keys {
			v := record[key]
			switch v.(type) {
			case nil:
				row = append(row, nil)
			case []any, map[string]any:
				raw, _ := json.Marshal(v)
				row = append(row, string(raw))
			default:
				row = append(row, fmt.Sprint(v))
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out.CheckedCopy()
}
func Definition() sdk.Definition {
	return sdk.Definition{Info: sdk.ProviderInfo{Kind: "json", Name: "HTTP JSON", Category: "api", Capabilities: []string{"data", "json"}, CollectionSource: "default", OnDemand: true, DefaultAuth: "none", AuthMethods: []string{"none"}}, Validate: func(c sdk.Connection) error {
		if c.Username != "" || c.Tenant != "" || c.CAFile != "" {
			return errors.New("HTTP JSON supports public endpoints with system TLS trust only")
		}
		return nil
	}, Create: func(c sdk.Connection, _ string) (sdk.Provider, error) { return &Provider{url: c.URL}, nil }}
}
func Manifest() sdk.Manifest {
	schema := func(mode, filter string) json.RawMessage {
		raw, _ := json.Marshal(sdk.FormSchema{Type: "object", Properties: map[string]sdk.FormField{"path": {Type: "string", Title: "Relative API path", Default: ""}, "filter": {Type: "string", Title: "jq", Default: filter}, "mode": {Type: "string", Enum: []any{mode}, Default: mode}}, Required: []string{"filter"}})
		return raw
	}
	return sdk.Manifest{ID: "json", Version: "0.1.0", ContractVersion: 1, Providers: []sdk.ProviderContribution{{ID: "json", Name: "HTTP JSON"}}, Cards: []sdk.CardContribution{{ID: "table", Title: "JSON table", Capability: "data", Renderer: "table", ConfigSchema: schema("table", ".")}, {ID: "scalar", Title: "JSON value", Capability: "data", Renderer: "scalar", ConfigSchema: schema("scalar", "length")}}, Dashboards: []sdk.DashboardContribution{{ID: "overview", Title: "HTTP JSON", Cards: []string{"table", "scalar"}}}, Queries: []sdk.QueryContribution{{ID: "jq", Title: "HTTP JSON / jq", Capability: "data", Renderer: "table", InputSchema: schema("table", ".")}}}
}
