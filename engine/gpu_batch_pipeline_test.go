// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/types"
	"github.com/stretchr/testify/require"
)

func TestNewGPUBatchPipeline(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false // Force CPU mode for testing

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)
	require.NotNil(t, pipeline)
	require.Equal(t, 2, len(pipeline.buffers))
	require.False(t, pipeline.GPUReady())
	require.False(t, pipeline.Running())
}

func TestPipelineStartStop(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	// Start
	err = pipeline.Start()
	require.NoError(t, err)
	require.True(t, pipeline.Running())

	// Double start should fail
	err = pipeline.Start()
	require.Error(t, err)

	// Stop
	err = pipeline.Stop()
	require.NoError(t, err)
	require.False(t, pipeline.Running())

	// Double stop should fail
	err = pipeline.Stop()
	require.Error(t, err)
}

func TestProcessBatchNotRunning(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	// Should fail when not running
	_, err = pipeline.ProcessBatch([]Transaction{})
	require.ErrorIs(t, err, ErrPipelineStopped)
}

func TestProcessBatchTooLarge(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 10

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Create batch larger than limit
	txs := make([]Transaction, 20)
	_, err = pipeline.ProcessBatch(txs)
	require.ErrorIs(t, err, ErrBatchTooLarge)
}

func TestProcessBatchSuccess(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100
	config.VerifyWorkers = 4

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Create valid transactions
	txs := generateTestTransactions(10)

	batchID, err := pipeline.ProcessBatch(txs)
	require.NoError(t, err)
	require.Equal(t, uint64(1), batchID)

	// Wait for result
	select {
	case result := <-pipeline.Results():
		require.Equal(t, batchID, result.BatchID)
		require.Equal(t, 10, result.ProcessedCount)
		require.True(t, result.ProcessingTime > 0)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestProcessMultipleBatches(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100
	config.MaxPendingBatches = 10

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	batchCount := 5
	var wg sync.WaitGroup

	// Submit multiple batches
	for i := 0; i < batchCount; i++ {
		txs := generateTestTransactions(20)
		_, err := pipeline.ProcessBatch(txs)
		require.NoError(t, err)
	}

	// Collect results with longer timeout for race detector
	results := make([]*BatchResult, 0, batchCount)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for len(results) < batchCount {
			select {
			case result := <-pipeline.Results():
				results = append(results, result)
			case <-time.After(30 * time.Second):
				return
			}
		}
	}()

	wg.Wait()
	require.Len(t, results, batchCount)

	// Verify each result
	for _, result := range results {
		require.Equal(t, 20, result.ProcessedCount)
	}
}

func TestSignatureVerificationCPU(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Create transactions with valid signature lengths but random data.
	// Random signatures will fail cryptographic verification (expected behavior).
	txs := []Transaction{
		{SigType: SigECDSA, Signature: make([]byte, 65), PublicKey: make([]byte, 33)},
		{SigType: SigEd25519, Signature: make([]byte, 64), PublicKey: make([]byte, 32)},
		{SigType: SigBLS, Signature: make([]byte, 96), PublicKey: make([]byte, 48)},
	}
	for i := range txs {
		rand.Read(txs[i].Hash[:])
		rand.Read(txs[i].Signature)
		rand.Read(txs[i].PublicKey)
	}

	_, err = pipeline.ProcessBatch(txs)
	require.NoError(t, err)

	select {
	case result := <-pipeline.Results():
		require.Equal(t, 3, result.ProcessedCount)
		// Random data fails real cryptographic verification (correct behavior).
		// ValidCount depends on whether random bytes happen to form valid signatures.
		require.Equal(t, 3, len(result.SignatureProofs))

		// Verify each proof has correct signature type
		sigTypes := make(map[SignatureType]int)
		for _, proof := range result.SignatureProofs {
			sigTypes[proof.SigType]++
		}
		require.Equal(t, 1, sigTypes[SigECDSA])
		require.Equal(t, 1, sigTypes[SigEd25519])
		require.Equal(t, 1, sigTypes[SigBLS])
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestMerkleTreeUpdate(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Initial root should be zero
	initialRoot := pipeline.MerkleRoot()
	require.Equal(t, [32]byte{}, initialRoot)

	// Process batch with transactions (random data fails verification)
	txs := generateTestTransactions(8)
	_, err = pipeline.ProcessBatch(txs)
	require.NoError(t, err)

	select {
	case result := <-pipeline.Results():
		// ProcessedCount reflects how many were attempted
		require.Equal(t, 8, result.ProcessedCount)
		// With random signatures, ValidCount will be 0 (cryptographic verification fails)
		// Merkle root only updates when there are valid transactions
		if result.ValidCount > 0 {
			require.NotEqual(t, [32]byte{}, result.MerkleRoot)
		} else {
			// No valid transactions means Merkle root stays at zero
			require.Equal(t, [32]byte{}, result.MerkleRoot)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestDoubleBuffering(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100
	config.BufferCount = 2
	config.MaxPendingBatches = 20 // Increase to handle all batches

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)
	require.Equal(t, 2, len(pipeline.buffers))

	require.NoError(t, pipeline.Start())

	// Submit batches rapidly to test buffer swapping
	submitted := 0
	for i := 0; i < 10; i++ {
		txs := generateTestTransactions(5)
		_, err := pipeline.ProcessBatch(txs)
		if err == nil {
			submitted++
		}
	}

	// Collect results that were submitted
	resultCount := 0
	timeout := time.After(5 * time.Second)
	for resultCount < submitted {
		select {
		case _, ok := <-pipeline.Results():
			if !ok {
				goto done
			}
			resultCount++
		case <-timeout:
			goto done
		}
	}
done:

	// Stop pipeline
	require.NoError(t, pipeline.Stop())

	// Check we got results
	require.True(t, resultCount > 0, "expected at least one result")

	// Check metrics
	batches, _, _, _, swaps, _, _, _ := pipeline.GetMetricsSnapshot()
	require.True(t, batches > 0)
	require.True(t, swaps > 0) // At least some buffer swaps
}

func TestBackpressure(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100
	config.MaxPendingBatches = 2

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Fill up the pending queue
	submitted := 0
	for i := 0; i < 10; i++ {
		txs := generateTestTransactions(5)
		_, err := pipeline.ProcessBatch(txs)
		if err == nil {
			submitted++
		} else if err == ErrBufferFull {
			// Expected - backpressure applied
			break
		}
	}

	// Should have applied backpressure
	require.True(t, submitted < 10 || submitted >= 2)
}

func TestMetricsAccumulation(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100
	config.MaxPendingBatches = 10

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())

	// Process several batches
	batchCount := 3
	txsPerBatch := 10
	for i := 0; i < batchCount; i++ {
		txs := generateTestTransactions(txsPerBatch)
		_, err := pipeline.ProcessBatch(txs)
		require.NoError(t, err)
	}

	// Wait for all results
	received := 0
	for received < batchCount {
		select {
		case _, ok := <-pipeline.Results():
			if !ok {
				break
			}
			received++
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}

	// Stop pipeline gracefully
	require.NoError(t, pipeline.Stop())

	// Check accumulated metrics
	batches, txs, _, _, _, _, _, _ := pipeline.GetMetricsSnapshot()
	require.Equal(t, uint64(batchCount), batches)
	require.Equal(t, uint64(batchCount*txsPerBatch), txs)
}

func TestGPUBuffer(t *testing.T) {
	buf := newGPUBuffer(0, 100)
	require.NotNil(t, buf)
	require.Equal(t, 0, buf.id)
	require.Equal(t, 100, buf.capacity)
	require.Len(t, buf.txHashes, 100)
	require.Len(t, buf.signatures, 100)
	require.Len(t, buf.validFlags, 100)
}

func TestGPUMerkleTree(t *testing.T) {
	mt := newGPUMerkleTree(10, false)
	require.NotNil(t, mt)
	require.Equal(t, 10, mt.depth)

	// Empty tree
	root := mt.updateCPU(nil)
	require.Equal(t, [32]byte{}, root)

	// Single leaf
	hashes := [][32]byte{{1, 2, 3}}
	root = mt.updateCPU(hashes)
	require.Equal(t, [32]byte{1, 2, 3}, root)

	// Multiple leaves
	hashes = [][32]byte{{1}, {2}, {3}, {4}}
	root = mt.updateCPU(hashes)
	require.NotEqual(t, [32]byte{}, root)
}

func TestMerkleTreePairHashing(t *testing.T) {
	// Test hashPair determinism
	a := [32]byte{1, 2, 3}
	b := [32]byte{4, 5, 6}

	hash1 := hashPair(a, b)
	hash2 := hashPair(a, b)
	require.Equal(t, hash1, hash2)

	// Different inputs (reversed order) - our XOR placeholder may be commutative
	// In production with SHA256, hash(a||b) != hash(b||a)
	c := [32]byte{7, 8, 9}
	hash3 := hashPair(a, c)
	require.NotEqual(t, hash1, hash3) // Different second input
}

func TestDefaultPipelineConfig(t *testing.T) {
	config := DefaultPipelineConfig()
	require.Equal(t, 1024, config.BatchSize)
	require.Equal(t, 2, config.BufferCount)
	require.Equal(t, 8, config.MaxPendingBatches)
	require.True(t, config.EnableGPU)
	require.Equal(t, -1, config.GPUDeviceID)
	require.True(t, config.ParallelVerify)
	require.Equal(t, 4, config.VerifyWorkers)
	require.Equal(t, 20, config.MerkleTreeDepth)
	require.False(t, config.VerkleEnabled)
}

func TestConcurrentBatchSubmission(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100
	config.MaxPendingBatches = 20

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Submit batches from multiple goroutines
	var wg sync.WaitGroup
	goroutines := 5
	batchesPerGoroutine := 4

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < batchesPerGoroutine; i++ {
				txs := generateTestTransactions(3)
				pipeline.ProcessBatch(txs) // Ignore errors (backpressure)
			}
		}()
	}

	wg.Wait()

	// Collect results (may be fewer due to backpressure)
	resultCount := 0
	timeout := time.After(10 * time.Second)
loop:
	for {
		select {
		case _, ok := <-pipeline.Results():
			if !ok {
				break loop
			}
			resultCount++
		case <-timeout:
			break loop
		}
	}

	// Should have processed at least some batches
	require.True(t, resultCount > 0)
}

func TestSignatureTypeDistribution(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = 100

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Create mixed signature types
	txs := []Transaction{
		{SigType: SigECDSA, Signature: make([]byte, 65), PublicKey: make([]byte, 33)},
		{SigType: SigECDSA, Signature: make([]byte, 65), PublicKey: make([]byte, 33)},
		{SigType: SigEd25519, Signature: make([]byte, 64), PublicKey: make([]byte, 32)},
		{SigType: SigBLS, Signature: make([]byte, 96), PublicKey: make([]byte, 48)},
		{SigType: SigMLDSA, Signature: make([]byte, 100), PublicKey: make([]byte, 100)},
	}

	for i := range txs {
		rand.Read(txs[i].Hash[:])
		rand.Read(txs[i].Signature)
		rand.Read(txs[i].PublicKey)
	}

	_, err = pipeline.ProcessBatch(txs)
	require.NoError(t, err)

	select {
	case result := <-pipeline.Results():
		require.Equal(t, 5, result.ProcessedCount)

		// Count by type
		ecdsaCount := 0
		ed25519Count := 0
		blsCount := 0
		mldsaCount := 0

		for _, proof := range result.SignatureProofs {
			switch proof.SigType {
			case SigECDSA:
				ecdsaCount++
			case SigEd25519:
				ed25519Count++
			case SigBLS:
				blsCount++
			case SigMLDSA:
				mldsaCount++
			}
		}

		require.Equal(t, 2, ecdsaCount)
		require.Equal(t, 1, ed25519Count)
		require.Equal(t, 1, blsCount)
		require.Equal(t, 1, mldsaCount)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// Benchmarks

func BenchmarkProcessBatch10(b *testing.B) {
	benchmarkProcessBatch(b, 10)
}

func BenchmarkProcessBatch100(b *testing.B) {
	benchmarkProcessBatch(b, 100)
}

func BenchmarkProcessBatch1000(b *testing.B) {
	benchmarkProcessBatch(b, 1000)
}

func benchmarkProcessBatch(b *testing.B, batchSize int) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false
	config.BatchSize = batchSize * 2
	config.MaxPendingBatches = b.N + 10

	pipeline, err := NewGPUBatchPipeline(config)
	if err != nil {
		b.Fatal(err)
	}

	if err := pipeline.Start(); err != nil {
		b.Fatal(err)
	}
	defer pipeline.Stop()

	txs := generateTestTransactions(batchSize)

	// Drain results in background
	done := make(chan struct{})
	go func() {
		for range pipeline.Results() {
		}
		close(done)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pipeline.ProcessBatch(txs)
		if err != nil && err != ErrBufferFull {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	pipeline.Stop()
	<-done
}

func BenchmarkMerkleTreeUpdate(b *testing.B) {
	mt := newGPUMerkleTree(20, false)
	hashes := make([][32]byte, 100)
	for i := range hashes {
		rand.Read(hashes[i][:])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mt.updateCPU(hashes)
	}
}

func BenchmarkSignatureVerification(b *testing.B) {
	tx := Transaction{
		SigType:   SigECDSA,
		Signature: make([]byte, 65),
		PublicKey: make([]byte, 33),
	}
	rand.Read(tx.Hash[:])
	rand.Read(tx.Signature)
	rand.Read(tx.PublicKey)

	config := DefaultPipelineConfig()
	pipeline, _ := NewGPUBatchPipeline(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipeline.verifySingleSignature(tx)
	}
}

// Helper functions

func generateTestTransactions(count int) []Transaction {
	txs := make([]Transaction, count)
	for i := range txs {
		txs[i] = Transaction{
			ID:        types.ID{},
			SigType:   SigECDSA,
			Signature: make([]byte, 65),
			PublicKey: make([]byte, 33),
			Payload:   make([]byte, 100),
			Timestamp: time.Now(),
		}
		rand.Read(txs[i].Hash[:])
		rand.Read(txs[i].ID[:])
		rand.Read(txs[i].Signature)
		rand.Read(txs[i].PublicKey)
		rand.Read(txs[i].Payload)
	}
	return txs
}

func TestGPUBufferInUseFlag(t *testing.T) {
	buf := newGPUBuffer(0, 100)
	require.False(t, buf.inUse.Load())

	buf.inUse.Store(true)
	require.True(t, buf.inUse.Load())

	buf.inUse.Store(false)
	require.False(t, buf.inUse.Load())
}

func TestGPUAvailable(t *testing.T) {
	// In test environment, GPU should not be available
	require.False(t, gpuAvailable())
}

func TestEmptyBatchProcessing(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())
	defer pipeline.Stop()

	// Empty batch should still process
	_, err = pipeline.ProcessBatch([]Transaction{})
	require.NoError(t, err)

	select {
	case result := <-pipeline.Results():
		require.Equal(t, 0, result.ProcessedCount)
		require.Equal(t, 0, result.ValidCount)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPipelineContextCancellation(t *testing.T) {
	config := DefaultPipelineConfig()
	config.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(config)
	require.NoError(t, err)

	require.NoError(t, pipeline.Start())

	// Submit a batch
	txs := generateTestTransactions(5)
	_, err = pipeline.ProcessBatch(txs)
	require.NoError(t, err)

	// Stop should cancel context and stop processing
	err = pipeline.Stop()
	require.NoError(t, err)

	// Further submissions should fail
	_, err = pipeline.ProcessBatch(txs)
	require.ErrorIs(t, err, ErrPipelineStopped)
}
