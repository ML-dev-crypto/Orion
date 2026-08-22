package cluster

import (
	"errors"
	"sync"
	"time"
)

// Cluster holds cluster membership and leadership state. It knows WHO
// the nodes are, WHO the leader is, and whether the leader has been
// heard from recently — it does not know how flags are stored or
// replicated. That's Manager's job.
type Cluster struct {
	Self  Node
	Peers []Node

	mu                  sync.RWMutex
	leaderID            string
	lastLeaderHeartbeat time.Time
}

// NewCluster builds a Cluster, filtering Self out of `all` automatically.
//
// lastLeaderHeartbeat is initialized to "now" rather than left at
// Go's zero value: a follower checking LeaderConsideredDead()
// immediately after startup, before the leader's first real
// heartbeat has had a chance to arrive, would otherwise report the
// leader dead purely due to startup timing. Starting optimistic gives
// the first heartbeat interval a grace period.
func NewCluster(self Node, all []Node, initialLeaderID string) (*Cluster, error) {
	if self.ID == "" {
		return nil, errors.New("cluster: self.ID must not be empty")
	}
	if initialLeaderID == "" {
		return nil, errors.New("cluster: initialLeaderID must not be empty")
	}

	var peers []Node
	for _, n := range all {
		if n.ID != self.ID {
			peers = append(peers, n)
		}
	}

	return &Cluster{
		Self:                self,
		Peers:               peers,
		leaderID:            initialLeaderID,
		lastLeaderHeartbeat: time.Now(),
	}, nil
}

func (c *Cluster) LeaderID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leaderID
}

func (c *Cluster) SetLeader(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.leaderID = id
}

func (c *Cluster) IsLeader() bool {
	if c.Self.ID == "" {
		return false
	}
	return c.LeaderID() == c.Self.ID
}

func (c *Cluster) Leader() (Node, bool) {
	return c.NodeByID(c.LeaderID())
}

func (c *Cluster) NodeByID(id string) (Node, bool) {
	if c.Self.ID == id {
		return c.Self, true
	}
	for _, p := range c.Peers {
		if p.ID == id {
			return p, true
		}
	}
	return Node{}, false
}

func (c *Cluster) Majority() int {
	n := len(c.Peers) + 1
	return n/2 + 1
}

func (c *Cluster) Address() string {
	return c.Self.Address
}

// RecordHeartbeat marks that a heartbeat was just received from the
// leader. Called by RPCHandler when this node receives a heartbeat RPC.
func (c *Cluster) RecordHeartbeat() {
	c.recordHeartbeatAt(time.Now())
}

func (c *Cluster) recordHeartbeatAt(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastLeaderHeartbeat = t
}

// LeaderConsideredDead reports whether it's been longer than timeout
// since the last heartbeat was recorded. This ONLY reports state — it
// does not trigger an election or change write behavior.
func (c *Cluster) LeaderConsideredDead(timeout time.Duration) bool {
	return c.leaderConsideredDeadAt(time.Now(), timeout)
}

// leaderConsideredDeadAt is the clock-injected variant used by tests
// for deterministic timing — same pattern as flags.evaluateAt.
func (c *Cluster) leaderConsideredDeadAt(now time.Time, timeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return now.Sub(c.lastLeaderHeartbeat) > timeout
}
