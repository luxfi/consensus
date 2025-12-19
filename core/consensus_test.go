package core

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing

type mockBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	bytes     []byte
	verifyErr error
	acceptErr error
	rejectErr error
}

func (m *mockBlock) ID() ids.ID                   { return m.id }
func (m *mockBlock) ParentID() ids.ID             { return m.parentID }
func (m *mockBlock) Height() uint64               { return m.height }
func (m *mockBlock) Timestamp() int64             { return m.timestamp }
func (m *mockBlock) Bytes() []byte                { return m.bytes }
func (m *mockBlock) Verify(context.Context) error { return m.verifyErr }
func (m *mockBlock) Accept(context.Context) error { return m.acceptErr }
func (m *mockBlock) Reject(context.Context) error { return m.rejectErr }

type mockState struct {
	blocks       map[ids.ID]Block
	lastAccepted ids.ID
	getBlockErr  error
	putBlockErr  error
	getLastErr   error
	setLastErr   error
}

func newMockState() *mockState {
	return &mockState{
		blocks: make(map[ids.ID]Block),
	}
}

func (m *mockState) GetBlock(id ids.ID) (Block, error) {
	if m.getBlockErr != nil {
		return nil, m.getBlockErr
	}
	if block, ok := m.blocks[id]; ok {
		return block, nil
	}
	return nil, errors.New("block not found")
}

func (m *mockState) PutBlock(block Block) error {
	if m.putBlockErr != nil {
		return m.putBlockErr
	}
	m.blocks[block.ID()] = block
	return nil
}

func (m *mockState) GetLastAccepted() (ids.ID, error) {
	if m.getLastErr != nil {
		return ids.Empty, m.getLastErr
	}
	return m.lastAccepted, nil
}

func (m *mockState) SetLastAccepted(id ids.ID) error {
	if m.setLastErr != nil {
		return m.setLastErr
	}
	m.lastAccepted = id
	return nil
}

type mockTx struct {
	id        ids.ID
	bytes     []byte
	verifyErr error
	acceptErr error
}

func (m *mockTx) ID() ids.ID                   { return m.id }
func (m *mockTx) Bytes() []byte                { return m.bytes }
func (m *mockTx) Verify(context.Context) error { return m.verifyErr }
func (m *mockTx) Accept(context.Context) error { return m.acceptErr }

type mockUTXO struct {
	id          ids.ID
	txID        ids.ID
	outputIndex uint32
	amount      uint64
	spent       bool
}

func (m *mockUTXO) ID() ids.ID          { return m.id }
func (m *mockUTXO) TxID() ids.ID        { return m.txID }
func (m *mockUTXO) OutputIndex() uint32 { return m.outputIndex }
func (m *mockUTXO) Amount() uint64      { return m.amount }
func (m *mockUTXO) IsSpent() bool       { return m.spent }

// Tests

func TestBlockInterface(t *testing.T) {
	ctx := context.Background()

	t.Run("Block operations", func(t *testing.T) {
		block := &mockBlock{
			id:        ids.GenerateTestID(),
			parentID:  ids.GenerateTestID(),
			height:    100,
			timestamp: 1234567890,
			bytes:     []byte("block data"),
		}

		require.NotEqual(t, ids.Empty, block.ID())
		require.NotEqual(t, ids.Empty, block.ParentID())
		require.Equal(t, uint64(100), block.Height())
		require.Equal(t, int64(1234567890), block.Timestamp())
		require.Equal(t, []byte("block data"), block.Bytes())

		// Test Verify
		err := block.Verify(ctx)
		require.NoError(t, err)

		block.verifyErr = errors.New("verify error")
		err = block.Verify(ctx)
		require.Error(t, err)
		block.verifyErr = nil

		// Test Accept
		err = block.Accept(ctx)
		require.NoError(t, err)

		block.acceptErr = errors.New("accept error")
		err = block.Accept(ctx)
		require.Error(t, err)
		block.acceptErr = nil

		// Test Reject
		err = block.Reject(ctx)
		require.NoError(t, err)

		block.rejectErr = errors.New("reject error")
		err = block.Reject(ctx)
		require.Error(t, err)
	})
}

func TestStateInterface(t *testing.T) {
	t.Run("State operations", func(t *testing.T) {
		state := newMockState()

		block1 := &mockBlock{
			id:        ids.GenerateTestID(),
			parentID:  ids.GenerateTestID(),
			height:    1,
			timestamp: 1000,
			bytes:     []byte("block1"),
		}

		block2 := &mockBlock{
			id:        ids.GenerateTestID(),
			parentID:  block1.id,
			height:    2,
			timestamp: 2000,
			bytes:     []byte("block2"),
		}

		// Test PutBlock
		err := state.PutBlock(block1)
		require.NoError(t, err)

		err = state.PutBlock(block2)
		require.NoError(t, err)

		// Test GetBlock
		retrieved, err := state.GetBlock(block1.ID())
		require.NoError(t, err)
		require.Equal(t, block1.ID(), retrieved.ID())

		retrieved, err = state.GetBlock(block2.ID())
		require.NoError(t, err)
		require.Equal(t, block2.ID(), retrieved.ID())

		// Test GetBlock with non-existent block
		_, err = state.GetBlock(ids.GenerateTestID())
		require.Error(t, err)

		// Test SetLastAccepted
		err = state.SetLastAccepted(block2.ID())
		require.NoError(t, err)

		// Test GetLastAccepted
		lastAccepted, err := state.GetLastAccepted()
		require.NoError(t, err)
		require.Equal(t, block2.ID(), lastAccepted)
	})

	t.Run("State error conditions", func(t *testing.T) {
		state := newMockState()

		block := &mockBlock{
			id: ids.GenerateTestID(),
		}

		// Test PutBlock error
		state.putBlockErr = errors.New("put error")
		err := state.PutBlock(block)
		require.Error(t, err)
		state.putBlockErr = nil

		// Test GetBlock error
		state.getBlockErr = errors.New("get error")
		_, err = state.GetBlock(block.ID())
		require.Error(t, err)
		state.getBlockErr = nil

		// Test SetLastAccepted error
		state.setLastErr = errors.New("set error")
		err = state.SetLastAccepted(block.ID())
		require.Error(t, err)
		state.setLastErr = nil

		// Test GetLastAccepted error
		state.getLastErr = errors.New("get error")
		_, err = state.GetLastAccepted()
		require.Error(t, err)
		state.getLastErr = nil
	})
}

func TestTxInterface(t *testing.T) {
	ctx := context.Background()

	t.Run("Tx operations", func(t *testing.T) {
		tx := &mockTx{
			id:    ids.GenerateTestID(),
			bytes: []byte("tx data"),
		}

		require.NotEqual(t, ids.Empty, tx.ID())
		require.Equal(t, []byte("tx data"), tx.Bytes())

		// Test Verify
		err := tx.Verify(ctx)
		require.NoError(t, err)

		tx.verifyErr = errors.New("verify error")
		err = tx.Verify(ctx)
		require.Error(t, err)
		tx.verifyErr = nil

		// Test Accept
		err = tx.Accept(ctx)
		require.NoError(t, err)

		tx.acceptErr = errors.New("accept error")
		err = tx.Accept(ctx)
		require.Error(t, err)
	})
}

func TestUTXOInterface(t *testing.T) {
	t.Run("UTXO operations", func(t *testing.T) {
		utxo := &mockUTXO{
			id:          ids.GenerateTestID(),
			txID:        ids.GenerateTestID(),
			outputIndex: 5,
			amount:      1000,
			spent:       false,
		}

		require.NotEqual(t, ids.Empty, utxo.ID())
		require.NotEqual(t, ids.Empty, utxo.TxID())
		require.Equal(t, uint32(5), utxo.OutputIndex())
		require.Equal(t, uint64(1000), utxo.Amount())
		require.False(t, utxo.IsSpent())

		// Test spent UTXO
		utxo.spent = true
		require.True(t, utxo.IsSpent())
	})
}

func TestStateWithMultipleBlocks(t *testing.T) {
	state := newMockState()

	// Create a chain of blocks
	blocks := make([]*mockBlock, 10)
	for i := range blocks {
		parentID := ids.Empty
		if i > 0 {
			parentID = blocks[i-1].id
		}

		blocks[i] = &mockBlock{
			id:        ids.GenerateTestID(),
			parentID:  parentID,
			height:    uint64(i),
			timestamp: int64(i * 1000),
			bytes:     []byte(string(rune('a' + i))),
		}

		err := state.PutBlock(blocks[i])
		require.NoError(t, err)
	}

	// Verify all blocks are stored
	for _, block := range blocks {
		retrieved, err := state.GetBlock(block.ID())
		require.NoError(t, err)
		require.Equal(t, block.ID(), retrieved.ID())
		require.Equal(t, block.Height(), retrieved.Height())
	}

	// Set and verify last accepted
	err := state.SetLastAccepted(blocks[len(blocks)-1].ID())
	require.NoError(t, err)

	lastAccepted, err := state.GetLastAccepted()
	require.NoError(t, err)
	require.Equal(t, blocks[len(blocks)-1].ID(), lastAccepted)
}

func TestInterfaceCompliance(t *testing.T) {
	// Verify mock implementations satisfy interfaces
	var _ BlockState = (*mockState)(nil)
	var _ Block = (*mockBlock)(nil)
	var _ Tx = (*mockTx)(nil)
	var _ UTXO = (*mockUTXO)(nil)
}

func BenchmarkStateOperations(b *testing.B) {
	state := newMockState()
	blocks := make([]*mockBlock, 100)

	for i := range blocks {
		blocks[i] = &mockBlock{
			id:        ids.GenerateTestID(),
			parentID:  ids.GenerateTestID(),
			height:    uint64(i),
			timestamp: int64(i * 1000),
			bytes:     []byte("benchmark block"),
		}
	}

	b.ResetTimer()

	b.Run("PutBlock", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = state.PutBlock(blocks[i%len(blocks)])
		}
	})

	b.Run("GetBlock", func(b *testing.B) {
		// Pre-populate state
		for _, block := range blocks {
			_ = state.PutBlock(block)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = state.GetBlock(blocks[i%len(blocks)].ID())
		}
	})

	b.Run("SetGetLastAccepted", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = state.SetLastAccepted(blocks[i%len(blocks)].ID())
			_, _ = state.GetLastAccepted()
		}
	})
}

func BenchmarkBlockOperations(b *testing.B) {
	ctx := context.Background()
	block := &mockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.GenerateTestID(),
		height:    100,
		timestamp: 1234567890,
		bytes:     []byte("benchmark block data"),
	}

	b.Run("ID", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = block.ID()
		}
	})

	b.Run("Verify", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = block.Verify(ctx)
		}
	})

	b.Run("Accept", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = block.Accept(ctx)
		}
	})
}
