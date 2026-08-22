package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orion/internal/audit"
	"orion/internal/flags"
	"orion/internal/service"
	"orion/internal/store"
)

// fakeRPCClient is a no-op RPCClient used in tests, so unit tests
// exercising Manager don't make real network calls to fake peer
// addresses. Real end-to-end replication is proven separately in
// replication_test.go using actual httptest servers.
type fakeRPCClient struct{}

func (fakeRPCClient) Replicate(ctx context.Context, peer Node, f flags.Flag) error {
	return nil
}

func (fakeRPCClient) ReplicateDelete(ctx context.Context, peer Node, key string) error {
	return nil
}

func (fakeRPCClient) Heartbeat(ctx context.Context, peer Node) error {
	return nil
}

func newTestManager(t *testing.T, selfID, leaderID string) *Manager {
	t.Helper()
	self := Node{ID: selfID, Address: selfID + ":8080"}
	all := []Node{
		{ID: "node-1", Address: "node-1:8080"},
		{ID: "node-2", Address: "node-2:8080"},
		{ID: "node-3", Address: "node-3:8080"},
	}
	c, err := NewCluster(self, all, leaderID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}
	s := store.NewMemoryStore()
	a := audit.NewMemoryStore()
	w := service.NewMemoryFlagWriter(s, a)
	return NewManager(c, w, fakeRPCClient{})
}

func TestManager_Upsert_LeaderAccepts(t *testing.T) {
	m := newTestManager(t, "node-1", "node-1")
	result, err := m.Upsert(context.Background(), flags.Flag{Key: "f1", Enabled: true})
	if err != nil {
		t.Fatalf("leader's Upsert failed: %v", err)
	}
	if result.Flag.Key != "f1" {
		t.Errorf("got key %q, want f1", result.Flag.Key)
	}
}

func TestManager_Upsert_FollowerRejects(t *testing.T) {
	m := newTestManager(t, "node-2", "node-1")
	_, err := m.Upsert(context.Background(), flags.Flag{Key: "f1", Enabled: true})
	if !errors.Is(err, ErrNotLeader) {
		t.Errorf("got err=%v, want ErrNotLeader", err)
	}
}

func TestManager_Delete_LeaderAccepts(t *testing.T) {
	m := newTestManager(t, "node-1", "node-1")
	if _, err := m.Upsert(context.Background(), flags.Flag{Key: "f1", Enabled: true}); err != nil {
		t.Fatalf("seed upsert failed: %v", err)
	}
	deleted, err := m.Delete(context.Background(), "f1")
	if err != nil {
		t.Fatalf("leader's Delete failed: %v", err)
	}
	if deleted.Key != "f1" {
		t.Errorf("got deleted key %q, want f1", deleted.Key)
	}
}

func TestManager_Delete_FollowerRejects(t *testing.T) {
	m := newTestManager(t, "node-2", "node-1")
	_, err := m.Delete(context.Background(), "f1")
	if !errors.Is(err, ErrNotLeader) {
		t.Errorf("got err=%v, want ErrNotLeader", err)
	}
}

func TestNewCluster_RejectsEmptySelfID(t *testing.T) {
	_, err := NewCluster(Node{ID: ""}, nil, "node-1")
	if err == nil {
		t.Error("expected error for empty self.ID, got nil")
	}
}

func TestNewCluster_RejectsEmptyInitialLeaderID(t *testing.T) {
	_, err := NewCluster(Node{ID: "node-1"}, nil, "")
	if err == nil {
		t.Error("expected error for empty initialLeaderID, got nil")
	}
}

func TestNewCluster_ExcludesSelfFromPeers(t *testing.T) {
	self := Node{ID: "node-1"}
	all := []Node{
		{ID: "node-1"},
		{ID: "node-2"},
		{ID: "node-3"},
	}
	c, err := NewCluster(self, all, "node-1")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}
	if len(c.Peers) != 2 {
		t.Fatalf("got %d peers, want 2", len(c.Peers))
	}
	for _, p := range c.Peers {
		if p.ID == "node-1" {
			t.Error("Peers contains Self — NewCluster failed to filter it out")
		}
	}
}

func TestCluster_Majority(t *testing.T) {
	tests := []struct {
		totalPeers int
		want       int
	}{
		{0, 1},
		{1, 2},
		{2, 2},
		{3, 3},
		{4, 3},
	}
	for _, tt := range tests {
		var all []Node
		for i := 0; i < tt.totalPeers; i++ {
			all = append(all, Node{ID: "peer" + string(rune('a'+i))})
		}
		c, err := NewCluster(Node{ID: "self"}, all, "self")
		if err != nil {
			t.Fatalf("NewCluster failed: %v", err)
		}
		if got := c.Majority(); got != tt.want {
			t.Errorf("%d peers: Majority() = %d, want %d", tt.totalPeers, got, tt.want)
		}
	}
}

func TestCluster_IsLeader_FalseWhenSelfIDAccidentallyEmpty(t *testing.T) {
	c := &Cluster{Self: Node{ID: ""}}
	c.SetLeader("")
	if c.IsLeader() {
		t.Error("IsLeader() returned true for an empty Self.ID matching an empty leaderID")
	}
}

func TestCluster_Leader_ResolvesToFullNode(t *testing.T) {
	self := Node{ID: "node-1", Address: "node-1:8080"}
	all := []Node{
		{ID: "node-1", Address: "node-1:8080"},
		{ID: "node-2", Address: "node-2:8080"},
	}
	c, err := NewCluster(self, all, "node-2")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}
	leader, ok := c.Leader()
	if !ok {
		t.Fatal("Leader() returned ok=false, expected to resolve node-2")
	}
	if leader.Address != "node-2:8080" {
		t.Errorf("got address %q, want node-2:8080", leader.Address)
	}
}

func TestParsePeers_DuplicateID(t *testing.T) {
	_, err := ParsePeers("node1=localhost:8081,node1=localhost:9999,node2=localhost:8082")
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "node1") {
		t.Errorf("error should mention 'duplicate' and 'node1', got: %v", err)
	}
}

func TestParsePeers_DuplicateAddress(t *testing.T) {
	_, err := ParsePeers("node1=localhost:8081,node2=localhost:8081")
	if err == nil {
		t.Fatal("expected error for duplicate address, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "localhost:8081") {
		t.Errorf("error should mention 'duplicate' and 'localhost:8081', got: %v", err)
	}
}

func TestParsePeers_ValidMultiNode(t *testing.T) {
	nodes, err := ParsePeers("node1=localhost:8081,node2=localhost:8082,node3=localhost:8083")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("got %d nodes, want 3", len(nodes))
	}
	if nodes[0].ID != "node1" || nodes[0].Address != "localhost:8081" {
		t.Errorf("unexpected first node: %+v", nodes[0])
	}
}

func TestCluster_Address(t *testing.T) {
	c, err := NewCluster(
		Node{ID: "self", Address: "localhost:9000"},
		[]Node{{ID: "self", Address: "localhost:9000"}},
		"self")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}
	if c.Address() != "localhost:9000" {
		t.Errorf("Address() = %q, want localhost:9000", c.Address())
	}
}

func TestCluster_NodeByID(t *testing.T) {
	self := Node{ID: "node-1", Address: "node-1:8080"}
	all := []Node{
		{ID: "node-1", Address: "node-1:8080"},
		{ID: "node-2", Address: "node-2:8082"},
		{ID: "node-3", Address: "node-3:8083"},
	}
	c, err := NewCluster(self, all, "node-1")
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	// Resolve self
	n, ok := c.NodeByID("node-1")
	if !ok || n.Address != "node-1:8080" {
		t.Errorf("NodeByID(node-1) = %+v, %v; want node-1:8080, true", n, ok)
	}

	// Resolve a peer
	n, ok = c.NodeByID("node-2")
	if !ok || n.Address != "node-2:8082" {
		t.Errorf("NodeByID(node-2) = %+v, %v; want node-2:8082, true", n, ok)
	}

	// Unknown ID
	_, ok = c.NodeByID("node-99")
	if ok {
		t.Error("NodeByID(node-99) returned ok=true, expected false")
	}
}
