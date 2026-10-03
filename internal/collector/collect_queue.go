package collector

import (
	"context"

	"idp-dashboard/internal/platform"
)

func (c *Collector) pollQueue(ctx context.Context, p platform.QueueSource, next *platform.Snapshot) {
	queue, err := p.Queue(ctx)
	if err != nil {
		next.Error = err.Error()
	} else {
		next.Queue = []platform.QueueItem{}
		for _, q := range queue {
			if c.selected(q.Job, "queue") {
				next.Queue = append(next.Queue, q)
			}
		}
	}
}
