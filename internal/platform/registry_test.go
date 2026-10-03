package platform_test

import (
	"context"
	"errors"
	"testing"

	"idp-dashboard/internal/platform"
)

type metricFixture struct{}

func (*metricFixture) Check(context.Context) error                         { return nil }
func (*metricFixture) Discover(context.Context) ([]platform.Target, error) { return nil, nil }
func (*metricFixture) Query(context.Context, platform.Query) (platform.QueryResult, error) {
	return platform.QueryResult{}, nil
}

func TestRegistryRejectsDuplicateAndInaccurateCapabilities(t *testing.T) {
	d := platform.Definition{
		Info:   platform.ProviderInfo{Kind: "fixture", Name: "Fixture", AuthMethods: []string{"none"}, DefaultAuth: "none", Capabilities: []string{"metrics"}, Query: &platform.QueryInfo{Language: "SQL"}},
		Create: func(platform.Connection, string) (platform.Provider, error) { return &metricFixture{}, nil },
	}
	if _, err := platform.NewRegistry(d, d); err == nil {
		t.Fatal("accepted duplicate kind")
	}
	d.Info.Capabilities = []string{"builds"}
	r, err := platform.NewRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.New(platform.Connection{Kind: "fixture", Name: "test", URL: "https://example.invalid", Auth: "none"}, ""); err == nil {
		t.Fatal("accepted inaccurate capabilities")
	}
	d.Info.Capabilities = []string{"metrics"}
	d.Info.Query = nil
	if _, err := platform.NewRegistry(d); err == nil {
		t.Fatal("accepted metrics without query metadata")
	}
}

func TestProviderValidationRunsBeforeConstruction(t *testing.T) {
	calls := 0
	invalid := errors.New("provider-specific validation")
	r, err := platform.NewRegistry(platform.Definition{
		Info:     platform.ProviderInfo{Kind: "fixture", AuthMethods: []string{"none"}, DefaultAuth: "none"},
		Validate: func(platform.Connection) error { return invalid },
		Create:   func(platform.Connection, string) (platform.Provider, error) { calls++; return &metricFixture{}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	c := platform.Connection{Kind: "fixture", Name: "test", URL: "https://example.invalid", Auth: "none"}
	if err := r.Validate(c); !errors.Is(err, invalid) {
		t.Fatal("configuration validation skipped")
	}
	if _, err := r.New(c, ""); !errors.Is(err, invalid) || calls != 0 {
		t.Fatal("constructed an invalid provider")
	}
}
