package file

import (
	"time"

	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/hashicorp/nomad-autoscaler/policy"
)

func decodeFile(file string, p *policy.ClusterScalingPolicy) error {

	if err := hclsimple.DecodeFile(file, nil, p); err != nil {
		return err
	}

	if p.Policy.CooldownHCL != "" {
		d, err := time.ParseDuration(p.Policy.CooldownHCL)
		if err != nil {
			return err
		}
		p.Policy.Cooldown = d
	}

	if p.Policy.EvaluationIntervalHCL != "" {
		d, err := time.ParseDuration(p.Policy.EvaluationIntervalHCL)
		if err != nil {
			return err
		}
		p.Policy.EvaluationInterval = d
	}

	return nil
}
