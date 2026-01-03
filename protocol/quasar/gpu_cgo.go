//go:build cgo
// +build cgo

// Package quasar provides GPU-accelerated consensus operations.
// This file provides the GPU orchestrator that integrates:
// - crypto/gpu for BLS, ML-DSA, and hash operations
// - lattice/gpu for NTT operations (used by Corona)
//
// The GPU orchestrator enables full hardware acceleration for
// consensus operations, achieving high throughput signature
// verification and aggregation.
package quasar

import (
	"errors"
	"sync"

	cryptogpu "github.com/luxfi/crypto/gpu"
	coronagpu "github.com/luxfi/corona/gpu"
)

// GPUOrchestrator coordinates GPU-accelerated cryptographic operations
// for consensus. It integrates BLS signatures, ML-DSA post-quantum
// signatures, and NTT-accelerated Corona operations.
type GPUOrchestrator struct {
	mu sync.RWMutex

	// Configuration
	enabled    bool
	backend    string
	batchSize  int
	maxWorkers int

	// Corona GPU context for post-quantum threshold signatures
	corona *coronagpu.CoronaGPU

	// State
	initialized bool
}

// GPUConfig configures the GPU orchestrator
type GPUConfig struct {
	// Enable GPU acceleration (default: auto-detect)
	Enabled *bool

	// BatchSize for parallel operations (default: 100)
	BatchSize int

	// MaxWorkers for CPU fallback (default: runtime.NumCPU())
	MaxWorkers int
}

// DefaultGPUConfig returns sensible defaults for GPU acceleration
func DefaultGPUConfig() GPUConfig {
	return GPUConfig{
		BatchSize:  100,
		MaxWorkers: 8,
	}
}

// NewGPUOrchestrator creates a new GPU orchestrator for consensus operations
func NewGPUOrchestrator(cfg GPUConfig) (*GPUOrchestrator, error) {
	o := &GPUOrchestrator{
		batchSize:  cfg.BatchSize,
		maxWorkers: cfg.MaxWorkers,
	}

	if o.batchSize <= 0 {
		o.batchSize = 100
	}
	if o.maxWorkers <= 0 {
		o.maxWorkers = 8
	}

	// Auto-detect GPU availability if not explicitly set
	if cfg.Enabled != nil {
		o.enabled = *cfg.Enabled
	} else {
		o.enabled = cryptogpu.GPUAvailable()
	}

	o.backend = cryptogpu.GetBackend()

	// Initialize Corona GPU for post-quantum threshold signatures
	corona, err := coronagpu.NewCoronaGPU(coronagpu.DefaultConfig())
	if err != nil {
		return nil, err
	}
	o.corona = corona

	o.initialized = true

	return o, nil
}

// Close releases all GPU resources
func (o *GPUOrchestrator) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.corona != nil {
		o.corona.Close()
		o.corona = nil
	}
	o.initialized = false
}

// IsGPUEnabled returns true if GPU acceleration is available and enabled
func (o *GPUOrchestrator) IsGPUEnabled() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.enabled
}

// GetBackend returns the active backend name ("Metal", "CUDA", or "CPU")
func (o *GPUOrchestrator) GetBackend() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.backend
}

// =============================================================================
// BLS Operations (GPU-accelerated)
// =============================================================================

// BLSBatchVerify verifies multiple BLS signatures in parallel using GPU
// Returns a slice of verification results (true=valid, false=invalid)
func (o *GPUOrchestrator) BLSBatchVerify(sigs, pks, msgs [][]byte) ([]bool, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.BLSBatchVerify(sigs, pks, msgs)
}

// BLSAggregateSignatures aggregates multiple BLS signatures using GPU
func (o *GPUOrchestrator) BLSAggregateSignatures(sigs [][]byte) ([]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.BLSAggregateSignatures(sigs)
}

// BLSAggregatePublicKeys aggregates multiple BLS public keys using GPU
func (o *GPUOrchestrator) BLSAggregatePublicKeys(pks [][]byte) ([]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.BLSAggregatePublicKeys(pks)
}

// BLSSign signs a message with a BLS secret key using GPU
func (o *GPUOrchestrator) BLSSign(sk, msg []byte) ([]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.BLSSign(sk, msg)
}

// BLSVerify verifies a BLS signature using GPU
func (o *GPUOrchestrator) BLSVerify(sig, pk, msg []byte) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return false
	}

	return cryptogpu.BLSVerify(sig, pk, msg)
}

// =============================================================================
// ML-DSA (Post-Quantum) Operations (GPU-accelerated)
// =============================================================================

// MLDSASign signs a message with ML-DSA using GPU
func (o *GPUOrchestrator) MLDSASign(sk, msg []byte) ([]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.MLDSASign(sk, msg)
}

// MLDSAVerify verifies an ML-DSA signature using GPU
func (o *GPUOrchestrator) MLDSAVerify(sig, msg, pk []byte) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return false
	}

	return cryptogpu.MLDSAVerify(sig, msg, pk)
}

// MLDSABatchVerify verifies multiple ML-DSA signatures in parallel using GPU
func (o *GPUOrchestrator) MLDSABatchVerify(sigs, msgs [][]byte, pks [][]byte) ([]bool, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.MLDSABatchVerify(sigs, msgs, pks)
}

// =============================================================================
// Threshold Operations (GPU-accelerated)
// =============================================================================

// GPUThresholdContext wraps the C++ threshold context for GPU operations
type GPUThresholdContext struct {
	ctx *cryptogpu.ThresholdContext
}

// NewGPUThresholdContext creates a GPU-accelerated threshold context
func (o *GPUOrchestrator) NewThresholdContext(t, n uint32) (*GPUThresholdContext, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	ctx, err := cryptogpu.NewThresholdContext(t, n)
	if err != nil {
		return nil, err
	}

	return &GPUThresholdContext{ctx: ctx}, nil
}

// Close releases the threshold context resources
func (tc *GPUThresholdContext) Close() {
	if tc.ctx != nil {
		tc.ctx.Close()
	}
}

// Keygen generates threshold key shares using GPU
func (tc *GPUThresholdContext) Keygen(seed []byte) (shares [][]byte, pk []byte, err error) {
	return tc.ctx.Keygen(seed)
}

// PartialSign creates a partial signature share using GPU
func (tc *GPUThresholdContext) PartialSign(shareIndex uint32, share, msg []byte) ([]byte, error) {
	return tc.ctx.PartialSign(shareIndex, share, msg)
}

// Combine combines partial signatures using GPU-accelerated Lagrange interpolation
func (tc *GPUThresholdContext) Combine(partialSigs [][]byte, indices []uint32) ([]byte, error) {
	return tc.ctx.Combine(partialSigs, indices)
}

// Verify verifies a threshold signature using GPU
func (tc *GPUThresholdContext) Verify(sig, pk, msg []byte) bool {
	return tc.ctx.Verify(sig, pk, msg)
}

// =============================================================================
// Hash Operations (GPU-accelerated batch hashing)
// =============================================================================

// BatchSHA3_256 computes multiple SHA3-256 hashes in parallel using GPU
func (o *GPUOrchestrator) BatchSHA3_256(inputs [][]byte) ([][]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.BatchHash(inputs, cryptogpu.HashTypeSHA3_256)
}

// BatchSHA3_512 computes multiple SHA3-512 hashes in parallel using GPU
func (o *GPUOrchestrator) BatchSHA3_512(inputs [][]byte) ([][]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.BatchHash(inputs, cryptogpu.HashTypeSHA3_512)
}

// BatchBLAKE3 computes multiple BLAKE3 hashes in parallel using GPU
func (o *GPUOrchestrator) BatchBLAKE3(inputs [][]byte) ([][]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return nil, errors.New("GPU orchestrator not initialized")
	}

	return cryptogpu.BatchHash(inputs, cryptogpu.HashTypeBLAKE3)
}

// SHA3_256 computes a single SHA3-256 hash
func (o *GPUOrchestrator) SHA3_256(data []byte) []byte {
	return cryptogpu.SHA3_256(data)
}

// SHA3_512 computes a single SHA3-512 hash
func (o *GPUOrchestrator) SHA3_512(data []byte) []byte {
	return cryptogpu.SHA3_512(data)
}

// BLAKE3 computes a single BLAKE3 hash
func (o *GPUOrchestrator) BLAKE3(data []byte) []byte {
	return cryptogpu.BLAKE3(data)
}

// =============================================================================
// Consensus Block Verification (GPU-accelerated)
// =============================================================================

// VerifyBlock verifies a block's signatures using GPU-accelerated batch operations
// This is the main entry point for consensus signature verification
func (o *GPUOrchestrator) VerifyBlock(blsSigs, blsPKs [][]byte, thresholdSig, thresholdPK, blockHash []byte) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized {
		return false
	}

	return cryptogpu.ConsensusVerifyBlock(blsSigs, blsPKs, thresholdSig, thresholdPK, blockHash)
}

// =============================================================================
// Statistics and Monitoring
// =============================================================================

// GPUStats contains GPU acceleration statistics
type GPUStats struct {
	Enabled    bool
	Backend    string
	BatchSize  int
	MaxWorkers int
}

// Stats returns current GPU orchestrator statistics
func (o *GPUOrchestrator) Stats() GPUStats {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return GPUStats{
		Enabled:    o.enabled,
		Backend:    o.backend,
		BatchSize:  o.batchSize,
		MaxWorkers: o.maxWorkers,
	}
}

// ClearCache clears any internal GPU caches
func (o *GPUOrchestrator) ClearCache() {
	o.mu.Lock()
	defer o.mu.Unlock()

	cryptogpu.ClearCache()
}

// =============================================================================
// Global GPU Orchestrator (for package-level access)
// =============================================================================

var (
	globalGPU     *GPUOrchestrator
	globalGPUOnce sync.Once
	globalGPUErr  error
)

// GetGPUOrchestrator returns the global GPU orchestrator instance
// It is safe for concurrent use and initializes lazily on first call
func GetGPUOrchestrator() (*GPUOrchestrator, error) {
	globalGPUOnce.Do(func() {
		globalGPU, globalGPUErr = NewGPUOrchestrator(DefaultGPUConfig())
	})
	return globalGPU, globalGPUErr
}

// GPUEnabled returns true if GPU acceleration is available
// This is a convenience function for quick checks
func GPUEnabled() bool {
	gpu, err := GetGPUOrchestrator()
	if err != nil {
		return false
	}
	return gpu.IsGPUEnabled()
}

// =============================================================================
// Corona Operations (GPU-accelerated post-quantum threshold signatures)
// =============================================================================

// CoronaNTTForward computes forward NTT of polynomials using GPU
func (o *GPUOrchestrator) CoronaNTTForward(polys [][]uint64) ([][]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.NTTForward(polys)
}

// CoronaNTTInverse computes inverse NTT of polynomials using GPU
func (o *GPUOrchestrator) CoronaNTTInverse(polys [][]uint64) ([][]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.NTTInverse(polys)
}

// CoronaPolyMul multiplies batches of polynomials using GPU
func (o *GPUOrchestrator) CoronaPolyMul(a, b [][]uint64) ([][]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.PolyMul(a, b)
}

// CoronaPolyMulNTT multiplies polynomials in NTT domain using GPU
func (o *GPUOrchestrator) CoronaPolyMulNTT(a, b []uint64) ([]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.PolyMulNTT(a, b)
}

// CoronaPolyAdd adds two polynomials using GPU
func (o *GPUOrchestrator) CoronaPolyAdd(a, b []uint64) ([]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.PolyAdd(a, b)
}

// CoronaPolySub subtracts two polynomials using GPU
func (o *GPUOrchestrator) CoronaPolySub(a, b []uint64) ([]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.PolySub(a, b)
}

// CoronaMatrixVectorMul computes matrix-vector multiplication using GPU
// Both matrix and vector must be in NTT domain
func (o *GPUOrchestrator) CoronaMatrixVectorMul(matrix [][][]uint64, vector [][]uint64) ([][]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.MatrixVectorMul(matrix, vector)
}

// CoronaSampleGaussian samples from discrete Gaussian distribution using GPU
func (o *GPUOrchestrator) CoronaSampleGaussian(sigma float64, seed []byte) ([]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.SampleGaussian(sigma, seed)
}

// CoronaSampleUniform samples uniformly at random using GPU
func (o *GPUOrchestrator) CoronaSampleUniform(seed []byte) ([]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.SampleUniform(seed)
}

// CoronaSampleTernary samples ternary polynomial using GPU
func (o *GPUOrchestrator) CoronaSampleTernary(density float64, seed []byte) ([]uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.initialized || o.corona == nil {
		return nil, errors.New("Corona GPU not initialized")
	}

	return o.corona.SampleTernary(density, seed)
}

// CoronaGPUEnabled returns true if Corona GPU acceleration is available
func (o *GPUOrchestrator) CoronaGPUEnabled() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.initialized && o.corona != nil && coronagpu.CoronaGPUEnabled()
}
