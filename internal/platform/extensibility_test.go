package platform_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"idp-dashboard/internal/platform"
)

// Deliberately outside package platform: extension code sees only public contracts.
type hybrid struct {
	deploymentError bool
	targets         []string
}

func (*hybrid) Check(context.Context) error { return nil }
func (*hybrid) Discover(context.Context) ([]platform.Target, error) {
	return []platform.Target{{ID: "pipeline", Name: "Pipeline", Capabilities: []string{"builds"}}, {ID: "service", Name: "Service", Capabilities: []string{"deployments"}}}, nil
}
func (h *hybrid) Builds(_ context.Context, target string, _ int) ([]platform.Build, bool, error) {
	h.targets = append(h.targets, target)
	return []platform.Build{{ConnectionID: "custom", Job: target, Number: 7, Started: 1000, Observed: 2000, Status: "SUCCESS"}}, false, nil
}
func (*hybrid) Build(context.Context, string, int64) (platform.Build, error) {
	return platform.Build{}, errors.New("unexpected detail request")
}
func (h *hybrid) Deployments(context.Context) ([]platform.Deployment, error) {
	if h.deploymentError {
		return nil, errors.New("deployment API unavailable")
	}
	return []platform.Deployment{{ID: "service", Name: "Service", Sync: "Synced", Health: "Healthy"}}, nil
}
func (*hybrid) Query(_ context.Context, q platform.Query) (platform.QueryResult, error) {
	if q.Expression != "SELECT usage" {
		return platform.QueryResult{}, errors.New("unexpected query language")
	}
	v := 95.0
	return platform.QueryResult{Type: "vector", Series: []platform.Series{{Labels: map[string]string{"node": "one"}, Points: []platform.Point{{Time: float64(time.Now().Unix()), Value: &v}}}}}, nil
}

func definition(source *hybrid) platform.Definition {
	return platform.Definition{
		Info:   platform.ProviderInfo{Kind: "custom", Name: "External fixture", Category: "combined", DefaultAuth: "none", AuthMethods: []string{"none"}, Capabilities: []string{"builds", "deployments", "metrics", "instant", "range", "rules"}, Query: &platform.QueryInfo{Language: "SQL", DefaultExpression: "SELECT usage"}},
		Create: func(platform.Connection, string) (platform.Provider, error) { return source, nil },
	}
}

func TestExternalProviderComposesCapabilitiesAndIsolatesFailure(t *testing.T) {
	source := &hybrid{}
	r, err := platform.NewRegistry(definition(source))
	if err != nil {
		t.Fatal(err)
	}
	store, err := platform.OpenStore(filepath.Join(t.TempDir(), "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := platform.Connection{ID: "custom", Kind: "custom", Name: "Custom", URL: "https://example.invalid", Auth: "none", Rules: []platform.Rule{{ID: "usage", Expression: "SELECT usage", Threshold: 90, Enabled: true}}}
	c.Targets, _ = source.Discover(context.Background())
	p, err := r.New(c, "")
	if err != nil {
		t.Fatal(err)
	}
	collector := platform.NewCollector(c, p, store)
	first := collector.Poll(context.Background())
	if first.Error != "" || first.StorageError != "" || len(first.Modules) != 3 || len(first.Builds) != 1 || len(first.Deployments) != 1 || len(first.Rules) != 1 || first.Rules[0].Breaches != 1 {
		t.Fatalf("combined collection failed: %+v", first)
	}
	if _, exists := first.Modules["queue"]; exists {
		t.Fatal("required a queue from build-only capability")
	}
	if len(source.targets) != 1 || source.targets[0] != "pipeline" {
		t.Fatal("sent deployment targets to build collection")
	}
	source.deploymentError = true
	second := collector.Poll(context.Background())
	if second.Modules["deployments"].Error == "" || second.Modules["metrics"].Error != "" || second.Modules["builds"].Error != "" {
		t.Fatal("one capability failure contaminated other capabilities")
	}
	if first.Modules["deployments"].Error != "" {
		t.Fatal("mutated a published snapshot")
	}
	if len(second.Deployments) != 1 || second.Modules["deployments"].LastSuccess != first.Modules["deployments"].LastSuccess {
		t.Fatal("lost last successful deployment evidence")
	}
	history, err := store.History(platform.HistoryFilter{})
	if err != nil || history.Total != 1 {
		t.Fatal("archive duplication or loss")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped := collector.Poll(ctx)
	for _, status := range stopped.Modules {
		if status.Error == "" {
			t.Fatal("cancellation reported success")
		}
	}
}

func TestRegistryRejectsDuplicateAndInaccurateCapabilities(t *testing.T) {
	d := definition(&hybrid{})
	if _, err := platform.NewRegistry(d, d); err == nil {
		t.Fatal("accepted duplicate kind")
	}
	d.Info.Capabilities = []string{"metrics"}
	r, err := platform.NewRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.New(platform.Connection{Kind: "custom", Name: "test", URL: "https://example.invalid", Auth: "none"}, "")
	if err == nil {
		t.Fatal("accepted hidden build/deployment capabilities")
	}
	d.Info.Query = nil
	if _, err = platform.NewRegistry(d); err == nil {
		t.Fatal("accepted metrics without query metadata")
	}
}
