package nomad

import (
	"testing"
	"time"

	"github.com/hashicorp/nomad-autoscaler/policy"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/api"
)

// TestNomadSource returns a default policy.Source that retrieves policies
// from Nomad.
//
// The Nomad client and the agent can be configured by passing a cb function.
func TestNomadSource(t *testing.T, cb func(*api.Config, *policy.ConfigDefaults)) *Source {
	nomadConfig := api.DefaultConfig()
	sourceConfig := &policy.ConfigDefaults{
		DefaultEvaluationInterval: 10 * time.Second,
	}

	if cb != nil {
		cb(nomadConfig, sourceConfig)
	}

	nomad, err := api.NewClient(nomadConfig)
	if err != nil {
		t.Fatal(err)
	}

	log := hclog.New(&hclog.LoggerOptions{
		Level: hclog.Trace,
	})
	return NewNomadSource(log, nomad, sourceConfig)
}
