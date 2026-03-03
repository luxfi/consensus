// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/luxfi/ids"
)

// --- Metrics ---

func TestPipelineMetrics(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(cfg)
	if err != nil {
		t.Fatalf("NewGPUBatchPipeline failed: %v", err)
	}

	m := pipeline.Metrics()
	if m == nil {
		t.Fatal("Metrics should not be nil")
	}

	// Check initial values
	batches, txs, valid, invalid, swaps, bp, gpuNs, cpuNs := pipeline.GetMetricsSnapshot()
	if batches != 0 || txs != 0 || valid != 0 || invalid != 0 || swaps != 0 || bp != 0 || gpuNs != 0 || cpuNs != 0 {
		t.Error("all metrics should be 0 initially")
	}
}

func TestMerkleRootInitial(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(cfg)
	if err != nil {
		t.Fatalf("NewGPUBatchPipeline failed: %v", err)
	}

	root := pipeline.MerkleRoot()
	// Initially zero since no transactions processed
	if root != [32]byte{} {
		t.Error("MerkleRoot should be zero initially")
	}
}

// --- FlushPending ---

func TestFlushPendingEmpty(t *testing.T) {
	mt := &GPUMerkleTree{
		pending: make([][32]byte, 0),
	}
	// Should not panic on empty
	mt.flushPending(false)
}

func TestFlushPendingCPU(t *testing.T) {
	mt := &GPUMerkleTree{
		pending: make([][32]byte, 0, 1024),
	}

	// Add some hashes
	h := sha256.Sum256([]byte("test"))
	mt.pending = append(mt.pending, h)

	mt.flushPending(false) // CPU path

	// Pending should be drained
	mt.pendMu.Lock()
	if len(mt.pending) != 0 {
		t.Errorf("pending should be empty after flush, got %d", len(mt.pending))
	}
	mt.pendMu.Unlock()

	// Root should be updated
	mt.rootMu.RLock()
	if mt.root == ([32]byte{}) {
		t.Error("root should be non-zero after flush with data")
	}
	mt.rootMu.RUnlock()
}

func TestFlushPendingGPUFallback(t *testing.T) {
	mt := &GPUMerkleTree{
		pending: make([][32]byte, 0, 1024),
	}

	h := sha256.Sum256([]byte("test"))
	mt.pending = append(mt.pending, h)

	// GPU path (falls back to CPU since no real GPU)
	mt.flushPending(true)

	mt.pendMu.Lock()
	if len(mt.pending) != 0 {
		t.Errorf("pending should be empty after GPU flush, got %d", len(mt.pending))
	}
	mt.pendMu.Unlock()
}

// --- updateGPU ---

func TestUpdateGPUEmptyPending(t *testing.T) {
	mt := &GPUMerkleTree{
		pending: make([][32]byte, 0),
	}

	// Set a known root
	mt.root = sha256.Sum256([]byte("existing"))

	// updateGPU with empty pending should return existing root
	root := mt.updateGPU(nil)
	if root != mt.root {
		t.Error("updateGPU with nil hashes should return existing root")
	}
}

func TestUpdateGPUWithHashes(t *testing.T) {
	mt := &GPUMerkleTree{
		pending: make([][32]byte, 0),
	}

	hashes := [][32]byte{
		sha256.Sum256([]byte("a")),
		sha256.Sum256([]byte("b")),
	}

	root := mt.updateGPU(hashes)
	if root == ([32]byte{}) {
		t.Error("updateGPU should produce non-zero root")
	}
}

// --- updateMerkleTree ---

func TestUpdateMerkleTreeNoValidTx(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}

	buf := &GPUBuffer{
		count:      2,
		validFlags: []bool{false, false},
		txHashes:   make([][32]byte, 2),
	}
	result := &BatchResult{}

	pipeline.updateMerkleTree(buf, result)
	if result.MerkleRoot != ([32]byte{}) {
		t.Error("no valid txs should not update merkle root")
	}
}

func TestUpdateMerkleTreeCPU(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.EnableGPU = false

	pipeline, err := NewGPUBatchPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}

	buf := &GPUBuffer{
		count:      2,
		validFlags: []bool{true, true},
		txHashes: [][32]byte{
			sha256.Sum256([]byte("tx1")),
			sha256.Sum256([]byte("tx2")),
		},
	}
	result := &BatchResult{}

	pipeline.updateMerkleTree(buf, result)
	if result.MerkleRoot == ([32]byte{}) {
		t.Error("valid txs should update merkle root")
	}
}

// --- Signature verification edge cases ---

func TestVerifyECDSAShortSig(t *testing.T) {
	if verifyECDSA([]byte("hash"), []byte("short"), []byte("pubkey")) {
		t.Error("should reject short signature")
	}
}

func TestVerifyEd25519WrongSizes(t *testing.T) {
	// Wrong sig size
	if verifyEd25519([]byte("hash"), make([]byte, 10), make([]byte, ed25519.PublicKeySize)) {
		t.Error("should reject wrong sig size")
	}
	// Wrong pubkey size
	if verifyEd25519([]byte("hash"), make([]byte, ed25519.SignatureSize), make([]byte, 10)) {
		t.Error("should reject wrong pubkey size")
	}
}

func TestVerifyBLSInvalidKey(t *testing.T) {
	if verifyBLS([]byte("hash"), []byte("sig"), []byte("bad-pubkey")) {
		t.Error("should reject invalid BLS pubkey")
	}
}

func TestVerifyBLSInvalidSig(t *testing.T) {
	// Valid-length pubkey (48 bytes) but garbage
	pubkey := make([]byte, 48)
	if verifyBLS([]byte("hash"), []byte("bad-sig"), pubkey) {
		t.Error("should reject invalid BLS signature with garbage key")
	}
}

func TestVerifyMLDSAWrongKeySize(t *testing.T) {
	// Key size that doesn't match any ML-DSA mode
	if verifyMLDSA([]byte("hash"), []byte("sig"), make([]byte, 100)) {
		t.Error("should reject unknown key size")
	}
}

func TestVerifyMLDSAInvalidKey44(t *testing.T) {
	// MLDSA44 key size (1312) but garbage content
	if verifyMLDSA([]byte("hash"), make([]byte, 2420), make([]byte, 1312)) {
		t.Error("should reject invalid MLDSA44 key")
	}
}

func TestVerifyMLDSAInvalidKey65(t *testing.T) {
	// MLDSA65 key size (1952) but garbage content
	if verifyMLDSA([]byte("hash"), make([]byte, 3293), make([]byte, 1952)) {
		t.Error("should reject invalid MLDSA65 key")
	}
}

func TestVerifyMLDSAInvalidKey87(t *testing.T) {
	// MLDSA87 key size (2592) but garbage content
	if verifyMLDSA([]byte("hash"), make([]byte, 4595), make([]byte, 2592)) {
		t.Error("should reject invalid MLDSA87 key")
	}
}

// --- WithTransport ---

func TestWithTransport(t *testing.T) {
	opts := &options{}
	WithTransport(nil)(opts)
	if opts.transport != nil {
		t.Error("transport should be nil when set to nil")
	}
}

// --- LuxConsensus ---

func TestNewLuxConsensusWithTransport(t *testing.T) {
	lc := NewLuxConsensus(3, 2, 2, WithTransport(nil))
	if lc == nil {
		t.Fatal("NewLuxConsensus should not return nil")
	}
}

func TestLuxConsensusPollDecidedSkip(t *testing.T) {
	lc := NewLuxConsensus(1, 1, 1)

	item := ids.GenerateTestID()
	// Vote and poll to decide
	lc.RecordVote(item)
	lc.Poll(map[ids.ID]int{item: 1})

	// Second poll for same item should be a no-op (already decided)
	lc.Poll(map[ids.ID]int{item: 1})
}
