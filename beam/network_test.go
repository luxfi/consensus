// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

var (
	errAccept = errors.New("unexpectedly called Accept")
	errReject = errors.New("unexpectedly called Reject")
)

// TestBlock is a test implementation of Block
type TestBlock struct {
	choices.TestDecidable

	ParentV    ids.ID
	HeightV    uint64
	TimestampV int64
	VerifyV    error
	BytesV     []byte
}

func (b *TestBlock) Parent() ids.ID {
	return b.ParentV
}

func (b *TestBlock) Verify(context.Context) error {
	return b.VerifyV
}

func (b *TestBlock) Bytes() []byte {
	return b.BytesV
}

func (b *TestBlock) Height() uint64 {
	return b.HeightV
}

func (b *TestBlock) Timestamp() int64 {
	return b.TimestampV
}

// Test beam network single decision
func TestBeamNetworkSingleDecision(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters

	// Create genesis block
	genesis := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		HeightV: 0,
	}

	// Create blocks
	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: genesis.ID(),
		HeightV: 1,
	}

	// Set up the beam instance
	beam := &Topological{
		metrics: &mockMetrics{},
	}

	beam.Initialize(params, genesis.ID(), genesis.Height(), genesis.Timestamp())

	// Add the blocks
	require.NoError(beam.Add(context.Background(), block0))

	// Poll should cause block0 to be accepted
	votes := bag.Of(block0.ID())
	require.True(beam.RecordPoll(context.Background(), votes))

	require.Equal(choices.Accepted, block0.Status())
}

func TestBeamNetworkConflict(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters

	// Create genesis block
	genesis := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		HeightV: 0,
	}

	// Create conflicting blocks
	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: genesis.ID(),
		HeightV: 1,
	}

	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: genesis.ID(),
		HeightV: 1,
	}

	// Set up the beam instance
	beam := &Topological{
		metrics: &mockMetrics{},
	}

	beam.Initialize(params, genesis.ID(), genesis.Height(), genesis.Timestamp())

	// Add the blocks
	require.NoError(beam.Add(context.Background(), block0))
	require.NoError(beam.Add(context.Background(), block1))

	// Poll for block0
	votes := bag.Of(block0.ID())
	require.False(beam.RecordPoll(context.Background(), votes))
	require.False(beam.RecordPoll(context.Background(), votes))

	// Should accept block0 and reject block1
	require.True(beam.RecordPoll(context.Background(), votes))

	require.Equal(choices.Accepted, block0.Status())
	require.Equal(choices.Rejected, block1.Status())
}

// Mock metrics for testing
type mockMetrics struct{}

func (m *mockMetrics) ProcessingLen() int                 { return 0 }
func (m *mockMetrics) NumProcessing() int                 { return 0 }
func (m *mockMetrics) Accepted(ids.ID)                    {}
func (m *mockMetrics) Rejected(ids.ID)                    {}
func (m *mockMetrics) MeasureAndSetOldestTimestamp(int64) {}
func (m *mockMetrics) MarkAccepted(block) error           { return nil }

// Test cascading accepts
func TestBeamNetworkCascade(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters

	// Create genesis block
	genesis := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		HeightV: 0,
	}

	// Create a chain of blocks
	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: genesis.ID(),
		HeightV: 1,
	}

	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: block0.ID(),
		HeightV: 2,
	}

	block2 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: block1.ID(),
		HeightV: 3,
	}

	// Set up the beam instance
	beam := &Topological{
		metrics: &mockMetrics{},
	}

	beam.Initialize(params, genesis.ID(), genesis.Height(), genesis.Timestamp())

	// Add the blocks
	require.NoError(beam.Add(context.Background(), block0))
	require.NoError(beam.Add(context.Background(), block1))
	require.NoError(beam.Add(context.Background(), block2))

	// Poll for block2 should cascade accepts
	votes := bag.Of(block2.ID())
	require.False(beam.RecordPoll(context.Background(), votes))
	require.False(beam.RecordPoll(context.Background(), votes))
	require.True(beam.RecordPoll(context.Background(), votes))

	// All blocks should be accepted
	require.Equal(choices.Accepted, block0.Status())
	require.Equal(choices.Accepted, block1.Status())
	require.Equal(choices.Accepted, block2.Status())
}

// Test poll reset on conflict
func TestBeamNetworkPollReset(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters

	// Create genesis block
	genesis := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		HeightV: 0,
	}

	// Create conflicting blocks
	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: genesis.ID(),
		HeightV: 1,
	}

	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: genesis.ID(),
		HeightV: 1,
	}

	// Set up the beam instance
	beam := &Topological{
		metrics: &mockMetrics{},
	}

	beam.Initialize(params, genesis.ID(), genesis.Height(), genesis.Timestamp())

	// Add the blocks
	require.NoError(beam.Add(context.Background(), block0))
	require.NoError(beam.Add(context.Background(), block1))

	// Poll for block0 twice
	votes0 := bag.Of(block0.ID())
	require.False(beam.RecordPoll(context.Background(), votes0))
	require.False(beam.RecordPoll(context.Background(), votes0))

	// Switch to block1 - should reset confidence
	votes1 := bag.Of(block1.ID())
	require.False(beam.RecordPoll(context.Background(), votes1))
	require.False(beam.RecordPoll(context.Background(), votes1))

	// Should now accept block1
	require.True(beam.RecordPoll(context.Background(), votes1))

	require.Equal(choices.Rejected, block0.Status())
	require.Equal(choices.Accepted, block1.Status())
}
