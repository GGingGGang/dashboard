package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"idp-dashboard/internal/platform"
)

type metricFixture struct {
	checkErr, queryErr error
	result             platform.QueryResult
}

func (p *metricFixture) Check(context.Context) error { return p.checkErr }
func (p *metricFixture) Query(context.Context, platform.Query) (platform.QueryResult, error) {
	return p.result, p.queryErr
}
func TestFailedAndIncompleteRulesKeepLastSuccess(t *testing.T) {
	v := 9.0
	p := &metricFixture{result: platform.QueryResult{Series: []platform.Series{{Points: []platform.Point{{Time: float64(time.Now().Unix()), Value: &v}}}}}}
	c := NewCollector(platform.Connection{Rules: []platform.Rule{{ID: "r", Enabled: true, Threshold: 5}}}, p, nil)
	first := c.Poll(context.Background())
	if first.Error != "" || first.Rules[0].Breaches != 1 {
		t.Fatalf("initial rule: %+v", first)
	}
	p.queryErr = errors.New("query denied")
	failed := c.Poll(context.Background())
	if failed.Error == "" || !failed.Modules["metrics"].LastSuccess.Equal(first.Modules["metrics"].LastSuccess) || len(failed.Rules[0].Result.Series) != 1 || failed.Rules[0].Breaches != 0 {
		t.Fatalf("failed query erased evidence or looked fresh: %+v", failed)
	}
	p.queryErr = nil
	for _, result := range []platform.QueryResult{{}, {Series: []platform.Series{{Points: []platform.Point{{Time: float64(time.Now().Add(time.Hour).Unix()), Value: &v}}}}}, {Series: first.Rules[0].Result.Series, Truncated: true}} {
		p.result = result
		s := c.Poll(context.Background())
		if s.Error == "" || s.Rules[0].Error == "" || s.Rules[0].Breaches != 0 {
			t.Fatal("incomplete result evaluated as healthy")
		}
	}
	p.checkErr = errors.New("offline")
	before := c.previous.Clone()
	s := c.Poll(context.Background())
	if len(s.Rules) != len(before.Rules) || !s.Modules["metrics"].LastSuccess.Equal(first.Modules["metrics"].LastSuccess) {
		t.Fatal("probe failure erased history")
	}
}
