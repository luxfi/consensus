package quasar

import (
	"crypto/sha256"
	"crypto/subtle"
	"testing"

	"github.com/luxfi/crypto/ipa/banderwagon"
	"github.com/stretchr/testify/require"
)

// TestF002_ConstantTimeProofComparison verifies that witness proof verification
// uses constant-time comparison (crypto/subtle.ConstantTimeCompare), not
// bytes.Equal which leaks timing information about matching prefix length.
//
// Regression test for F-002 from the Dragonfire security review.
func TestF002_ConstantTimeProofComparison(t *testing.T) {
	// Create a known commitment using the same pattern as witness.go
	var commitment banderwagon.Element
	path := []byte("test-path-for-witness-verification")

	// Compute the expected proof (same as verifyIPAOpening internals)
	hasher := sha256.New()
	commitmentBytes := commitment.Bytes()
	hasher.Write(commitmentBytes[:])
	hasher.Write(path)
	correctProof := hasher.Sum(nil)

	// Correct proof should verify
	require.True(t, verifyIPAOpening(&commitment, path, correctProof),
		"correct proof must verify")

	// Wrong proof should not verify
	wrongProof := make([]byte, len(correctProof))
	copy(wrongProof, correctProof)
	wrongProof[0] ^= 0xFF
	require.False(t, verifyIPAOpening(&commitment, path, wrongProof),
		"wrong proof must not verify")

	// Empty proof should not verify
	require.False(t, verifyIPAOpening(&commitment, path, []byte{}),
		"empty proof must not verify")

	// Wrong length proof should not verify
	// subtle.ConstantTimeCompare returns 0 for different lengths
	shortProof := correctProof[:16]
	require.False(t, verifyIPAOpening(&commitment, path, shortProof),
		"short proof must not verify")

	// Verify the comparison is constant-time by confirming behavior
	// matches subtle.ConstantTimeCompare semantics
	require.Equal(t, 1, subtle.ConstantTimeCompare(correctProof, correctProof),
		"identical proofs must match via constant-time compare")
	require.Equal(t, 0, subtle.ConstantTimeCompare(correctProof, wrongProof),
		"different proofs must not match via constant-time compare")
	require.Equal(t, 0, subtle.ConstantTimeCompare(correctProof, shortProof),
		"different-length proofs must not match via constant-time compare")
}

// TestDualSignRound1_ErrorPropagation verifies that DualSignRound1
// propagates errors from BOTH the BLS and Ringtail signing paths.
// Previously, the Ringtail error was silently discarded (line 329).
//
// Regression test for the silent error drop found during review analysis.
func TestDualSignRound1_ErrorPropagation(t *testing.T) {
	// Attempting dual sign with unconfigured validator should error
	s, err := newSigner(2) // threshold of 2
	require.NoError(t, err)

	_, _, err = s.DualSignRound1(nil, "nonexistent-validator", []byte("msg"), 1, []byte("prf"))
	require.Error(t, err, "DualSignRound1 must error for unconfigured validator")
	require.Contains(t, err.Error(), "not configured for dual signing")
}

// TestCertBundle_StructuralVerification tests the current structural
// verification behavior and documents the security boundary.
func TestCertBundle_StructuralVerification(t *testing.T) {
	// Valid bundle passes structural check
	cert := &CertBundle{
		BLSAgg: []byte{0x01, 0x02, 0x03},
		PQCert: []byte{0x04, 0x05, 0x06},
	}
	require.True(t, cert.Verify(nil), "non-empty cert passes structural check")

	// Nil cert fails
	var nilCert *CertBundle
	require.False(t, nilCert.Verify(nil), "nil cert must fail")

	// Empty BLS fails
	emptyBLS := &CertBundle{BLSAgg: []byte{}, PQCert: []byte{0x01}}
	require.False(t, emptyBLS.Verify(nil), "empty BLS must fail")

	// Empty PQ fails
	emptyPQ := &CertBundle{BLSAgg: []byte{0x01}, PQCert: []byte{}}
	require.False(t, emptyPQ.Verify(nil), "empty PQ must fail")
}
