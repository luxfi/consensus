// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/ids"
	netzmq4 "github.com/luxfi/consensus/networking/zmq4"
	"github.com/luxfi/consensus/validators"
)

func TestConsensus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Consensus Suite")
}

var _ = Describe("Pure Consensus Protocol", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		nodes  []*ConsensusNode
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		nodes = make([]*ConsensusNode, 0)
	})

	AfterEach(func() {
		cancel()
		for _, node := range nodes {
			node.Stop()
		}
	})

	Describe("Validator Set Management", func() {
		Context("with 3 nodes", func() {
			BeforeEach(func() {
				// Create 3 consensus nodes
				for i := 0; i < 3; i++ {
					node := NewConsensusNode(ctx, ids.GenerateTestNodeID(), 7000+i*10)
					nodes = append(nodes, node)
					Expect(node.Start()).To(Succeed())
				}

				// Connect nodes in full mesh
				for i, node := range nodes {
					for j, peer := range nodes {
						if i != j {
							Expect(node.ConnectToPeer(peer.nodeID, peer.basePort)).To(Succeed())
						}
					}
				}

				// Allow connections to establish
				time.Sleep(50 * time.Millisecond)
			})

			It("should synchronize validator sets across nodes", func() {
				// Create test validators
				validatorList := make([]*validators.GetValidatorOutput, 3)
				for i := 0; i < 3; i++ {
					sk, err := localsigner.New()
					Expect(err).NotTo(HaveOccurred())
					
					validatorList[i] = &validators.GetValidatorOutput{
						NodeID:    ids.GenerateTestNodeID(),
						PublicKey: sk.PublicKey(),
						Weight:    uint64((i + 1) * 100),
					}
				}

				// Node 0 proposes validator set update
				nodes[0].ProposeValidatorSet(1000, validatorList)

				// Wait for consensus
				Eventually(func() bool {
					for _, node := range nodes {
						if !node.HasValidatorSet(1000) {
							return false
						}
					}
					return true
				}, 1*time.Second, 50*time.Millisecond).Should(BeTrue())

				// Verify all nodes have the same validator set
				set0 := nodes[0].GetValidatorSet(1000)
				for i := 1; i < len(nodes); i++ {
					setI := nodes[i].GetValidatorSet(1000)
					Expect(len(setI)).To(Equal(len(set0)))
					for nodeID, vdr := range set0 {
						Expect(setI[nodeID].Weight).To(Equal(vdr.Weight))
					}
				}
			})

			It("should reach consensus on height progression", func() {
				initialHeight := uint64(2000)
				
				// All nodes propose to advance height
				for _, node := range nodes {
					node.ProposeHeight(initialHeight + 1)
				}

				// Wait for consensus
				Eventually(func() uint64 {
					minHeight := uint64(^uint64(0))
					for _, node := range nodes {
						h := node.GetCurrentHeight()
						if h < minHeight {
							minHeight = h
						}
					}
					return minHeight
				}, 1*time.Second, 50*time.Millisecond).Should(Equal(initialHeight + 1))
			})

			It("should handle concurrent proposals correctly", func() {
				// Each node proposes different validator sets concurrently
				var wg sync.WaitGroup
				for i, node := range nodes {
					wg.Add(1)
					go func(n *ConsensusNode, index int) {
						defer wg.Done()
						
						sk, _ := localsigner.New()
						vdrs := []*validators.GetValidatorOutput{
							{
								NodeID:    ids.GenerateTestNodeID(),
								PublicKey: sk.PublicKey(),
								Weight:    uint64(1000 + index),
							},
						}
						n.ProposeValidatorSet(3000, vdrs)
					}(node, i)
				}
				wg.Wait()

				// Wait for consensus
				time.Sleep(500 * time.Millisecond)

				// All nodes should have converged on the same validator set
				set0 := nodes[0].GetValidatorSet(3000)
				Expect(len(set0)).To(Equal(1)) // One of the proposals won
				
				for i := 1; i < len(nodes); i++ {
					setI := nodes[i].GetValidatorSet(3000)
					Expect(len(setI)).To(Equal(len(set0)))
					for nodeID := range set0 {
						Expect(setI).To(HaveKey(nodeID))
					}
				}
			})
		})

		Context("Byzantine fault tolerance with 5 nodes", func() {
			BeforeEach(func() {
				// Create 5 consensus nodes for BFT (tolerates 1 Byzantine node)
				for i := 0; i < 5; i++ {
					node := NewConsensusNode(ctx, ids.GenerateTestNodeID(), 8000+i*10)
					nodes = append(nodes, node)
					Expect(node.Start()).To(Succeed())
				}

				// Full mesh connectivity
				for i, node := range nodes {
					for j, peer := range nodes {
						if i != j {
							Expect(node.ConnectToPeer(peer.nodeID, peer.basePort)).To(Succeed())
						}
					}
				}

				time.Sleep(50 * time.Millisecond)
			})

			It("should reach consensus despite one faulty node", func() {
				// Node 4 will be Byzantine - it won't participate properly
				byzantineNode := nodes[4]
				byzantineNode.SetByzantine(true)

				// Honest nodes propose the same height
				targetHeight := uint64(5000)
				for i := 0; i < 4; i++ {
					nodes[i].ProposeHeight(targetHeight)
				}

				// Byzantine node proposes different height
				byzantineNode.ProposeHeight(targetHeight + 1000)

				// Wait for consensus among honest nodes
				Eventually(func() int {
					count := 0
					for i := 0; i < 4; i++ {
						if nodes[i].GetCurrentHeight() == targetHeight {
							count++
						}
					}
					return count
				}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 3))
			})
		})
	})
})

// ConsensusNode represents a pure consensus node for testing
type ConsensusNode struct {
	nodeID    ids.NodeID
	basePort  int
	ctx       context.Context
	cancel    context.CancelFunc
	transport *netzmq4.Transport
	
	mu            sync.RWMutex
	currentHeight uint64
	validatorSets map[uint64]map[ids.NodeID]*validators.GetValidatorOutput
	proposals     map[uint64]map[ids.NodeID]proposalData
	heightVotes   map[uint64]map[ids.NodeID]uint64
	byzantine     bool
	
	msgCount atomic.Int64
}

type proposalData struct {
	validators []*validators.GetValidatorOutput
	timestamp  time.Time
}

func NewConsensusNode(ctx context.Context, nodeID ids.NodeID, basePort int) *ConsensusNode {
	nodeCtx, cancel := context.WithCancel(ctx)
	return &ConsensusNode{
		nodeID:        nodeID,
		basePort:      basePort,
		ctx:           nodeCtx,
		cancel:        cancel,
		currentHeight: 1000,
		validatorSets: make(map[uint64]map[ids.NodeID]*validators.GetValidatorOutput),
		proposals:     make(map[uint64]map[ids.NodeID]proposalData),
		heightVotes:   make(map[uint64]map[ids.NodeID]uint64),
	}
}

func (n *ConsensusNode) Start() error {
	n.transport = netzmq4.NewTransport(n.ctx, n.nodeID.String(), n.basePort)
	
	// Register message handlers
	n.transport.RegisterHandler("validator_proposal", n.handleValidatorProposal)
	n.transport.RegisterHandler("height_proposal", n.handleHeightProposal)
	n.transport.RegisterHandler("consensus_vote", n.handleConsensusVote)
	
	return n.transport.Start()
}

func (n *ConsensusNode) Stop() {
	n.cancel()
	if n.transport != nil {
		n.transport.Stop()
	}
}

func (n *ConsensusNode) ConnectToPeer(peerID ids.NodeID, peerPort int) error {
	return n.transport.ConnectPeer(peerID.String(), peerPort)
}

func (n *ConsensusNode) SetByzantine(byzantine bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.byzantine = byzantine
}

func (n *ConsensusNode) ProposeValidatorSet(height uint64, validators []*validators.GetValidatorOutput) {
	n.mu.Lock()
	if n.byzantine {
		n.mu.Unlock()
		return // Byzantine nodes don't cooperate
	}
	
	// Store our own proposal
	if n.proposals[height] == nil {
		n.proposals[height] = make(map[ids.NodeID]proposalData)
	}
	n.proposals[height][n.nodeID] = proposalData{
		validators: validators,
		timestamp:  time.Now(),
	}
	n.mu.Unlock()
	
	// Broadcast proposal
	data, _ := json.Marshal(map[string]interface{}{
		"height":     height,
		"validators": validators,
	})
	
	msg := &netzmq4.Message{
		Type:   "validator_proposal",
		Height: height,
		Data:   data,
	}
	
	n.transport.Broadcast(msg)
	n.msgCount.Add(1)
}

func (n *ConsensusNode) ProposeHeight(height uint64) {
	n.mu.Lock()
	if n.byzantine && height > n.currentHeight {
		// Byzantine behavior: propose random height
		height = height + uint64(time.Now().UnixNano()%1000)
	}
	n.mu.Unlock()
	
	msg := &netzmq4.Message{
		Type:   "height_proposal",
		Height: height,
	}
	
	n.transport.Broadcast(msg)
	n.msgCount.Add(1)
}

func (n *ConsensusNode) GetCurrentHeight() uint64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.currentHeight
}

func (n *ConsensusNode) HasValidatorSet(height uint64) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	_, exists := n.validatorSets[height]
	return exists
}

func (n *ConsensusNode) GetValidatorSet(height uint64) map[ids.NodeID]*validators.GetValidatorOutput {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	set := n.validatorSets[height]
	result := make(map[ids.NodeID]*validators.GetValidatorOutput, len(set))
	for k, v := range set {
		result[k] = v
	}
	return result
}

func (n *ConsensusNode) handleValidatorProposal(msg *netzmq4.Message) {
	n.msgCount.Add(1)
	
	var data map[string]interface{}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	
	height := uint64(data["height"].(float64))
	fromNode, _ := ids.NodeIDFromString(msg.From)
	_ = fromNode // Mark as used for now
	
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if n.byzantine {
		return // Byzantine nodes might ignore messages
	}
	
	// Store proposal
	if n.proposals[height] == nil {
		n.proposals[height] = make(map[ids.NodeID]proposalData)
	}
	
	// Check for consensus (majority)
	if len(n.proposals[height]) >= 2 { // Majority for 3 nodes
		// Simple consensus: first proposal wins
		var chosen proposalData
		for _, proposal := range n.proposals[height] {
			chosen = proposal
			break
		}
		
		// Apply validator set
		n.validatorSets[height] = make(map[ids.NodeID]*validators.GetValidatorOutput)
		for _, v := range chosen.validators {
			n.validatorSets[height][v.NodeID] = v
		}
	}
}

func (n *ConsensusNode) handleHeightProposal(msg *netzmq4.Message) {
	n.msgCount.Add(1)
	
	fromNode, _ := ids.NodeIDFromString(msg.From)
	_ = fromNode // Mark as used for now
	
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if n.byzantine {
		return
	}
	
	// Record vote
	if n.heightVotes[msg.Height] == nil {
		n.heightVotes[msg.Height] = make(map[ids.NodeID]uint64)
	}
	n.heightVotes[msg.Height][fromNode] = msg.Height
	
	// Check for consensus
	if len(n.heightVotes[msg.Height]) >= 2 { // Majority for 3 nodes
		n.currentHeight = msg.Height
	}
}

func (n *ConsensusNode) handleConsensusVote(msg *netzmq4.Message) {
	n.msgCount.Add(1)
	// Handle consensus voting logic
}