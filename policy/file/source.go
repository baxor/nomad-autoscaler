package file

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"sync"

	hclog "github.com/hashicorp/go-hclog"
	multierror "github.com/hashicorp/go-multierror"
	fileHelper "github.com/hashicorp/nomad-autoscaler/helper/file"
	"github.com/hashicorp/nomad-autoscaler/helper/uuid"
	"github.com/hashicorp/nomad-autoscaler/policy"
)

// Ensure NomadSource satisfies the Source interface.
var _ policy.Source = (*Source)(nil)

type Source struct {
	config *policy.ConfigDefaults
	dir    string
	log    hclog.Logger

	// reloadChan is the channel that the agent sends to in order to trigger a
	// reload of all file policies.
	reloadChan chan bool

	// policyReloadChan is used internally to trigger a reload of individual
	// policies from disk.
	policyReloadChan chan bool

	// idMap stores a mapping between between the md5sum of the file path and
	// the associated policyID. This allows us to keep a consistent PolicyID in
	// the event of policy changes.
	idMap     map[[16]byte]policy.PolicyID
	idMapLock sync.RWMutex

	// policyMap maps our policyID to the file and policy which was decode from
	// the file. This is required with the current policy.Source interface
	// implementation, as the MonitorPolicy function only has access to the
	// policyID and not the underlying file path.
	policyMap     map[policy.PolicyID]*filePolicy
	policyMapLock sync.RWMutex
}

type filePolicy struct {
	file   string
	policy *policy.Policy
}

func NewFileSource(log hclog.Logger, cfg *policy.ConfigDefaults, dir string, reloadCh chan bool) policy.Source {
	return &Source{
		config:           cfg,
		dir:              dir,
		log:              log.Named("file_policy_source"),
		idMap:            make(map[[16]byte]policy.PolicyID),
		policyMap:        make(map[policy.PolicyID]*filePolicy),
		reloadChan:       reloadCh,
		policyReloadChan: make(chan bool),
	}
}

func (s *Source) MonitorIDs(ctx context.Context, resultCh chan<- policy.IDMessage, errCh chan<- error) {
	s.log.Debug("starting file policy source ID monitor")

	// Run the policyID identification method before entering the loop so we do
	// a first pass on the policies. Otherwise we wouldn't load any until a
	// reload is triggered.
	s.identifyPolicyIDs(resultCh, errCh)

	for {
		select {
		case <-ctx.Done():
			s.log.Trace("stopping file policy source ID monitor")
			return

		case <-s.reloadChan:
			s.log.Info("file policy source ID monitor received reload signal")

			// We are reloading all files within the directory so wipe our
			// current mapping data.
			s.idMapLock.Lock()
			s.idMap = make(map[[16]byte]policy.PolicyID)
			s.idMapLock.Unlock()

			s.identifyPolicyIDs(resultCh, errCh)

			// Tell the MonitorPolicy routines to reload their policy.
			s.policyReloadChan <- true
		}
	}
}

func (s *Source) MonitorPolicy(ctx context.Context, ID policy.PolicyID, resultCh chan<- policy.Policy, errCh chan<- error) {

	// Close channels when done with the monitoring loop.
	defer close(resultCh)
	defer close(errCh)

	log := s.log.With("policy_id", ID)
	log.Trace("starting file policy watcher")

	// There isn't a possibility that I can think of where this call wouldn't
	// be ok. Nevertheless check it to be safe before sending the policy to the
	// handler which starts the evaluation ticker.
	val, ok := s.policyMap[ID]
	if !ok {
		errCh <- fmt.Errorf("failed to get policy")
	} else {
		resultCh <- *val.policy
	}

	for {
		select {
		case <-ctx.Done():
			log.Trace("stopping file source ID monitor")
			return

		case <-s.policyReloadChan:
			newPolicy, err := s.retrievePolicy(ID)
			if err != nil {
				errCh <- fmt.Errorf("failed to get policy: %v", err)
				continue
			}

			// A non-nil policy indicates a change, therefore we send this to
			// the handler.
			if newPolicy != nil {
				resultCh <- *newPolicy
			}
		}
	}
}

func (s *Source) retrievePolicy(ID policy.PolicyID) (*policy.Policy, error) {

	val, ok := s.policyMap[ID]
	if !ok {
		return nil, errors.New("policy not found within internal store")
	}

	var newPolicy policy.Policy

	if err := decodeFile(val.file, &newPolicy); err != nil {
		return nil, fmt.Errorf("failed to decode file %s: %v", val.file, err)
	}
	newPolicy.ID = ID.String()

	// Check the new policy against the stored. If they are the same, and
	// therefore the policy has not changed indicate that to the caller.
	if md5Sum(newPolicy) == md5Sum(val) {
		return nil, nil
	}
	return &newPolicy, nil
}

func (s *Source) identifyPolicyIDs(resultCh chan<- policy.IDMessage, errCh chan<- error) {
	ids, err := s.handleDir()
	if err != nil {
		errCh <- err
	}

	if len(ids) > 0 {
		resultCh <- policy.IDMessage{IDs: ids, Source: policy.SourceNameFile}
	}
}

func (s *Source) handleDir() ([]policy.PolicyID, error) {

	files, err := fileHelper.GetFileListFromDir(s.dir, ".hcl", ".json")
	if err != nil {
		return nil, err
	}

	var (
		policyIDs []policy.PolicyID
		mErr      *multierror.Error
	)

	for _, file := range files {

		policyID := s.getFilePolicyID(file)

		var scalingPolicy policy.Policy

		// We have to decode the file to check whether the policy is enabled or
		// not.
		if err := decodeFile(file, &scalingPolicy); err != nil {
			return nil, fmt.Errorf("failed to decode file %s: %v", file, err)
		}

		if !scalingPolicy.Enabled {
			continue
		}

		scalingPolicy.ID = policyID.String()

		// We have had to decode the file, so store the information. This makes
		// the MonitorPolicy function simpler.
		s.policyMapLock.Lock()
		s.policyMap[policyID] = &filePolicy{file: file, policy: &scalingPolicy}
		s.policyMapLock.Unlock()

		policyIDs = append(policyIDs, policyID)
	}

	return policyIDs, mErr.ErrorOrNil()
}

func (s *Source) decodePolicyFromFile(file string) (*policy.Policy, error) {

	var filePolicy policy.Policy

	if err := decodeFile(file, &filePolicy); err != nil {
		return nil, fmt.Errorf("failed to decode file %s: %v", file, err)
	}

	if !filePolicy.Enabled {
		return nil, nil
	}
	return &filePolicy, nil
}

func (s *Source) getFilePolicyID(file string) policy.PolicyID {

	s.idMapLock.Lock()
	defer s.idMapLock.Unlock()

	md5Sum := md5Sum(file)

	policyID, ok := s.idMap[md5Sum]
	if !ok {
		policyID = policy.PolicyID(uuid.Generate())
		s.idMap[md5Sum] = policyID
	}

	return policyID
}

func md5Sum(i interface{}) [16]byte {
	return md5.Sum([]byte(fmt.Sprintf("%v", i)))
}
