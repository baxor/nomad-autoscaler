package policy

import (
	"time"

	"github.com/hashicorp/nomad-autoscaler/plugins/target"
)

type Policy struct {
	ID                 string
	Min                int64
	Max                int64
	Source             string
	Query              string
	Enabled            bool
	Cooldown           time.Duration
	EvaluationInterval time.Duration
	Target             *Target
	Strategy           *Strategy
}

type Evaluation struct {
	Policy       *Policy
	TargetStatus *target.Status
}

type ClusterScalingPolicy struct {
	ID      string
	Enabled bool                     `hcl:"enabled,optional"`
	Min     int64                    `hcl:"min,optional"`
	Max     int64                    `hcl:"max"`
	Policy  *ClusterScalingPolicyDoc `hcl:"policy,block"`
}

type ClusterScalingPolicyDoc struct {
	Cooldown              time.Duration
	CooldownHCL           string `hcl:"cooldown,optional"`
	EvaluationInterval    time.Duration
	EvaluationIntervalHCL string                 `hcl:"evaluation_interval,optional"`
	Checks                []*ClusterScalingCheck `hcl:"check,block"`
	Target                *Target                `hcl:"target,block"`
}

type ClusterScalingCheck struct {
	Name      string    `hcl:"name,label"`
	APMSource string    `hcl:"source"`
	APMQuery  string    `hcl:"query"`
	Strategy  *Strategy `hcl:"strategy,block"`
}

type Strategy struct {
	Name   string            `hcl:"name"`
	Config map[string]string `hcl:"config,optional"`
}

type Target struct {
	Name   string            `hcl:"name"`
	Config map[string]string `hcl:"config,optional"`
}
