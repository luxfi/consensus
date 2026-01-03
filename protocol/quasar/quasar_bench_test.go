// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Benchmarks for Quasar post-quantum consensus

package quasar

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
)

// =============================================================================
// BLS Signing and Verification Benchmarks
// =============================================================================

func BenchmarkBLSKeyGeneration(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bls.NewSecretKey()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBLSSigning(b *testing.B) {
	sk, err := bls.NewSecretKey()
	if err != nil {
		b.Fatal(err)
	}
	message := make([]byte, 32)
	rand.Read(message)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sk.Sign(message)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBLSVerification(b *testing.B) {
	sk, err := bls.NewSecretKey()
	if err != nil {
		b.Fatal(err)
	}
	pk := sk.PublicKey()
	message := make([]byte, 32)
	rand.Read(message)
	sig, err := sk.Sign(message)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !bls.Verify(pk, sig, message) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkBLSSigningVariableMessageSize(b *testing.B) {
	sizes := []int{32, 64, 128, 256, 512, 1024, 4096}
	sk, err := bls.NewSecretKey()
	if err != nil {
		b.Fatal(err)
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("msg_%d_bytes", size), func(b *testing.B) {
			message := make([]byte, size)
			rand.Read(message)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := sk.Sign(message)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// =============================================================================
// Hybrid Signature Benchmarks (BLS + ML-DSA/Ringtail)
// =============================================================================

func BenchmarkQuasarSigCreation(b *testing.B) {
	hybrid, err := NewSigner(1)
	if err != nil {
		b.Fatal(err)
	}
	if err := hybrid.AddValidator("bench-validator", 100); err != nil {
		b.Fatal(err)
	}

	message := make([]byte, 32)
	rand.Read(message)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := hybrid.SignMessage("bench-validator", message)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuasarSigVerification(b *testing.B) {
	hybrid, err := NewSigner(1)
	if err != nil {
		b.Fatal(err)
	}
	if err := hybrid.AddValidator("bench-validator", 100); err != nil {
		b.Fatal(err)
	}

	message := make([]byte, 32)
	rand.Read(message)
	sig, err := hybrid.SignMessage("bench-validator", message)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !hybrid.VerifyQuasarSig(message, sig) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkQuasarSigVariableMessageSize(b *testing.B) {
	sizes := []int{32, 64, 128, 256, 512, 1024}
	hybrid, err := NewSigner(1)
	if err != nil {
		b.Fatal(err)
	}
	if err := hybrid.AddValidator("bench-validator", 100); err != nil {
		b.Fatal(err)
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("sign_msg_%d_bytes", size), func(b *testing.B) {
			message := make([]byte, size)
			rand.Read(message)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := hybrid.SignMessage("bench-validator", message)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("verify_msg_%d_bytes", size), func(b *testing.B) {
			message := make([]byte, size)
			rand.Read(message)
			sig, err := hybrid.SignMessage("bench-validator", message)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !hybrid.VerifyQuasarSig(message, sig) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// =============================================================================
// Signature Aggregation Benchmarks
// =============================================================================

func BenchmarkBLSAggregation(b *testing.B) {
	counts := []int{4, 8, 16, 32, 64, 100}

	for _, count := range counts {
		b.Run(fmt.Sprintf("%d_signatures", count), func(b *testing.B) {
			// Generate keys and signatures
			message := make([]byte, 32)
			rand.Read(message)

			sigs := make([]*bls.Signature, count)
			for i := 0; i < count; i++ {
				sk, err := bls.NewSecretKey()
				if err != nil {
					b.Fatal(err)
				}
				sig, err := sk.Sign(message)
				if err != nil {
					b.Fatal(err)
				}
				sigs[i] = sig
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := bls.AggregateSignatures(sigs)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBLSAggregatedVerification(b *testing.B) {
	counts := []int{4, 8, 16, 32, 64, 100}

	for _, count := range counts {
		b.Run(fmt.Sprintf("%d_signers", count), func(b *testing.B) {
			message := make([]byte, 32)
			rand.Read(message)

			sigs := make([]*bls.Signature, count)
			pks := make([]*bls.PublicKey, count)
			for i := 0; i < count; i++ {
				sk, err := bls.NewSecretKey()
				if err != nil {
					b.Fatal(err)
				}
				pks[i] = sk.PublicKey()
				sig, err := sk.Sign(message)
				if err != nil {
					b.Fatal(err)
				}
				sigs[i] = sig
			}

			aggSig, err := bls.AggregateSignatures(sigs)
			if err != nil {
				b.Fatal(err)
			}
			aggPK, err := bls.AggregatePublicKeys(pks)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !bls.Verify(aggPK, aggSig, message) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

func BenchmarkSignerAggregation(b *testing.B) {
	counts := []int{4, 8, 16, 32}

	for _, count := range counts {
		b.Run(fmt.Sprintf("%d_validators", count), func(b *testing.B) {
			hybrid, err := NewSigner(count)
			if err != nil {
				b.Fatal(err)
			}

			message := make([]byte, 32)
			rand.Read(message)

			// Add validators and collect signatures
			sigs := make([]*QuasarSig, count)
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("validator-%d", i)
				if err := hybrid.AddValidator(id, 100); err != nil {
					b.Fatal(err)
				}
				sig, err := hybrid.SignMessage(id, message)
				if err != nil {
					b.Fatal(err)
				}
				sigs[i] = sig
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := hybrid.AggregateSignatures(message, sigs)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSignerAggregatedVerification(b *testing.B) {
	counts := []int{4, 8, 16, 32}

	for _, count := range counts {
		b.Run(fmt.Sprintf("%d_validators", count), func(b *testing.B) {
			hybrid, err := NewSigner(count)
			if err != nil {
				b.Fatal(err)
			}

			message := make([]byte, 32)
			rand.Read(message)

			// Add validators and collect signatures
			sigs := make([]*QuasarSig, count)
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("validator-%d", i)
				if err := hybrid.AddValidator(id, 100); err != nil {
					b.Fatal(err)
				}
				sig, err := hybrid.SignMessage(id, message)
				if err != nil {
					b.Fatal(err)
				}
				sigs[i] = sig
			}

			aggSig, err := hybrid.AggregateSignatures(message, sigs)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !hybrid.VerifyAggregatedSignature(message, aggSig) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// =============================================================================
// Threshold Signing Benchmarks
// =============================================================================

func BenchmarkThresholdRound(b *testing.B) {
	// Benchmark a complete threshold signing round
	thresholds := []struct {
		n int // total validators
		t int // threshold
	}{
		{4, 3},
		{10, 7},
		{20, 14},
		{50, 34},
		{100, 67},
	}

	for _, tc := range thresholds {
		b.Run(fmt.Sprintf("n%d_t%d", tc.n, tc.t), func(b *testing.B) {
			hybrid, err := NewSigner(tc.t)
			if err != nil {
				b.Fatal(err)
			}

			message := make([]byte, 32)
			rand.Read(message)

			// Add all validators
			ids := make([]string, tc.n)
			for i := 0; i < tc.n; i++ {
				id := fmt.Sprintf("validator-%d", i)
				ids[i] = id
				if err := hybrid.AddValidator(id, 100); err != nil {
					b.Fatal(err)
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Collect threshold signatures (simulate a round)
				sigs := make([]*QuasarSig, tc.t)
				for j := 0; j < tc.t; j++ {
					sig, err := hybrid.SignMessage(ids[j], message)
					if err != nil {
						b.Fatal(err)
					}
					sigs[j] = sig
				}

				// Aggregate
				aggSig, err := hybrid.AggregateSignatures(message, sigs)
				if err != nil {
					b.Fatal(err)
				}

				// Verify
				if !hybrid.VerifyAggregatedSignature(message, aggSig) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

func BenchmarkThresholdSigningOnly(b *testing.B) {
	// Benchmark just the signing phase (parallel in production)
	thresholds := []int{4, 8, 16, 32, 64}

	for _, t := range thresholds {
		b.Run(fmt.Sprintf("%d_signers", t), func(b *testing.B) {
			hybrid, err := NewSigner(t)
			if err != nil {
				b.Fatal(err)
			}

			message := make([]byte, 32)
			rand.Read(message)

			// Add validators
			ids := make([]string, t)
			for i := 0; i < t; i++ {
				id := fmt.Sprintf("validator-%d", i)
				ids[i] = id
				if err := hybrid.AddValidator(id, 100); err != nil {
					b.Fatal(err)
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < t; j++ {
					_, err := hybrid.SignMessage(ids[j], message)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// =============================================================================
// ML-DSA (Ringtail Post-Quantum) Benchmarks
// =============================================================================

func BenchmarkMLDSAKeyGeneration(b *testing.B) {
	modes := []mldsa.Mode{mldsa.MLDSA44, mldsa.MLDSA65, mldsa.MLDSA87}
	names := []string{"MLDSA44_L2", "MLDSA65_L3", "MLDSA87_L5"}

	for i, mode := range modes {
		b.Run(names[i], func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				_, err := mldsa.GenerateKey(rand.Reader, mode)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMLDSASigning(b *testing.B) {
	modes := []mldsa.Mode{mldsa.MLDSA44, mldsa.MLDSA65, mldsa.MLDSA87}
	names := []string{"MLDSA44_L2", "MLDSA65_L3", "MLDSA87_L5"}

	for i, mode := range modes {
		b.Run(names[i], func(b *testing.B) {
			sk, err := mldsa.GenerateKey(rand.Reader, mode)
			if err != nil {
				b.Fatal(err)
			}
			message := make([]byte, 32)
			rand.Read(message)

			b.ReportAllocs()
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				_, err := sk.Sign(rand.Reader, message, nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMLDSAVerification(b *testing.B) {
	modes := []mldsa.Mode{mldsa.MLDSA44, mldsa.MLDSA65, mldsa.MLDSA87}
	names := []string{"MLDSA44_L2", "MLDSA65_L3", "MLDSA87_L5"}

	for i, mode := range modes {
		b.Run(names[i], func(b *testing.B) {
			sk, err := mldsa.GenerateKey(rand.Reader, mode)
			if err != nil {
				b.Fatal(err)
			}
			message := make([]byte, 32)
			rand.Read(message)
			sig, err := sk.Sign(rand.Reader, message, nil)
			if err != nil {
				b.Fatal(err)
			}
			pk := sk.PublicKey

			b.ReportAllocs()
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				if !pk.Verify(message, sig, nil) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// =============================================================================
// Ringtail (Stub) Benchmarks
// =============================================================================

// NOTE: Ringtail benchmarks are in the real ringtail package at
// github.com/luxfi/ringtail/threshold. The quasar package uses the real
// implementation via the Signer type in quasar.go.

// =============================================================================
// End-to-End Quasar Benchmarks
// =============================================================================

func BenchmarkQuasarBlockProcessing(b *testing.B) {
	qa, err := NewQuasar(1)
	if err != nil {
		b.Fatal(err)
	}
	// Need at least 2 validators for epoch manager
	if _, err := qa.AddValidator("validator1", 100); err != nil {
		b.Fatal(err)
	}
	if _, err := qa.AddValidator("validator2", 100); err != nil {
		b.Fatal(err)
	}

	block := &ChainBlock{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:        [32]byte{0x01, 0x02, 0x03},
		Height:    100,
		Timestamp: time.Now(),
		Data:      make([]byte, 1024),
	}
	rand.Read(block.Data)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.Height = uint64(i)
		qa.processBlock(block)
	}
}

func BenchmarkQuasarQuantumHash(b *testing.B) {
	qa, err := NewQuasar(1)
	if err != nil {
		b.Fatal(err)
	}

	block := &Block{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:        [32]byte{0x01, 0x02, 0x03},
		Height:    100,
		Timestamp: time.Now(),
		Data:      make([]byte, 1024),
	}
	rand.Read(block.Data)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = qa.computeQuantumHash(block)
	}
}

func BenchmarkQuasarValidatorAddition(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		hybrid, err := NewSigner(1)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		err = hybrid.AddValidator(fmt.Sprintf("validator-%d", i), 100)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Comparison Benchmarks (BLS vs ML-DSA)
// =============================================================================

func BenchmarkComparisonSigning(b *testing.B) {
	message := make([]byte, 32)
	rand.Read(message)

	b.Run("BLS", func(b *testing.B) {
		sk, err := bls.NewSecretKey()
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := sk.Sign(message)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MLDSA65", func(b *testing.B) {
		sk, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := sk.Sign(rand.Reader, message, nil)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkComparisonVerification(b *testing.B) {
	message := make([]byte, 32)
	rand.Read(message)

	b.Run("BLS", func(b *testing.B) {
		sk, err := bls.NewSecretKey()
		if err != nil {
			b.Fatal(err)
		}
		pk := sk.PublicKey()
		sig, err := sk.Sign(message)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !bls.Verify(pk, sig, message) {
				b.Fatal("verification failed")
			}
		}
	})

	b.Run("MLDSA65", func(b *testing.B) {
		sk, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
		if err != nil {
			b.Fatal(err)
		}
		pk := sk.PublicKey
		sig, err := sk.Sign(rand.Reader, message, nil)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !pk.Verify(message, sig, nil) {
				b.Fatal("verification failed")
			}
		}
	})
}

func BenchmarkComparisonKeyGen(b *testing.B) {
	b.Run("BLS", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := bls.NewSecretKey()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MLDSA44", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA44)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MLDSA65", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MLDSA87", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA87)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
