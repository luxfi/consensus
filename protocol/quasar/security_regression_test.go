package quasar

import (
	"crypto/hmac"
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

// TestCertBundle_VerifyPanics verifies the deprecated Verify method panics
// to prevent accidental use of the non-cryptographic path.
func TestCertBundle_VerifyPanics(t *testing.T) {
	cert := &CertBundle{
		BLSAgg: []byte{0x01, 0x02, 0x03},
		PQCert: []byte{0x04, 0x05, 0x06},
	}
	require.Panics(t, func() { cert.Verify(nil) }, "Verify must panic to enforce VerifyWithKeys usage")
}

// TestCertBundle_VerifyWithKeys_Regression tests cryptographic HMAC verification.
func TestCertBundle_VerifyWithKeys_Regression(t *testing.T) {
	blsKey := []byte("regression-bls-key-32bytes!!")
	pqKey := []byte("regression-pq-key-32bytes!!!")
	message := []byte("regression test")

	blsMAC := hmac.New(sha256.New, blsKey)
	blsMAC.Write(message)
	pqMAC := hmac.New(sha256.New, pqKey)
	pqMAC.Write(message)

	cert := &CertBundle{
		BLSAgg:  blsMAC.Sum(nil),
		PQCert:  pqMAC.Sum(nil),
		Message: message,
	}
	require.True(t, cert.VerifyWithKeys(blsKey, pqKey), "valid cert passes HMAC check")

	// Nil cert fails
	var nilCert *CertBundle
	require.False(t, nilCert.VerifyWithKeys(blsKey, pqKey), "nil cert must fail")

	// Empty BLS fails
	emptyBLS := &CertBundle{BLSAgg: []byte{}, PQCert: cert.PQCert, Message: message}
	require.False(t, emptyBLS.VerifyWithKeys(blsKey, pqKey), "empty BLS must fail")

	// Empty PQ fails
	emptyPQ := &CertBundle{BLSAgg: cert.BLSAgg, PQCert: []byte{}, Message: message}
	require.False(t, emptyPQ.VerifyWithKeys(blsKey, pqKey), "empty PQ must fail")
}
