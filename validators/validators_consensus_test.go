// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/luxfi/consensus/validators"
	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/ids"
)

func TestConsensus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Consensus Suite")
}

var _ = Describe("Pure Consensus Protocol", func() {
	var cancel context.CancelFunc

	BeforeEach(func() {
		_, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Validator Set Management", func() {
		Context("with 3 nodes", func() {
			It("should synchronize validator sets across nodes", func() {
				// Create validator managers for each node
				subnetID := ids.GenerateTestID()
				managers := make([]validator.Manager, 3)
				for i := range managers {
					managers[i] = validator.NewManager()
				}

				// Create test validators
				validatorList := make([]validator.GetValidatorOutput, 3)
				for i := 0; i < 3; i++ {
					sk, err := localsigner.New()
					Expect(err).NotTo(HaveOccurred())

					validatorList[i] = validator.GetValidatorOutput{
						NodeID:    ids.GenerateTestNodeID(),
						PublicKey: sk.PublicKey(),
						Weight:    uint64((i + 1) * 100),
					}
				}

				// Simulate proposal: all managers receive the same validators
				for _, manager := range managers {
					for i := range validatorList {
						v := &validatorList[i]
						err := manager.AddStaker(subnetID, v.NodeID, v.PublicKey, ids.Empty, v.Weight)
						Expect(err).NotTo(HaveOccurred())
					}
				}

				// Verify all managers have the same total weight
				for _, manager := range managers {
					totalWeight, err := manager.TotalWeight(subnetID)
					Expect(err).NotTo(HaveOccurred())
					Expect(totalWeight).To(Equal(uint64(600))) // 100 + 200 + 300
				}
			})

			It("should reach consensus on height progression", func() {
				// Create consensus tracking structure
				cm := NewConsensusManager()

				targetHeight := uint64(2001)
				nodeCount := 3

				// Simulate nodes voting for new height
				for i := 0; i < nodeCount; i++ {
					nodeID := ids.GenerateTestNodeID()
					cm.RecordHeightVote(nodeID, targetHeight)
				}

				// Check consensus reached
				Expect(cm.HasConsensus(targetHeight, nodeCount)).To(BeTrue())
				Expect(cm.GetConsensusHeight()).To(Equal(targetHeight))
			})

			It("should handle concurrent proposals correctly", func() {
				subnetID := ids.GenerateTestID()
				manager := validator.NewManager()

				// Create multiple validators concurrently
				var wg sync.WaitGroup

				for i := 0; i < 3; i++ {
					wg.Add(1)
					go func(index int) {
						defer wg.Done()

						sk, _ := localsigner.New()
						validator := validator.GetValidatorOutput{
							NodeID:    ids.GenerateTestNodeID(),
							PublicKey: sk.PublicKey(),
							Weight:    uint64(1000 + index),
						}

						err := manager.AddStaker(subnetID, validator.NodeID, validator.PublicKey, ids.Empty, validator.Weight)
						Expect(err).NotTo(HaveOccurred())
					}(i)
				}
				wg.Wait()

				// Verify all validators were added
				totalWeight, err := manager.TotalWeight(subnetID)
				Expect(err).NotTo(HaveOccurred())
				Expect(totalWeight).To(Equal(uint64(3003))) // 1000 + 1001 + 1002
			})
		})

		Context("Byzantine fault tolerance with 5 nodes", func() {
			It("should reach consensus despite one faulty node", func() {
				cm := NewConsensusManager()

				targetHeight := uint64(5000)
				nodeCount := 5
				byzantineNodes := 1

				// Honest nodes vote for target height
				honestNodes := nodeCount - byzantineNodes
				for i := 0; i < honestNodes; i++ {
					nodeID := ids.GenerateTestNodeID()
					cm.RecordHeightVote(nodeID, targetHeight)
				}

				// Byzantine node votes for different height
				byzantineID := ids.GenerateTestNodeID()
				cm.RecordHeightVote(byzantineID, targetHeight+1000)

				// Check consensus - should succeed with 4/5 honest nodes
				requiredVotes := (nodeCount * 2 / 3) + 1 // 2f+1 for BFT
				Expect(cm.GetVoteCount(targetHeight)).To(BeNumerically(">=", requiredVotes))
			})
		})
	})

	Describe("Delegated Proof of Stake", func() {
		Context("delegation support", func() {
			It("should support delegation to validators", func() {
				subnetID := ids.GenerateTestID()
				manager := validator.NewManager()

				// Create main validator
				sk, err := localsigner.New()
				Expect(err).NotTo(HaveOccurred())

				validatorID := ids.GenerateTestNodeID()
				validatorWeight := uint64(1000)

				// Add validator
				err = manager.AddStaker(subnetID, validatorID, sk.PublicKey(), ids.Empty, validatorWeight)
				Expect(err).NotTo(HaveOccurred())

				// Simulate delegation by adding weight
				delegatedWeight := uint64(500)
				err = manager.AddWeight(subnetID, validatorID, delegatedWeight)
				Expect(err).NotTo(HaveOccurred())

				// Verify total weight includes delegation
				totalWeight := manager.GetWeight(subnetID, validatorID)
				Expect(totalWeight).To(Equal(validatorWeight + delegatedWeight))
			})

			It("should track delegator rewards separately", func() {
				dm := NewDelegationManager()

				validatorID := ids.GenerateTestNodeID()
				delegatorID := ids.GenerateTestNodeID()
				delegationAmount := uint64(1000)

				// Record delegation
				dm.AddDelegation(validatorID, delegatorID, delegationAmount)

				// Simulate reward distribution
				totalReward := uint64(100)
				validatorCommission := uint64(10) // 10%
				delegatorReward := totalReward - validatorCommission

				dm.RecordReward(validatorID, delegatorID, delegatorReward)

				// Verify delegation tracking
				delegation := dm.GetDelegation(validatorID, delegatorID)
				Expect(delegation.Amount).To(Equal(delegationAmount))
				Expect(delegation.Rewards).To(Equal(delegatorReward))
			})
		})
	})
})

// ConsensusManager provides clean consensus operations
type ConsensusManager struct {
	mu              sync.RWMutex
	heightVotes     map[uint64]map[ids.NodeID]bool
	consensusHeight uint64
}

func NewConsensusManager() *ConsensusManager {
	return &ConsensusManager{
		heightVotes: make(map[uint64]map[ids.NodeID]bool),
	}
}

func (cm *ConsensusManager) RecordHeightVote(nodeID ids.NodeID, height uint64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.heightVotes[height] == nil {
		cm.heightVotes[height] = make(map[ids.NodeID]bool)
	}
	cm.heightVotes[height][nodeID] = true
}

func (cm *ConsensusManager) GetVoteCount(height uint64) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if votes, exists := cm.heightVotes[height]; exists {
		return len(votes)
	}
	return 0
}

func (cm *ConsensusManager) HasConsensus(height uint64, totalNodes int) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	voteCount := len(cm.heightVotes[height])
	requiredVotes := (totalNodes / 2) + 1 // Simple majority

	if voteCount >= requiredVotes {
		cm.consensusHeight = height
		return true
	}
	return false
}

func (cm *ConsensusManager) GetConsensusHeight() uint64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.consensusHeight
}

// DelegationManager tracks delegations for DPoS
type DelegationManager struct {
	mu          sync.RWMutex
	delegations map[ids.NodeID]map[ids.NodeID]*Delegation
}

type Delegation struct {
	ValidatorID ids.NodeID
	DelegatorID ids.NodeID
	Amount      uint64
	Rewards     uint64
	StartTime   time.Time
}

func NewDelegationManager() *DelegationManager {
	return &DelegationManager{
		delegations: make(map[ids.NodeID]map[ids.NodeID]*Delegation),
	}
}

func (dm *DelegationManager) AddDelegation(validatorID, delegatorID ids.NodeID, amount uint64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.delegations[validatorID] == nil {
		dm.delegations[validatorID] = make(map[ids.NodeID]*Delegation)
	}

	dm.delegations[validatorID][delegatorID] = &Delegation{
		ValidatorID: validatorID,
		DelegatorID: delegatorID,
		Amount:      amount,
		StartTime:   time.Now(),
	}
}

func (dm *DelegationManager) GetDelegation(validatorID, delegatorID ids.NodeID) *Delegation {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if validators, exists := dm.delegations[validatorID]; exists {
		return validators[delegatorID]
	}
	return nil
}

func (dm *DelegationManager) RecordReward(validatorID, delegatorID ids.NodeID, reward uint64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if validators, exists := dm.delegations[validatorID]; exists {
		if delegation, exists := validators[delegatorID]; exists {
			delegation.Rewards += reward
		}
	}
}
