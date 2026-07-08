package audit

import (
	"context"
	"time"

	"orion/internal/flags"
)

type Action string

const (
	ActionCreate Action = "CREATE"
	ActionUpdate Action = "UPDATE"
	ActionDelete Action = "DELETE"
)

type Entry struct {
	ID        int64       `json:"id"`
	FlagKey   string      `json:"flag_key"`
	Action    Action      `json:"action"`
	Before    *flags.Flag `json:"before,omitempty"`
	After     *flags.Flag `json:"after,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

type Store interface {
	Append(ctx context.Context, e Entry) error
	List(ctx context.Context, flagKey string) ([]Entry, error)
}