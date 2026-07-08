package store

import (
	"context"
	"errors"

	"orion/internal/flags"
)

// ErrNotFound is returned when a flag doesn't exist.
// Go concept: sentinel errors. Instead of exceptions, Go functions return
// an `error` value. Callers check it with `if err != nil`. Defining a
// package-level error like this lets callers do `errors.Is(err, store.ErrNotFound)`
// to check for this *specific* failure instead of just "something went wrong."
var ErrNotFound = errors.New("flag not found")

// Storage is the interface every backend must satisfy.
//
// Go concept: interfaces here are implicit. Nothing says
// "MemoryStore implements Storage" anywhere — if a type has methods
// matching this signature, it satisfies the interface automatically.
// This is what lets you swap Postgres for OrionLSM later without
// changing a single line in handler.go: handler.go only ever talks
// to this interface, never to a concrete type.
//
// Every method takes a context.Context as its first argument — this is
// Go's standard way of carrying cancellation/timeouts through a call chain.
// If an HTTP request gets cancelled (client disconnects), ctx lets that
// signal propagate down into the database call so it can stop early.
type Storage interface {
	Get(ctx context.Context, key string) (flags.Flag, error)
	Put(ctx context.Context, f flags.Flag) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context) ([]flags.Flag, error)
}
