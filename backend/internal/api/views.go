package api

import (
	"context"
	"sync"
	"time"
)

// viewCounter batches thread view counts in memory.
//
// Counting a read used to be an UPDATE on the request path, which meant every
// page view was a write transaction competing with postings for the database.
// The count is not worth that: it is displayed once, on the thread page, and a
// reader who refreshes within the flush interval sees a number that is a few
// seconds stale.
type viewCounter struct {
	mu     sync.Mutex
	counts map[int64]int64
}

func newViewCounter() *viewCounter {
	return &viewCounter{counts: make(map[int64]int64)}
}

// Add records one read of a thread.
func (v *viewCounter) Add(id int64) {
	if id == 0 {
		return
	}
	v.mu.Lock()
	v.counts[id]++
	v.mu.Unlock()
}

// take empties the buffer and returns what was in it.
func (v *viewCounter) take() map[int64]int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.counts) == 0 {
		return nil
	}
	out := v.counts
	v.counts = make(map[int64]int64)
	return out
}

// flushViewInterval is how often buffered views reach the database. Short enough
// that a reader refreshing the page sees their own view counted.
const flushViewInterval = 10 * time.Second

// flushViews writes the buffered counts in one transaction.
func (s *Server) flushViews(ctx context.Context) {
	counts := s.views.take()
	if len(counts) == 0 {
		return
	}
	if err := s.store.AddThreadViews(ctx, counts); err != nil {
		s.log.Warn("flush thread views", "err", err, "threads", len(counts))
	}
}

// viewFlusher runs until the server shuts down.
func (s *Server) viewFlusher(ctx context.Context) {
	ticker := time.NewTicker(flushViewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// One last flush: a view counted a moment before shutdown is still a
			// view, and the write is cheap.
			s.flushViews(context.WithoutCancel(ctx))
			return
		case <-ticker.C:
			s.flushViews(ctx)
		}
	}
}
