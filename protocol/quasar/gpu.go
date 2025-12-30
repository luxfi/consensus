//go:build !cgo
// +build !cgo

// Package quasar provides GPU-accelerated consensus operations.
// This file provides stub implementations when CGO is disabled.
package quasar

import (
	"errors"
	"sync"
)

// ErrCGODisabled is returned when GPU operations are called without CGO
var ErrCGODisabled = errors.New("CGO required for GPU acceleration")

// GPUOrchestrator coordinates GPU-accelerated cryptographic operations
// This is a stub implementation when CGO is disabled
type GPUOrchestrator struct {
	mu sync.RWMutex
}

// GPUConfig configures the GPU orchestrator
type GPUConfig struct {
	Enabled    *bool
	BatchSize  int
	MaxWorkers int
}

// DefaultGPUConfig returns sensible defaults
func DefaultGPUConfig() GPUConfig {
	return GPUConfig{
		BatchSize:  100,
		MaxWorkers: 8,
	}
}

// NewGPUOrchestrator creates a stub GPU orchestrator
func NewGPUOrchestrator(cfg GPUConfig) (*GPUOrchestrator, error) {
	return &GPUOrchestrator{}, nil
}

// IsGPUEnabled always returns false when CGO is disabled
func (o *GPUOrchestrator) IsGPUEnabled() bool { return false }

// GetBackend returns "CPU (CGO disabled)"
func (o *GPUOrchestrator) GetBackend() string { return "CPU (CGO disabled)" }

// BLS Operations - all return errors without CGO
func (o *GPUOrchestrator) BLSBatchVerify(sigs, pks, msgs [][]byte) ([]bool, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) BLSAggregateSignatures(sigs [][]byte) ([]byte, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) BLSAggregatePublicKeys(pks [][]byte) ([]byte, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) BLSSign(sk, msg []byte) ([]byte, error) { return nil, ErrCGODisabled }
func (o *GPUOrchestrator) BLSVerify(sig, pk, msg []byte) bool     { return false }

// ML-DSA Operations - all return errors without CGO
func (o *GPUOrchestrator) MLDSASign(sk, msg []byte) ([]byte, error)    { return nil, ErrCGODisabled }
func (o *GPUOrchestrator) MLDSAVerify(sig, msg, pk []byte) bool        { return false }
func (o *GPUOrchestrator) MLDSABatchVerify(sigs, msgs [][]byte, pks [][]byte) ([]bool, error) {
	return nil, ErrCGODisabled
}

// Threshold Operations - stubs
type GPUThresholdContext struct{}

func (o *GPUOrchestrator) NewThresholdContext(t, n uint32) (*GPUThresholdContext, error) {
	return nil, ErrCGODisabled
}
func (tc *GPUThresholdContext) Close()                                                       {}
func (tc *GPUThresholdContext) Keygen(seed []byte) ([][]byte, []byte, error)                 { return nil, nil, ErrCGODisabled }
func (tc *GPUThresholdContext) PartialSign(shareIndex uint32, share, msg []byte) ([]byte, error) {
	return nil, ErrCGODisabled
}
func (tc *GPUThresholdContext) Combine(partialSigs [][]byte, indices []uint32) ([]byte, error) {
	return nil, ErrCGODisabled
}
func (tc *GPUThresholdContext) Verify(sig, pk, msg []byte) bool { return false }

// Hash Operations - stubs
func (o *GPUOrchestrator) BatchSHA3_256(inputs [][]byte) ([][]byte, error) { return nil, ErrCGODisabled }
func (o *GPUOrchestrator) BatchSHA3_512(inputs [][]byte) ([][]byte, error) { return nil, ErrCGODisabled }
func (o *GPUOrchestrator) BatchBLAKE3(inputs [][]byte) ([][]byte, error)   { return nil, ErrCGODisabled }
func (o *GPUOrchestrator) SHA3_256(data []byte) []byte                     { return nil }
func (o *GPUOrchestrator) SHA3_512(data []byte) []byte                     { return nil }
func (o *GPUOrchestrator) BLAKE3(data []byte) []byte                       { return nil }

// Block Verification - stub
func (o *GPUOrchestrator) VerifyBlock(blsSigs, blsPKs [][]byte, thresholdSig, thresholdPK, blockHash []byte) bool {
	return false
}

// Statistics
type GPUStats struct {
	Enabled    bool
	Backend    string
	BatchSize  int
	MaxWorkers int
}

func (o *GPUOrchestrator) Stats() GPUStats {
	return GPUStats{
		Enabled: false,
		Backend: "CPU (CGO disabled)",
	}
}

func (o *GPUOrchestrator) ClearCache() {}

// Global accessor
var globalGPU = &GPUOrchestrator{}

func GetGPUOrchestrator() (*GPUOrchestrator, error) {
	return globalGPU, nil
}

func GPUEnabled() bool {
	return false
}

// Close releases GPU resources (no-op without CGO)
func (o *GPUOrchestrator) Close() {}

// Ringtail Operations - all return errors without CGO
func (o *GPUOrchestrator) RingtailNTTForward(polys [][]uint64) ([][]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailNTTInverse(polys [][]uint64) ([][]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailPolyMul(a, b [][]uint64) ([][]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailPolyMulNTT(a, b []uint64) ([]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailPolyAdd(a, b []uint64) ([]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailPolySub(a, b []uint64) ([]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailMatrixVectorMul(matrix [][][]uint64, vector [][]uint64) ([][]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailSampleGaussian(sigma float64, seed []byte) ([]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailSampleUniform(seed []byte) ([]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailSampleTernary(density float64, seed []byte) ([]uint64, error) {
	return nil, ErrCGODisabled
}
func (o *GPUOrchestrator) RingtailGPUEnabled() bool {
	return false
}
