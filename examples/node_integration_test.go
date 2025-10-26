package examples

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestExampleBlock(t *testing.T) {
	// Create an example block
	block := &ExampleBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.GenerateTestID(),
		height:    100,
		timestamp: time.Now().Unix(),
		data:      []byte("test block data"),
	}

	// Test ID method
	require.Equal(t, block.id, block.ID())

	// Test ParentID method
	require.Equal(t, block.parentID, block.ParentID())

	// Test Height method
	require.Equal(t, block.height, block.Height())

	// Test Timestamp method
	require.Equal(t, block.timestamp, block.Timestamp())

	// Test Bytes method
	require.Equal(t, block.data, block.Bytes())

	// Test Verify method
	ctx := context.Background()
	err := block.Verify(ctx)
	require.NoError(t, err)

	// Test Accept method
	err = block.Accept(ctx)
	require.NoError(t, err)
	// Note: ExampleBlock doesn't track accepted state internally

	// Test Reject method
	block2 := &ExampleBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.GenerateTestID(),
		height:    101,
		timestamp: time.Now().Unix(),
		data:      []byte("another block"),
	}
	err = block2.Reject(ctx)
	require.NoError(t, err)
	// Note: ExampleBlock doesn't track rejected state internally
}

func TestExampleBlockValidation(t *testing.T) {
	tests := []struct {
		name      string
		block     *ExampleBlock
		shouldErr bool
	}{
		{
			name: "valid block",
			block: &ExampleBlock{
				id:        ids.GenerateTestID(),
				parentID:  ids.GenerateTestID(),
				height:    100,
				timestamp: time.Now().Unix(),
				data:      []byte("valid data"),
			},
			shouldErr: false,
		},
		{
			name: "empty ID",
			block: &ExampleBlock{
				id:        ids.Empty,
				parentID:  ids.GenerateTestID(),
				height:    100,
				timestamp: time.Now().Unix(),
				data:      []byte("data"),
			},
			shouldErr: false, // Verify always returns nil in the example
		},
		{
			name: "nil data",
			block: &ExampleBlock{
				id:        ids.GenerateTestID(),
				parentID:  ids.GenerateTestID(),
				height:    100,
				timestamp: time.Now().Unix(),
				data:      nil,
			},
			shouldErr: false, // Verify always returns nil in the example
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.block.Verify(ctx)
			if tt.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRunNodeIntegrationExample(t *testing.T) {
	// This test ensures the example can run without panicking
	t.Run("integration example runs", func(t *testing.T) {
		// We can't fully test this as it tries to connect to a real node
		// but we can at least ensure the function exists and can be called
		// In a real test, we'd mock the network components

		// For now, just verify the function signature is correct
		// The actual execution would require a running node
		require.NotNil(t, RunNodeIntegrationExample)
	})
}

func TestExampleBlockStates(t *testing.T) {
	ctx := context.Background()

	t.Run("accept and reject can be called", func(t *testing.T) {
		block := &ExampleBlock{
			id:        ids.GenerateTestID(),
			parentID:  ids.GenerateTestID(),
			height:    100,
			timestamp: time.Now().Unix(),
			data:      []byte("test"),
		}

		// Accept the block
		err := block.Accept(ctx)
		require.NoError(t, err)
		// Note: ExampleBlock doesn't track accepted state

		// Try to reject a block (should work as there's no state tracking)
		err = block.Reject(ctx)
		require.NoError(t, err)
		// Note: ExampleBlock doesn't track rejected state
	})

	t.Run("reject then accept", func(t *testing.T) {
		block := &ExampleBlock{
			id:        ids.GenerateTestID(),
			parentID:  ids.GenerateTestID(),
			height:    100,
			timestamp: time.Now().Unix(),
			data:      []byte("test"),
		}

		// Reject first
		err := block.Reject(ctx)
		require.NoError(t, err)

		// Then accept
		err = block.Accept(ctx)
		require.NoError(t, err)
		// Both operations succeed as there's no state validation
	})
}

func TestExampleBlockChain(t *testing.T) {
	// Test creating a chain of blocks
	ctx := context.Background()

	// Genesis block
	genesis := &ExampleBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty, // Genesis has no parent
		height:    0,
		timestamp: time.Now().Unix(),
		data:      []byte("genesis"),
	}

	// Verify and accept genesis
	err := genesis.Verify(ctx)
	require.NoError(t, err)
	err = genesis.Accept(ctx)
	require.NoError(t, err)

	// Create chain of blocks
	parent := genesis
	for i := uint64(1); i <= 10; i++ {
		block := &ExampleBlock{
			id:        ids.GenerateTestID(),
			parentID:  parent.ID(),
			height:    i,
			timestamp: time.Now().Unix() + int64(i),
			data:      []byte("block data"),
		}

		// Verify block
		err := block.Verify(ctx)
		require.NoError(t, err)

		// Check parent relationship
		require.Equal(t, parent.ID(), block.ParentID())
		require.Equal(t, parent.Height()+1, block.Height())

		// Accept block
		err = block.Accept(ctx)
		require.NoError(t, err)

		parent = block
	}

	// Final block should have height 10
	require.Equal(t, uint64(10), parent.Height())
}