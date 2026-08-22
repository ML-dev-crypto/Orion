// cmd/node is the memory-backed entry point for running a local
// multi-node Orion cluster during development.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"orion/internal/api"
	"orion/internal/audit"
	"orion/internal/cluster"
	"orion/internal/service"
	"orion/internal/store"
)

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
	rpcClient := cluster.NewClient()
	manager := cluster.NewManager(c, writer, rpcClient)
	rpcHandler := cluster.NewRPCHandler(backend, c)

	h := api.NewHandler(backend, auditBackend, manager)
	mux := h.Routes()
	rpcHandler.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Orion node %q listening on %s (leader=%s, is_leader=%v, peers=%d)",
		cfg.Cluster.NodeID, addr, leaderID, c.IsLeader(), len(c.Peers))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	if c.IsLeader() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("node %q starting heartbeat loop (interval=%s)", cfg.Cluster.NodeID, cluster.DefaultHeartbeatInterval)
			cluster.RunHeartbeatLoop(ctx, c, rpcClient, cluster.DefaultHeartbeatInterval)
			log.Printf("node %q heartbeat loop stopped", cfg.Cluster.NodeID)
		}()
	}

	srv := &http.Server{Addr: addr, Handler: mux}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("node %q received shutdown signal, stopping...", cfg.Cluster.NodeID)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}

	wg.Wait()
	log.Printf("node %q stopped cleanly", cfg.Cluster.NodeID)
}

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
