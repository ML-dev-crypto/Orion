package service

import (
	"context"

	"orion/internal/flags"
)

// WriteResult is returned by a write operation, describing what
// happened so the handler can respond appropriately.
type WriteResult struct {
	Flag     flags.Flag
	Existing *flags.Flag // nil if this write was a create
	Changed  bool        // false only for a no-op update (identical resend)
}

// FlagWriter is the single entry point for mutating a flag AND
// recording its audit trail as one atomic unit. Every implementation
// must guarantee: between reading the current value and committing
// the new value + its audit entry, no other writer can interleave on
// the same key. HOW that's guaranteed is entirely up to the
// implementation (a DB transaction + advisory lock for Postgres, a
// mutex for the in-memory backend) — this interface stays storage-
// agnostic on purpose, so V5's OrionLSM can satisfy it with its own
// synchronization primitives without this interface, or anything
// above it, changing at all.
type FlagWriter interface {
	Upsert(ctx context.Context, f flags.Flag) (WriteResult, error)
	Delete(ctx context.Context, key string) (flags.Flag, error)
}