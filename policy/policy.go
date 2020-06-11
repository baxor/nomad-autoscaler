package policy

import (
	"time"

	"github.com/hashicorp/nomad-autoscaler/plugins/target"
)

type Policy struct {
	ID                 string
	Min                int64
	Max                int64
	Enabled            bool
	Cooldown           time.Duration
	EvaluationInterval time.Duration
	Checks             []*Check
	Target             *Target
}

type Check struct {
	Name     string    `hcl:"name,label"`
	Source   string    `hcl:"source"`
	Query    string    `hcl:"query"`
	Strategy *Strategy `hcl:"strategy,block"`
}

type Strategy struct {
	Name   string            `hcl:"name"`
	Config map[string]string `hcl:"config,optional"`
}

type Target struct {
	Name   string            `hcl:"name"`
	Config map[string]string `hcl:"config,optional"`
}

type Evaluation struct {
	Policy       *Policy
	TargetStatus *target.Status
}

// FileDecodePolicy
type FileDecodePolicy struct {
	ID      string
	Enabled bool                 `hcl:"enabled,optional"`
	Min     int64                `hcl:"min,optional"`
	Max     int64                `hcl:"max"`
	Doc     *FileDecodePolicyDoc `hcl:"policy,block"`
}

// FileDecodePolicyDoc
type FileDecodePolicyDoc struct {
	Cooldown              time.Duration
	CooldownHCL           string `hcl:"cooldown,optional"`
	EvaluationInterval    time.Duration
	EvaluationIntervalHCL string   `hcl:"evaluation_interval,optional"`
	Checks                []*Check `hcl:"check,block"`
	Target                *Target  `hcl:"target,block"`
}

func (fpd *FileDecodePolicy) Translate(p *Policy) {
	p.ID = fpd.ID
	p.Min = fpd.Min
	p.Max = fpd.Max
	p.Enabled = fpd.Enabled
	p.Cooldown = fpd.Doc.Cooldown
	p.EvaluationInterval = fpd.Doc.EvaluationInterval
	p.Checks = fpd.Doc.Checks
	p.Target = fpd.Doc.Target
}
