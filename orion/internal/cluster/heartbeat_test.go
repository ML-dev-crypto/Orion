package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"orion/internal/flags"
	"orion/internal/store"
)

func TestCluster_LeaderConsideredDead_Deterministic(t *testing.T) {
	self := Node{ID: "follower"}
	c, err := NewCluster(self, []Node{self, {ID: "leader"}}, "leader")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.recordHeartbeatAt(base)

	if c.leaderConsideredDeadAt(base.Add(1*time.Second), 3*time.Second) {
		t.Error("should NOT be considered dead 1s after a heartbeat with a 3s timeout")
	}
	if !c.leaderConsideredDeadAt(base.Add(4*time.Second), 3*time.Second) {
		t.Error("SHOULD be considered dead 4s after a heartbeat with a 3s timeout")
	}
}

func TestCluster_LeaderConsideredDead_ExactBoundary(t *testing.T) {
	self := Node{ID: "follower"}
	c, err := NewCluster(self, []Node{self, {ID: "leader"}}, "leader")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.recordHeartbeatAt(base)

	if c.leaderConsideredDeadAt(base.Add(3*time.Second), 3*time.Second) {
		t.Error("exactly at the timeout boundary should not yet be considered dead")
	}
}

func TestCluster_NewCluster_GracePeriodAtStartup(t *testing.T) {
	self := Node{ID: "follower"}
	c, err := NewCluster(self, []Node{self, {ID: "leader"}}, "leader")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}
	if c.LeaderConsideredDead(3 * time.Second) {
		t.Error("should not be considered dead immediately after construction")
	}
}

func TestHeartbeat_EndToEnd_RealHTTP(t *testing.T) {
	followerSelf := Node{ID: "follower"}
	followerCluster, err := NewCluster(followerSelf, []Node{followerSelf, {ID: "leader"}}, "leader")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	followerStore := store.NewMemoryStore()
	h := NewRPCHandler(followerStore, followerCluster)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	followerAsPeer := Node{ID: "follower", Address: addressOf(srv)}
	client := NewClient()
	if err := client.Heartbeat(context.Background(), followerAsPeer); err != nil {
		t.Fatalf("Heartbeat call failed: %v", err)
	}

	if followerCluster.LeaderConsideredDead(5 * time.Second) {
		t.Error("expected leader to be considered alive right after a real heartbeat HTTP call")
	}
}

type countingRPCClient struct {
	mu    sync.Mutex
	calls map[string]int
}

func newCountingRPCClient() *countingRPCClient {
	return &countingRPCClient{calls: make(map[string]int)}
}

func (c *countingRPCClient) Replicate(ctx context.Context, peer Node, f flags.Flag) error {
	return nil
}

func (c *countingRPCClient) ReplicateDelete(ctx context.Context, peer Node, key string) error {
	return nil
}

func (c *countingRPCClient) Heartbeat(ctx context.Context, peer Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[peer.ID]++
	return nil
}

func (c *countingRPCClient) count(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[id]
}

func TestRunHeartbeatLoop_SendsToAllPeersRepeatedly(t *testing.T) {
	self := Node{ID: "leader"}
	peers := []Node{self, {ID: "p1"}, {ID: "p2"}}
	c, err := NewCluster(self, peers, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	rpc := newCountingRPCClient()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunHeartbeatLoop(ctx, c, rpc, 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(85 * time.Millisecond)
	cancel()
	<-done

	if rpc.count("p1") < 2 {
		t.Errorf("expected multiple heartbeats to p1, got %d", rpc.count("p1"))
	}
	if rpc.count("p2") < 2 {
		t.Errorf("expected multiple heartbeats to p2, got %d", rpc.count("p2"))
	}
}

func TestRunHeartbeatLoop_StopsOnContextCancel(t *testing.T) {
	self := Node{ID: "leader"}
	peers := []Node{self, {ID: "p1"}}
	c, err := NewCluster(self, peers, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	rpc := newCountingRPCClient()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunHeartbeatLoop(ctx, c, rpc, 10*time.Millisecond)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("heartbeat loop did not stop within 1s of context cancellation")
	}
}
