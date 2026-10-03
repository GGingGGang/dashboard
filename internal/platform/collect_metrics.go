package platform

import (
	"context"
	"time"
)

func (c *Collector) pollMetrics(ctx context.Context, p Monitoring, next *Snapshot) {
	next.Rules = []RuleResult{}
	if err := p.Check(ctx); err != nil {
		next.Error = err.Error()
		return
	}
	for _, r := range c.Connection.Rules {
		if !r.Enabled {
			continue
		}
		if ctx.Err() != nil {
			next.Error = "Collection interrupted"
			break
		}
		result, err := p.Query(ctx, Query{Expression: r.Expression})
		rr := RuleResult{Rule: r, Result: result}
		if err != nil {
			rr.Error = err.Error()
		} else {
			for _, series := range result.Series {
				if len(series.Points) > 0 {
					pt := series.Points[len(series.Points)-1]
					if pt.Value != nil && time.Since(time.Unix(int64(pt.Time), 0)) <= time.Minute*2 && *pt.Value > r.Threshold {
						rr.Breaches++
					}
				}
			}
		}
		next.Rules = append(next.Rules, rr)
	}
}
