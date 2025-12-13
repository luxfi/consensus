// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pq

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestPostQuantumNew(t *testing.T) {
	pq := New()

	require.NotNil(t, pq)
	require.False(t, pq.bootstrapped)
	require.Equal(t, "ML-DSA-65", pq.algorithm)
}

func TestPostQuantumStart(t *testing.T) {
	pq := New()
	ctx := context.Background()

	require.False(t, pq.bootstrapped)

	err := pq.Start(ctx, 1)

	require.NoError(t, err)
	require.True(t, pq.bootstrapped)
}

func TestPostQuantumStartWithRequestID(t *testing.T) {
	pq := New()
	ctx := context.Background()

	err := pq.Start(ctx, 12345)

	require.NoError(t, err)
	require.True(t, pq.bootstrapped)
}

func TestPostQuantumStop(t *testing.T) {
	pq := New()
	ctx := context.Background()

	// Start first
	err := pq.Start(ctx, 1)
	require.NoError(t, err)

	// Then stop
	err = pq.Stop(ctx)
	require.NoError(t, err)
}

func TestPostQuantumStopWithoutStart(t *testing.T) {
	pq := New()
	ctx := context.Background()

	// Stop without starting
	err := pq.Stop(ctx)
	require.NoError(t, err)
}

func TestPostQuantumHealthCheck(t *testing.T) {
	pq := New()
	ctx := context.Background()

	health, err := pq.HealthCheck(ctx)

	require.NoError(t, err)
	require.NotNil(t, health)

	healthMap, ok := health.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, healthMap["healthy"])
	require.Equal(t, "ML-DSA-65", healthMap["algorithm"])
}

func TestPostQuantumHealthCheckAfterStart(t *testing.T) {
	pq := New()
	ctx := context.Background()

	_ = pq.Start(ctx, 1)

	health, err := pq.HealthCheck(ctx)

	require.NoError(t, err)
	require.NotNil(t, health)

	healthMap := health.(map[string]interface{})
	require.Equal(t, true, healthMap["healthy"])
}

func TestPostQuantumIsBootstrapped(t *testing.T) {
	pq := New()

	// Initially not bootstrapped
	require.False(t, pq.IsBootstrapped())

	// After start, should be bootstrapped
	_ = pq.Start(context.Background(), 1)
	require.True(t, pq.IsBootstrapped())
}

func TestPostQuantumVerifyQuantumSignature(t *testing.T) {
	pq := New()

	message := []byte("test message")
	signature := []byte("test signature")
	publicKey := []byte("test public key")

	err := pq.VerifyQuantumSignature(message, signature, publicKey)

	require.NoError(t, err)
}

func TestPostQuantumVerifyQuantumSignatureEmpty(t *testing.T) {
	pq := New()

	// Test with empty inputs
	err := pq.VerifyQuantumSignature(nil, nil, nil)
	require.NoError(t, err)

	err = pq.VerifyQuantumSignature([]byte{}, []byte{}, []byte{})
	require.NoError(t, err)
}

func TestPostQuantumGenerateQuantumProof(t *testing.T) {
	pq := New()
	ctx := context.Background()
	blockID := ids.GenerateTestID()

	proof, err := pq.GenerateQuantumProof(ctx, blockID)

	require.NoError(t, err)
	require.NotNil(t, proof)
	require.Empty(t, proof) // Current implementation returns empty
}

func TestPostQuantumGenerateQuantumProofMultiple(t *testing.T) {
	pq := New()
	ctx := context.Background()

	// Generate proofs for multiple blocks
	for i := 0; i < 10; i++ {
		blockID := ids.GenerateTestID()
		proof, err := pq.GenerateQuantumProof(ctx, blockID)
		require.NoError(t, err)
		require.NotNil(t, proof)
	}
}

func TestPostQuantumInterfaceCompliance(t *testing.T) {
	// Verify PostQuantum implements Engine interface
	var _ Engine = (*PostQuantum)(nil)

	pq := New()
	var engine Engine = pq

	ctx := context.Background()

	// Test all interface methods
	err := engine.Start(ctx, 1)
	require.NoError(t, err)

	require.True(t, engine.IsBootstrapped())

	health, err := engine.HealthCheck(ctx)
	require.NoError(t, err)
	require.NotNil(t, health)

	err = engine.VerifyQuantumSignature([]byte("msg"), []byte("sig"), []byte("key"))
	require.NoError(t, err)

	proof, err := engine.GenerateQuantumProof(ctx, ids.GenerateTestID())
	require.NoError(t, err)
	require.NotNil(t, proof)

	err = engine.Stop(ctx)
	require.NoError(t, err)
}

func TestPostQuantumContextCancellation(t *testing.T) {
	pq := New()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context before operations
	cancel()

	// Operations should still complete (they don't use context extensively yet)
	err := pq.Start(ctx, 1)
	require.NoError(t, err)

	err = pq.Stop(ctx)
	require.NoError(t, err)

	_, err = pq.HealthCheck(ctx)
	require.NoError(t, err)

	_, err = pq.GenerateQuantumProof(ctx, ids.GenerateTestID())
	require.NoError(t, err)
}

func BenchmarkPostQuantumStart(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq := New()
		_ = pq.Start(ctx, uint32(i))
	}
}

func BenchmarkPostQuantumHealthCheck(b *testing.B) {
	pq := New()
	ctx := context.Background()
	_ = pq.Start(ctx, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pq.HealthCheck(ctx)
	}
}

func BenchmarkPostQuantumVerifyQuantumSignature(b *testing.B) {
	pq := New()
	message := []byte("test message for benchmark")
	signature := make([]byte, 256)
	publicKey := make([]byte, 128)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pq.VerifyQuantumSignature(message, signature, publicKey)
	}
}

func BenchmarkPostQuantumGenerateQuantumProof(b *testing.B) {
	pq := New()
	ctx := context.Background()
	blockID := ids.GenerateTestID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pq.GenerateQuantumProof(ctx, blockID)
	}
}
