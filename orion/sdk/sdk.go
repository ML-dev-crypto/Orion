package sdk

import (
	"context"
	"log"
)

// IsEnabled answers "is this flag on for this user?" and is fail-safe:
// ANY error — network failure, timeout, Orion being down, the flag
// not existing — returns defaultValue instead of propagating an error.
// The underlying error is logged, not swallowed silently.
func (c *Client) IsEnabled(ctx context.Context, key, userID string, attrs map[string]string, defaultValue bool) bool {
	enabled, err := c.Evaluate(ctx, key, userID, attrs)
	if err != nil {
		log.Printf("orion sdk: IsEnabled(%q) failed, falling back to default=%v: %v", key, defaultValue, err)
		return defaultValue
	}
	return enabled
}

// Evaluate is the non-fail-safe variant of IsEnabled, for callers who
// want to handle the error themselves.
func (c *Client) Evaluate(ctx context.Context, key, userID string, attrs map[string]string) (bool, error) {
	req := evalRequest{UserID: userID, Attributes: attrs}
	var resp evalResponse
	if err := c.do(ctx, "POST", "/flags/"+key+"/evaluate", req, &resp); err != nil {
		return false, err
	}
	return resp.Enabled, nil
}

// Get fetches a single flag by key. Returns ErrFlagNotFound if it
// doesn't exist.
func (c *Client) Get(ctx context.Context, key string) (Flag, error) {
	var f Flag
	err := c.do(ctx, "GET", "/flags/"+key, nil, &f)
	return f, err
}

// List returns every flag.
func (c *Client) List(ctx context.Context) ([]Flag, error) {
	var flags []Flag
	err := c.do(ctx, "GET", "/flags", nil, &flags)
	return flags, err
}

// Put creates or updates a flag and returns the saved state.
func (c *Client) Put(ctx context.Context, f Flag) (Flag, error) {
	var saved Flag
	err := c.do(ctx, "POST", "/flags", f, &saved)
	return saved, err
}

// Delete removes a flag by key.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.do(ctx, "DELETE", "/flags/"+key, nil, nil)
}

// Audit returns the change history for a flag, most recent first.
func (c *Client) Audit(ctx context.Context, key string) ([]AuditEntry, error) {
	var entries []AuditEntry
	err := c.do(ctx, "GET", "/flags/"+key+"/audit", nil, &entries)
	return entries, err
}
