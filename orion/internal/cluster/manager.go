package cluster

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"orion/internal/flags"
	"orion/internal/service"
)

var ErrNotLeader = errors.New("cluster: this node is not the leader")

// Manager is what the HTTP handler talks to for writes. It gates
// writes to the leader, delegates the write to FlagWriter, then
// replicates the result to every peer.
type Manager struct {
	cluster *Cluster
	writer  service.FlagWriter
	rpc     RPCClient
}

func NewManager(c *Cluster, w service.FlagWriter, rpc RPCClient) *Manager {
	return &Manager{cluster: c, writer: w, rpc: rpc}
}

func (m *Manager) Upsert(ctx context.Context, f flags.Flag) (service.WriteResult, error) {
	if !m.cluster.IsLeader() {
		return service.WriteResult{}, ErrNotLeader
	}
	result, err := m.writer.Upsert(ctx, f)
	if err != nil {
		return result, err
	}
	m.replicate(result.Flag)
	return result, nil
}

func (m *Manager) Delete(ctx context.Context, key string) (flags.Flag, error) {
	if !m.cluster.IsLeader() {
		return flags.Flag{}, ErrNotLeader
	}
	deleted, err := m.writer.Delete(ctx, key)
	if err != nil {
		return deleted, err
	}
	m.replicateDelete(key)
	return deleted, nil
}

// replicate sends f to every peer IN PARALLEL and waits for every
// attempt to finish (success or failure) before returning. This is
// "fire-and-forget" in the sense of no quorum requirement yet — a
// failed peer doesn't fail the write. It is NOT detached/background:
// waiting for attempts to complete (without requiring success) is
// what makes "leader has data, follower has identical data" checkable
// immediately after a write returns, rather than racy.
func (m *Manager) replicate(f flags.Flag) {
	var wg sync.WaitGroup
	for _, peer := range m.cluster.Peers {
		peer := peer
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := m.rpc.Replicate(ctx, peer, f); err != nil {
				log.Printf("cluster: replication of %q to %s failed: %v", f.Key, peer.ID, err)
			}
		}()
	}
	wg.Wait()
}

func (m *Manager) replicateDelete(key string) {
	var wg sync.WaitGroup
	for _, peer := range m.cluster.Peers {
		peer := peer
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := m.rpc.ReplicateDelete(ctx, peer, key); err != nil {
				log.Printf("cluster: replicate-delete of %q to %s failed: %v", key, peer.ID, err)
			}
		}()
	}
	wg.Wait()
}

type Status struct {
	SelfID   string `json:"self_id"`
	LeaderID string `json:"leader_id"`
	IsLeader bool   `json:"is_leader"`
	Peers    []Node `json:"peers"`
}

func (m *Manager) Status() Status {
	return Status{
		SelfID:   m.cluster.Self.ID,
		LeaderID: m.cluster.LeaderID(),
		IsLeader: m.cluster.IsLeader(),
		Peers:    m.cluster.Peers,
	}
}
