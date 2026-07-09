package sdk

import "time"

// Flag mirrors the server's JSON wire format. Deliberately NOT a
// reference to orion/internal/flags.Flag: Go's internal/ visibility
// rule means only code inside the orion module tree can import that
// package. This package is meant to be extractable into its own
// module/repo later — depending on internal/ would make that
// impossible without a rewrite at extraction time.
type Flag struct {
	Key            string          `json:"key"`
	Enabled        bool            `json:"enabled"`
	RolloutPct     int             `json:"rollout_pct"`
	Description    string          `json:"description,omitempty"`
	TargetingRules []TargetingRule `json:"targeting_rules,omitempty"`
	Schedule       *Schedule       `json:"schedule,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at,omitempty"`
}

type TargetingRule struct {
	Attribute string   `json:"attribute"`
	Operator  string   `json:"operator"`
	Value     string   `json:"value,omitempty"`
	Values    []string `json:"values,omitempty"`
}

type Schedule struct {
	StartAt *time.Time `json:"start_at,omitempty"`
	EndAt   *time.Time `json:"end_at,omitempty"`
}

type evalRequest struct {
	UserID     string            `json:"user_id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type evalResponse struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

type AuditAction string

const (
	AuditCreate AuditAction = "CREATE"
	AuditUpdate AuditAction = "UPDATE"
	AuditDelete AuditAction = "DELETE"
)

type AuditEntry struct {
	ID        int64       `json:"id"`
	FlagKey   string      `json:"flag_key"`
	Action    AuditAction `json:"action"`
	Before    *Flag       `json:"before,omitempty"`
	After     *Flag       `json:"after,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}
