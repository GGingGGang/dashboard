package collector_test

import (
	"context"
	"testing"

	"idp-dashboard/internal/collector"
	"idp-dashboard/internal/platform"
)

type reusableResults struct {
	result      platform.QueryResult
	deployments []platform.Deployment
}

func (*reusableResults) Check(context.Context) error { return nil }
func (p *reusableResults) Query(context.Context, platform.Query) (platform.QueryResult, error) {
	return p.result, nil
}
func (p *reusableResults) Deployments(context.Context) ([]platform.Deployment, error) {
	return p.deployments, nil
}

func TestSnapshotsOwnNestedProviderResults(t *testing.T) {
	value := 1.0
	p := &reusableResults{
		result:      platform.QueryResult{Warnings: []string{"first"}, Series: []platform.Series{{Labels: map[string]string{"env": "prod"}, Points: []platform.Point{{Time: 1, Value: &value}}}}},
		deployments: []platform.Deployment{{ID: "app", Revisions: []string{"first"}}},
	}
	c := collector.NewCollector(platform.Connection{ID: "c", Targets: []platform.Target{{ID: "app"}}, Rules: []platform.Rule{{Enabled: true}}}, p, nil)
	first := c.Poll(context.Background())
	p.result.Series[0].Labels["env"] = "dev"
	value = 2
	p.result.Warnings[0] = "second"
	p.deployments[0].Revisions[0] = "second"
	second := c.Poll(context.Background())
	if first.Rules[0].Result.Series[0].Labels["env"] != "prod" || *first.Rules[0].Result.Series[0].Points[0].Value != 1 || first.Rules[0].Result.Warnings[0] != "first" || first.Deployments[0].Revisions[0] != "first" {
		t.Fatal("provider reused published memory")
	}
	second.Deployments[0].Revisions[0] = "consumer mutation"
	second.Rules[0].Result.Series[0].Labels["env"] = "consumer mutation"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	third := c.Poll(ctx)
	if third.Deployments[0].Revisions[0] != "second" || third.Rules[0].Result.Series[0].Labels["env"] != "dev" {
		t.Fatal("consumer mutated retained snapshot")
	}
}
