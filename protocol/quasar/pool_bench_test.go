// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Benchmarks for sync.Pool optimizations in hot paths

package quasar

import (
	"context"
	"testing"
)

// BenchmarkSignMessage benchmarks signature creation with pooled buffers
func BenchmarkSignMessage(b *testing.B) {
	hybrid, err := NewSigner(1)
	if err != nil {
		b.Fatal(err)
	}
	if err := hybrid.AddValidator("bench-validator", 100); err != nil {
		b.Fatal(err)
	}

	msg := []byte("benchmark message for signing")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sig, err := hybrid.SignMessage("bench-validator", msg)
		if err != nil {
			b.Fatal(err)
		}
		ReleaseQuasarSig(sig)
	}
}

// BenchmarkSignMessageParallel benchmarks parallel signature creation
func BenchmarkSignMessageParallel(b *testing.B) {
	hybrid, err := NewSigner(1)
	if err != nil {
		b.Fatal(err)
	}
	if err := hybrid.AddValidator("bench-validator", 100); err != nil {
		b.Fatal(err)
	}

	msg := []byte("benchmark message for signing")

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sig, err := hybrid.SignMessage("bench-validator", msg)
			if err != nil {
				b.Fatal(err)
			}
			ReleaseQuasarSig(sig)
		}
	})
}

// BenchmarkAggregateSignatures benchmarks signature aggregation with pooled slices
func BenchmarkAggregateSignatures(b *testing.B) {
	hybrid, err := NewSigner(3)
	if err != nil {
		b.Fatal(err)
	}

	// Add validators
	for i := 0; i < 10; i++ {
		if err := hybrid.AddValidator(string(rune('A'+i)), 100); err != nil {
			b.Fatal(err)
		}
	}

	msg := []byte("benchmark message for aggregation")

	// Pre-create signatures
	sigs := make([]*QuasarSig, 5)
	for i := 0; i < 5; i++ {
		sig, err := hybrid.SignMessage(string(rune('A'+i)), msg)
		if err != nil {
			b.Fatal(err)
		}
		// Copy to avoid pool reuse during benchmark
		sigs[i] = &QuasarSig{
			BLS:         append([]byte(nil), sig.BLS...),
			Ringtail:    append([]byte(nil), sig.Ringtail...),
			ValidatorID: sig.ValidatorID,
		}
		ReleaseQuasarSig(sig)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := hybrid.AggregateSignatures(msg, sigs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifyAggregatedSignature benchmarks aggregated signature verification with pooled slices
func BenchmarkVerifyAggregatedSignature(b *testing.B) {
	hybrid, err := NewSigner(3)
	if err != nil {
		b.Fatal(err)
	}

	// Add validators
	for i := 0; i < 10; i++ {
		if err := hybrid.AddValidator(string(rune('A'+i)), 100); err != nil {
			b.Fatal(err)
		}
	}

	msg := []byte("benchmark message for verification")

	// Pre-create signatures
	sigs := make([]*QuasarSig, 5)
	for i := 0; i < 5; i++ {
		sig, err := hybrid.SignMessage(string(rune('A'+i)), msg)
		if err != nil {
			b.Fatal(err)
		}
		sigs[i] = &QuasarSig{
			BLS:         append([]byte(nil), sig.BLS...),
			Ringtail:    append([]byte(nil), sig.Ringtail...),
			ValidatorID: sig.ValidatorID,
		}
		ReleaseQuasarSig(sig)
	}

	aggSig, err := hybrid.AggregateSignatures(msg, sigs)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if !hybrid.VerifyAggregatedSignature(msg, aggSig) {
			b.Fatal("verification failed")
		}
	}
}

// BenchmarkVerifyAggregatedSignatureParallel benchmarks parallel verification
func BenchmarkVerifyAggregatedSignatureParallel(b *testing.B) {
	hybrid, err := NewSigner(3)
	if err != nil {
		b.Fatal(err)
	}

	// Add validators
	for i := 0; i < 10; i++ {
		if err := hybrid.AddValidator(string(rune('A'+i)), 100); err != nil {
			b.Fatal(err)
		}
	}

	msg := []byte("benchmark message for verification")
	ctx := context.Background()

	// Pre-create signatures
	sigs := make([]*QuasarSig, 5)
	for i := 0; i < 5; i++ {
		sig, err := hybrid.SignMessage(string(rune('A'+i)), msg)
		if err != nil {
			b.Fatal(err)
		}
		sigs[i] = &QuasarSig{
			BLS:         append([]byte(nil), sig.BLS...),
			Ringtail:    append([]byte(nil), sig.Ringtail...),
			ValidatorID: sig.ValidatorID,
		}
		ReleaseQuasarSig(sig)
	}

	aggSig, err := hybrid.AggregateSignatures(msg, sigs)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !hybrid.VerifyAggregatedSignatureWithContext(ctx, msg, aggSig) {
				b.Fatal("verification failed")
			}
		}
	})
}
