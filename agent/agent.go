package agent

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-autoscaler/agent/config"
	nomadHelper "github.com/hashicorp/nomad-autoscaler/helper/nomad"
	"github.com/hashicorp/nomad-autoscaler/plugins"
	apmpkg "github.com/hashicorp/nomad-autoscaler/plugins/apm"
	"github.com/hashicorp/nomad-autoscaler/plugins/manager"
	strategypkg "github.com/hashicorp/nomad-autoscaler/plugins/strategy"
	targetpkg "github.com/hashicorp/nomad-autoscaler/plugins/target"
	"github.com/hashicorp/nomad-autoscaler/policy"
	filePolicy "github.com/hashicorp/nomad-autoscaler/policy/file"
	"github.com/hashicorp/nomad/api"
)

type Agent struct {
	logger        hclog.Logger
	config        *config.Agent
	nomadClient   *api.Client
	pluginManager *manager.PluginManager
	policyManager *policy.Manager
	healthServer  *healthServer
}

func NewAgent(c *config.Agent, logger hclog.Logger) *Agent {
	return &Agent{
		logger: logger,
		config: c,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	defer a.stop()

	// Generate the Nomad client.
	if err := a.generateNomadClient(); err != nil {
		return err
	}

	// launch plugins
	if err := a.setupPlugins(); err != nil {
		return fmt.Errorf("failed to setup plugins: %v", err)
	}

	// Setup and start the HTTP health server.
	healthServer, err := newHealthServer(a.config.HTTP, a.logger)
	if err != nil {
		return fmt.Errorf("failed to setup HTTP getHealth server: %v", err)
	}

	a.healthServer = healthServer
	go a.healthServer.run()

	policyEvalCh := a.setupPolicyManager()
	go a.policyManager.Run(ctx, policyEvalCh)

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("context closed, shutting down")
			return nil
		case policyEval := <-policyEvalCh:
			if policyEval == nil || policyEval.Policy == nil {
				continue
			}

			actions := []strategypkg.Action{}
			for _, c := range policyEval.Policy.Checks {
				actions = append(actions, a.handlePolicyCheck(policyEval.Policy, c)...)
			}
			// TODO: reconcile actions and execute them
		}
	}
}

func (a *Agent) setupPolicyManager() chan *policy.Evaluation {
	sourceConfig := &policy.ConfigDefaults{
		DefaultCooldown:           a.config.Policy.DefaultCooldown,
		DefaultEvaluationInterval: a.config.DefaultEvaluationInterval,
	}

	sources := map[policy.SourceName]policy.Source{
		//policy.SourceNameNomad: nomadpolicy.NewNomadSource(a.logger, a.nomadClient, sourceConfig),
		policy.SourceNameFile: filePolicy.NewFileSource(a.logger, sourceConfig, "/Users/jrasell/go/src/github.com/hashicorp/nomad-autoscaler/policy/file/test-fixtures", nil),
	}
	a.policyManager = policy.NewManager(a.logger, sources, a.pluginManager)

	return make(chan *policy.Evaluation, 10)
}

func (a *Agent) stop() {
	// Stop the health server.
	if a.healthServer != nil {
		a.healthServer.stop()
	}

	// Kill all the plugins.
	if a.pluginManager != nil {
		a.pluginManager.KillPlugins()
	}
}

// generateNomadClient takes the internal Nomad configuration, translates and
// merges it into a Nomad API config object and creates a client.
func (a *Agent) generateNomadClient() error {

	// Use the Nomad API default config which gets populated by defaults and
	// also checks for environment variables.
	cfg := api.DefaultConfig()

	// Merge our top level configuration options in.
	if a.config.Nomad.Address != "" {
		cfg.Address = a.config.Nomad.Address
	}
	if a.config.Nomad.Region != "" {
		cfg.Region = a.config.Nomad.Region
	}
	if a.config.Nomad.Namespace != "" {
		cfg.Namespace = a.config.Nomad.Namespace
	}
	if a.config.Nomad.Token != "" {
		cfg.SecretID = a.config.Nomad.Token
	}

	// Merge HTTP auth.
	if a.config.Nomad.HTTPAuth != "" {
		cfg.HttpAuth = nomadHelper.HTTPAuthFromString(a.config.Nomad.HTTPAuth)
	}

	// Merge TLS.
	if a.config.Nomad.CACert != "" {
		cfg.TLSConfig.CACert = a.config.Nomad.CACert
	}
	if a.config.Nomad.CAPath != "" {
		cfg.TLSConfig.CAPath = a.config.Nomad.CAPath
	}
	if a.config.Nomad.ClientCert != "" {
		cfg.TLSConfig.ClientCert = a.config.Nomad.ClientCert
	}
	if a.config.Nomad.ClientKey != "" {
		cfg.TLSConfig.ClientKey = a.config.Nomad.ClientKey
	}
	if a.config.Nomad.TLSServerName != "" {
		cfg.TLSConfig.TLSServerName = a.config.Nomad.TLSServerName
	}
	if a.config.Nomad.SkipVerify {
		cfg.TLSConfig.Insecure = a.config.Nomad.SkipVerify
	}

	// Generate the Nomad client.
	client, err := api.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to instantiate Nomad client: %v", err)
	}
	a.nomadClient = client

	return nil
}

func (a *Agent) handlePolicyCheck(p *policy.Policy, c *policy.Check) []strategypkg.Action {
	logger := a.logger.With(
		"policy_id", p.ID,
		"source", c.Source,
		"target", p.Target.Name,
		"strategy", c.Strategy.Name,
	)

	logger.Info("received policy for evaluation")

	var targetInst targetpkg.Target
	var apmInst apmpkg.APM
	var strategyInst strategypkg.Strategy

	// dispense plugins
	targetPlugin, err := a.pluginManager.Dispense(p.Target.Name, plugins.PluginTypeTarget)
	if err != nil {
		logger.Error("target plugin not initialized", "error", err, "plugin", p.Target.Name)
		return []strategypkg.Action{}
	}
	targetInst = targetPlugin.Plugin().(targetpkg.Target)

	apmPlugin, err := a.pluginManager.Dispense(c.Source, plugins.PluginTypeAPM)
	if err != nil {
		logger.Error("apm plugin not initialized", "error", err, "plugin", c.Source)
		return []strategypkg.Action{}
	}
	apmInst = apmPlugin.Plugin().(apmpkg.APM)

	strategyPlugin, err := a.pluginManager.Dispense(c.Strategy.Name, plugins.PluginTypeStrategy)
	if err != nil {
		logger.Error("strategy plugin not initialized", "error", err, "plugin", c.Strategy.Name)
		return []strategypkg.Action{}
	}
	strategyInst = strategyPlugin.Plugin().(strategypkg.Strategy)

	// fetch target count
	logger.Info("fetching current count")
	currentStatus, err := targetInst.Status(p.Target.Config)
	if err != nil {
		logger.Error("failed to fetch current count", "error", err)
		return []strategypkg.Action{}
	}
	if !currentStatus.Ready {
		logger.Info("target not ready")
		return []strategypkg.Action{}
	}

	// query policy's APM
	logger.Info("querying APM")
	value, err := apmInst.Query(c.Query)
	if err != nil {
		logger.Error("failed to query APM", "error", err)
		return []strategypkg.Action{}
	}

	// calculate new count using policy's Strategy
	logger.Info("calculating new count")
	req := strategypkg.RunRequest{
		PolicyID: p.ID,
		Count:    currentStatus.Count,
		Metric:   value,
		Config:   c.Strategy.Config,
	}
	results, err := strategyInst.Run(req)
	if err != nil {
		logger.Error("failed to calculate strategy", "error", err)
		return []strategypkg.Action{}
	}

	if len(results.Actions) == 0 {
		// Make sure we are currently within [min, max] limits even if there's
		// no action to execute
		var minMaxAction *strategypkg.Action

		if currentStatus.Count < p.Min {
			minMaxAction = &strategypkg.Action{
				Count:  p.Min,
				Reason: fmt.Sprintf("current count (%d) below limit (%d)", currentStatus.Count, p.Min),
			}
		} else if currentStatus.Count > p.Max {
			minMaxAction = &strategypkg.Action{
				Count:  p.Max,
				Reason: fmt.Sprintf("current count (%d) above limit (%d)", currentStatus.Count, p.Max),
			}
		}

		if minMaxAction != nil {
			results.Actions = append(results.Actions, *minMaxAction)
		} else {
			logger.Info("nothing to do")
			return []strategypkg.Action{}
		}
	}

	return results.Actions

	// TODO: lazily commented out for now
	//	// scale target
	//	for _, action := range results.Actions {
	//		actionLogger := logger.With("target_config", p.Target.Config)
	//
	//		// Make sure returned action has sane defaults instead of relying on
	//		// plugins doing this.
	//		action.Canonicalize()
	//
	//		// Make sure new count value is within [min, max] limits
	//		action.CapCount(p.Min, p.Max)
	//
	//		// If the policy is configured with dry-run:true then we set the
	//		// action count to nil so its no-nop. This allows us to still
	//		// submit the job, but not alter its state.
	//		if val, ok := p.Target.Config["dry-run"]; ok && val == "true" {
	//			actionLogger.Info("scaling dry-run is enabled, using no-op task group count")
	//			action.SetDryRun()
	//		}
	//
	//		if action.Count == strategypkg.MetaValueDryRunCount {
	//			actionLogger.Info("registering scaling event",
	//				"count", currentStatus.Count, "reason", action.Reason, "meta", action.Meta)
	//		} else {
	//			// Skip action if count doesn't change.
	//			if currentStatus.Count == action.Count {
	//				actionLogger.Info("nothing to do", "from", currentStatus.Count, "to", action.Count)
	//				continue
	//			}
	//
	//			actionLogger.Info("scaling target",
	//				"from", currentStatus.Count, "to", action.Count,
	//				"reason", action.Reason, "meta", action.Meta)
	//		}
	//
	//		if err = targetInst.Scale(action, p.Target.Config); err != nil {
	//			actionLogger.Error("failed to scale target", "error", err)
	//			continue
	//		}
	//		actionLogger.Info("successfully submitted scaling action to target",
	//			"desired_count", action.Count)
	//
	//		// Enforce the cooldown after a successful scaling event.
	//		a.policyManager.EnforceCooldown(p.ID, p.Cooldown)
	//	}
}
