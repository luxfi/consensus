// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// MockBlock implements the Block interface for testing
type MockBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	bytes     []byte
	choice    int
	signature quasar.Signature
}

func (b *MockBlock) ID() ids.ID                  { return b.id }
func (b *MockBlock) ParentID() ids.ID            { return b.parentID }
func (b *MockBlock) Height() uint64              { return b.height }
func (b *MockBlock) Bytes() []byte               { return b.bytes }
func (b *MockBlock) Choice() int                 { return b.choice }
func (b *MockBlock) Signature() quasar.Signature { return b.signature }

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		params  Parameters
		wantErr bool
	}{
		{
			name: "valid parameters",
			params: Parameters{
				K:               21,
				AlphaPreference: 13,
				AlphaConfidence: 18,
				Beta:            8,
				SecurityLevel:   quasar.SecurityLow,
			},
			wantErr: false,
		},
		{
			name:    "empty parameters",
			params:  Parameters{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			ctx := context.Background()
			engine, err := New(ctx, tt.params)
			if tt.wantErr {
				require.Error(err)
				return
			}

			require.NoError(err)
			require.NotNil(engine)
			require.Equal(tt.params, engine.params)
			require.NotNil(engine.state)
			require.Equal(PhotonStage, engine.state.stage)
		})
	}
}

func TestInitialize(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Initialize(context.Background())
	require.NoError(err)
	require.Equal(PhotonStage, engine.state.stage)
}

func TestProcessBlock(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Initialize(context.Background())
	require.NoError(err)

	// Create a mock block
	block := &MockBlock{
		id:       ids.GenerateTestID(),
		parentID: ids.GenerateTestID(),
		height:   1,
		bytes:    []byte("test block"),
		choice:   1,
	}

	// Process the block
	err = engine.ProcessBlock(context.Background(), block)
	require.NoError(err)
}

func TestStateTransitions(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	// Test initial state
	require.Equal(PhotonStage, engine.state.stage)
	require.False(engine.state.finalized)

	// Test processPhoton transition
	block := &MockBlock{
		id:     ids.GenerateTestID(),
		choice: 1,
	}
	engine.processPhoton(block)
	require.Equal(WaveStage, engine.state.stage)

	// Test processWave transition
	engine.processWave(block)
	require.Equal(FocusStage, engine.state.stage)

	// Test processFocus - no transition implemented yet
	engine.processFocus(block)
	require.Equal(FocusStage, engine.state.stage)

	// Test processBeam
	engine.processBeam(block)
	require.Equal(CompletedStage, engine.state.stage)
	require.True(engine.state.finalized)
}

func TestChainState(t *testing.T) {
	require := require.New(t)

	state := newChainState()
	require.NotNil(state)
	require.Equal(PhotonStage, state.Stage())
	require.False(state.Finalized())
	require.NotNil(state.Confidence())
	require.Empty(state.Preference())

	// Test state mutations
	state.stage = WaveStage
	require.Equal(WaveStage, state.Stage())

	state.preference = ids.GenerateTestID()
	require.Equal(state.preference, state.Preference())

	state.finalized = true
	require.True(state.Finalized())

	// Test confidence map
	id1 := ids.GenerateTestID()
	id2 := ids.GenerateTestID()
	state.confidence[id1] = 5
	state.confidence[id2] = 10

	confidence := state.Confidence()
	require.Equal(5, confidence[id1])
	require.Equal(10, confidence[id2])
}

func TestHelperMethods(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	// Test shouldTransitionToWave
	require.True(engine.shouldTransitionToWave())

	// Test countVotes
	block := &MockBlock{
		id: ids.GenerateTestID(),
	}
	count := engine.countVotes(block)
	require.Equal(params.AlphaPreference, count)

	// Test collectVotes
	votes := engine.collectVotes(block)
	require.NotNil(votes)
	require.Equal(1, votes.Len())
}

func TestMultipleBlockProcessing(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Initialize(context.Background())
	require.NoError(err)

	// Process multiple blocks
	for i := 0; i < 5; i++ {
		block := &MockBlock{
			id:       ids.GenerateTestID(),
			parentID: ids.GenerateTestID(),
			height:   uint64(i + 1),
			bytes:    []byte("test block"),
			choice:   i % 2,
		}

		err = engine.ProcessBlock(context.Background(), block)
		require.NoError(err)
	}
}

func TestEngineWithDifferentParameters(t *testing.T) {
	tests := []struct {
		name   string
		params Parameters
	}{
		{
			name: "small network",
			params: Parameters{
				K:               5,
				AlphaPreference: 3,
				AlphaConfidence: 4,
				Beta:            2,
				SecurityLevel:   quasar.SecurityLow,
			},
		},
		{
			name: "large network",
			params: Parameters{
				K:               100,
				AlphaPreference: 60,
				AlphaConfidence: 80,
				Beta:            40,
				SecurityLevel:   quasar.SecurityMedium,
			},
		},
		{
			name: "max security",
			params: Parameters{
				K:               50,
				AlphaPreference: 30,
				AlphaConfidence: 40,
				Beta:            20,
				SecurityLevel:   quasar.SecurityHigh,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			ctx := context.Background()
			engine, err := New(ctx, tt.params)
			require.NoError(err)
			require.NotNil(engine)

			err = engine.Initialize(context.Background())
			require.NoError(err)

			// Process a block
			block := &MockBlock{
				id:       ids.GenerateTestID(),
				parentID: ids.GenerateTestID(),
				height:   1,
				bytes:    []byte("test block"),
				choice:   1,
			}

			err = engine.ProcessBlock(context.Background(), block)
			require.NoError(err)
		})
	}
}

func TestConcurrentBlockProcessing(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Initialize(context.Background())
	require.NoError(err)

	// Process blocks concurrently
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			block := &MockBlock{
				id:       ids.GenerateTestID(),
				parentID: ids.GenerateTestID(),
				height:   uint64(idx + 1),
				bytes:    []byte("test block"),
				choice:   idx % 3,
			}
			done <- engine.ProcessBlock(context.Background(), block)
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		require.NoError(err)
	}
}

func TestStateGetters(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	// Get state
	state := engine.State()
	require.NotNil(state)
	require.Equal(PhotonStage, state.Stage())
	require.False(state.Finalized())
}

func BenchmarkProcessBlock(b *testing.B) {
	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(b, err)

	err = engine.Initialize(context.Background())
	require.NoError(b, err)

	block := &MockBlock{
		id:       ids.GenerateTestID(),
		parentID: ids.GenerateTestID(),
		height:   1,
		bytes:    []byte("test block"),
		choice:   1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.ProcessBlock(context.Background(), block)
	}
}

func BenchmarkNew(b *testing.B) {
	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		_, _ = New(ctx, params)
	}
}

func TestBlockVerification(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	block := &MockBlock{
		id:       ids.GenerateTestID(),
		parentID: ids.GenerateTestID(),
		height:   1,
		bytes:    []byte("test block"),
		choice:   1,
	}

	// Test verifyBlockPQ (currently a stub)
	err = engine.verifyBlockPQ(block)
	require.NoError(err)
}

func TestProcessBlockWithContext(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		SecurityLevel:   quasar.SecurityLow,
	}

	ctx := context.Background()
	engine, err := New(ctx, params)
	require.NoError(err)

	// Test with cancelled context
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()

	err = engine.Initialize(ctxCancel)
	require.NoError(err) // Initialize doesn't check context yet

	// Test with timeout context
	ctxTimeout, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()

	block := &MockBlock{
		id:       ids.GenerateTestID(),
		parentID: ids.GenerateTestID(),
		height:   1,
		bytes:    []byte("test block"),
		choice:   1,
	}

	err = engine.ProcessBlock(ctxTimeout, block)
	require.NoError(err) // ProcessBlock doesn't check context yet
}
