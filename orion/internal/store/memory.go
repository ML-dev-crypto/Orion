package store

import (
	"context"
	"sync"

	"orion/internal/flags"
)

// MemoryStore is a Storage implementation backed by a plain Go map.
// It satisfies the Storage interface just by having these four methods —
// no explicit declaration needed, unlike Java's `implements` keyword.
//
// Go concept: sync.RWMutex. Every HTTP request runs on its own goroutine,
// so multiple requests can call these methods at the same time. A plain
// map is NOT safe for concurrent read/write in Go — it will panic at
// runtime ("concurrent map writes"). RWMutex fixes this: RLock() lets
// many readers in at once, Lock() gives one writer exclusive access.
type MemoryStore struct {
	mu    sync.RWMutex
	flags map[string]flags.Flag
}

// NewMemoryStore constructs a ready-to-use MemoryStore.
// Go concept: constructor functions. Go has no constructors like
// `new MemoryStore()` — the convention is a `NewX` function that
// returns a properly initialized value. Here we must initialize the
// map explicitly; a zero-value map (var m map[string]Flag) is nil and
// panics on write.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		flags: make(map[string]flags.Flag),
	}
}

// Get retrieves a flag by key.
// Go concept: multiple return values. Instead of throwing an exception
// for "not found," Get returns a zero-value Flag AND an error. The
// caller is forced to check the error before trusting the Flag.
func (s *MemoryStore) Get(ctx context.Context, key string) (flags.Flag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock() // runs when the function returns, whichever path it takes

	f, ok := s.flags[key]
	if !ok {
		return flags.Flag{}, ErrNotFound
	}
	return f, nil
}

// Put creates or updates a flag.
func (s *MemoryStore) Put(ctx context.Context, f flags.Flag) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.flags[f.Key] = f
	return nil
}

// Delete removes a flag.
func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.flags[key]; !ok {
		return ErrNotFound
	}
	delete(s.flags, key)
	return nil
}

// List returns every flag currently stored.
func (s *MemoryStore) List(ctx context.Context) ([]flags.Flag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]flags.Flag, 0, len(s.flags))
	for _, f := range s.flags {
		result = append(result, f)
	}
	return result, nil
}
