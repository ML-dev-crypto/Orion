package main

import (
	"os"
	"strings"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		old, existed := os.LookupEnv(k)
		os.Setenv(k, v)
		t.Cleanup(func() {
			if existed {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	setEnv(t, map[string]string{
		"NODE_ID": "node2",
		"PORT":    "8082",
		"PEERS":   "node1=localhost:8081,node2=localhost:8082,node3=localhost:8083",
	})
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Cluster.NodeID != "node2" || cfg.Port != 8082 {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.Self.ID != "node2" || cfg.Self.Address != "localhost:8082" {
		t.Errorf("unexpected self: %+v", cfg.Self)
	}
	if len(cfg.Cluster.Peers) != 3 {
		t.Errorf("got %d peers, want 3 (Peers includes self)", len(cfg.Cluster.Peers))
	}
}

func TestLoadConfig_MissingNodeID(t *testing.T) {
	setEnv(t, map[string]string{
		"NODE_ID": "",
		"PORT":    "8081",
		"PEERS":   "node1=localhost:8081",
	})
	_, err := loadConfig()
	if err == nil {
		t.Error("expected error for missing NODE_ID, got nil")
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	setEnv(t, map[string]string{
		"NODE_ID": "node1",
		"PORT":    "not-a-number",
		"PEERS":   "node1=localhost:8081",
	})
	_, err := loadConfig()
	if err == nil {
		t.Error("expected error for non-numeric PORT, got nil")
	}
}

func TestLoadConfig_NodeIDNotInPeers(t *testing.T) {
	setEnv(t, map[string]string{
		"NODE_ID": "node4",
		"PORT":    "8081",
		"PEERS":   "node1=localhost:8081,node2=localhost:8082,node3=localhost:8083",
	})
	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected error when NODE_ID isn't in PEERS, got nil")
	}
	if !strings.Contains(err.Error(), "node4") {
		t.Errorf("error message should mention the offending NODE_ID, got: %v", err)
	}
}
