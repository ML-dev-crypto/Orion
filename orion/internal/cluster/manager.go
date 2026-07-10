package cluster

import (
	"context"
	"errors"

	"orion/internal/flags"
	"orion/internal/service"
)

// ErrNotLeader is returned when a write is attempted against a node
// that isn't the current leader. Stage 1 has no networking yet, so
// there's no redirect or proxy here — that's stage 6.
var ErrNotLeader = errors.New("cluster: this node is not the leader")

// Manager is what the HTTP handler talks to for writes, instead of
// talking to service.FlagWriter directly. Its only job right now is
// gating: refuse writes on a non-leader node, delegate to the
// existing FlagWriter otherwise.
type Manager struct {
	cluster *Cluster
	writer  service.FlagWriter
}

func NewManager(c *Cluster, w service.FlagWriter) *Manager {
	return &Manager{cluster: c, writer: w}
}

func (m *Manager) Upsert(ctx context.Context, f flags.Flag) (service.WriteResult, error) {
	if !m.cluster.IsLeader() {
		return service.WriteResult{}, ErrNotLeader
	}
	return m.writer.Upsert(ctx, f)
}

func (m *Manager) Delete(ctx context.Context, key string) (flags.Flag, error) {
	if !m.cluster.IsLeader() {
		return flags.Flag{}, ErrNotLeader
	}
	return m.writer.Delete(ctx, key)
}
