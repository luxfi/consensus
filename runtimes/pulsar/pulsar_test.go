// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// MockTransaction implements the Transaction interface for testing
type MockTransaction struct {
	id      ids.ID
	data    []byte
	valid   bool
}

func (m *MockTransaction) ID() ids.ID {
	return m.id
}

func (m *MockTransaction) Bytes() []byte {
	return m.data
}

func (m *MockTransaction) Verify() error {
	if !m.valid {
		return fmt.Errorf("invalid transaction")
	}
	return nil
}

func TestNewPulsar(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()

	pulsar := NewPulsar(params, nodeID)

	require.NotNil(pulsar)
	require.Equal(params, pulsar.params)
	require.Equal(nodeID, pulsar.nodeID)
	require.Equal(uint64(0), pulsar.height)
	require.Nil(pulsar.lastBlock)
	require.Empty(pulsar.chain)
	require.Empty(pulsar.pendingTxs)
	require.Empty(pulsar.photons)
	require.Equal(time.Duration(params.MaxItemProcessingTime), pulsar.pulseInterval)
	require.Equal(uint64(1), pulsar.pulseStrength)
	require.Equal(1.0, pulsar.rotationRate)
	require.Equal(0, pulsar.confidence)
}

func TestPulsarAddTransaction(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)

	// Valid transaction
	validTx := &MockTransaction{
		id:    ids.GenerateTestID(),
		data:  []byte("valid tx"),
		valid: true,
	}
	require.NoError(pulsar.AddTransaction(validTx))
	require.Len(pulsar.pendingTxs, 1)

	// Invalid transaction
	invalidTx := &MockTransaction{
		id:    ids.GenerateTestID(),
		data:  []byte("invalid tx"),
		valid: false,
	}
	require.Error(pulsar.AddTransaction(invalidTx))
	require.Len(pulsar.pendingTxs, 1) // Still 1
}

func TestPulsarAddPhoton(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)

	photon := NewPhoton(nodeID, []byte("photon data"))
	photon.Energy = 5

	pulsar.AddPhoton(photon)

	require.Len(pulsar.photons, 1)
	require.Equal(uint64(6), pulsar.pulseStrength) // 1 + 5
}

func TestPulsarRecordVote(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)
	ctx := context.Background()

	// Create a block
	block := &LinearBlock{
		Height:     0,
		BlockID:    ids.GenerateTestID(),
		ProposerID: nodeID,
	}
	pulsar.chain = append(pulsar.chain, block)

	// Vote for the block
	require.NoError(pulsar.RecordVote(ctx, block.BlockID, true))
	require.Equal(1, pulsar.confidence)
	// preferredBlock is only set when confidence >= AlphaPreference
	// With default params, AlphaPreference is 13, so we need more votes
	require.Nil(pulsar.preferredBlock)

	// Vote against
	require.NoError(pulsar.RecordVote(ctx, block.BlockID, false))
	require.Equal(0, pulsar.confidence)

	// Vote for non-existent block
	require.Error(pulsar.RecordVote(ctx, ids.GenerateTestID(), true))
}

func TestPulsarEmitPulse(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)
	ctx := context.Background()

	// Add transactions
	for i := 0; i < 5; i++ {
		tx := &MockTransaction{
			id:    ids.GenerateTestID(),
			data:  []byte(fmt.Sprintf("tx %d", i)),
			valid: true,
		}
		require.NoError(pulsar.AddTransaction(tx))
	}

	// Add photons
	for i := 0; i < 3; i++ {
		photon := NewPhoton(nodeID, []byte("photon"))
		photon.Energy = 2
		pulsar.AddPhoton(photon)
	}

	// Store initial state
	initialHeight := pulsar.height

	// Pulse strength before emission is 1 + 3*2 = 7
	prePulseStrength := pulsar.pulseStrength
	require.Equal(uint64(7), prePulseStrength)

	// Emit pulse
	pulsar.emitPulse(ctx, 1)

	// Verify block created
	require.Equal(initialHeight+1, pulsar.height)
	require.Len(pulsar.chain, 1)
	require.NotNil(pulsar.lastBlock)
	
	block := pulsar.lastBlock
	require.Equal(uint64(0), block.Height)
	require.Equal(nodeID, block.ProposerID)
	// All 5 transactions should be included since pulseStrength is 7
	require.Len(block.Transactions, 5)
	require.Equal(uint64(1), block.PulseNumber)
	require.Equal(uint64(7), block.PulseEnergy) // The energy at time of pulse
	require.Equal(uint64(3), block.PhotonCount)

	// Verify state reset
	require.Equal(uint64(1), pulsar.pulseStrength)
	require.Empty(pulsar.photons)
	require.Equal(1, pulsar.confidence)
	require.Equal(block, pulsar.preferredBlock)
}

func TestPulsarEmitPulseNoEnergy(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)
	ctx := context.Background()

	// Set pulse strength to 0
	pulsar.pulseStrength = 0

	// Try to emit pulse
	pulsar.emitPulse(ctx, 1)

	// No block should be created
	require.Equal(uint64(0), pulsar.height)
	require.Empty(pulsar.chain)
	require.Nil(pulsar.lastBlock)
}

func TestPulsarFinalizeBlock(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	params.AlphaConfidence = 3
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)
	ctx := context.Background()

	var finalizedBlock *LinearBlock
	pulsar.SetFinalizeCallback(func(block *LinearBlock) {
		finalizedBlock = block
	})

	// Create a block
	block := &LinearBlock{
		Height:     0,
		BlockID:    ids.GenerateTestID(),
		ProposerID: nodeID,
		Finalized:  false,
	}
	pulsar.chain = append(pulsar.chain, block)

	// Vote until finalization
	for i := 0; i < params.AlphaConfidence; i++ {
		require.NoError(pulsar.RecordVote(ctx, block.BlockID, true))
	}

	// Verify finalization
	require.True(block.Finalized)
	require.NotZero(block.FinalizedAt)
	require.Equal(block, finalizedBlock)
	require.Equal(0, pulsar.confidence) // Reset after finalization
}

func TestPulsarGetters(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)

	// Initial state
	require.Equal(uint64(0), pulsar.GetHeight())
	
	_, ok := pulsar.GetLastBlock()
	require.False(ok)
	
	_, ok = pulsar.GetPreferredBlock()
	require.False(ok)
	
	require.Empty(pulsar.GetChain())

	// Add a block
	block := &LinearBlock{
		Height:     0,
		BlockID:    ids.GenerateTestID(),
		ProposerID: nodeID,
	}
	pulsar.chain = append(pulsar.chain, block)
	pulsar.lastBlock = block
	pulsar.preferredBlock = block
	pulsar.height = 1

	// Verify getters
	require.Equal(uint64(1), pulsar.GetHeight())
	
	lastBlock, ok := pulsar.GetLastBlock()
	require.True(ok)
	require.Equal(block, lastBlock)
	
	prefBlock, ok := pulsar.GetPreferredBlock()
	require.True(ok)
	require.Equal(block, prefBlock)
	
	chain := pulsar.GetChain()
	require.Len(chain, 1)
	require.Equal(block, chain[0])
}

func TestPulsarStats(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)

	// Add some state
	for i := 0; i < 3; i++ {
		tx := &MockTransaction{
			id:    ids.GenerateTestID(),
			data:  []byte("tx"),
			valid: true,
		}
		pulsar.AddTransaction(tx)
	}

	for i := 0; i < 2; i++ {
		photon := NewPhoton(nodeID, []byte("photon"))
		pulsar.AddPhoton(photon)
	}

	block1 := &LinearBlock{Height: 0, Finalized: true}
	block2 := &LinearBlock{Height: 1, Finalized: false}
	pulsar.chain = []*LinearBlock{block1, block2}
	pulsar.height = 2
	pulsar.confidence = 5
	pulsar.rotationRate = 2.5

	stats := pulsar.GetStats()

	require.Equal(uint64(2), stats.Height)
	require.Equal(2, stats.ChainLength)
	require.Equal(3, stats.PendingTxCount)
	require.Equal(uint64(3), stats.PulseStrength) // 1 + 2 photons
	require.Equal(2.5, stats.RotationRate)
	require.Equal(2, stats.PhotonCount)
	require.Equal(5, stats.Confidence)
	require.Equal(1, stats.FinalizedBlocks)
}

func TestPulsarIntegration(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	params.MaxItemProcessingTime = 50 * time.Millisecond
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize pulsar
	require.NoError(pulsar.Initialize(ctx))

	// Set callbacks
	var mu sync.Mutex
	var pulsedBlocks []*LinearBlock
	pulsar.SetPulseCallback(func(block *LinearBlock) {
		mu.Lock()
		pulsedBlocks = append(pulsedBlocks, block)
		mu.Unlock()
	})

	// Add transactions
	for i := 0; i < 10; i++ {
		tx := &MockTransaction{
			id:    ids.GenerateTestID(),
			data:  []byte(fmt.Sprintf("tx %d", i)),
			valid: true,
		}
		require.NoError(pulsar.AddTransaction(tx))
	}

	// Wait for pulses
	time.Sleep(200 * time.Millisecond)

	// Verify blocks were created
	mu.Lock()
	blocks := make([]*LinearBlock, len(pulsedBlocks))
	copy(blocks, pulsedBlocks)
	mu.Unlock()
	require.NotEmpty(blocks)
	require.Greater(pulsar.GetHeight(), uint64(0))
	
	chain := pulsar.GetChain()
	require.NotEmpty(chain)
	
	// Verify block linkage
	for i := 1; i < len(chain); i++ {
		require.Equal(chain[i-1].BlockID, chain[i].ParentID)
	}
}

func BenchmarkPulsarAddTransaction(b *testing.B) {
	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := &MockTransaction{
			id:    ids.GenerateTestID(),
			data:  []byte("benchmark tx"),
			valid: true,
		}
		pulsar.AddTransaction(tx)
	}
}

func BenchmarkPulsarEmitPulse(b *testing.B) {
	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	pulsar := NewPulsar(params, nodeID)
	ctx := context.Background()

	// Add many transactions
	for i := 0; i < 1000; i++ {
		tx := &MockTransaction{
			id:    ids.GenerateTestID(),
			data:  []byte("tx"),
			valid: true,
		}
		pulsar.AddTransaction(tx)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pulsar.emitPulse(ctx, uint64(i))
	}
}