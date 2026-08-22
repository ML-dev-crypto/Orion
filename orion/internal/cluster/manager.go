package cluster

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"orion/internal/flags"
	"orion/internal/service"
)

var ErrNotLeader = errors.New("cluster: this node is not the leader")

// ErrQuorumNotReached means the write was applied to the leader's own
// store, but too few peers acknowledged it to satisfy Majority().
//
// KNOWN LIMITATION, stated explicitly rather than hidden: the leader's
// local write already happened BEFORE replication was attempted, so
// when this error is returned, the leader's local state has already
// diverged from what was just reported as a failure — the data exists
// locally but isn't durable across a majority. Real Raft solves this
// with a retry loop and a commit-index concept; that complexity is
// out of scope for this stage. This is accepted as a documented gap
// for now, not silently pretended away with a fake rollback (which
// would need real undo semantics: was this a create that should be
// deleted, or an update that should revert to a specific prior
// value?).
var ErrQuorumNotReached = errors.New("cluster: write did not reach quorum")

// Manager is what the HTTP handler talks to for writes. It gates
// writes to the leader, delegates the write to FlagWriter, replicates
// the result to every peer, and requires a majority of the cluster
// (including the leader's own local write) to have the data before
// reporting success.
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
	acks := 1 + m.replicate(result.Flag)
	if acks < m.cluster.Majority() {
		return result, fmt.Errorf("%w: %d/%d acks, need %d",
			ErrQuorumNotReached, acks, len(m.cluster.Peers)+1, m.cluster.Majority())
	}
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
	acks := 1 + m.replicateDelete(key)
	if acks < m.cluster.Majority() {
		return deleted, fmt.Errorf("%w: %d/%d acks, need %d",
			ErrQuorumNotReached, acks, len(m.cluster.Peers)+1, m.cluster.Majority())
	}
	return deleted, nil
}

// replicate sends f to every peer in parallel, waits for every
// attempt to finish, and returns how many SUCCEEDED.
func (m *Manager) replicate(f flags.Flag) int {
	var wg sync.WaitGroup
	var successes atomic.Int32
	for _, peer := range m.cluster.Peers {
		peer := peer
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := m.rpc.Replicate(ctx, peer, f); err != nil {
				log.Printf("cluster: replication of %q to %s failed: %v", f.Key, peer.ID, err)
				return
			}
			successes.Add(1)
		}()
	}
	wg.Wait()
	return int(successes.Load())
}

func (m *Manager) replicateDelete(key string) int {
	var wg sync.WaitGroup
	var successes atomic.Int32
	for _, peer := range m.cluster.Peers {
		peer := peer
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := m.rpc.ReplicateDelete(ctx, peer, key); err != nil {
				log.Printf("cluster: replicate-delete of %q to %s failed: %v", key, peer.ID, err)
				return
			}
			successes.Add(1)
		}()
	}
	wg.Wait()
	return int(successes.Load())
}

type Status struct {
	SelfID          string `json:"self_id"`
	LeaderID        string `json:"leader_id"`
	IsLeader        bool   `json:"is_leader"`
	LeaderReachable bool   `json:"leader_reachable"`
	Peers           []Node `json:"peers"`
}

func (m *Manager) Status() Status {
	reachable := m.cluster.IsLeader() || !m.cluster.LeaderConsideredDead(DefaultLeaderTimeout)
	return Status{
		SelfID:          m.cluster.Self.ID,
		LeaderID:        m.cluster.LeaderID(),
		IsLeader:        m.cluster.IsLeader(),
		LeaderReachable: reachable,
		Peers:           m.cluster.Peers,
	}
}
