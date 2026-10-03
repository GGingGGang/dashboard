package platform

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Collector belongs to one immutable connection configuration. Replace it on edits.
type Collector struct {
	Connection Connection
	Provider   Provider
	Store      *Store
	mu         sync.Mutex
	cursors    map[string]int
	previous   Snapshot
}

func NewCollector(c Connection, p Provider, s *Store) *Collector {
	return &Collector{Connection: c, Provider: p, Store: s, cursors: map[string]int{}, previous: Snapshot{ConnectionID: c.ID, Builds: []Build{}, Queue: []QueueItem{}, Deployments: []Deployment{}, Rules: []RuleResult{}}}
}

func (c *Collector) Poll(ctx context.Context) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.previous
	next.Attempted = time.Now()
	next.Error = ""
	next.StorageError = ""
	switch p := c.Provider.(type) {
	case CI:
		c.pollCI(ctx, p, &next)
	case CD:
		apps, err := p.Deployments(ctx)
		if err != nil {
			next.Error = err.Error()
			break
		}
		next.Deployments = []Deployment{}
		for _, a := range apps {
			if c.selected(a.ID) {
				next.Deployments = append(next.Deployments, a)
			}
		}
	case Monitoring:
		next.Rules = []RuleResult{}
		if err := p.Check(ctx); err != nil {
			next.Error = err.Error()
			break
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
	if next.Error == "" {
		next.LastSuccess = time.Now()
	}
	c.previous = next
	return next
}

func (c *Collector) selected(id string) bool {
	for _, t := range c.Connection.Targets {
		if t.ID == id {
			return true
		}
	}
	return false
}

func (c *Collector) pollCI(ctx context.Context, p CI, next *Snapshot) {
	queue, err := p.Queue(ctx)
	if err != nil {
		next.Error = err.Error()
	} else {
		next.Queue = []QueueItem{}
		for _, q := range queue {
			if c.selected(q.Job) {
				next.Queue = append(next.Queue, q)
			}
		}
	}
	current := []Build{}
	allOK := true
	next.BackfillPending = false
	for _, t := range c.Connection.Targets {
		if ctx.Err() != nil {
			next.Error = "Collection interrupted"
			allOK = false
			break
		}
		builds, more, e := p.Builds(ctx, t.ID, 0)
		if e != nil {
			next.Error = e.Error()
			allOK = false
			continue
		}
		current = append(current, builds...)
		known := false
		for _, b := range builds {
			k, e := c.Store.Known(b)
			if e != nil {
				next.StorageError = "Cannot read local archive"
				break
			}
			known = known || k
		}
		if e = c.Store.SaveBuilds(builds); e != nil {
			next.StorageError = "Cannot save local archive; check disk space and file permissions"
			continue
		}
		cursor, exists := c.cursors[t.ID]
		if !exists || (cursor < 0 && !known && more) {
			cursor = 80
		}
		if !more {
			cursor = -1
		}
		if cursor >= 0 {
			older, hasMore, e := p.Builds(ctx, t.ID, cursor)
			if e != nil {
				next.Error = e.Error()
			} else if e = c.Store.SaveBuilds(older); e != nil {
				next.StorageError = "Cannot save historical builds"
			} else {
				next.Imported += len(older)
				if hasMore {
					cursor += 80
				} else {
					cursor = -1
				}
			}
		}
		c.cursors[t.ID] = cursor
		next.BackfillPending = next.BackfillPending || cursor >= 0
	}
	if allOK {
		next.Builds = current
	}
	// Revisit previously observed running builds even when they have fallen off page one.
	running, e := c.Store.Running(c.Connection.ID)
	if e != nil {
		next.StorageError = "Cannot read unfinished builds"
		return
	}
	for _, old := range running {
		if !c.selected(old.Job) {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		fresh, e := p.Build(ctx, old.Job, old.Number)
		var ae *APIError
		if e != nil {
			if errors.As(e, &ae) && ae.Status == 404 {
				old.Status = "UNCONFIRMED"
				old.Observed = time.Now().UnixMilli()
				if e = c.Store.SaveBuilds([]Build{old}); e != nil {
					next.StorageError = "Cannot save missing-build state"
				}
			}
			continue
		}
		if fresh.Started != old.Started {
			old.Status = "UNCONFIRMED"
			old.Observed = time.Now().UnixMilli()
			if e = c.Store.SaveBuilds([]Build{old}); e != nil {
				next.StorageError = "Cannot save replaced-build state"
			}
		}
		if e = c.Store.SaveBuilds([]Build{fresh}); e != nil {
			next.StorageError = "Cannot save completed build"
		}
	}
}
