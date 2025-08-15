// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/dag/witness"
	"github.com/luxfi/consensus/flare"
	"github.com/stretchr/testify/require"
)

// TestDAGBlock represents a block in the test DAG
type TestDAGBlock struct {
	ID          [32]byte
	Parents     [][32]byte
	Height      uint64
	Txs         []Transaction
	WitnessData []byte
	Timestamp   time.Time
}

// Transaction in the DAG
type Transaction struct {
	ID    [32]byte
	Nonce uint64
	Data  []byte
}

// TestDAG structure for testing
type TestDAG struct {
	mu      sync.RWMutex
	blocks  map[[32]byte]*TestDAGBlock
	tips    map[[32]byte]bool
	height  uint64
	witness witness.Manager
	// graph   flare.Graph // TODO: Implement graph type
	fastTxs map[TxID]int // Track transaction votes for fast path
}

type TxID [32]byte

// NewTestDAG creates a new DAG for testing
func NewTestDAG() *TestDAG {
	return &TestDAG{
		blocks: make(map[[32]byte]*TestDAGBlock),
		tips:   make(map[[32]byte]bool),
		witness: witness.NewCache(witness.Policy{
			Mode:     witness.RequireFull,
			MaxBytes: 1024 * 1024, // 1MB
		}, 1000, 10*1024*1024),
		// graph:   flare.NewGraph(), // TODO: Implement graph type
		fastTxs: make(map[TxID]int),
	}
}

// AddBlock adds a block to the test DAG
func (d *TestDAG) AddBlock(block *TestDAGBlock) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Validate parents exist
	for _, parent := range block.Parents {
		if _, exists := d.blocks[parent]; !exists {
			return errors.New("missing parent")
		}
		// Remove parent from tips
		delete(d.tips, parent)
	}

	// Add block
	d.blocks[block.ID] = block
	d.tips[block.ID] = true

	// Update height
	if block.Height > d.height {
		d.height = block.Height
	}

	// Process transactions for fast path (simulating voting)
	for _, tx := range block.Txs {
		txID := TxID(tx.ID)
		d.fastTxs[txID]++
	}

	return nil
}

// GetTips returns current DAG tips
func (d *TestDAG) GetTips() [][32]byte {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tips := make([][32]byte, 0, len(d.tips))
	for tip := range d.tips {
		tips = append(tips, tip)
	}
	return tips
}

// GetExecutableTxs returns transactions ready for execution
func (d *TestDAG) GetExecutableTxs() []TxID {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	var executable []TxID
	// With f=3, need 7 votes for fast path
	const threshold = 7
	for txID, votes := range d.fastTxs {
		if votes >= threshold {
			executable = append(executable, txID)
		}
	}
	return executable
}

// TestDAGBasic tests basic DAG operations
func TestDAGBasic(t *testing.T) {
	dag := NewTestDAG()

	// Create genesis block
	genesis := &TestDAGBlock{
		ID:        [32]byte{0},
		Parents:   nil,
		Height:    0,
		Timestamp: time.Now(),
	}

	err := dag.AddBlock(genesis)
	require.NoError(t, err)

	// Verify genesis is a tip
	tips := dag.GetTips()
	require.Len(t, tips, 1)
	require.Equal(t, genesis.ID, tips[0])

	// Add child blocks
	block1 := &TestDAGBlock{
		ID:        [32]byte{1},
		Parents:   [][32]byte{genesis.ID},
		Height:    1,
		Timestamp: time.Now(),
	}

	block2 := &TestDAGBlock{
		ID:        [32]byte{2},
		Parents:   [][32]byte{genesis.ID},
		Height:    1,
		Timestamp: time.Now(),
	}

	err = dag.AddBlock(block1)
	require.NoError(t, err)

	err = dag.AddBlock(block2)
	require.NoError(t, err)

	// Both should be tips now
	tips = dag.GetTips()
	require.Len(t, tips, 2)
}

// TestDAGWithWitness tests DAG with witness validation
func TestDAGWithWitness(t *testing.T) {
	dag := NewTestDAG()

	// Create block with witness
	witnessData := make([]byte, 1000)
	_, _ = rand.Read(witnessData)

	block := &TestDAGBlock{
		ID:          [32]byte{1},
		Parents:     nil,
		Height:      0,
		WitnessData: witnessData,
		Timestamp:   time.Now(),
	}

	// Create mock header for witness validation
	h := mockHeader{
		id:    witness.BlockID(block.ID),
		round: block.Height,
	}

	// Create payload with witness
	payload := makePayloadWithWitness(100, witnessData)

	// Validate witness
	ok, size, deltaRoot := dag.witness.Validate(h, payload)
	require.True(t, ok)
	require.Equal(t, len(witnessData), size)
	require.NotEqual(t, [32]byte{}, deltaRoot)

	// Add block to DAG
	err := dag.AddBlock(block)
	require.NoError(t, err)
}

// TestDAGFastPath tests fast path execution with DAG
func TestDAGFastPath(t *testing.T) {
	dag := NewTestDAG()

	// Create transactions
	txs := make([]Transaction, 10)
	for i := range txs {
		txs[i] = Transaction{
			ID:    [32]byte{byte(i)},
			Nonce: uint64(i),
			Data:  []byte("test transaction"),
		}
	}

	// Create blocks voting for same transactions
	for i := 0; i < 7; i++ { // Need 7 votes for f=3
		block := &TestDAGBlock{
			ID:        [32]byte{byte(100 + i)},
			Parents:   nil,
			Height:    uint64(i),
			Txs:       txs[:3], // First 3 transactions
			Timestamp: time.Now(),
		}
		err := dag.AddBlock(block)
		require.NoError(t, err)
	}

	// Check which transactions are executable
	executable := dag.GetExecutableTxs()
	require.Len(t, executable, 3) // First 3 should be executable

	for i := 0; i < 3; i++ {
		require.Contains(t, executable, TxID([32]byte{byte(i)}))
	}
}

// TestDAGConcurrent tests concurrent DAG operations
func TestDAGConcurrent(t *testing.T) {
	dag := NewTestDAG()

	// Add genesis
	genesis := &TestDAGBlock{
		ID:        [32]byte{0},
		Parents:   nil,
		Height:    0,
		Timestamp: time.Now(),
	}
	_ = dag.AddBlock(genesis)

	// Concurrent block additions
	var wg sync.WaitGroup
	numBlocks := 100

	for i := 1; i <= numBlocks; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Random parent selection
			parents := [][32]byte{genesis.ID}
			if id > 10 {
				// Can reference earlier blocks
				parentID := [32]byte{byte(id / 2)}
				dag.mu.RLock()
				_, exists := dag.blocks[parentID]
				dag.mu.RUnlock()
				if exists {
					parents = append(parents, parentID)
				}
			}

			block := &TestDAGBlock{
				ID:        [32]byte{byte(id)},
				Parents:   parents,
				Height:    uint64(id/10 + 1),
				Timestamp: time.Now(),
			}

			_ = dag.AddBlock(block)
		}(i)
	}

	wg.Wait()

	// Verify DAG consistency
	dag.mu.RLock()
	blockCount := len(dag.blocks)
	tipCount := len(dag.tips)
	dag.mu.RUnlock()

	require.GreaterOrEqual(t, blockCount, numBlocks/2) // Some may fail due to missing parents
	require.Greater(t, tipCount, 0)
}

// TestDAGMerge tests merging of DAG branches
func TestDAGMerge(t *testing.T) {
	dag := NewTestDAG()

	// Create two branches
	genesis := &TestDAGBlock{
		ID:      [32]byte{0},
		Parents: nil,
		Height:  0,
	}
	_ = dag.AddBlock(genesis)

	// Branch A
	blockA1 := &TestDAGBlock{
		ID:      [32]byte{1},
		Parents: [][32]byte{genesis.ID},
		Height:  1,
	}
	blockA2 := &TestDAGBlock{
		ID:      [32]byte{2},
		Parents: [][32]byte{{1}},
		Height:  2,
	}

	// Branch B
	blockB1 := &TestDAGBlock{
		ID:      [32]byte{11},
		Parents: [][32]byte{genesis.ID},
		Height:  1,
	}
	blockB2 := &TestDAGBlock{
		ID:      [32]byte{12},
		Parents: [][32]byte{{11}},
		Height:  2,
	}

	// Add branches
	_ = dag.AddBlock(blockA1)
	_ = dag.AddBlock(blockA2)
	_ = dag.AddBlock(blockB1)
	_ = dag.AddBlock(blockB2)

	// Merge block references both branches
	merge := &TestDAGBlock{
		ID:      [32]byte{100},
		Parents: [][32]byte{{2}, {12}},
		Height:  3,
	}
	err := dag.AddBlock(merge)
	require.NoError(t, err)

	// Only merge block should be tip
	tips := dag.GetTips()
	require.Len(t, tips, 1)
	require.Equal(t, merge.ID, tips[0])
}

// Helper functions

type mockHeader struct {
	id          witness.BlockID
	round       uint64
	parents     []witness.BlockID
	witnessRoot [32]byte
}

func (h mockHeader) ID() witness.BlockID        { return h.id }
func (h mockHeader) Round() uint64              { return h.round }
func (h mockHeader) Parents() []witness.BlockID { return h.parents }
func (h mockHeader) WitnessRoot() [32]byte      { return h.witnessRoot }

func makePayloadWithWitness(txLen int, witness []byte) []byte {
	// varint(txLen) | tx | witness
	tx := make([]byte, txLen)
	_, _ = rand.Read(tx)

	buf := make([]byte, 0, 10+txLen+len(witness))
	tmp := make([]byte, 10)
	n := binary.PutUvarint(tmp, uint64(txLen))
	buf = append(buf, tmp[:n]...)
	buf = append(buf, tx...)
	buf = append(buf, witness...)
	return buf
}

// Benchmarks

func BenchmarkDAGAddBlock(b *testing.B) {
	dag := NewTestDAG()

	// Add genesis
	genesis := &TestDAGBlock{
		ID:      [32]byte{0},
		Parents: nil,
		Height:  0,
	}
	_ = dag.AddBlock(genesis)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := &TestDAGBlock{
			ID:      [32]byte{byte(i % 256), byte(i / 256)},
			Parents: [][32]byte{genesis.ID},
			Height:  uint64(i + 1),
		}
		_ = dag.AddBlock(block)
	}
}

func BenchmarkDAGGetTips(b *testing.B) {
	dag := NewTestDAG()

	// Build a DAG with multiple tips
	for i := 0; i < 100; i++ {
		block := &TestDAGBlock{
			ID:      [32]byte{byte(i)},
			Parents: nil,
			Height:  uint64(i),
		}
		_ = dag.AddBlock(block)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dag.GetTips()
	}
}
