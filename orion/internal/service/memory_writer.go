package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"orion/internal/audit"
	"orion/internal/flags"
	"orion/internal/store"
)

// MemoryFlagWriter provides the same atomicity guarantee as
// PostgresFlagWriter, using a plain Go mutex instead of a DB
// transaction + advisory lock.
type MemoryFlagWriter struct {
	mu    sync.Mutex
	store *store.MemoryStore
	audit *audit.MemoryStore
}

func NewMemoryFlagWriter(s *store.MemoryStore, a *audit.MemoryStore) *MemoryFlagWriter {
	return &MemoryFlagWriter{store: s, audit: a}
}

func (w *MemoryFlagWriter) Upsert(ctx context.Context, f flags.Flag) (WriteResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	existing, err := w.store.Get(ctx, f.Key)
	existed := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return WriteResult{}, err
	}

	if err := w.store.Put(ctx, f); err != nil {
		return WriteResult{}, err
	}

	saved, err := w.store.Get(ctx, f.Key)
	if err != nil {
		return WriteResult{}, err
	}

	result := WriteResult{Flag: saved}
	if existed {
		e := existing
		result.Existing = &e
		result.Changed = !flags.Equal(existing, saved)
	} else {
		result.Changed = true
	}

	if result.Changed {
		action := audit.ActionUpdate
		if !existed {
			action = audit.ActionCreate
		}
		entry := audit.Entry{
			FlagKey:   f.Key,
			Action:    action,
			Before:    result.Existing,
			After:     &saved,
			Timestamp: time.Now().UTC(),
		}
		if err := w.audit.Append(ctx, entry); err != nil {
			return WriteResult{}, err
		}
	}

	return result, nil
}

func (w *MemoryFlagWriter) Delete(ctx context.Context, key string) (flags.Flag, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	existing, err := w.store.Get(ctx, key)
	if err != nil {
		return flags.Flag{}, err
	}

	if err := w.store.Delete(ctx, key); err != nil {
		return flags.Flag{}, err
	}

	entry := audit.Entry{
		FlagKey:   key,
		Action:    audit.ActionDelete,
		Before:    &existing,
		Timestamp: time.Now().UTC(),
	}
	if err := w.audit.Append(ctx, entry); err != nil {
		return flags.Flag{}, err
	}

	return existing, nil
}