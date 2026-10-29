package application

import (
	"context"
	"encoding/json"
	"errors"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
	"sync/atomic"
	"testing"
)

type customData struct {
	read func(context.Context, map[string]any) (sdk.DataResult, error)
}

func (*customData) Check(context.Context) error { return nil }
func (p *customData) ReadData(ctx context.Context, v map[string]any) (sdk.DataResult, error) {
	return p.read(ctx, v)
}
func TestCustomContributionUsesSDKContractAndValidatesInput(t *testing.T) {
	var closed atomic.Int32
	a, _ := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		return &queryProvider{closed: &closed}, nil
	})
	calls := 0
	schema := json.RawMessage(`{"type":"object","properties":{"region":{"type":"string","enum":["local"],"default":"local"}}}`)
	manifest := sdk.Manifest{ID: "inventory", Version: "1", ContractVersion: 1, Providers: []sdk.ProviderContribution{{ID: "inventory", Name: "Inventory", ConfigSchema: schema}}, Cards: []sdk.CardContribution{{ID: "count", Title: "Count", Capability: "data", Renderer: "scalar", ConfigSchema: json.RawMessage(`{"type":"object","properties":{"region":{"type":"string","enum":["local"],"default":"local"},"period":{"type":"number","default":172800}}}`)}}, Dashboards: []sdk.DashboardContribution{{ID: "inventory", Title: "Inventory", Cards: []string{"count"}}}}
	err := a.RegisterPlugin(manifest, sdk.Definition{Info: sdk.ProviderInfo{Kind: "inventory", Name: "Inventory", Capabilities: []string{"data"}, AuthMethods: []string{"none"}, DefaultAuth: "none", OnDemand: true}, Create: func(c sdk.Connection, _ string) (sdk.Provider, error) {
		if string(c.Config) != `{"region":"local"}` {
			t.Errorf("provider config default lost: %s", c.Config)
		}
		return &customData{read: func(_ context.Context, v map[string]any) (sdk.DataResult, error) {
			calls++
			if v["period"] != float64(172800) {
				t.Error("generic input was changed")
			}
			if v["region"] != "local" {
				t.Error("input default missing")
			}
			return sdk.DataResult{Type: "scalar", Value: 42}, nil
		}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := a.SaveConnection(ConnectionInput{Connection: sdk.Connection{Kind: "inventory", Name: "Inventory", URL: "https://example.invalid", Auth: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	request := ContributionRequest{PluginID: "inventory", Kind: "card", ID: "count", Input: json.RawMessage(`{}`)}
	result, err := a.ExecuteContribution(id, "default", request)
	if err != nil || result.Value != float64(42) {
		t.Fatal(result, err)
	}
	request.Input = json.RawMessage(`{"region":"unknown"}`)
	if _, err = a.ExecuteContribution(id, "default", request); err == nil || calls != 1 {
		t.Fatal("invalid input reached source")
	}
	request.Input = json.RawMessage(`{}`)
	if err = a.SetPluginEnabled("inventory", false); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ExecuteContribution(id, "default", request); err == nil {
		t.Fatal("disabled contribution executed")
	}
}
func TestContributionDiscardsObsoleteResults(t *testing.T) {
	for _, mutation := range []string{"delete source", "toggle plugin"} {
		t.Run(mutation, func(t *testing.T) {
			var closed atomic.Int32
			a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
				return &queryProvider{closed: &closed}, nil
			})
			entered, release := make(chan struct{}), make(chan struct{})
			if err := a.RegisterPlugin(sdk.Manifest{ID: "cards", Version: "1", ContractVersion: 1, Cards: []sdk.CardContribution{{ID: "value", Title: "Value", Capability: "data", Renderer: "scalar"}}}); err != nil {
				t.Fatal(err)
			}
			var err error
			if _, err = a.Sources(c.ID); err != nil {
				t.Fatal(err)
			}
			a.mu.Lock()
			sources := a.runners[c.ID].sources
			a.mu.Unlock()
			if err = sources.Register(context.Background(), sdk.SourceDescriptor{ID: "custom", Name: "Custom", Capabilities: []string{"data"}}, &customData{read: func(context.Context, map[string]any) (sdk.DataResult, error) {
				close(entered)
				<-release
				return sdk.DataResult{Type: "scalar", Value: "stale"}, nil
			}}); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				r, e := a.ExecuteContribution(c.ID, "custom", ContributionRequest{PluginID: "cards", Kind: "card", ID: "value"})
				if r.Type != "" {
					e = errors.New("stale result escaped")
				}
				finished <- e
			}()
			receive(t, entered)
			if mutation == "delete source" {
				if err = sources.Delete(context.Background(), "custom"); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = a.SetPluginEnabled("cards", false); err != nil {
					t.Fatal(err)
				}
				if err = a.SetPluginEnabled("cards", true); err != nil {
					t.Fatal(err)
				}
			}
			close(release)
			if err = receive(t, finished); err == nil || (mutation == "delete source" && !errors.Is(err, context.Canceled)) {
				t.Fatal(err)
			}
		})
	}
}

func TestTypedRecordsKeepLargeNumericIDsAsText(t *testing.T) {
	r, err := tableResult([]sdk.Build{{Number: 9223372036854775807}})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range r.Columns {
		if c.Name == "number" {
			if r.Rows[0][i] != "9223372036854775807" {
				t.Fatal("numeric ID lost precision", r.Rows)
			}
			return
		}
	}
	t.Fatal("missing number column")
}

func TestContributionPreservesPartialLogs(t *testing.T) {
	var closed atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		return &queryProvider{closed: &closed}, nil
	})
	if err := a.RegisterPlugin(sdk.Manifest{ID: "logcards", Version: "1", ContractVersion: 1, Cards: []sdk.CardContribution{{ID: "logs", Title: "Logs", Capability: "logs", Renderer: "table", ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"expression":{"type":"string","default":"test"}}}`)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sources(c.ID); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	sources := a.runners[c.ID].sources
	a.mu.Unlock()
	p := &logFixture{closed: &closed, query: func(context.Context) (platform.LogResult, error) {
		return platform.LogResult{Entries: []platform.LogEntry{{Line: "kept", TimestampNS: "1700000000123456789"}}}, errors.New("upstream interrupted")
	}}
	if err := sources.Register(context.Background(), sdk.SourceDescriptor{ID: "partial", Name: "Partial", Capabilities: []string{"logs"}}, p); err != nil {
		t.Fatal(err)
	}
	result, err := a.ExecuteContribution(c.ID, "partial", ContributionRequest{PluginID: "logcards", Kind: "card", ID: "logs"})
	if err != nil || len(result.Rows) != 1 || len(result.Warnings) == 0 {
		t.Fatalf("lost partial result: %+v, %v", result, err)
	}
	found := false
	for _, cell := range result.Rows[0] {
		if cell == "1700000000123456789" {
			found = true
		}
	}
	if !found {
		t.Fatal("timestamp precision lost", result)
	}
	if result.Warnings[len(result.Warnings)-1] != "Partial result: upstream interrupted" {
		t.Fatal(result.Warnings)
	}
}
