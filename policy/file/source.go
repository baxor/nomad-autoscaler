package file

import (
	"context"
	"crypto/md5"
	"fmt"
	"sync"

	hclog "github.com/hashicorp/go-hclog"
	multierror "github.com/hashicorp/go-multierror"
	fileHelper "github.com/hashicorp/nomad-autoscaler/helper/file"
	"github.com/hashicorp/nomad-autoscaler/helper/uuid"
	"github.com/hashicorp/nomad-autoscaler/policy"
)

// Ensure NomadSource satisfies the Source interface.
//var _ policy.Source = (*Source)(nil)

type Source struct {
	config *policy.ConfigDefaults
	dir    string
	log    hclog.Logger

	reloadChan       chan bool
	policyReloadChan chan bool

	idMap     map[[16]byte]policy.PolicyID
	idMapLock sync.RWMutex

	policyMap     map[policy.PolicyID]*filePolicy
	policyMapLock sync.RWMutex
}

type filePolicy struct {
	file   string
	policy *policy.ClusterScalingPolicy
}

func NewFileSource(log hclog.Logger, cfg *policy.ConfigDefaults, dir string, reloadCh chan bool) *Source {
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

func (s *Source) MonitorIDs(ctx context.Context, resultCh chan<- []policy.PolicyID, errCh chan<- error) {
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
			s.log.Info("received reload signal")

			// We are reloading all files within the directory so wipe our
			// current data.
			s.idMapLock.Lock()
			s.idMap = make(map[[16]byte]policy.PolicyID)
			s.idMapLock.Unlock()

			s.policyMapLock.Lock()
			s.policyMap = make(map[policy.PolicyID]*filePolicy)
			s.policyMapLock.Unlock()

			s.identifyPolicyIDs(resultCh, errCh)
			s.policyReloadChan <- true
		}
	}
}

func (s *Source) MonitorPolicy(ctx context.Context, ID policy.PolicyID, resultCh chan<- interface{}, errCh chan<- error) {
	log := s.log.With("policy_id", ID)

	// Close channels when done with the monitoring loop.
	defer close(resultCh)
	defer close(errCh)

	log.Trace("starting file policy watcher")

	for {
		select {
		case <-ctx.Done():
			log.Trace("stopping file source ID monitor")
			return

		case <-s.policyReloadChan:
		}
	}
}

func (s *Source) identifyPolicyIDs(resultCh chan<- []policy.PolicyID, errCh chan<- error) {
	ids, err := s.handleDir()
	if err != nil {
		errCh <- err
	}

	if len(ids) > 0 {
		resultCh <- ids
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

		readPolicy, err := s.decodePolicyFromFile(file)
		if err != nil {
			mErr = multierror.Append(err, mErr)
			continue
		}
		readPolicy.ID = string(policyID)

		s.policyMapLock.Lock()
		s.policyMap[policyID] = &filePolicy{file: file, policy: readPolicy}
		s.policyMapLock.Unlock()

		policyIDs = append(policyIDs, policyID)
	}

	return policyIDs, mErr.ErrorOrNil()
}

func (s *Source) decodePolicyFromFile(file string) (*policy.ClusterScalingPolicy, error) {

	filePolicy := policy.ClusterScalingPolicy{}

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

	md5Sum := md5String(file)

	policyID, ok := s.idMap[md5Sum]
	if !ok {
		policyID = policy.PolicyID(uuid.Generate())
		s.idMap[md5Sum] = policyID
	}

	return policyID
}

func md5String(s string) [16]byte {
	return md5.Sum([]byte(fmt.Sprintf("%v", s)))
}
