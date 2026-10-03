package platform

import (
	"context"
)

func (c *Collector) pollDeployments(ctx context.Context, p CD, next *Snapshot) {
	apps, err := p.Deployments(ctx)
	if err != nil {
		next.Error = err.Error()
		return
	}
	next.Deployments = []Deployment{}
	for _, a := range apps {
		if c.selected(a.ID, "deployments") {
			next.Deployments = append(next.Deployments, a)
		}
	}
}
