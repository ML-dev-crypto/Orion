package flags

import (
	"hash/fnv"
	"time"
)

// Evaluate is the public entry point production code calls. It captures
// "now" exactly once and delegates to evaluateAt, so every check inside
// a single evaluation call agrees on the same instant.
func Evaluate(f Flag, req EvalRequest) bool {
	return evaluateAt(f, req, time.Now().UTC())
}

// evaluateAt is the real logic, with time injected as a parameter
// instead of read from the wall clock. This is what tests call
// directly — pass any fixed `now` you want (yesterday, tomorrow, a
// boundary second) and get a fully deterministic result, no flakiness,
// no waiting for real time to pass.
func evaluateAt(f Flag, req EvalRequest, now time.Time) bool {
	if !f.Enabled {
		return false
	}
	if !isWithinSchedule(f.Schedule, now) {
		return false
	}
	if !matchesAllRules(f.TargetingRules, req.Attributes) {
		return false
	}
	return inRollout(req.UserID, f.Key, f.RolloutPct)
}

// isWithinSchedule reports whether `now` falls inside the schedule
// window. nil means unrestricted. Taking now as a parameter (rather
// than calling time.Now() internally) is what makes this unit-testable
// without mocking or waiting on real time.
func isWithinSchedule(s *Schedule, now time.Time) bool {
	if s == nil {
		return true
	}
	if s.StartAt != nil && now.Before(*s.StartAt) {
		return false
	}
	if s.EndAt != nil && now.After(*s.EndAt) {
		return false
	}
	return true
}

func matchesAllRules(rules []TargetingRule, attrs map[string]string) bool {
	for _, rule := range rules {
		if !matchesRule(rule, attrs) {
			return false
		}
	}
	return true
}

func matchesRule(rule TargetingRule, attrs map[string]string) bool {
	actual, present := attrs[rule.Attribute]
	if !present {
		return false
	}
	switch rule.Operator {
	case "equals":
		return actual == rule.Value
	case "in":
		for _, v := range rule.Values {
			if actual == v {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func inRollout(userID, flagKey string, pct int) bool {
	if pct >= 100 {
		return true
	}
	if pct <= 0 {
		return false
	}
	h := fnv.New32a()
	h.Write([]byte(userID + ":" + flagKey))
	bucket := h.Sum32() % 100
	return int(bucket) < pct
}