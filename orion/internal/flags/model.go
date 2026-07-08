package flags

import "time"

// Flag represents a single feature flag.
type Flag struct {
	Key            string          `json:"key"`
	Enabled        bool            `json:"enabled"`
	RolloutPct     int             `json:"rollout_pct"`
	Description    string          `json:"description"`
	TargetingRules []TargetingRule `json:"targeting_rules,omitempty"`
	Schedule       *Schedule       `json:"schedule,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TargetingRule is one condition on a user attribute, e.g.
// {"attribute": "country", "operator": "equals", "value": "India"}
// or {"attribute": "plan", "operator": "in", "values": ["premium","enterprise"]}
type TargetingRule struct {
	Attribute string   `json:"attribute"`
	Operator  string   `json:"operator"` // "equals" | "in"
	Value     string   `json:"value,omitempty"`
	Values    []string `json:"values,omitempty"`
}

// Schedule restricts WHEN an enabled flag is actually active. Both
// bounds are optional and independent: StartAt alone means "active
// from this point on, no end"; EndAt alone means "active until this
// point, no defined start." A nil *Schedule on the Flag means no
// restriction at all.
type Schedule struct {
	StartAt *time.Time `json:"start_at,omitempty"`
	EndAt   *time.Time `json:"end_at,omitempty"`
}

// EvalRequest is what a client sends when asking "is this flag on for
// this user?" Attributes carries whatever the caller knows about the
// user (country, plan, etc.) so targeting rules can be checked against it.
type EvalRequest struct {
	UserID     string            `json:"user_id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// EvalResponse is what Orion sends back.
type EvalResponse struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}