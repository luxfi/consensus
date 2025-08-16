// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"testing"

	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/stretchr/testify/require"
)

// TestIntegrationLifecycle tests the full integration lifecycle
func TestIntegrationLifecycle(t *testing.T) {
	// Create mock context
	nodeID := ids.GenerateTestNodeID()
	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 4,
		F:                 1,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()

	// Create integration
	integration := NewIntegration(ctx, cfg, cls)

	// Should be enabled by default now
	require.True(t, integration.enabled)
	require.True(t, integration.votingEnabled)
	require.True(t, integration.executionEnabled)

	// Test disable and re-enable
	integration.enabled = false
	integration.Enable()
	require.True(t, integration.enabled)

	integration.votingEnabled = false
	integration.EnableVoting()
	require.True(t, integration.votingEnabled)

	integration.executionEnabled = false
	integration.EnableExecution()
	require.True(t, integration.executionEnabled)
}

// TestIntegrationBlockProcessing tests block processing hooks
func TestIntegrationBlockProcessing(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 4,
		F:                 1,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	integration := NewIntegration(ctx, cfg, cls)

	// Test OnBuildBlock
	mockBlock := struct {
		ID ids.ID
	}{
		ID: ids.GenerateTestID(),
	}

	// Should not panic
	integration.OnBuildBlock(mockBlock)

	// Test OnBlockReceived
	integration.OnBlockReceived(mockBlock)

	// Test OnBlockAccepted
	integration.OnBlockAccepted(mockBlock)
}

// TestIntegrationExecution tests execution checks
func TestIntegrationExecution(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	validators := []ids.NodeID{nodeID}

	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 1,
		F:                 0,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	integration := NewIntegration(ctx, cfg, cls)

	// Override FPC with proper validators
	integration.fpc = New(cfg, cls, newMockDAG(), nil, nodeID, validators)

	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)

	// Initially cannot execute
	canExec := integration.CanExecute(tx)
	require.False(t, canExec)

	// Must wait when pending
	mustWait := integration.MustWaitForFinal(tx)
	require.True(t, mustWait)

	// Vote to make executable
	block := &Block{
		ID:     ids.GenerateTestID(),
		Author: nodeID,
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx[:]},
		},
	}
	integration.fpc.OnBlockObserved(block)

	// Now can execute
	canExec = integration.CanExecute(tx)
	require.True(t, canExec)

	// No longer must wait
	mustWait = integration.MustWaitForFinal(tx)
	require.False(t, mustWait)
}

// TestIntegrationEpochManagement tests epoch transitions
func TestIntegrationEpochManagement(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 4,
		F:                 1,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	integration := NewIntegration(ctx, cfg, cls)

	// Start epoch close
	integration.StartEpochClose()

	// Complete epoch close
	integration.CompleteEpochClose()

	// Test with disabled FPC
	integration.enabled = false
	integration.StartEpochClose()
	integration.CompleteEpochClose()
}

// TestIntegrationStatusAndMetrics tests status queries
func TestIntegrationStatusAndMetrics(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	validators := []ids.NodeID{nodeID}

	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 1,
		F:                 0,
		Epoch:             1,
		VoteLimitPerBlock: 256,
		EnableBLS:         true,
		EnableRingtail:    true,
	}

	cls := newMockClassifier()
	integration := NewIntegration(ctx, cfg, cls)

	// Override FPC with proper validators
	integration.fpc = New(cfg, cls, newMockDAG(), nil, nodeID, validators)

	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)

	// Get status
	status, proof := integration.GetStatus(tx)
	require.Equal(t, Pending, status)
	require.NotNil(t, proof)

	// Vote to make executable
	block := &Block{
		ID:     ids.GenerateTestID(),
		Author: nodeID,
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx[:]},
		},
	}
	integration.fpc.OnBlockObserved(block)

	// Status should be executable with BLS proof
	status, proof = integration.GetStatus(tx)
	require.Equal(t, Executable, status)
	require.NotNil(t, proof.BLSProof)

	// Check dual finality
	hasDual := integration.HasDualFinality(tx)
	require.False(t, hasDual) // No Ringtail proof yet

	// Get metrics
	metrics := integration.GetMetrics()
	require.NotNil(t, metrics)
	require.Greater(t, metrics.TotalVotes, uint64(0))

	// Test with disabled FPC
	integration.enabled = false

	status2, proof2 := integration.GetStatus(tx)
	require.Equal(t, Pending, status2)
	require.Equal(t, Proof{}, proof2)

	hasDual2 := integration.HasDualFinality(tx)
	require.False(t, hasDual2)

	metrics2 := integration.GetMetrics()
	require.Equal(t, Metrics{}, metrics2)
}

// TestIntegrationMixedTransactions tests mixed tx handling
func TestIntegrationMixedTransactions(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	validators := []ids.NodeID{nodeID}

	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 1,
		F:                 0,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	integration := NewIntegration(ctx, cfg, cls)

	// Override FPC with proper validators
	integration.fpc = New(cfg, cls, newMockDAG(), nil, nodeID, validators)

	tx := TxRef{1}

	// Mark as mixed
	integration.fpc.MarkMixed(tx)

	// Mixed must wait for final
	mustWait := integration.MustWaitForFinal(tx)
	require.True(t, mustWait)

	// Cannot execute even with votes
	canExec := integration.CanExecute(tx)
	require.False(t, canExec)
}

// TestIntegrationDisabledBehavior tests behavior when disabled
func TestIntegrationDisabledBehavior(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 4,
		F:                 1,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	integration := NewIntegration(ctx, cfg, cls)

	// Disable everything
	integration.enabled = false
	integration.votingEnabled = false
	integration.executionEnabled = false

	tx := TxRef{1}

	// Cannot execute when disabled
	canExec := integration.CanExecute(tx)
	require.False(t, canExec)

	// Must wait when disabled (conservative)
	mustWait := integration.MustWaitForFinal(tx)
	require.True(t, mustWait)
}

// TestIntegrationDualFinalityDetection tests dual finality
func TestIntegrationDualFinalityDetection(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()

	ctx := &interfaces.Runtime{
		NodeID:  nodeID,
		Log:     log.NewNoOpLogger(),
		Metrics: nil,
	}

	cfg := Config{
		N:                 1,
		F:                 0,
		Epoch:             1,
		VoteLimitPerBlock: 256,
		EnableBLS:         true,
		EnableRingtail:    true,
	}

	cls := newMockClassifier()
	integration := NewIntegration(ctx, cfg, cls)

	// Create mock FPC with mock proofs
	mockFPC := &mockWaveFPC{
		hasBLS:      false,
		hasRingtail: false,
	}
	integration.fpc = mockFPC

	tx := TxRef{1}

	// No dual finality initially
	hasDual := integration.HasDualFinality(tx)
	require.False(t, hasDual)

	// Add BLS only
	mockFPC.hasBLS = true
	hasDual = integration.HasDualFinality(tx)
	require.False(t, hasDual)

	// Add Ringtail too
	mockFPC.hasRingtail = true
	hasDual = integration.HasDualFinality(tx)
	require.True(t, hasDual)

	// Check status is Final with dual proofs
	status, _ := integration.GetStatus(tx)
	require.Equal(t, Final, status)
}

// Mock WaveFPC for testing
type mockWaveFPC struct {
	hasBLS      bool
	hasRingtail bool
}

func (m *mockWaveFPC) OnBlockObserved(b *Block)     {}
func (m *mockWaveFPC) OnBlockAccepted(b *Block)     {}
func (m *mockWaveFPC) OnEpochCloseStart()           {}
func (m *mockWaveFPC) OnEpochClosed()               {}
func (m *mockWaveFPC) NextVotes(budget int) []TxRef { return nil }
func (m *mockWaveFPC) MarkMixed(tx TxRef)           {}
func (m *mockWaveFPC) GetMetrics() Metrics          { return Metrics{} }

func (m *mockWaveFPC) Status(tx TxRef) (Status, Proof) {
	proof := Proof{}
	if m.hasBLS {
		proof.BLSProof = &BLSBundle{}
	}
	if m.hasRingtail {
		proof.RingtailProof = &PQBundle{}
	}

	status := Pending
	if m.hasBLS && m.hasRingtail {
		status = Final
	} else if m.hasBLS {
		status = Executable
	}

	return status, proof
}
