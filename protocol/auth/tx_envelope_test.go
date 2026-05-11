// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/luxfi/consensus/config"
)

// canonicalTxAuth returns a deeply non-zero TxAuthEnvelope fixture used
// by digest-mutation tests. Every field is set to a value that distinguishes
// it from the other fields, so a digest collision under a mutation
// indicates a missing binding, not a fixture accident.
func canonicalTxAuth() *TxAuthEnvelope {
	return &TxAuthEnvelope{
		Version:          1,
		ProfileID:        config.ProfileLuxStrictPQ,
		ChainID:          43114,
		NetworkID:        1,
		AccountID:        [48]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48},
		Nonce:            42,
		ExpiryHeight:     1_000_000,
		WalletSchemeID:   WalletSchemeMLDSA65,
		HashSuiteID:      config.HashSuiteSHA3NIST,
		FeePayer:         [48]byte{49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96},
		GasLimit:         21000,
		MaxFee:           [32]byte{97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128},
		CallRoot:         [32]byte{129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143, 144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159, 160},
		AccessListRoot:   [32]byte{161, 162, 163, 164, 165, 166, 167, 168, 169, 170, 171, 172, 173, 174, 175, 176, 177, 178, 179, 180, 181, 182, 183, 184, 185, 186, 187, 188, 189, 190, 191, 192},
		ZIdentityRoot:    [32]byte{193, 194, 195, 196, 197, 198, 199, 200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219, 220, 221, 222, 223, 224},
		AccountStateRoot: [32]byte{225, 226, 227, 228, 229, 230, 231, 232, 233, 234, 235, 236, 237, 238, 239, 240, 241, 242, 243, 244, 245, 246, 247, 248, 249, 250, 251, 252, 253, 254, 255, 0},
		PublicKeyRef:     [32]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A},
		Signature:        bytes.Repeat([]byte{0x42}, 3309), // ML-DSA-65 signature size
	}
}

// TestTxAuthEnvelope_SigningDigest_BindsEveryField — for every
// security-relevant field, mutating that field MUST change the digest.
// One subtest per field; a failure names exactly which field is missing
// from the binding.
func TestTxAuthEnvelope_SigningDigest_BindsEveryField(t *testing.T) {
	base := canonicalTxAuth()
	baseDigest := base.SigningDigest()

	// Mutation table: each entry mutates one field of a fresh copy and
	// asserts the digest changes. Signature is intentionally not in the
	// table — Signature is what the digest is signed under, not bound.
	mutations := []struct {
		name string
		mut  func(*TxAuthEnvelope)
	}{
		{"Version", func(e *TxAuthEnvelope) { e.Version = 99 }},
		{"ProfileID", func(e *TxAuthEnvelope) { e.ProfileID = config.ProfileLuxPermissive }},
		{"ChainID", func(e *TxAuthEnvelope) { e.ChainID = 1 }},
		{"NetworkID", func(e *TxAuthEnvelope) { e.NetworkID = 5 }},
		{"AccountID", func(e *TxAuthEnvelope) { e.AccountID[0] ^= 0xFF }},
		{"Nonce", func(e *TxAuthEnvelope) { e.Nonce = 43 }},
		{"ExpiryHeight", func(e *TxAuthEnvelope) { e.ExpiryHeight = 1_000_001 }},
		{"WalletSchemeID", func(e *TxAuthEnvelope) { e.WalletSchemeID = WalletSchemeMLDSA87 }},
		{"HashSuiteID", func(e *TxAuthEnvelope) { e.HashSuiteID = config.HashSuiteBLAKE3Legacy }},
		{"FeePayer", func(e *TxAuthEnvelope) { e.FeePayer[0] ^= 0xFF }},
		{"GasLimit", func(e *TxAuthEnvelope) { e.GasLimit = 21001 }},
		{"MaxFee", func(e *TxAuthEnvelope) { e.MaxFee[0] ^= 0xFF }},
		{"CallRoot", func(e *TxAuthEnvelope) { e.CallRoot[0] ^= 0xFF }},
		{"AccessListRoot", func(e *TxAuthEnvelope) { e.AccessListRoot[0] ^= 0xFF }},
		{"ZIdentityRoot", func(e *TxAuthEnvelope) { e.ZIdentityRoot[0] ^= 0xFF }},
		{"AccountStateRoot", func(e *TxAuthEnvelope) { e.AccountStateRoot[0] ^= 0xFF }},
		{"PublicKeyRef", func(e *TxAuthEnvelope) { e.PublicKeyRef[0] ^= 0xFF }},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			cpy := *base
			m.mut(&cpy)
			mutated := cpy.SigningDigest()
			if mutated == baseDigest {
				t.Fatalf("digest did not change after mutating %s — binding missing", m.name)
			}
		})
	}

	// Sanity: mutating ONLY the Signature MUST NOT change the digest.
	// Signature is what the wallet computes over the digest; the digest
	// itself MUST be independent of Signature.
	t.Run("Signature_NotBound", func(t *testing.T) {
		cpy := *base
		cpy.Signature = bytes.Repeat([]byte{0x00}, 3309)
		if cpy.SigningDigest() != baseDigest {
			t.Fatalf("digest changed when Signature was mutated — Signature must not be bound into the signing digest")
		}
	})
}

// TestTxAuthEnvelope_SigningDigest_Deterministic — two SigningDigest()
// calls on the same envelope produce the same 48 bytes.
func TestTxAuthEnvelope_SigningDigest_Deterministic(t *testing.T) {
	base := canonicalTxAuth()
	d1 := base.SigningDigest()
	d2 := base.SigningDigest()
	if d1 != d2 {
		t.Fatalf("SigningDigest not deterministic: %x vs %x", d1, d2)
	}
}

// TestTxAuthEnvelope_Marshal_RoundTrip — Marshal followed by
// UnmarshalTxAuthEnvelope yields a byte-equal envelope.
func TestTxAuthEnvelope_Marshal_RoundTrip(t *testing.T) {
	base := canonicalTxAuth()
	wire, err := base.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	decoded, err := UnmarshalTxAuthEnvelope(wire)
	if err != nil {
		t.Fatalf("UnmarshalTxAuthEnvelope failed: %v", err)
	}

	// Compare every field. Signature is a slice so use bytes.Equal.
	if decoded.Version != base.Version {
		t.Errorf("Version: got %d want %d", decoded.Version, base.Version)
	}
	if decoded.ProfileID != base.ProfileID {
		t.Errorf("ProfileID: got %d want %d", decoded.ProfileID, base.ProfileID)
	}
	if decoded.ChainID != base.ChainID {
		t.Errorf("ChainID: got %d want %d", decoded.ChainID, base.ChainID)
	}
	if decoded.NetworkID != base.NetworkID {
		t.Errorf("NetworkID: got %d want %d", decoded.NetworkID, base.NetworkID)
	}
	if decoded.AccountID != base.AccountID {
		t.Errorf("AccountID: got %x want %x", decoded.AccountID, base.AccountID)
	}
	if decoded.Nonce != base.Nonce {
		t.Errorf("Nonce: got %d want %d", decoded.Nonce, base.Nonce)
	}
	if decoded.ExpiryHeight != base.ExpiryHeight {
		t.Errorf("ExpiryHeight: got %d want %d", decoded.ExpiryHeight, base.ExpiryHeight)
	}
	if decoded.WalletSchemeID != base.WalletSchemeID {
		t.Errorf("WalletSchemeID: got 0x%02x want 0x%02x", decoded.WalletSchemeID, base.WalletSchemeID)
	}
	if decoded.HashSuiteID != base.HashSuiteID {
		t.Errorf("HashSuiteID: got 0x%02x want 0x%02x", decoded.HashSuiteID, base.HashSuiteID)
	}
	if decoded.FeePayer != base.FeePayer {
		t.Errorf("FeePayer: got %x want %x", decoded.FeePayer, base.FeePayer)
	}
	if decoded.GasLimit != base.GasLimit {
		t.Errorf("GasLimit: got %d want %d", decoded.GasLimit, base.GasLimit)
	}
	if decoded.MaxFee != base.MaxFee {
		t.Errorf("MaxFee: got %x want %x", decoded.MaxFee, base.MaxFee)
	}
	if decoded.CallRoot != base.CallRoot {
		t.Errorf("CallRoot: got %x want %x", decoded.CallRoot, base.CallRoot)
	}
	if decoded.AccessListRoot != base.AccessListRoot {
		t.Errorf("AccessListRoot: got %x want %x", decoded.AccessListRoot, base.AccessListRoot)
	}
	if decoded.ZIdentityRoot != base.ZIdentityRoot {
		t.Errorf("ZIdentityRoot: got %x want %x", decoded.ZIdentityRoot, base.ZIdentityRoot)
	}
	if decoded.AccountStateRoot != base.AccountStateRoot {
		t.Errorf("AccountStateRoot: got %x want %x", decoded.AccountStateRoot, base.AccountStateRoot)
	}
	if decoded.PublicKeyRef != base.PublicKeyRef {
		t.Errorf("PublicKeyRef: got %x want %x", decoded.PublicKeyRef, base.PublicKeyRef)
	}
	if !bytes.Equal(decoded.Signature, base.Signature) {
		t.Errorf("Signature: got len=%d want len=%d (bytes-equal=%v)",
			len(decoded.Signature), len(base.Signature), bytes.Equal(decoded.Signature, base.Signature))
	}

	// The decoded envelope's digest MUST equal the original's digest:
	// round-tripping cannot change the signed-over bytes.
	if decoded.SigningDigest() != base.SigningDigest() {
		t.Fatalf("round-trip digest changed: original=%x decoded=%x",
			base.SigningDigest(), decoded.SigningDigest())
	}
}

// TestTxAuthEnvelope_Marshal_NilReceiver — calling Marshal on a nil
// receiver returns a typed error (no panic).
func TestTxAuthEnvelope_Marshal_NilReceiver(t *testing.T) {
	var e *TxAuthEnvelope
	_, err := e.Marshal()
	if !errors.Is(err, ErrTxAuthNilEnvelope) {
		t.Fatalf("nil-receiver Marshal: got %v, want ErrTxAuthNilEnvelope", err)
	}
}

// TestTxAuthEnvelope_Unmarshal_RefusesZeroEnums — the codec refuses any
// zero-init security-relevant enum at decode time, even if the framing
// itself is valid. Closes the "construct a zero envelope and trick a
// downstream profile check that skips a path" attack class.
func TestTxAuthEnvelope_Unmarshal_RefusesZeroEnums(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*TxAuthEnvelope)
	}{
		{
			name: "ProfileID=None",
			mut:  func(e *TxAuthEnvelope) { e.ProfileID = config.ProfileNone },
		},
		{
			name: "WalletSchemeID=None",
			mut:  func(e *TxAuthEnvelope) { e.WalletSchemeID = WalletSchemeNone },
		},
		{
			name: "HashSuiteID=None",
			mut:  func(e *TxAuthEnvelope) { e.HashSuiteID = config.HashSuiteNone },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := canonicalTxAuth()
			tc.mut(env)
			wire, err := env.Marshal()
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			_, decodeErr := UnmarshalTxAuthEnvelope(wire)
			if !errors.Is(decodeErr, ErrTxAuthZeroEnum) {
				t.Fatalf("Unmarshal accepted zero enum %s: err=%v", tc.name, decodeErr)
			}
		})
	}
}

// TestTxAuthEnvelope_Unmarshal_TruncatedInput — a buffer that's one
// byte short MUST be refused, not silently truncated.
func TestTxAuthEnvelope_Unmarshal_TruncatedInput(t *testing.T) {
	env := canonicalTxAuth()
	wire, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	// Cut the last byte off — within the Signature payload.
	_, decodeErr := UnmarshalTxAuthEnvelope(wire[:len(wire)-1])
	if decodeErr == nil {
		t.Fatalf("UnmarshalTxAuthEnvelope accepted truncated input")
	}
}

// TestTxAuthEnvelope_Unmarshal_TrailingBytes — a buffer with extra
// trailing bytes after the envelope MUST be refused.
func TestTxAuthEnvelope_Unmarshal_TrailingBytes(t *testing.T) {
	env := canonicalTxAuth()
	wire, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	wire = append(wire, 0xFF)
	_, decodeErr := UnmarshalTxAuthEnvelope(wire)
	if !errors.Is(decodeErr, ErrTxAuthTrailingBytes) {
		t.Fatalf("UnmarshalTxAuthEnvelope accepted trailing bytes: err=%v", decodeErr)
	}
}

// =============================================================================
// VerifyTxAuthEnvelope tests
// =============================================================================

// stubLookup constructs an AccountStateLookupFn that returns the given
// pubkey and accountStateRoot. Used by VerifyTxAuthEnvelope tests to
// inject a stable account-state lookup.
func stubLookup(pubkey []byte, accountStateRoot [32]byte) AccountStateLookupFn {
	return func(accountID [48]byte, pkRef [32]byte) ([]byte, [32]byte, error) {
		return pubkey, accountStateRoot, nil
	}
}

// stubSigVerifier returns a verifier that returns ok=true for any input.
// Used to test the happy path independently of an actual ML-DSA
// implementation.
func stubSigVerifier(returnOK bool) SignatureVerifierFn {
	return func(scheme WalletSchemeID, pubkey, msg, sig []byte) (bool, error) {
		return returnOK, nil
	}
}

// strictPQProfile returns a fresh ProfileLuxStrictPQ for tests.
func strictPQProfile() *config.ChainSecurityProfile {
	return config.LuxStrictPQ()
}

// TestVerifyTxAuthEnvelope_HappyPath — a synthetic ML-DSA-65 wallet
// signing under LuxStrictPQ verifies cleanly. The signature itself is
// stubbed (the actual ML-DSA verifier lives in luxfi/pulsar and is
// injected by coreth at runtime).
func TestVerifyTxAuthEnvelope_HappyPath(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0x77}, 1952) // ML-DSA-65 pubkey size
	profile := strictPQProfile()

	env := canonicalTxAuth()
	// AccountID MUST be derived from (profileID, chainID, scheme, pubkey).
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)

	currentASR := env.AccountStateRoot // happy path: envelope matches chain state

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, currentASR),
		stubSigVerifier(true),
		func() uint64 { return env.ExpiryHeight - 1 },
	)
	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsECDSAWalletUnderStrict — a strict-PQ
// profile MUST refuse an envelope whose WalletSchemeID is the ECDSA
// legacy marker.
func TestVerifyTxAuthEnvelope_RejectsECDSAWalletUnderStrict(t *testing.T) {
	profile := strictPQProfile()
	env := canonicalTxAuth()
	env.WalletSchemeID = WalletSchemeECDSASecp256k1Legacy

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup([]byte{1, 2, 3}, env.AccountStateRoot),
		stubSigVerifier(true),
		nil,
	)
	if !errors.Is(err, ErrTxAuthWalletSchemeNotAllowed) {
		t.Fatalf("strict-PQ accepted ECDSA wallet: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsExpiredHeight — an envelope whose
// ExpiryHeight is at or below currentHeight MUST be refused.
func TestVerifyTxAuthEnvelope_RejectsExpiredHeight(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0x88}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)
	env.ExpiryHeight = 100

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, env.AccountStateRoot),
		stubSigVerifier(true),
		func() uint64 { return 101 }, // current height past expiry
	)
	if !errors.Is(err, ErrTxAuthExpired) {
		t.Fatalf("expired envelope accepted: err=%v", err)
	}

	// Equal heights also refuse (expiry is exclusive of current height).
	err = VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, env.AccountStateRoot),
		stubSigVerifier(true),
		func() uint64 { return 100 },
	)
	if !errors.Is(err, ErrTxAuthExpired) {
		t.Fatalf("expiry at current height accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsHashSuiteMismatch — an envelope whose
// HashSuiteID does not match the profile's pinned suite is refused.
func TestVerifyTxAuthEnvelope_RejectsHashSuiteMismatch(t *testing.T) {
	profile := strictPQProfile() // pins HashSuiteSHA3NIST
	pubkey := bytes.Repeat([]byte{0x55}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)
	env.HashSuiteID = config.HashSuiteBLAKE3Legacy

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, env.AccountStateRoot),
		stubSigVerifier(true),
		nil,
	)
	if !errors.Is(err, ErrTxAuthHashSuiteMismatch) {
		t.Fatalf("hash-suite drift accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsProfileMismatch — env.ProfileID must
// match profile.ProfileID byte-for-byte.
func TestVerifyTxAuthEnvelope_RejectsProfileMismatch(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0x66}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)
	env.ProfileID = config.ProfileLuxPermissive // mismatch

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, env.AccountStateRoot),
		stubSigVerifier(true),
		nil,
	)
	if !errors.Is(err, ErrTxAuthInvalidProfile) {
		t.Fatalf("profile drift accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsAccountIDMismatch — if the resolved
// pubkey does not derive to env.AccountID, the envelope is refused.
// Closes the "wrong key for the account" attack class deterministically.
func TestVerifyTxAuthEnvelope_RejectsAccountIDMismatch(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0x11}, 1952)
	wrongPubkey := bytes.Repeat([]byte{0x22}, 1952)

	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(wrongPubkey, env.AccountStateRoot), // lookup returns the wrong key
		stubSigVerifier(true),
		nil,
	)
	if !errors.Is(err, ErrTxAuthAccountIDMismatch) {
		t.Fatalf("account-id mismatch accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsStaleAccountStateRoot — if the chain's
// current AccountStateRoot differs from env.AccountStateRoot, the
// envelope is rejected.
func TestVerifyTxAuthEnvelope_RejectsStaleAccountStateRoot(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0x99}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)

	freshASR := env.AccountStateRoot
	freshASR[0] ^= 0xFF // chain-side root differs from envelope's

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, freshASR),
		stubSigVerifier(true),
		nil,
	)
	if !errors.Is(err, ErrTxAuthAccountStateRoot) {
		t.Fatalf("stale account-state root accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsInvalidSignature — when the injected
// SignatureVerifierFn returns ok=false, the envelope is refused.
func TestVerifyTxAuthEnvelope_RejectsInvalidSignature(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0xAA}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, env.AccountStateRoot),
		stubSigVerifier(false), // signature does not verify
		nil,
	)
	if !errors.Is(err, ErrTxAuthSignatureInvalid) {
		t.Fatalf("invalid signature accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsEmptySignature — an envelope with a
// zero-length Signature is refused before the verifier is invoked.
func TestVerifyTxAuthEnvelope_RejectsEmptySignature(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0xBB}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)
	env.Signature = nil

	sigCalled := false
	err := VerifyTxAuthEnvelope(
		profile,
		env,
		stubLookup(pubkey, env.AccountStateRoot),
		func(scheme WalletSchemeID, pk, msg, sig []byte) (bool, error) {
			sigCalled = true
			return true, nil
		},
		nil,
	)
	if !errors.Is(err, ErrTxAuthSignatureInvalid) {
		t.Fatalf("empty signature accepted: err=%v", err)
	}
	if sigCalled {
		t.Fatalf("verifier called on empty-signature envelope — early refusal broken")
	}
}

// TestVerifyTxAuthEnvelope_RejectsNilArguments — nil profile, nil
// envelope, missing lookup, missing verifier are typed errors.
func TestVerifyTxAuthEnvelope_RejectsNilArguments(t *testing.T) {
	profile := strictPQProfile()
	env := canonicalTxAuth()

	// nil envelope
	if err := VerifyTxAuthEnvelope(profile, nil, stubLookup(nil, [32]byte{}), stubSigVerifier(true), nil); !errors.Is(err, ErrTxAuthNilEnvelope) {
		t.Errorf("nil envelope: got %v", err)
	}
	// nil profile
	if err := VerifyTxAuthEnvelope(nil, env, stubLookup(nil, [32]byte{}), stubSigVerifier(true), nil); !errors.Is(err, ErrTxAuthInvalidProfile) {
		t.Errorf("nil profile: got %v", err)
	}
	// missing lookup
	if err := VerifyTxAuthEnvelope(profile, env, nil, stubSigVerifier(true), nil); !errors.Is(err, ErrTxAuthMissingAccountLookup) {
		t.Errorf("missing lookup: got %v", err)
	}
	// missing verifier
	if err := VerifyTxAuthEnvelope(profile, env, stubLookup(nil, [32]byte{}), nil, nil); !errors.Is(err, ErrTxAuthMissingSigVerifier) {
		t.Errorf("missing verifier: got %v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsVersionZero — Version=0 is unsupported.
func TestVerifyTxAuthEnvelope_RejectsVersionZero(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0xCC}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)
	env.Version = 0

	err := VerifyTxAuthEnvelope(
		profile, env,
		stubLookup(pubkey, env.AccountStateRoot),
		stubSigVerifier(true), nil,
	)
	if !errors.Is(err, ErrTxAuthVersionUnsupported) {
		t.Fatalf("Version=0 accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_RejectsFutureVersion — Version > current.
func TestVerifyTxAuthEnvelope_RejectsFutureVersion(t *testing.T) {
	profile := strictPQProfile()
	pubkey := bytes.Repeat([]byte{0xDD}, 1952)
	env := canonicalTxAuth()
	env.AccountID = DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)
	env.Version = math.MaxUint16

	err := VerifyTxAuthEnvelope(
		profile, env,
		stubLookup(pubkey, env.AccountStateRoot),
		stubSigVerifier(true), nil,
	)
	if !errors.Is(err, ErrTxAuthVersionUnsupported) {
		t.Fatalf("future Version accepted: err=%v", err)
	}
}

// TestVerifyTxAuthEnvelope_LookupErrorSurfaces — the lookup's error
// propagates as ErrTxAuthAccountStateLookup.
func TestVerifyTxAuthEnvelope_LookupErrorSurfaces(t *testing.T) {
	profile := strictPQProfile()
	env := canonicalTxAuth()
	myErr := errors.New("synthetic lookup failure")

	err := VerifyTxAuthEnvelope(
		profile,
		env,
		func(accountID [48]byte, pkRef [32]byte) ([]byte, [32]byte, error) {
			return nil, [32]byte{}, myErr
		},
		stubSigVerifier(true),
		nil,
	)
	if !errors.Is(err, ErrTxAuthAccountStateLookup) {
		t.Fatalf("lookup error not surfaced as ErrTxAuthAccountStateLookup: %v", err)
	}
}
