package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"orion/internal/audit"
	"orion/internal/cluster"
	"orion/internal/service"
	"orion/internal/store"
)

// newTestHandler wires up a Handler with in-memory backends and a
// single-node cluster where self is the leader — the minimum needed
// to exercise the status endpoint without any network.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()

	self := cluster.Node{ID: "node-1", Address: "localhost:8080"}
	c, err := cluster.NewCluster(self, []cluster.Node{self}, self.ID)
	if err != nil {
		t.Fatalf("NewCluster failed: %v", err)
	}

	s := store.NewMemoryStore()
	a := audit.NewMemoryStore()
	w := service.NewMemoryFlagWriter(s, a)
	rpc := cluster.NewClient()
	mgr := cluster.NewManager(c, w, rpc)

	return NewHandler(s, a, mgr)
}

func TestClusterStatus_ReturnsAllFields(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/cluster/status", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var status map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	required := []string{"self_id", "leader_id", "is_leader", "leader_reachable", "peers"}
	for _, field := range required {
		if _, ok := status[field]; !ok {
			t.Errorf("response missing field %q", field)
		}
	}

	if status["self_id"] != "node-1" {
		t.Errorf("self_id = %v, want node-1", status["self_id"])
	}
	if status["leader_id"] != "node-1" {
		t.Errorf("leader_id = %v, want node-1", status["leader_id"])
	}
	if status["is_leader"] != true {
		t.Errorf("is_leader = %v, want true", status["is_leader"])
	}
	if status["leader_reachable"] != true {
		t.Errorf("leader_reachable = %v, want true", status["leader_reachable"])
	}
}
