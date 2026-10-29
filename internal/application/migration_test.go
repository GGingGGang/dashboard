package application

import (
	"context"
	"encoding/json"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/demo"
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers"
	"idp-dashboard/internal/storage"
	"path/filepath"
	"testing"
	"time"
)

func TestAllShippedPluginsUsePublishedSourcesAndKeepLegacyQueries(t *testing.T) {
	base, closeDemo := demo.Start()
	defer closeDemo()
	store, err := storage.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(context.Background(), store, &memoryCredentials{values: map[string]string{}}, providers.DefaultRegistry(), false)
	defer a.Close()
	ids := map[string]string{}
	for _, c := range demo.Connections(base) {
		c.ID = ""
		id, err := a.SaveConnection(ConnectionInput{Connection: c})
		if err != nil {
			t.Fatal(c.Kind, err)
		}
		ids[c.Kind] = id
		sources, err := a.Sources(id)
		if err != nil || len(sources) != 1 || sources[0].Revision != 2 {
			t.Fatalf("%s not published: %+v %v", c.Kind, sources, err)
		}
	}
	end := time.Now().Unix()
	q := sdk.Query{Expression: "up", Start: end - 300, End: end}
	a.mu.Lock()
	scope := a.runners[ids["prometheus"]].sources
	a.mu.Unlock()
	if result, err := a.Query(ids["prometheus"], q); err != nil || len(result.Series) == 0 {
		t.Fatal(result, err)
	}
	a.mu.Lock()
	same := a.runners[ids["prometheus"]].sources == scope
	a.mu.Unlock()
	if !same {
		t.Fatal("legacy query replaced source")
	}
	if result, err := a.QuerySource(ids["prometheus"], "default", q); err != nil || len(result.Series) == 0 {
		t.Fatal(result, err)
	}
	logs, err := a.SearchLogs(ids["loki"], sdk.SearchRequest{Expression: `{service_name="payments"}`, Start: end - 300, End: end, Limit: 10})
	if err != nil || len(logs.Entries) != 1 || len(logs.Entries[0].TimestampNS) < 16 {
		t.Fatal(logs, err)
	}
	traces, err := a.SearchTraces(ids["tempo"], sdk.SearchRequest{Expression: `{true}`, Start: end - 300, End: end, Limit: 10})
	if err != nil || len(traces.Traces) != 1 {
		t.Fatal(traces, err)
	}
	detail, err := a.Trace(ids["tempo"], traces.Traces[0].TraceID)
	if err != nil || len(detail.Spans) != 1 {
		t.Fatal(detail, err)
	}
	for _, kind := range []string{"argocd", "prometheus", "loki", "tempo", "json"} {
		m := a.registry.Plugins()
		var card sdk.CardContribution
		for _, p := range m {
			if p.Manifest.ID == kind {
				card = p.Manifest.Cards[0]
			}
		}
		result, err := a.ExecuteContribution(ids[kind], "default", ContributionRequest{PluginID: kind, Kind: "card", ID: card.ID, Input: json.RawMessage(`{}`)})
		if err != nil || result.Type != card.Renderer {
			t.Fatalf("%s contribution: %+v %v", kind, result, err)
		}
	}
	legacy, err := a.ReadJSON(platform.JSONRequest{Source: "http", URL: base + "/json", Filter: ".[0].id"})
	if err != nil || legacy.Text != "9223372036854775807" {
		t.Fatal(legacy, err)
	}
	if err = a.SetPluginEnabled("json", false); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ReadJSON(platform.JSONRequest{Source: "http", URL: base + "/json", Filter: "."}); err == nil {
		t.Fatal("legacy JSON ignored disabled plugin")
	}
	if len(a.GetState().Connections) != 6 {
		t.Fatal("migration lost connections")
	}
}
