// cmd/node is the memory-backed entry point for running a local
// multi-node Orion cluster during development — separate from
// cmd/server, which stays the Postgres-backed single-node production
// path and is untouched by any of this.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"orion/internal/api"
	"orion/internal/audit"
	"orion/internal/cluster"
	"orion/internal/service"
	"orion/internal/store"
)

// nodeConfig bundles cluster.Config (NodeID + Peers, what the cluster
// package actually needs) together with Port and the resolved Self
// node — both of which are cmd/node's own concerns, not the cluster
// package's.
type nodeConfig struct {
	Cluster cluster.Config
	Self    cluster.Node
	Port    int
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	leaderID := cluster.BootstrapLeader(cfg.Cluster.Peers)
	c, err := cluster.NewCluster(cfg.Self, cfg.Cluster.Peers, leaderID)
	if err != nil {
		log.Fatal(err)
	}

	backend := store.NewMemoryStore()
	auditBackend := audit.NewMemoryStore()
	writer := service.NewMemoryFlagWriter(backend, auditBackend)
	manager := cluster.NewManager(c, writer)

	h := api.NewHandler(backend, auditBackend, manager)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Orion node %q listening on %s (leader=%s, is_leader=%v, peers=%d)",
		cfg.Cluster.NodeID, addr, leaderID, c.IsLeader(), len(c.Peers))

	if err := http.ListenAndServe(addr, h.Routes()); err != nil {
		log.Fatal(err)
	}
}

// loadConfig reads NODE_ID, PORT, and PEERS from the environment and
// validates them as a single unit, including that NODE_ID actually
// appears somewhere in PEERS. os.Getenv only ever happens here —
// never inside the cluster package.
func loadConfig() (nodeConfig, error) {
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		return nodeConfig{}, fmt.Errorf("NODE_ID environment variable is required")
	}

	portStr := os.Getenv("PORT")
	if portStr == "" {
		return nodeConfig{}, fmt.Errorf("PORT environment variable is required")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nodeConfig{}, fmt.Errorf("PORT must be a number, got %q: %w", portStr, err)
	}

	peersStr := os.Getenv("PEERS")
	if peersStr == "" {
		return nodeConfig{}, fmt.Errorf("PEERS environment variable is required")
	}
	peers, err := cluster.ParsePeers(peersStr)
	if err != nil {
		return nodeConfig{}, err
	}

	var self cluster.Node
	found := false
	for _, n := range peers {
		if n.ID == nodeID {
			self = n
			found = true
			break
		}
	}
	if !found {
		return nodeConfig{}, fmt.Errorf(
			"NODE_ID=%q not found in PEERS list %q — PEERS must include an entry for this node itself",
			nodeID, peersStr)
	}

	return nodeConfig{
		Cluster: cluster.Config{NodeID: nodeID, Peers: peers},
		Self:    self,
		Port:    port,
	}, nil
}
