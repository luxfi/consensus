// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestKeyGen(t *testing.T) {
	sk, pk, err := KeyGen()
	if err != nil {
		t.Fatal(err)
	}
	
	// Check keys are not empty
	if len(sk) == 0 {
		t.Error("Secret key is empty")
	}
	if len(pk) == 0 {
		t.Error("Public key is empty")
	}
}

func TestQuickSignVerify(t *testing.T) {
	// Generate keys
	sk, pk, err := KeyGen()
	if err != nil {
		t.Fatal(err)
	}
	
	// Create a share
	share, err := Precompute(sk)
	if err != nil {
		t.Fatal(err)
	}
	
	// Create block ID
	blockID := sha256.Sum256([]byte("test block"))
	
	// Sign
	sig, err := QuickSign(share, blockID)
	if err != nil {
		t.Fatal(err)
	}
	
	// Verify
	if !QuickVerify(pk, blockID, sig) {
		t.Error("Signature verification failed")
	}
	
	// Verify with wrong block ID should fail
	wrongBlockID := sha256.Sum256([]byte("wrong block"))
	if QuickVerify(pk, wrongBlockID, sig) {
		t.Error("Verification should fail with wrong block ID")
	}
}

func TestPool(t *testing.T) {
	sk, _, err := KeyGen()
	if err != nil {
		t.Fatal(err)
	}
	
	// Create pool with target of 5 shares
	pool := NewPool(sk, 5)
	
	// Wait for pool to fill
	time.Sleep(50 * time.Millisecond)
	
	// Check available shares
	available := pool.Available()
	if available < 1 {
		t.Errorf("Pool should have shares, got %d", available)
	}
	
	// Get a share
	share := pool.Get()
	if share == nil {
		t.Error("Got nil share from pool")
	}
	
	// Available should decrease
	newAvailable := pool.Available()
	if newAvailable >= available {
		t.Error("Available shares should decrease after Get()")
	}
}

func TestAggregator(t *testing.T) {
	// Create aggregator with quorum of 3
	agg := NewAggregator(3, time.Second)
	
	// Create block ID
	blockID := sha256.Sum256([]byte("test block"))
	
	// Generate some valid shares for testing
	sk, _, err := KeyGen()
	if err != nil {
		t.Fatal(err)
	}
	
	// Add shares
	for i := 0; i < 3; i++ {
		share, err := Precompute(sk)
		if err != nil {
			t.Fatal(err)
		}
		agg.Add(blockID, share)
	}
	
	// Should receive certificate
	select {
	case cert := <-agg.Certs():
		if len(cert) == 0 {
			t.Error("Received empty certificate")
		}
		
		// Test hash function
		hash := Hash(cert)
		if len(hash) != 32 {
			t.Error("Hash should be 32 bytes")
		}
		
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive certificate")
	}
	
	// Pending count should be 0
	if agg.PendingCount() != 0 {
		t.Error("Pending count should be 0 after aggregation")
	}
}

func BenchmarkQuickSign(b *testing.B) {
	sk, _, _ := KeyGen()
	share, _ := Precompute(sk)
	blockID := sha256.Sum256([]byte("benchmark block"))
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		QuickSign(share, blockID)
	}
}

func BenchmarkQuickVerify(b *testing.B) {
	sk, pk, _ := KeyGen()
	share, _ := Precompute(sk)
	blockID := sha256.Sum256([]byte("benchmark block"))
	sig, _ := QuickSign(share, blockID)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		QuickVerify(pk, blockID, sig)
	}
}

func BenchmarkAggregate(b *testing.B) {
	// Create shares
	sk, _, _ := KeyGen()
	shares := make([]Share, 15)
	for i := range shares {
		share, _ := Precompute(sk)
		shares[i] = share
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Aggregate(shares)
	}
}