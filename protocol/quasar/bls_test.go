package quasar

import (
	"crypto/hmac"
	"crypto/sha256"
	"testing"
)

// TestCertBundle_Verify_C2_Panics is the regression test for C-2.
// The deprecated Verify method must panic to prevent accidental use
// of the non-cryptographic verification path.
func TestCertBundle_Verify_C2_Panics(t *testing.T) {
	cert := &CertBundle{
		BLSAgg: []byte{0x01, 0x02, 0x03},
		PQCert: []byte{0x04, 0x05, 0x06},
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("CertBundle.Verify must panic")
		}
	}()
	cert.Verify(nil)
}

// TestCertBundle_VerifyWithKeys_Valid tests the correct verification path.
func TestCertBundle_VerifyWithKeys_Valid(t *testing.T) {
	blsKey := []byte("bls-key-for-testing-32-bytes!")
	pqKey := []byte("pq-key-for-testing-32-bytes!!")
	message := []byte("test-message-data")

	blsMAC := hmac.New(sha256.New, blsKey)
	blsMAC.Write(message)
	pqMAC := hmac.New(sha256.New, pqKey)
	pqMAC.Write(message)

	cert := &CertBundle{
		BLSAgg:  blsMAC.Sum(nil),
		PQCert:  pqMAC.Sum(nil),
		Message: message,
	}

	if !cert.VerifyWithKeys(blsKey, pqKey) {
		t.Error("valid cert should pass VerifyWithKeys")
	}

	// Wrong BLS key
	if cert.VerifyWithKeys([]byte("wrong-key"), pqKey) {
		t.Error("wrong BLS key should fail")
	}

	// Wrong PQ key
	if cert.VerifyWithKeys(blsKey, []byte("wrong-key")) {
		t.Error("wrong PQ key should fail")
	}

	// Nil cert
	var nilCert *CertBundle
	if nilCert.VerifyWithKeys(blsKey, pqKey) {
		t.Error("nil cert should fail")
	}

	// Missing message
	noMsg := &CertBundle{BLSAgg: cert.BLSAgg, PQCert: cert.PQCert}
	if noMsg.VerifyWithKeys(blsKey, pqKey) {
		t.Error("cert with no message should fail")
	}
}
