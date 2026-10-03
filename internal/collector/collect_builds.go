package collector

import (
	"context"
	"errors"
	"time"

	"idp-dashboard/internal/platform"
)

func (c *Collector) pollBuilds(ctx context.Context, p platform.BuildSource, next *platform.Snapshot) {
	current := []platform.Build{}
	allOK := true
	next.BackfillPending = false
	for _, t := range c.Connection.Targets {
		if !c.selected(t.ID, "builds") {
			continue
		}
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
		if !c.selected(old.Job, "builds") {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		fresh, e := p.Build(ctx, old.Job, old.Number)
		var ae *platform.APIError
		if e != nil {
			if errors.As(e, &ae) && ae.Status == 404 {
				old.Status = "UNCONFIRMED"
				old.Observed = time.Now().UnixMilli()
				if e = c.Store.SaveBuilds([]platform.Build{old}); e != nil {
					next.StorageError = "Cannot save missing-build state"
				}
			}
			continue
		}
		if fresh.Started != old.Started {
			old.Status = "UNCONFIRMED"
			old.Observed = time.Now().UnixMilli()
			if e = c.Store.SaveBuilds([]platform.Build{old}); e != nil {
				next.StorageError = "Cannot save replaced-build state"
			}
		}
		if e = c.Store.SaveBuilds([]platform.Build{fresh}); e != nil {
			next.StorageError = "Cannot save completed build"
		}
	}
}
