package audit

import (
	"context"
	"sync"
)

type MemoryStore struct {
	mu      sync.RWMutex
	entries []Entry
	nextID  int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Append(ctx context.Context, e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	e.ID = s.nextID
	s.entries = append(s.entries, e)
	return nil
}

func (s *MemoryStore) List(ctx context.Context, flagKey string) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Entry
	for _, e := range s.entries {
		if e.FlagKey == flagKey {
			result = append(result, e)
		}
	}
	return result, nil
}