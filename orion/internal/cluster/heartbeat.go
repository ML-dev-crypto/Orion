package cluster

import (
	"context"
	"log"
	"time"
)

// DefaultHeartbeatInterval and DefaultLeaderTimeout are deliberately
// slower than a real production Raft implementation would use. This
// project's value has consistently come from being able to WATCH it
// work — kill a process, see a log line, see a curl response change —
// so these are picked to be observable by a human, not optimized for
// latency.
const (
	DefaultHeartbeatInterval = 1 * time.Second
	DefaultLeaderTimeout     = 3 * time.Second
)

// RunHeartbeatLoop sends a heartbeat to every peer once per interval,
// for as long as ctx is not cancelled. Only the leader should run
// this. Blocks until ctx is cancelled, so callers run it in its own
// goroutine and use ctx cancellation as the shutdown signal.
func RunHeartbeatLoop(ctx context.Context, c *Cluster, rpc RPCClient, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendHeartbeats(ctx, c, rpc)
		}
	}
}

// sendHeartbeats fires a heartbeat at every peer in parallel and does
// NOT wait for them to finish before returning — unlike Manager's
// replicate(), which deliberately does wait. Heartbeats have no
// corresponding "did this tick succeed" requirement; waiting here
// would let one slow peer create backpressure on the ticker itself.
func sendHeartbeats(ctx context.Context, c *Cluster, rpc RPCClient) {
	for _, peer := range c.Peers {
		peer := peer
		go func() {
			hbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := rpc.Heartbeat(hbCtx, peer); err != nil {
				log.Printf("cluster: heartbeat to %s failed: %v", peer.ID, err)
			}
		}()
	}
}
