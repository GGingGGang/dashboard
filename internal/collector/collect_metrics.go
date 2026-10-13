package collector

import (
	"context"
	"time"

	"idp-dashboard/internal/platform"
)

func (c *Collector) pollMetrics(ctx context.Context, p platform.MetricSource, next *platform.Snapshot) {
	if err := p.Check(ctx); err != nil {
		next.Error = err.Error()
		return
	}
	next.Rules = []platform.RuleResult{}
	for _, r := range c.Connection.Rules {
		if !r.Enabled {
			continue
		}
		if ctx.Err() != nil {
			next.Error = "Collection interrupted"
			break
		}
		result, err := p.Query(ctx, platform.Query{Expression: r.Expression})
		rr := platform.RuleResult{Rule: r, Result: result}
		if err != nil {
			rr.Error = err.Error()
			for _, previous := range c.previous.Rules {
				if previous.Rule.ID == r.ID {
					rr.Result = previous.Result.Clone()
					break
				}
			}
		} else {
			valid := 0
			for _, series := range result.Series {
				if len(series.Points) > 0 {
					pt := series.Points[len(series.Points)-1]
					age := time.Since(time.Unix(int64(pt.Time), 0))
					if pt.Value != nil && age >= -time.Second && age <= time.Minute*2 {
						valid++
						if *pt.Value > r.Threshold {
							rr.Breaches++
						}
					}
				}
			}
			if valid == 0 || valid != len(result.Series) {
				rr.Error = "Missing or stale samples; rule cannot be evaluated"
			}
			if result.Truncated || len(result.Warnings) > 0 {
				rr.Error = "Incomplete metric result; rule cannot be evaluated"
			}
		}
		if rr.Error != "" {
			next.Error = "One or more metric rules could not be evaluated"
			rr.Breaches = 0
		}
		next.Rules = append(next.Rules, rr)
	}
}
