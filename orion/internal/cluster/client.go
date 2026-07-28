package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"orion/internal/flags"
)

// RPCClient is what Manager depends on for node-to-node communication.
// Kept as an interface — not the concrete *Client below — specifically
// so tests can inject a fake implementation instead of making real
// network calls to peer addresses that don't exist in a unit test.
type RPCClient interface {
	Replicate(ctx context.Context, peer Node, f flags.Flag) error
	ReplicateDelete(ctx context.Context, peer Node, key string) error
}

// Client is the real, HTTP-based RPCClient implementation used in
// production. Deliberately separate from sdk.Client: the SDK is
// fail-safe by design (falls back to a default on error), which is
// correct for external consumers but wrong for replication, where
// every failure must be visible and returned, not swallowed.
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{Timeout: 2 * time.Second},
	}
}

func (c *Client) Replicate(ctx context.Context, peer Node, f flags.Flag) error {
	body, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("cluster: encoding replicate request for %s: %w", peer.ID, err)
	}
	url := fmt.Sprintf("http://%s/internal/cluster/replicate", peer.Address)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cluster: building replicate request to %s: %w", peer.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cluster: replicate request to %s failed: %w", peer.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cluster: peer %s returned status %d for replicate", peer.ID, resp.StatusCode)
	}
	return nil
}

func (c *Client) ReplicateDelete(ctx context.Context, peer Node, key string) error {
	url := fmt.Sprintf("http://%s/internal/cluster/replicate/%s", peer.Address, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("cluster: building replicate-delete request to %s: %w", peer.ID, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cluster: replicate-delete request to %s failed: %w", peer.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cluster: peer %s returned status %d for replicate-delete", peer.ID, resp.StatusCode)
	}
	return nil
}
