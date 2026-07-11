package cluster

import (
	"errors"
	"sync"
)

// Cluster holds cluster membership and leadership state. It knows WHO
// the nodes are and WHO the leader is — it does not know how flags
// are stored or replicated. That's Manager's job (and, beneath that,
// service.FlagWriter's).
type Cluster struct {
	Self  Node
	Peers []Node // filtered by NewCluster; never includes Self

	mu       sync.RWMutex
	leaderID string
}

// NewCluster builds a Cluster, filtering Self out of `all` automatically
// so no caller can accidentally leave Self in Peers (which would make
// a node try to replicate to itself once that logic exists).
//
// initialLeaderID is required, not inferred or defaulted — an empty
// string here would be indistinguishable from Go's zero-value for
// leaderID, and if Self.ID were ever also accidentally empty,
// IsLeader() would silently evaluate "" == "" and a misconfigured
// node would believe it's the leader. Requiring both explicitly and
// erroring on empty closes that off at construction time rather than
// leaving it as a latent bug to hit at runtime.
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
		Self:     self,
		Peers:    peers,
		leaderID: initialLeaderID,
	}, nil
}

// LeaderID returns the current leader's ID.
func (c *Cluster) LeaderID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leaderID
}

// SetLeader updates the current leader. Concurrency-safe — this will
// be called from a background election/heartbeat goroutine in later
// stages while HTTP handler goroutines concurrently call IsLeader().
func (c *Cluster) SetLeader(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.leaderID = id
}

// IsLeader reports whether THIS node is currently the leader. Guards
// against an empty Self.ID explicitly — NewCluster already rejects
// this at construction time, but Self is an exported field, so a
// Cluster built via a bare struct literal (bypassing NewCluster
// entirely) wouldn't get that protection. This check makes the
// invariant hold regardless of how the Cluster was constructed.
func (c *Cluster) IsLeader() bool {
	if c.Self.ID == "" {
		return false
	}
	return c.LeaderID() == c.Self.ID
}

// Leader resolves the current leader ID to a full Node, searching Self
// and Peers. The bool is false if the leader ID doesn't match any
// known node.
func (c *Cluster) Leader() (Node, bool) {
	id := c.LeaderID()
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

// Majority returns the number of acks — including the leader's own
// local write, which always counts as the first one — needed to
// commit a write. floor(N/2)+1 where N is total cluster size
// (Peers + Self).
func (c *Cluster) Majority() int {
	n := len(c.Peers) + 1
	return n/2 + 1
}

// Address returns this node's own address. Convenience accessor so
// callers don't need to reach into c.Self.Address directly.
func (c *Cluster) Address() string {
	return c.Self.Address
}

// NodeByID resolves any node ID — including Self — to a full Node.
// Returns false if the ID is unknown. Useful during replication when
// the leader needs to address a specific peer by ID.
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
