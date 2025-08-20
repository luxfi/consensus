package quasar

import (
	"testing"
)

func TestRingtail_Initialize(t *testing.T) {
	rt := NewRingtail()
	
	err := rt.Initialize(SecurityMedium)
	if err != nil {
		t.Errorf("Initialize() failed: %v", err)
	}
	
	// Test all security levels
	levels := []SecurityLevel{SecurityLow, SecurityMedium, SecurityHigh}
	for _, level := range levels {
		err := rt.Initialize(level)
		if err != nil {
			t.Errorf("Initialize(%v) failed: %v", level, err)
		}
	}
}

func TestKeyGen(t *testing.T) {
	seed := []byte("test-seed-for-key-generation")
	
	sk, pk, err := KeyGen(seed)
	if err != nil {
		t.Fatalf("KeyGen() failed: %v", err)
	}
	
	if len(sk) == 0 {
		t.Error("KeyGen() returned empty secret key")
	}
	
	if len(pk) == 0 {
		t.Error("KeyGen() returned empty public key")
	}
}

func TestPrecompute(t *testing.T) {
	sk := []byte("secret-key-for-precompute")
	
	precomp, err := Precompute(sk)
	if err != nil {
		t.Fatalf("Precompute() failed: %v", err)
	}
	
	if len(precomp) == 0 {
		t.Error("Precompute() returned empty precomputation")
	}
}

func TestQuickSign(t *testing.T) {
	sk := []byte("secret-key")
	precomp, err := Precompute(sk)
	if err != nil {
		t.Fatalf("Precompute() failed: %v", err)
	}
	
	msg := []byte("message to sign quickly")
	share, err := QuickSign(precomp, msg)
	if err != nil {
		t.Fatalf("QuickSign() failed: %v", err)
	}
	
	if len(share) == 0 {
		t.Error("QuickSign() returned empty share")
	}
}

func TestVerifyShare(t *testing.T) {
	pk := []byte("public-key")
	msg := []byte("message")
	share := []byte("signature-share")
	
	valid := VerifyShare(pk, msg, share)
	
	// Stub implementation always returns true
	if !valid {
		t.Error("VerifyShare() should return true in stub implementation")
	}
}

func TestAggregate(t *testing.T) {
	// Create shares
	shares := []Share{
		[]byte("share1"),
		[]byte("share2"),
		[]byte("share3"),
	}
	
	cert, err := Aggregate(shares)
	if err != nil {
		t.Fatalf("Aggregate() failed: %v", err)
	}
	
	if len(cert) == 0 {
		t.Error("Aggregate() returned empty certificate")
	}
	
	// Test empty shares
	emptyCert, err := Aggregate([]Share{})
	if err == nil {
		t.Error("Aggregate() should fail with empty shares")
	}
	if emptyCert != nil {
		t.Error("Aggregate() should return nil certificate for empty shares")
	}
}

func TestVerify(t *testing.T) {
	pk := []byte("public-key")
	msg := []byte("message")
	cert := []byte("certificate")
	
	valid := Verify(pk, msg, cert)
	
	// Stub implementation always returns true
	if !valid {
		t.Error("Verify() should return true in stub implementation")
	}
}

func TestRingtail_ErrorCases(t *testing.T) {
	rt := NewRingtail()
	
	t.Run("Encapsulate with empty key", func(t *testing.T) {
		ct, ss, err := rt.Encapsulate([]byte{})
		// Stub doesn't validate, so it should succeed
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(ct) == 0 || len(ss) == 0 {
			t.Error("Encapsulate should return non-empty values")
		}
	})
	
	t.Run("Decapsulate with empty ciphertext", func(t *testing.T) {
		ss, err := rt.Decapsulate([]byte{}, []byte("key"))
		// Stub doesn't validate, so it should succeed
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(ss) == 0 {
			t.Error("Decapsulate should return non-empty shared secret")
		}
	})
	
	t.Run("CombineSharedSecrets with mismatched lengths", func(t *testing.T) {
		ss1 := []byte("short")
		ss2 := []byte("much-longer-shared-secret-value")
		combined := rt.CombineSharedSecrets(ss1, ss2)
		if len(combined) != 32 {
			t.Errorf("Combined secret should be 32 bytes, got %d", len(combined))
		}
	})
	
	t.Run("DeriveKey with various lengths", func(t *testing.T) {
		secret := []byte("shared-secret")
		lengths := []int{16, 32, 64, 128}
		
		for _, length := range lengths {
			key := rt.DeriveKey(secret, length)
			if len(key) != length {
				t.Errorf("DeriveKey should return %d bytes, got %d", length, len(key))
			}
		}
	})
}

func TestRingtail_SignVerify(t *testing.T) {
	rt := NewRingtail()
	rt.Initialize(SecurityMedium)
	
	// Generate key pair
	sk, pk, err := rt.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() failed: %v", err)
	}
	
	// Sign message
	msg := []byte("test message for signing")
	sig, err := rt.Sign(msg, sk)
	if err != nil {
		t.Fatalf("Sign() failed: %v", err)
	}
	
	// Verify signature
	valid := rt.Verify(msg, sig, pk)
	if !valid {
		t.Error("Valid signature failed verification")
	}
	
	// Test invalid signature (all zeros)
	invalidSig := make([]byte, 32)
	invalid := rt.Verify(msg, invalidSig, pk)
	if invalid {
		t.Error("Invalid signature passed verification")
	}
}

func BenchmarkKeyGen(b *testing.B) {
	seed := []byte("benchmark-seed")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = KeyGen(seed)
	}
}

func BenchmarkPrecompute(b *testing.B) {
	sk := []byte("secret-key-for-benchmark")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Precompute(sk)
	}
}

func BenchmarkQuickSign(b *testing.B) {
	sk := []byte("secret-key")
	precomp, _ := Precompute(sk)
	msg := []byte("message to sign")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = QuickSign(precomp, msg)
	}
}

func BenchmarkAggregate(b *testing.B) {
	shares := []Share{
		[]byte("share1"),
		[]byte("share2"),
		[]byte("share3"),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Aggregate(shares)
	}
}

func BenchmarkVerifyShare(b *testing.B) {
	pk := []byte("public-key")
	msg := []byte("message")
	share := []byte("share")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VerifyShare(pk, msg, share)
	}
}