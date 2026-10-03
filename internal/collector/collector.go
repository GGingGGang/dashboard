package collector

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"idp-dashboard/internal/platform"
)

// BuildArchive is the persistence required by collection; it does not expose SQLite.
type BuildArchive interface {
	Known(platform.Build) (bool, error)
	SaveBuilds([]platform.Build) error
	Running(string) ([]platform.Build, error)
}

// Collector belongs to one immutable connection configuration. Replace it on edits.
type Collector struct {
	Connection platform.Connection
	Provider   platform.Provider
	Store      BuildArchive
	mu         sync.Mutex
	cursors    map[string]int
	previous   platform.Snapshot
}

func NewCollector(c platform.Connection, p platform.Provider, s BuildArchive) *Collector {
	return &Collector{Connection: c, Provider: p, Store: s, cursors: map[string]int{}, previous: platform.Snapshot{ConnectionID: c.ID, Builds: []platform.Build{}, Queue: []platform.QueueItem{}, Deployments: []platform.Deployment{}, Rules: []platform.RuleResult{}}}
}

func (c *Collector) Poll(ctx context.Context) platform.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.previous
	next.Attempted = time.Now()
	next.Error = ""
	next.StorageError = ""
	next.Modules = map[string]platform.CollectionStatus{}
	failures := []string{}
	collect := func(name string, run func()) {
		status := c.previous.Modules[name]
		status.Attempted = next.Attempted
		next.Error = ""
		if ctx.Err() == nil {
			run()
		} else {
			next.Error = "Collection interrupted"
		}
		status.Error = next.Error
		if status.Error == "" {
			status.LastSuccess = time.Now()
		} else {
			failures = append(failures, name+": "+status.Error)
		}
		next.Modules[name] = status
	}
	if p, ok := c.Provider.(platform.BuildSource); ok {
		collect("builds", func() { c.pollBuilds(ctx, p, &next) })
	}
	if p, ok := c.Provider.(platform.QueueSource); ok {
		collect("queue", func() { c.pollQueue(ctx, p, &next) })
	}
	if p, ok := c.Provider.(platform.DeploymentSource); ok {
		collect("deployments", func() { c.pollDeployments(ctx, p, &next) })
	}
	if p, ok := c.Provider.(platform.MetricSource); ok {
		collect("metrics", func() { c.pollMetrics(ctx, p, &next) })
	}
	if len(next.Modules) == 0 {
		failures = append(failures, "Provider has no supported collection capability")
	}
	next.Error = strings.Join(failures, "; ")
	if next.Error == "" {
		next.LastSuccess = time.Now()
	}
	c.previous = next
	return next
}

func (c *Collector) selected(id string, capability string) bool {
	for _, t := range c.Connection.Targets {
		if t.ID == id && (len(t.Capabilities) == 0 || slices.Contains(t.Capabilities, capability)) {
			return true
		}
	}
	return false
}
