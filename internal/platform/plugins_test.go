package platform_test

import (
	"sync"
	"testing"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
)

func pluginDefinition(kind string) platform.Definition {
	return platform.Definition{Info: platform.ProviderInfo{Kind: kind, DefaultAuth: "none", AuthMethods: []string{"none"}, Capabilities: []string{"metrics"}, Query: &platform.QueryInfo{Language: "fixture"}}, Create: func(platform.Connection, string) (platform.Provider, error) { return &metricFixture{}, nil }}
}

func TestPluginRegistrationIsAtomicAndOwnsDeclarations(t *testing.T) {
	r, _ := platform.NewRegistry(pluginDefinition("existing"))
	m := sdk.Manifest{ID: "bundle", Version: "1.0.0", ContractVersion: sdk.ContractVersion, Providers: []sdk.ProviderContribution{{ID: "new"}, {ID: "existing"}}}
	if err := r.RegisterPlugin(m, pluginDefinition("new"), pluginDefinition("existing")); err == nil {
		t.Fatal("accepted duplicate provider")
	}
	if _, ok := r.Info("new"); ok {
		t.Fatal("partial registration escaped")
	}
	m.Providers = m.Providers[:1]
	m.ContractVersion++
	if err := r.RegisterPlugin(m, pluginDefinition("new")); err == nil {
		t.Fatal("accepted incompatible contract")
	}
	m.ContractVersion = sdk.ContractVersion
	if err := r.RegisterPlugin(m, pluginDefinition("new")); err != nil {
		t.Fatal(err)
	}
	m.Providers[0].ID = "mutated"
	states := r.Plugins()
	states[0].Manifest.Providers[0].ID = "also-mutated"
	if r.Owner("new") != "bundle" || r.Plugins()[0].Manifest.Providers[0].ID != "new" {
		t.Fatal("caller mutated registry")
	}
	c := platform.Connection{Kind: "new", Name: "new", URL: "http://example.invalid", Auth: "none"}
	if err := r.SetPluginEnabled("bundle", false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.New(c, ""); err == nil {
		t.Fatal("disabled plugin constructed")
	}
	if _, ok := r.Info("new"); !ok {
		t.Fatal("disabled metadata lost")
	}
	if err := r.SetPluginEnabled("bundle", true); err != nil {
		t.Fatal(err)
	}
	p, err := r.New(c, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = platform.CloseProvider(p)
}

func TestPluginRegistryConcurrentReadsAndActivation(t *testing.T) {
	r, _ := platform.NewRegistry(pluginDefinition("fixture"))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = r.SetPluginEnabled("fixture", j%2 == 0)
				r.Info("fixture")
				r.Infos()
				r.Plugins()
				r.Enabled("fixture")
			}
		}()
	}
	wg.Wait()
}

func TestTypedNilAndUnknownCapabilityRejected(t *testing.T) {
	var p *metricFixture
	if err := platform.ValidateCapabilities(p, []string{"metrics"}); err == nil {
		t.Fatal("accepted typed nil")
	}
	if err := platform.ValidateCapabilities(&metricFixture{}, []string{"metrics", "unknown"}); err == nil {
		t.Fatal("accepted unknown capability")
	}
}

func TestInvalidContributionsNeverEnterRegistry(t *testing.T) {
	cases := []sdk.Manifest{
		{Cards: []sdk.CardContribution{{ID: "x", Title: "X", Capability: "data", Renderer: "script"}}},
		{Cards: []sdk.CardContribution{{ID: "x", Title: "X", Capability: "unknown", Renderer: "table"}}},
		{Cards: []sdk.CardContribution{{ID: "x", Title: "X", Capability: "builds", Renderer: "scalar"}}},
		{Dashboards: []sdk.DashboardContribution{{ID: "view", Title: "View", Cards: []string{"missing"}}}},
		{Cards: []sdk.CardContribution{{ID: "x", Title: "X", Capability: "data", Renderer: "scalar"}, {ID: "x", Title: "X", Capability: "data", Renderer: "scalar"}}},
	}
	for _, m := range cases {
		r, _ := platform.NewRegistry()
		m.ID = "fixture"
		m.Version = "1"
		m.ContractVersion = 1
		if err := r.RegisterPlugin(m); err == nil {
			t.Fatalf("invalid manifest accepted: %+v", m)
		}
		if len(r.Plugins()) != 0 {
			t.Fatal("partial registration")
		}
	}
}
