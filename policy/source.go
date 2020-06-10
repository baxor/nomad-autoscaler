package policy

import (
	"context"
	"time"
)

const (
	DefaultEvaluationInterval = 10 * time.Second
)

// ConfigDefaults holds default configuration for unspecified values.
type ConfigDefaults struct {
	DefaultEvaluationInterval time.Duration
	DefaultCooldown           time.Duration
}

// Source is the interface that must be implemented by backends which
// provide the canonical source for scaling policies.
type Source interface {
	MonitorIDs(ctx context.Context, resultCh chan<- []PolicyID, errCh chan<- error)
	MonitorPolicy(ctx context.Context, ID PolicyID, resultCh chan<- Policy, errCh chan<- error)
}

type PolicyID string
