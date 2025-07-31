// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/networking/zmq4"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/testutils"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// ValidatorNode represents a validator in the consensus network
type ValidatorNode struct {
	nodeID    string
	transport *zmq4.Transport
	consensus interfaces.Consensus
	ctx       *interfaces.Context
	params    config.Parameters
	cancel    context.CancelFunc
	
	// Metrics
	messagesReceived uint64
	messagesSent     uint64
	blocksAccepted   uint64
	consensusReached uint64
}

// NetworkSimulator manages multiple validators
type NetworkSimulator struct {
	validators []*ValidatorNode
	basePort   int
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewNetworkSimulator creates a new network simulator
func NewNetworkSimulator(numValidators int, basePort int) *NetworkSimulator {
	ctx, cancel := context.WithCancel(context.Background())
	return &NetworkSimulator{
		validators: make([]*ValidatorNode, 0, numValidators),
		basePort:   basePort,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// AddValidator adds a new validator to the network
func (ns *NetworkSimulator) AddValidator(protocol string, params config.Parameters) (*ValidatorNode, error) {
	nodeID := fmt.Sprintf("validator-%d", len(ns.validators))
	port := ns.basePort + len(ns.validators)*10
	
	// Create transport
	transport := zmq4.NewTransport(ns.ctx, nodeID, port)
	
	// Create consensus context
	ctx := &interfaces.Context{
		Log:        log.NewNoOpLogger(),
		Registerer: testutils.NewNoOpRegisterer(),
	}
	
	// Create consensus instance based on protocol
	var consensus interfaces.Consensus
	switch protocol {
	case "photon":
		consensus = photon.NewPhoton(params)
	case "pulse":
		consensus = pulse.NewPulse(params)
	case "wave":
		consensus = wave.NewWave(params)
	default:
		return nil, fmt.Errorf("unknown protocol: %s", protocol)
	}
	
	validator := &ValidatorNode{
		nodeID:    nodeID,
		transport: transport,
		consensus: consensus,
		ctx:       ctx,
		params:    params,
	}
	
	ns.validators = append(ns.validators, validator)
	return validator, nil
}

// Start starts all validators
func (ns *NetworkSimulator) Start() error {
	// Start all transports
	for _, v := range ns.validators {
		if err := v.transport.Start(); err != nil {
			return fmt.Errorf("failed to start transport for %s: %w", v.nodeID, err)
		}
	}
	
	// Connect all validators to each other (full mesh)
	for i, v1 := range ns.validators {
		for j, v2 := range ns.validators {
			if i != j {
				port := ns.basePort + j*10
				if err := v1.transport.ConnectPeer(v2.nodeID, port); err != nil {
					return fmt.Errorf("failed to connect %s to %s: %w", v1.nodeID, v2.nodeID, err)
				}
			}
		}
	}
	
	// Register message handlers and start validators
	for _, v := range ns.validators {
		ctx, cancel := context.WithCancel(ns.ctx)
		v.cancel = cancel
		v.registerHandlers()
		ns.wg.Add(1)
		go v.run(ctx, &ns.wg)
	}
	
	// Wait for connections to establish
	time.Sleep(200 * time.Millisecond)
	
	return nil
}

// Stop stops all validators
func (ns *NetworkSimulator) Stop() {
	// Cancel each validator's context
	for _, v := range ns.validators {
		if v.cancel != nil {
			v.cancel()
		}
	}
	
	// Cancel main context
	ns.cancel()
	ns.wg.Wait()
	
	// Stop transports
	for _, v := range ns.validators {
		v.transport.Stop()
	}
}

// registerHandlers sets up message handlers for consensus
func (v *ValidatorNode) registerHandlers() {
	// Handle consensus votes
	v.transport.RegisterHandler("vote", func(msg *zmq4.Message) {
		atomic.AddUint64(&v.messagesReceived, 1)
		
		var vote struct {
			BlockID ids.ID `json:"block_id"`
			Height  uint64 `json:"height"`
		}
		
		if err := json.Unmarshal(msg.Data, &vote); err != nil {
			return
		}
		
		// Process vote in consensus
		votes := bag.Bag[ids.ID]{}
		votes.Add(vote.BlockID)
		
		v.consensus.RecordPrism(votes)
	})
	
	// Handle block proposals
	v.transport.RegisterHandler("propose", func(msg *zmq4.Message) {
		atomic.AddUint64(&v.messagesReceived, 1)
		
		var proposal struct {
			BlockID   ids.ID `json:"block_id"`
			ParentID  ids.ID `json:"parent_id"`
			Height    uint64 `json:"height"`
			Timestamp int64  `json:"timestamp"`
		}
		
		if err := json.Unmarshal(msg.Data, &proposal); err != nil {
			return
		}
		
		// Add block to consensus
		v.consensus.Add(proposal.BlockID)
	})
	
	// Handle consensus reached notifications
	v.transport.RegisterHandler("consensus", func(msg *zmq4.Message) {
		atomic.AddUint64(&v.messagesReceived, 1)
		atomic.AddUint64(&v.consensusReached, 1)
	})
}

// run is the main loop for a validator
func (v *ValidatorNode) run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Periodically check consensus state and broadcast votes
			pref := v.consensus.Preference()
			if !pref.IsZero() {
				vote := struct {
					BlockID ids.ID `json:"block_id"`
					Height  uint64 `json:"height"`
				}{
					BlockID: pref,
					Height:  0, // Would be actual height in real implementation
				}
				
				data, _ := json.Marshal(vote)
				msg := &zmq4.Message{
					Type:      "vote",
					From:      v.nodeID,
					Data:      data,
					Timestamp: time.Now().Unix(),
				}
				
				v.transport.Broadcast(msg)
				atomic.AddUint64(&v.messagesSent, 1)
			}
		}
	}
}

// GetMetrics returns current metrics for the validator
func (v *ValidatorNode) GetMetrics() (received, sent, accepted, consensus uint64) {
	return atomic.LoadUint64(&v.messagesReceived),
		atomic.LoadUint64(&v.messagesSent),
		atomic.LoadUint64(&v.blocksAccepted),
		atomic.LoadUint64(&v.consensusReached)
}

// BenchmarkPhotonConsensusWithZMQ benchmarks Photon consensus with ZMQ transport
func BenchmarkPhotonConsensusWithZMQ(b *testing.B) {
	benchmarkConsensusWithZMQ(b, "photon", []int{5, 10, 20})
}

// BenchmarkPulseConsensusWithZMQ benchmarks Pulse consensus with ZMQ transport
func BenchmarkPulseConsensusWithZMQ(b *testing.B) {
	benchmarkConsensusWithZMQ(b, "pulse", []int{5, 10, 20})
}

// BenchmarkWaveConsensusWithZMQ benchmarks Wave consensus with ZMQ transport
func BenchmarkWaveConsensusWithZMQ(b *testing.B) {
	benchmarkConsensusWithZMQ(b, "wave", []int{5, 10, 20})
}

// benchmarkConsensusWithZMQ runs consensus benchmarks with varying validator counts
func benchmarkConsensusWithZMQ(b *testing.B, protocol string, validatorCounts []int) {
	for _, numValidators := range validatorCounts {
		b.Run(fmt.Sprintf("validators-%d", numValidators), func(b *testing.B) {
			// Create network simulator
			ns := NewNetworkSimulator(numValidators, 20000)
			
			// Add validators
			params := config.TestParameters
			for i := 0; i < numValidators; i++ {
				_, err := ns.AddValidator(protocol, params)
				require.NoError(b, err)
			}
			
			// Start network
			err := ns.Start()
			require.NoError(b, err)
			defer ns.Stop()
			
			// Add genesis block to all validators
			genesis := ids.GenerateTestID()
			
			for _, v := range ns.validators {
				err := v.consensus.Add(genesis)
				require.NoError(b, err)
			}
			
			b.ResetTimer()
			
			// Run benchmark
			for i := 0; i < b.N; i++ {
				// Create and propose a new block
				blockID := ids.GenerateTestID()
				
				// Propose block from first validator
				proposal := struct {
					BlockID   ids.ID `json:"block_id"`
					ParentID  ids.ID `json:"parent_id"`
					Height    uint64 `json:"height"`
					Timestamp int64  `json:"timestamp"`
				}{
					BlockID:   blockID,
					ParentID:  genesis,
					Height:    uint64(i + 1),
					Timestamp: time.Now().Unix(),
				}
				
				data, _ := json.Marshal(proposal)
				msg := &zmq4.Message{
					Type:      "propose",
					From:      ns.validators[0].nodeID,
					Data:      data,
					Timestamp: time.Now().Unix(),
				}
				
				// Broadcast proposal
				err := ns.validators[0].transport.Broadcast(msg)
				require.NoError(b, err)
				
				// Wait for consensus
				time.Sleep(50 * time.Millisecond)
			}
			
			b.StopTimer()
			
			// Collect and report metrics
			var totalReceived, totalSent, totalAccepted, totalConsensus uint64
			for _, v := range ns.validators {
				r, s, a, c := v.GetMetrics()
				totalReceived += r
				totalSent += s
				totalAccepted += a
				totalConsensus += c
			}
			
			b.Logf("Network stats - Messages: %d received, %d sent; Blocks: %d accepted; Consensus: %d reached",
				totalReceived, totalSent, totalAccepted, totalConsensus)
			
			// Calculate messages per operation
			if b.N > 0 {
				b.ReportMetric(float64(totalSent)/float64(b.N), "msgs/op")
				b.ReportMetric(float64(totalReceived)/float64(b.N*numValidators), "msgs_received/validator/op")
			}
		})
	}
}

// BenchmarkConsensusScalability tests how consensus scales with network size
func BenchmarkConsensusScalability(b *testing.B) {
	validatorCounts := []int{3, 5, 10, 20, 50}
	protocols := []string{"photon", "pulse", "wave"}
	
	for _, protocol := range protocols {
		b.Run(protocol, func(b *testing.B) {
			for _, numValidators := range validatorCounts {
				b.Run(fmt.Sprintf("validators-%d", numValidators), func(b *testing.B) {
					// Skip if too many validators for the test environment
					if numValidators > 20 && testing.Short() {
						b.Skip("Skipping large validator count in short mode")
					}
					
					// Create network
					ns := NewNetworkSimulator(numValidators, 30000)
					
					// Add validators
					params := config.TestParameters
					// Adjust parameters for larger networks
					if numValidators > 10 {
						params.K = numValidators
						params.AlphaPreference = (numValidators + 1) / 2
						params.AlphaConfidence = (numValidators * 2) / 3
					}
					
					for i := 0; i < numValidators; i++ {
						_, err := ns.AddValidator(protocol, params)
						require.NoError(b, err)
					}
					
					// Start network
					err := ns.Start()
					require.NoError(b, err)
					defer ns.Stop()
					
					// Add genesis block
					genesis := ids.GenerateTestID()
					
					for _, v := range ns.validators {
						err := v.consensus.Add(genesis)
						require.NoError(b, err)
					}
					
					// Measure time to consensus
					start := time.Now()
					consensusReached := make(chan bool)
					
					// Monitor for consensus
					go func() {
						ticker := time.NewTicker(10 * time.Millisecond)
						defer ticker.Stop()
						
						for {
							select {
							case <-ticker.C:
								// Check if all validators reached consensus
								allAgreed := true
								var pref ids.ID
								for i, v := range ns.validators {
									p := v.consensus.Preference()
									if i == 0 {
										pref = p
									} else if p != pref {
										allAgreed = false
										break
									}
								}
								
								if allAgreed && !pref.IsZero() {
									consensusReached <- true
									return
								}
							}
						}
					}()
					
					// Propose a block
					blockID := ids.GenerateTestID()
					
					// Add block to all validators
					for _, v := range ns.validators {
						err := v.consensus.Add(blockID)
						require.NoError(b, err)
					}
					
					// Wait for consensus or timeout
					select {
					case <-consensusReached:
						elapsed := time.Since(start)
						b.ReportMetric(float64(elapsed.Microseconds()), "μs/consensus")
						b.ReportMetric(float64(numValidators), "validators")
					case <-time.After(5 * time.Second):
						b.Fatal("Consensus timeout")
					}
				})
			}
		})
	}
}

// testBlock implements interfaces.Block for testing
type testBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp time.Time
	status    interfaces.Status
}

func (b *testBlock) ID() ids.ID             { return b.id }
func (b *testBlock) Parent() ids.ID        { return b.parentID }
func (b *testBlock) Height() uint64        { return b.height }
func (b *testBlock) Timestamp() time.Time  { return b.timestamp }
func (b *testBlock) Bytes() []byte         { return nil }
func (b *testBlock) Verify() error         { return nil }
func (b *testBlock) Accept(context.Context) error {
	b.status = interfaces.Accepted
	return nil
}
func (b *testBlock) Reject(context.Context) error {
	b.status = interfaces.Rejected
	return nil
}
func (b *testBlock) Status() (interfaces.Status, error) {
	return b.status, nil
}