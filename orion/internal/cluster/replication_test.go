package cluster

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orion/internal/audit"
	"orion/internal/flags"
	"orion/internal/service"
	"orion/internal/store"
)

func newFollowerServer(t *testing.T) (*httptest.Server, store.Storage) {
	t.Helper()
	s := store.NewMemoryStore()
	h := NewRPCHandler(s)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, s
}

func addressOf(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestReplication_LeaderWriteReachesFollowers(t *testing.T) {
	follower1, follower1Store := newFollowerServer(t)
	follower2, follower2Store := newFollowerServer(t)

	self := Node{ID: "leader", Address: "leader:9999"}
	peers := []Node{
		self,
		{ID: "follower1", Address: addressOf(follower1)},
		{ID: "follower2", Address: addressOf(follower2)},
	}
	c, err := NewCluster(self, peers, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	leaderStore := store.NewMemoryStore()
	leaderAudit := audit.NewMemoryStore()
	writer := service.NewMemoryFlagWriter(leaderStore, leaderAudit)
	rpc := NewClient()
	m := NewManager(c, writer, rpc)

	f := flags.Flag{Key: "replicated_flag", Enabled: true, RolloutPct: 100}
	result, err := m.Upsert(context.Background(), f)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	got1, err := follower1Store.Get(context.Background(), "replicated_flag")
	if err != nil {
		t.Fatalf("follower1 does not have the replicated flag: %v", err)
	}
	if got1.Key != result.Flag.Key || got1.Enabled != result.Flag.Enabled || got1.RolloutPct != result.Flag.RolloutPct {
		t.Errorf("follower1 has %+v, want it to match leader's %+v", got1, result.Flag)
	}

	got2, err := follower2Store.Get(context.Background(), "replicated_flag")
	if err != nil {
		t.Fatalf("follower2 does not have the replicated flag: %v", err)
	}
	if got2.Key != result.Flag.Key {
		t.Errorf("follower2 has %+v, want key %q", got2, result.Flag.Key)
	}
}

func TestReplication_DeleteReachesFollowers(t *testing.T) {
	follower, followerStore := newFollowerServer(t)

	self := Node{ID: "leader", Address: "leader:9999"}
	peers := []Node{self, {ID: "follower", Address: addressOf(follower)}}
	c, err := NewCluster(self, peers, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	leaderStore := store.NewMemoryStore()
	leaderAudit := audit.NewMemoryStore()
	writer := service.NewMemoryFlagWriter(leaderStore, leaderAudit)
	rpc := NewClient()
	m := NewManager(c, writer, rpc)

	if _, err := m.Upsert(context.Background(), flags.Flag{Key: "to_delete", Enabled: true}); err != nil {
		t.Fatalf("seed upsert failed: %v", err)
	}
	if _, err := followerStore.Get(context.Background(), "to_delete"); err != nil {
		t.Fatalf("seed didn't replicate: %v", err)
	}

	if _, err := m.Delete(context.Background(), "to_delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = followerStore.Get(context.Background(), "to_delete")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected follower to no longer have the flag after replicated delete, got err=%v", err)
	}
}

func TestReplication_OneFollowerDownDoesNotFailTheWrite(t *testing.T) {
	healthyFollower, healthyStore := newFollowerServer(t)

	self := Node{ID: "leader", Address: "leader:9999"}
	peers := []Node{
		self,
		{ID: "healthy", Address: addressOf(healthyFollower)},
		{ID: "dead", Address: "127.0.0.1:1"},
	}
	c, err := NewCluster(self, peers, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	leaderStore := store.NewMemoryStore()
	leaderAudit := audit.NewMemoryStore()
	writer := service.NewMemoryFlagWriter(leaderStore, leaderAudit)
	rpc := NewClient()
	m := NewManager(c, writer, rpc)

	_, err = m.Upsert(context.Background(), flags.Flag{Key: "resilient", Enabled: true})
	if err != nil {
		t.Fatalf("Upsert should succeed even with one peer down, got error: %v", err)
	}
	if _, err := healthyStore.Get(context.Background(), "resilient"); err != nil {
		t.Errorf("healthy follower should still have received the write: %v", err)
	}
}
