package cluster

import (
	"context"
	"errors"
	"testing"

	"orion/internal/audit"
	"orion/internal/flags"
	"orion/internal/service"
	"orion/internal/store"
)

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
	return NewManager(c, w)
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
