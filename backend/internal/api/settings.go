package api

import (
	"context"
	"sync"
)

// settingsCache keeps the settings table in memory.
//
// The table is tiny but was read from SQLite several times per request — once for
// each `getSetting` call, again for the mail configuration and again for the spam
// rules — and every one of those reads was a round trip to the database. A single
// snapshot per process removes them all.
//
// Invalidation is explicit: every write path calls invalidateSettings. Settings
// are only ever changed through the API, so there is no second writer to miss.
type settingsCache struct {
	mu     sync.RWMutex
	values map[string]string
	valid  bool
}

// settings returns the settings, reading them from the database on first use and
// after an invalidation.
func (s *Server) settings(ctx context.Context) (map[string]string, error) {
	s.cfgCache.mu.RLock()
	if s.cfgCache.valid {
		values := s.cfgCache.values
		s.cfgCache.mu.RUnlock()
		return values, nil
	}
	s.cfgCache.mu.RUnlock()

	values, err := s.store.Settings(ctx)
	if err != nil {
		return nil, err
	}

	s.cfgCache.mu.Lock()
	s.cfgCache.values = values
	s.cfgCache.valid = true
	s.cfgCache.mu.Unlock()
	return values, nil
}

// invalidateSettings makes the next read go to the database.
func (s *Server) invalidateSettings() {
	s.cfgCache.mu.Lock()
	s.cfgCache.valid = false
	s.cfgCache.values = nil
	s.cfgCache.mu.Unlock()
}
