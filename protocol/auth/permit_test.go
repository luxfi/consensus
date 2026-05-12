// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// canonicalPermit returns a deeply non-zero PQPermit fixture used by
// digest-mutation tests. Every field is set to a value distinct from
// every other field.
func canonicalPermit() *PQPermit {
	return &PQPermit{
		Version:           1,
		ProfileID:         config.ProfileStrictPQ,
		ChainID:           96369,
		VerifyingContract: [48]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48},
		OwnerAccountID:    [48]byte{49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96},
		Spender:           [48]byte{97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143, 144},
		Value:             [32]byte{145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159, 160, 161, 162, 163, 164, 165, 166, 167, 168, 169, 170, 171, 172, 173, 174, 175, 176},
		Nonce:             7,
		Deadline:          2_000_000,
		AuthSchemeID:      ContractAuthMLDSA65,
		HashSuiteID:       config.HashSuiteSHA3NIST,
		Signature:         bytes.Repeat([]byte{0x55}, 3309),
	}
}

// TestPQPermit_Digest_BindsEveryField — for every field that's bound
// into Digest(), a mutation MUST change the output. One subtest per
// field; a failure names exactly which field is missing.
func TestPQPermit_Digest_BindsEveryField(t *testing.T) {
	base := canonicalPermit()
	baseDigest := base.Digest()

	mutations := []struct {
		name string
		mut  func(*PQPermit)
	}{
		{"Version", func(p *PQPermit) { p.Version = 99 }},
		{"ProfileID", func(p *PQPermit) { p.ProfileID = config.ProfilePermissive }},
		{"ChainID", func(p *PQPermit) { p.ChainID = 1 }},
		{"VerifyingContract", func(p *PQPermit) { p.VerifyingContract[0] ^= 0xFF }},
		{"OwnerAccountID", func(p *PQPermit) { p.OwnerAccountID[0] ^= 0xFF }},
		{"Spender", func(p *PQPermit) { p.Spender[0] ^= 0xFF }},
		{"Value", func(p *PQPermit) { p.Value[0] ^= 0xFF }},
		{"Nonce", func(p *PQPermit) { p.Nonce = 8 }},
		{"Deadline", func(p *PQPermit) { p.Deadline = 2_000_001 }},
		{"AuthSchemeID", func(p *PQPermit) { p.AuthSchemeID = ContractAuthMLDSA87 }},
		{"HashSuiteID", func(p *PQPermit) { p.HashSuiteID = config.HashSuiteBLAKE3Legacy }},
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			cpy := *base
			m.mut(&cpy)
			if cpy.Digest() == baseDigest {
				t.Fatalf("digest did not change after mutating %s — binding missing", m.name)
			}
		})
	}

	// Signature is NOT bound — mutating it MUST NOT change the digest.
	t.Run("Signature_NotBound", func(t *testing.T) {
		cpy := *base
		cpy.Signature = bytes.Repeat([]byte{0x00}, 3309)
		if cpy.Digest() != baseDigest {
			t.Fatalf("digest changed when Signature was mutated — Signature must not be bound")
		}
	})
}

// TestPQPermit_Digest_Deterministic — two Digest() calls yield the same
// bytes.
func TestPQPermit_Digest_Deterministic(t *testing.T) {
	p := canonicalPermit()
	d1 := p.Digest()
	d2 := p.Digest()
	if d1 != d2 {
		t.Fatalf("PQPermit.Digest not deterministic: %x vs %x", d1, d2)
	}
}

// TestPQPermit_Verify_HappyPath — well-formed permit verifies cleanly
// when the injected signature verifier returns true.
func TestPQPermit_Verify_HappyPath(t *testing.T) {
	profile := config.StrictPQ()
	permit := canonicalPermit()
	pubkey := bytes.Repeat([]byte{0x77}, 1952)

	err := VerifyPQPermit(profile, permit, pubkey, stubSigVerifier(true))
	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
}

// TestPQPermit_Verify_RejectsNilArguments — nil receivers / missing
// verifier / nil profile are typed errors.
func TestPQPermit_Verify_RejectsNilArguments(t *testing.T) {
	profile := config.StrictPQ()
	permit := canonicalPermit()
	pubkey := bytes.Repeat([]byte{0x11}, 1952)

	if err := VerifyPQPermit(profile, nil, pubkey, stubSigVerifier(true)); !errors.Is(err, ErrPQPermitNil) {
		t.Errorf("nil permit: got %v", err)
	}
	if err := VerifyPQPermit(nil, permit, pubkey, stubSigVerifier(true)); !errors.Is(err, ErrPQPermitInvalidProfile) {
		t.Errorf("nil profile: got %v", err)
	}
	if err := VerifyPQPermit(profile, permit, pubkey, nil); !errors.Is(err, ErrPQPermitMissingSigVerifier) {
		t.Errorf("nil sig verifier: got %v", err)
	}
}

// TestPQPermit_Verify_RejectsClassicalScheme — a classical
// AuthSchemeID under any profile is refused. The strict-PQ check
// piggy-backs on IsPostQuantum.
func TestPQPermit_Verify_RejectsClassicalScheme(t *testing.T) {
	profile := config.StrictPQ()
	permit := canonicalPermit()
	permit.AuthSchemeID = ContractAuthECDSASecp256k1Legacy

	pubkey := bytes.Repeat([]byte{0xCC}, 64)
	err := VerifyPQPermit(profile, permit, pubkey, stubSigVerifier(true))
	if !errors.Is(err, ErrPQPermitAuthSchemeNotAllowed) {
		t.Fatalf("classical auth-scheme accepted: err=%v", err)
	}
}

// TestPQPermit_Verify_RejectsHashSuiteMismatch — permit.HashSuiteID
// must match profile.HashSuiteID.
func TestPQPermit_Verify_RejectsHashSuiteMismatch(t *testing.T) {
	profile := config.StrictPQ() // pins SHA3_NIST
	permit := canonicalPermit()
	permit.HashSuiteID = config.HashSuiteBLAKE3Legacy

	pubkey := bytes.Repeat([]byte{0xEE}, 1952)
	err := VerifyPQPermit(profile, permit, pubkey, stubSigVerifier(true))
	if !errors.Is(err, ErrPQPermitHashSuiteMismatch) {
		t.Fatalf("hash-suite drift accepted: err=%v", err)
	}
}

// TestPQPermit_Verify_RejectsProfileMismatch — permit.ProfileID must
// match profile.ProfileID.
func TestPQPermit_Verify_RejectsProfileMismatch(t *testing.T) {
	profile := config.StrictPQ()
	permit := canonicalPermit()
	permit.ProfileID = config.ProfileFIPS

	pubkey := bytes.Repeat([]byte{0xFF}, 1952)
	err := VerifyPQPermit(profile, permit, pubkey, stubSigVerifier(true))
	if !errors.Is(err, ErrPQPermitInvalidProfile) {
		t.Fatalf("profile drift accepted: err=%v", err)
	}
}

// TestPQPermit_Verify_RejectsInvalidSignature — sigVerifier returning
// false is surfaced as ErrPQPermitSignatureInvalid.
func TestPQPermit_Verify_RejectsInvalidSignature(t *testing.T) {
	profile := config.StrictPQ()
	permit := canonicalPermit()
	pubkey := bytes.Repeat([]byte{0x88}, 1952)

	err := VerifyPQPermit(profile, permit, pubkey, stubSigVerifier(false))
	if !errors.Is(err, ErrPQPermitSignatureInvalid) {
		t.Fatalf("invalid sig accepted: err=%v", err)
	}
}

// TestPQPermit_Verify_RejectsEmptyOwnerPubkey — caller passing an empty
// pubkey is refused before sigVerifier is invoked.
func TestPQPermit_Verify_RejectsEmptyOwnerPubkey(t *testing.T) {
	profile := config.StrictPQ()
	permit := canonicalPermit()

	sigCalled := false
	err := VerifyPQPermit(profile, permit, nil, func(scheme WalletSchemeID, pk, msg, sig []byte) (bool, error) {
		sigCalled = true
		return true, nil
	})
	if !errors.Is(err, ErrPQPermitEmptyOwnerPubkey) {
		t.Fatalf("empty pubkey accepted: err=%v", err)
	}
	if sigCalled {
		t.Fatalf("sigVerifier called on empty-pubkey permit — early refusal broken")
	}
}

// TestPQPermit_Verify_RejectsVersionZero — Version=0 is unsupported.
func TestPQPermit_Verify_RejectsVersionZero(t *testing.T) {
	profile := config.StrictPQ()
	permit := canonicalPermit()
	permit.Version = 0

	pubkey := bytes.Repeat([]byte{0x66}, 1952)
	err := VerifyPQPermit(profile, permit, pubkey, stubSigVerifier(true))
	if !errors.Is(err, ErrPQPermitVersionUnsupported) {
		t.Fatalf("Version=0 accepted: err=%v", err)
	}
}
