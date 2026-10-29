package fetch

import (
	"context"
	"sync"
	"time"
)

// every is how long a Cache keeps an answer: Radarr's film list is about a
// megabyte, and qBittorrent's torrents several, too much to read every time
// the snapshot is.
const every = 10 * time.Minute

// Cache keeps a source's last answer for ten minutes, for an answer too big
// to read with every snapshot, or a source that allows few reads, such as
// GitHub without a token. A read that fails isn't kept, so the next snapshot
// tries again.
//
// ponytail: an answer kept longer than a snapshot (ADR 0004), in memory and
// one per source, so a film can take ten minutes to count. If a source ever
// serves on its own the totals read from it, read those with every snapshot
// and drop its cache.
type Cache[T any] struct {
	// Now is the clock, time.Now unless a test sets it.
	Now func() time.Time

	mu   sync.Mutex
	at   time.Time
	last T
}

// Get returns the kept answer, or read's if the kept one is ten minutes old.
func (c *Cache[T]) Get(ctx context.Context, read func(context.Context) (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	if !c.at.IsZero() && now().Sub(c.at) < every {
		return c.last, nil
	}
	v, err := read(ctx)
	if err != nil {
		return v, err
	}
	c.at, c.last = now(), v
	return v, nil
}
