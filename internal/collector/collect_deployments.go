package collector

import (
	"context"

	"idp-dashboard/internal/platform"
)

func (c *Collector) pollDeployments(ctx context.Context, p platform.DeploymentSource, next *platform.Snapshot) {
	apps, err := p.Deployments(ctx)
	if err != nil {
		next.Error = err.Error()
		return
	}
	next.Deployments = []platform.Deployment{}
	for _, a := range apps {
		if c.selected(a.ID, "deployments") {
			next.Deployments = append(next.Deployments, a)
		}
	}
}
