package file

import (
	"testing"
	"time"

	"github.com/hashicorp/nomad-autoscaler/policy"
	"github.com/stretchr/testify/assert"
)

func Test_decodeFile(t *testing.T) {
	testCases := []struct {
		inputFile            string
		inputPolicy          *policy.ClusterScalingPolicy
		expectedOutputPolicy *policy.ClusterScalingPolicy
		expectedOutputError  error
		name                 string
	}{
		{
			inputFile:   "./test-fixtures/full-cluster-policy.hcl",
			inputPolicy: &policy.ClusterScalingPolicy{},
			expectedOutputPolicy: &policy.ClusterScalingPolicy{
				ID:      "",
				Enabled: true,
				Min:     10,
				Max:     100,
				Policy: &policy.ClusterScalingPolicyDoc{
					Cooldown:              10 * time.Minute,
					CooldownHCL:           "10m",
					EvaluationInterval:    1 * time.Minute,
					EvaluationIntervalHCL: "1m",
					Checks: []*policy.ClusterScalingCheck{
						{
							Name:      "cpu_nomad",
							APMSource: "nomad_apm",
							APMQuery:  "cpu_high-memory",
							Strategy: &policy.Strategy{
								Name: "target-value",
								Config: map[string]string{
									"target": "80",
								},
							},
						},
						{
							Name:      "memory_prom",
							APMSource: "prometheus",
							APMQuery:  "nomad_client_allocated_memory * 100 / (nomad_client_allocated_memory + nomad_client_unallocated_memory)",
							Strategy: &policy.Strategy{
								Name: "target-value",
								Config: map[string]string{
									"target": "80",
								},
							},
						},
					},
					Target: &policy.Target{
						Name: "aws-asg",
						Config: map[string]string{
							"asg_name":       "my-target-asg",
							"class":          "high-memory",
							"drain_deadline": "15m",
						},
					},
				},
			},
			expectedOutputError: nil,
			name:                "full parsable cluster scaling policy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualError := decodeFile(tc.inputFile, tc.inputPolicy)
			assert.Equal(t, tc.expectedOutputPolicy, tc.inputPolicy, tc.name)
			assert.Equal(t, tc.expectedOutputError, actualError, tc.name)
		})
	}
}
