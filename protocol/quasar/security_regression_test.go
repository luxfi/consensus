package quasar

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// NOTE: TestF002_ConstantTimeProofComparison was removed together with the dead
// VerkleWitness apparatus (witness.go) it exercised — verifyIPAOpening was a
// helper of that forgery-sink-adjacent, zero-non-test-caller code. The
// constant-time-compare property it asserted lives at the (still-tested) callers
// of crypto/subtle elsewhere; nothing in production depended on verifyIPAOpening.

// TestDualSignRound1_ErrorPropagation verifies that DualSignRound1
// propagates errors from BOTH the BLS and Corona signing paths.
// Previously, the Corona error was silently discarded (line 329).
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

// TestCertBundle_VerifyWithKeys_Regression tests cryptographic KMAC256 verification.
func TestCertBundle_VerifyWithKeys_Regression(t *testing.T) {
	blsKey := []byte("regression-bls-key-32bytes!!")
	pqKey := []byte("regression-pq-key-32bytes!!!")
	message := []byte("regression test")

	cert := &CertBundle{
		BLSAgg:  kmac256(blsKey, message, kmacMACOutLen, customQuasarEventHorizonBLSMAC),
		PQCert:  kmac256(pqKey, message, kmacMACOutLen, customQuasarEventHorizonPQMAC),
		Message: message,
	}
	require.True(t, cert.VerifyWithKeys(blsKey, pqKey), "valid cert passes KMAC256 check")

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
