package platform

import "context"

func (c *Collector) pollQueue(ctx context.Context, p QueueSource, next *Snapshot) {
	queue, err := p.Queue(ctx)
	if err != nil {
		next.Error = err.Error()
	} else {
		next.Queue = []QueueItem{}
		for _, q := range queue {
			if c.selected(q.Job, "queue") {
				next.Queue = append(next.Queue, q)
			}
		}
	}
}
