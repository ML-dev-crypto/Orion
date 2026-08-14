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

func setupClusterWithFollowers(t *testing.T, deadCount int) (*Manager, store.Storage) {
	t.Helper()
	self := Node{ID: "leader", Address: "leader:9999"}
	peers := []Node{self}

	aliveCount := 2 - deadCount
	for i := 0; i < aliveCount; i++ {
		srv, _ := newFollowerServer(t)
		peers = append(peers, Node{ID: "alive", Address: addressOf(srv)})
	}
	for i := 0; i < deadCount; i++ {
		peers = append(peers, Node{ID: "dead", Address: "127.0.0.1:1"})
	}

	c, err := NewCluster(self, peers, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	leaderStore := store.NewMemoryStore()
	leaderAudit := audit.NewMemoryStore()
	writer := service.NewMemoryFlagWriter(leaderStore, leaderAudit)
	m := NewManager(c, writer, NewClient())
	return m, leaderStore
}

func TestQuorum_AllPeersHealthy_Succeeds(t *testing.T) {
	m, _ := setupClusterWithFollowers(t, 0)
	_, err := m.Upsert(context.Background(), flags.Flag{Key: "f", Enabled: true})
	if err != nil {
		t.Errorf("expected success with all peers healthy, got: %v", err)
	}
}

func TestQuorum_OnePeerDown_StillSucceeds(t *testing.T) {
	m, _ := setupClusterWithFollowers(t, 1)
	_, err := m.Upsert(context.Background(), flags.Flag{Key: "f", Enabled: true})
	if err != nil {
		t.Errorf("expected success with 1/2 peers down (still meets majority), got: %v", err)
	}
}

func TestQuorum_BothPeersDown_Fails(t *testing.T) {
	m, _ := setupClusterWithFollowers(t, 2)
	_, err := m.Upsert(context.Background(), flags.Flag{Key: "f", Enabled: true})
	if !errors.Is(err, ErrQuorumNotReached) {
		t.Errorf("expected ErrQuorumNotReached with both peers down, got: %v", err)
	}
}

func TestQuorum_FailedWrite_StillExistsLocally(t *testing.T) {
	m, leaderStore := setupClusterWithFollowers(t, 2)
	_, err := m.Upsert(context.Background(), flags.Flag{Key: "orphaned", Enabled: true})
	if !errors.Is(err, ErrQuorumNotReached) {
		t.Fatalf("expected ErrQuorumNotReached, got: %v", err)
	}
	got, getErr := leaderStore.Get(context.Background(), "orphaned")
	if getErr != nil {
		t.Fatalf("expected the failed write to still exist locally, got: %v", getErr)
	}
	if got.Key != "orphaned" {
		t.Errorf("got %+v, want key=orphaned", got)
	}
}

func TestQuorum_Delete_SameSemantics(t *testing.T) {
	m, _ := setupClusterWithFollowers(t, 0)
	if _, err := m.Upsert(context.Background(), flags.Flag{Key: "to_delete", Enabled: true}); err != nil {
		t.Fatalf("seed upsert failed: %v", err)
	}
	if _, err := m.Delete(context.Background(), "to_delete"); err != nil {
		t.Errorf("expected delete to succeed with all peers healthy, got: %v", err)
	}
}

func TestQuorum_Delete_FailsWithoutMajority(t *testing.T) {
	m, leaderStore := setupClusterWithFollowers(t, 2)
	if err := leaderStore.Put(context.Background(), flags.Flag{Key: "to_delete", Enabled: true}); err != nil {
		t.Fatalf("seed put failed: %v", err)
	}
	_, err := m.Delete(context.Background(), "to_delete")
	if !errors.Is(err, ErrQuorumNotReached) {
		t.Errorf("expected ErrQuorumNotReached for delete with both peers down, got: %v", err)
	}
}

func TestQuorum_SingleNodeCluster_AlwaysSucceeds(t *testing.T) {
	self := Node{ID: "solo"}
	c, err := NewCluster(self, []Node{self}, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}
	s := store.NewMemoryStore()
	a := audit.NewMemoryStore()
	w := service.NewMemoryFlagWriter(s, a)
	m := NewManager(c, w, NewClient())
	_, err = m.Upsert(context.Background(), flags.Flag{Key: "f", Enabled: true})
	if err != nil {
		t.Errorf("single-node cluster should always succeed, got: %v", err)
	}
}
