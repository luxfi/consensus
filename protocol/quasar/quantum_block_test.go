// Copyright (C) 2025, Lux Industries Inc All rights reserved.

package quasar

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQuantumBundle_Hash(t *testing.T) {
	qb := &QuantumBundle{
		Epoch:        1,
		Sequence:     5,
		StartHeight:  100,
		EndHeight:    105,
		BlockCount:   6,
		MerkleRoot:   sha256.Sum256([]byte("merkle")),
		PreviousHash: sha256.Sum256([]byte("prev")),
		Timestamp:    1234567890,
	}

	hash1 := qb.Hash()
	hash2 := qb.Hash()

	// Same bundle produces same hash
	require.Equal(t, hash1, hash2)

	// Different bundle produces different hash
	qb2 := *qb
	qb2.Sequence = 6
	hash3 := qb2.Hash()
	require.NotEqual(t, hash1, hash3)
}

func TestQuantumBundle_SignableMessage(t *testing.T) {
	qb := &QuantumBundle{
		Epoch:        1,
		Sequence:     0,
		StartHeight:  0,
		EndHeight:    5,
		BlockCount:   6,
		MerkleRoot:   sha256.Sum256([]byte("test")),
		PreviousHash: [32]byte{},
		Timestamp:    time.Now().Unix(),
	}

	msg := qb.SignableMessage()
	require.Contains(t, msg, "QUASAR-QB-v1:")
	require.Len(t, msg, 77) // "QUASAR-QB-v1:" (13 chars) + 64 hex chars
}

func TestComputeMerkleRoot(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		root := ComputeMerkleRoot(nil)
		require.Equal(t, [32]byte{}, root)
	})

	t.Run("single", func(t *testing.T) {
		h := sha256.Sum256([]byte("block1"))
		root := ComputeMerkleRoot([][32]byte{h})
		require.Equal(t, h, root)
	})

	t.Run("two", func(t *testing.T) {
		h1 := sha256.Sum256([]byte("block1"))
		h2 := sha256.Sum256([]byte("block2"))
		root := ComputeMerkleRoot([][32]byte{h1, h2})

		// Manual calculation
		combined := append(h1[:], h2[:]...)
		expected := sha256.Sum256(combined)
		require.Equal(t, expected, root)
	})

	t.Run("three_pads", func(t *testing.T) {
		h1 := sha256.Sum256([]byte("block1"))
		h2 := sha256.Sum256([]byte("block2"))
		h3 := sha256.Sum256([]byte("block3"))
		root := ComputeMerkleRoot([][32]byte{h1, h2, h3})

		// Should pad h3 and compute correctly
		require.NotEqual(t, [32]byte{}, root)
	})

	t.Run("six_blocks", func(t *testing.T) {
		hashes := make([][32]byte, 6)
		for i := 0; i < 6; i++ {
			hashes[i] = sha256.Sum256([]byte{byte(i)})
		}
		root := ComputeMerkleRoot(hashes)
		require.NotEqual(t, [32]byte{}, root)

		// Consistent with same input
		root2 := ComputeMerkleRoot(hashes)
		require.Equal(t, root, root2)
	})
}

func TestBundleSigner_Basic(t *testing.T) {
	em := NewEpochManager(2, 3) // 2-of-N threshold, keep 3 epochs
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	signer := NewBundleSigner(em)
	require.NotNil(t, signer)
	require.Equal(t, 0, signer.PendingCount())
	require.Nil(t, signer.LastBundle())
}

func TestBundleSigner_AddBLSBlock(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewBundleSigner(em)

	// Add 6 BLS blocks (simulating 3 seconds at 500ms each)
	for i := 0; i < 6; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(i), hash)
	}

	require.Equal(t, 6, signer.PendingCount())
}

func TestBundleSigner_CreateBundle(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewBundleSigner(em)

	// No pending blocks returns nil
	qb := signer.CreateBundle()
	require.Nil(t, qb)

	// Add 6 BLS blocks
	for i := 0; i < 6; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(100+i), hash)
	}

	// Create bundle
	qb = signer.CreateBundle()
	require.NotNil(t, qb)
	require.Equal(t, uint64(0), qb.Epoch)
	require.Equal(t, uint64(0), qb.Sequence)
	require.Equal(t, uint64(100), qb.StartHeight)
	require.Equal(t, uint64(105), qb.EndHeight)
	require.Equal(t, 6, qb.BlockCount)
	require.Len(t, qb.BlockHashes, 6)
	require.NotEqual(t, [32]byte{}, qb.MerkleRoot)
	require.Equal(t, [32]byte{}, qb.PreviousHash) // First bundle has no previous

	// Pending cleared
	require.Equal(t, 0, signer.PendingCount())

	// Create another - should link to previous
	for i := 0; i < 4; i++ {
		hash := sha256.Sum256([]byte{byte(10 + i)})
		signer.AddBLSBlock(uint64(106+i), hash)
	}

	qb2 := signer.CreateBundle()
	require.NotNil(t, qb2)
	require.Equal(t, uint64(1), qb2.Sequence)
	require.Equal(t, qb.Hash(), qb2.PreviousHash) // Linked to previous
	require.Equal(t, 4, qb2.BlockCount)
}

func TestBundleSigner_SignBundle(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	signer := NewBundleSigner(em)

	// Add BLS blocks
	for i := 0; i < 6; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(i), hash)
	}

	qb := signer.CreateBundle()
	require.NotNil(t, qb)
	require.Nil(t, qb.Signature)

	// Sign with all validators
	prfKey := []byte("prf-key-for-quantum-bundle-test!")
	err = signer.SignBundle(qb, 1, prfKey, validators)
	require.NoError(t, err)
	require.NotNil(t, qb.Signature)

	// Verify
	valid := signer.VerifyBundle(qb)
	require.True(t, valid)
}

func TestBundleSigner_SignWithThreshold(t *testing.T) {
	em := NewEpochManager(2, 3) // 2-of-3 threshold
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	signer := NewBundleSigner(em)

	for i := 0; i < 6; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(i), hash)
	}

	qb := signer.CreateBundle()
	prfKey := []byte("prf-key-for-threshold-signing!!!")

	// Sign with exactly 2 validators (threshold)
	err = signer.SignBundle(qb, 1, prfKey, []string{"v0", "v1"})
	require.NoError(t, err)
	require.NotNil(t, qb.Signature)

	valid := signer.VerifyBundle(qb)
	require.True(t, valid)
}

func TestBundleSigner_InsufficientSigners(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewBundleSigner(em)

	for i := 0; i < 3; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(i), hash)
	}

	qb := signer.CreateBundle()
	prfKey := []byte("prf-key-for-insufficient-test!!!")

	// Sign with only 1 validator (below threshold)
	err := signer.SignBundle(qb, 1, prfKey, []string{"v0"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient signers")
}

func TestAsyncBundleSigner_SignAsync(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewAsyncBundleSigner(em)

	for i := 0; i < 6; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(i), hash)
	}

	qb := signer.CreateBundle()
	prfKey := []byte("prf-key-for-async-signing-test!!")

	// Start async signing
	signer.SignBundleAsync(qb, 1, prfKey, validators)

	// Wait for signed bundle
	select {
	case signedQB := <-signer.SignedBundles():
		require.NotNil(t, signedQB.Signature)
		valid := signer.VerifyBundle(signedQB)
		require.True(t, valid)
	case <-time.After(5 * time.Second):
		t.Fatal("async signing timed out")
	}
}

func TestBundleSigner_MerkleVerification(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewBundleSigner(em)

	hashes := make([][32]byte, 6)
	for i := 0; i < 6; i++ {
		hashes[i] = sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(i), hashes[i])
	}

	qb := signer.CreateBundle()
	prfKey := []byte("prf-key-for-merkle-verify-test!!")
	signer.SignBundle(qb, 1, prfKey, validators)

	// Valid bundle verifies
	require.True(t, signer.VerifyBundle(qb))

	// Tampered Merkle root fails
	qbTampered := *qb
	qbTampered.MerkleRoot = sha256.Sum256([]byte("tampered"))
	require.False(t, signer.VerifyBundle(&qbTampered))

	// Tampered block hashes fail
	qbTampered2 := *qb
	qbTampered2.BlockHashes = make([][32]byte, 6)
	require.False(t, signer.VerifyBundle(&qbTampered2))
}

func TestBundleRunner_StartStop(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewAsyncBundleSigner(em)
	prfKey := []byte("prf-key-for-runner-test-signing!")

	runner := NewBundleRunner(signer, validators, prfKey)
	require.NotNil(t, runner)

	// Start runner
	runner.Start()

	// Add some BLS blocks
	for i := 0; i < 6; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		signer.AddBLSBlock(uint64(i), hash)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait a bit for runner to process
	time.Sleep(100 * time.Millisecond)

	// Stop should complete cleanly
	done := make(chan struct{})
	go func() {
		runner.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("runner.Stop() timed out")
	}
}

func TestBundleSigner_ChainLinkage(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewBundleSigner(em)
	prfKey := []byte("prf-key-for-chain-linkage-test!!")

	// Create a chain of 3 bundles
	var bundles []*QuantumBundle

	for qbIdx := 0; qbIdx < 3; qbIdx++ {
		for i := 0; i < 6; i++ {
			height := uint64(qbIdx*6 + i)
			hash := sha256.Sum256([]byte{byte(height)})
			signer.AddBLSBlock(height, hash)
		}

		qb := signer.CreateBundle()
		signer.SignBundle(qb, qbIdx+1, prfKey, validators)
		bundles = append(bundles, qb)
	}

	// Verify chain linkage
	require.Equal(t, [32]byte{}, bundles[0].PreviousHash)            // Genesis has no previous
	require.Equal(t, bundles[0].Hash(), bundles[1].PreviousHash)     // Bundle 1 links to 0
	require.Equal(t, bundles[1].Hash(), bundles[2].PreviousHash)     // Bundle 2 links to 1

	// Verify sequences
	require.Equal(t, uint64(0), bundles[0].Sequence)
	require.Equal(t, uint64(1), bundles[1].Sequence)
	require.Equal(t, uint64(2), bundles[2].Sequence)

	// All bundles verify
	for i, qb := range bundles {
		require.True(t, signer.VerifyBundle(qb), "bundle %d should verify", i)
	}
}

func BenchmarkBundleSigner_Create(b *testing.B) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewBundleSigner(em)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Add 6 BLS blocks
		for j := 0; j < 6; j++ {
			hash := sha256.Sum256([]byte{byte(i), byte(j)})
			signer.AddBLSBlock(uint64(i*6+j), hash)
		}
		signer.CreateBundle()
	}
}

func BenchmarkBundleSigner_SignAndVerify(b *testing.B) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	signer := NewBundleSigner(em)
	prfKey := []byte("prf-key-for-benchmark-signing!!!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 6; j++ {
			hash := sha256.Sum256([]byte{byte(i), byte(j)})
			signer.AddBLSBlock(uint64(i*6+j), hash)
		}
		qb := signer.CreateBundle()
		signer.SignBundle(qb, i, prfKey, validators)
		signer.VerifyBundle(qb)
	}
}

func BenchmarkMerkleRoot(b *testing.B) {
	sizes := []int{6, 12, 24, 60}
	for _, n := range sizes {
		b.Run("blocks="+string(rune('0'+n/10))+string(rune('0'+n%10)), func(b *testing.B) {
			hashes := make([][32]byte, n)
			for i := 0; i < n; i++ {
				hashes[i] = sha256.Sum256([]byte{byte(i)})
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ComputeMerkleRoot(hashes)
			}
		})
	}
}
