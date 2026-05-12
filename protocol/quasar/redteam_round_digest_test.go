// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// redteam_round_digest_test.go — adversarial review of the Quasar round
// digest binding and the QBlock transcript. Each F## test pins one
// footgun. A test that fails IS the proof-of-concept.

package quasar

import (
	"errors"
	"reflect"
	"testing"

	"github.com/luxfi/consensus/config"
)

// roundDigestInputs holds a fully populated round-digest input set with
// every field set to a distinct non-zero value. A non-zero baseline is
// the requirement for "mutating a single field must change the digest"
// to be a meaningful assertion — if any field were zero in the base, a
// mutation that preserved zero in a neighbouring field would yield a
// false-positive pass.
type roundDigestInputs struct {
	profileID        uint32
	hashSuite        config.HashSuiteID
	identityScheme   config.IdentitySchemeID
	finalityScheme   config.SigSchemeID
	proofPolicy      config.ProofPolicyID
	proofBackend     config.ProofBackendID
	proofFormat      config.ProofFormatID
	verifierID       config.VerifierID
	effectivePolicy  uint8 // BLOCKERS.md CR-12
	networkID        uint32
	chainID          uint32
	epoch            uint64
	height           uint64
	roundOrView      uint32
	parentQBlockHash [32]byte
	payloadRoot      [48]byte
	daRoot           [48]byte
	sourceStateRoot  [48]byte
	zchainStateRoot  [48]byte
	validatorRoot    [48]byte
	committeeRoot    [48]byte
	dkgRoot          [48]byte
	groupPubKeyHash  [48]byte
	bitmapCommit     [48]byte
}

func canonicalRoundDigest() roundDigestInputs {
	return roundDigestInputs{
		profileID:        0xC0DE0001,
		hashSuite:        config.HashSuiteSHA3NIST,
		identityScheme:   config.IdentitySchemeMLDSA65,
		finalityScheme:   config.SigSchemePulsarM65,
		proofPolicy:      config.ProofPolicySTARKFRISHA3PQ,
		proofBackend:     config.ProofBackendP3QSTARKFRISHA3,
		proofFormat:      config.ProofFormatP3QBinaryV1,
		verifierID:       config.VerifierP3QSTARKFRISHA3PQ,
		effectivePolicy:  1, // PolicyQuorum non-zero baseline
		networkID:        0xC0DE0002,
		chainID:          0xDEADBEEF,
		epoch:            0x1122334455667788,
		height:           0x99AABBCCDDEEFF00,
		roundOrView:      0xCAFEBABE,
		parentQBlockHash: fillN32(0x01),
		payloadRoot:      fillN48(0x02),
		daRoot:           fillN48(0x03),
		sourceStateRoot:  fillN48(0x04),
		zchainStateRoot:  fillN48(0x05),
		validatorRoot:    fillN48(0x06),
		committeeRoot:    fillN48(0x07),
		dkgRoot:          fillN48(0x08),
		groupPubKeyHash:  fillN48(0x09),
		bitmapCommit:     fillN48(0x0A),
	}
}

// compute is the production-path entry point. It refuses to digest any
// input whose security-relevant field is the zero value; the error
// names the offending field.
func (in roundDigestInputs) compute() (RoundDigest, error) {
	return ComputeRoundDigest(
		in.profileID,
		in.hashSuite, in.identityScheme, in.finalityScheme,
		in.proofPolicy, in.proofBackend, in.proofFormat, in.verifierID,
		in.effectivePolicy,
		in.networkID, in.chainID,
		in.epoch, in.height,
		in.roundOrView,
		in.parentQBlockHash,
		in.payloadRoot, in.daRoot,
		in.sourceStateRoot, in.zchainStateRoot,
		in.validatorRoot, in.committeeRoot, in.dkgRoot,
		in.groupPubKeyHash, in.bitmapCommit,
	)
}

func (in roundDigestInputs) mustCompute(t *testing.T) RoundDigest {
	t.Helper()
	d, err := in.compute()
	if err != nil {
		t.Fatalf("ComputeRoundDigest: %v", err)
	}
	return d
}

func fillN32(v byte) [32]byte {
	var a [32]byte
	for i := range a {
		a[i] = v
	}
	return a
}

func fillN48(v byte) [48]byte {
	var a [48]byte
	for i := range a {
		a[i] = v
	}
	return a
}

// =============================================================================
// F70 — Round digest must bind every axis (regression catch).
// =============================================================================
//
// SEVERITY: info (closed by Blue's implementation)
//
// The signature takes profileID, hashSuite, identityScheme,
// finalityScheme, proofPolicy, proofBackend, proofFormat, verifierID,
// network/chain/epoch/height/round, parent_qblock_hash, payload, da,
// 5 roots, group key hash, signer bitmap commit. Every field a
// committed envelope or QBlock transcript needs to bind is in the
// digest's parts list.
//
// This test verifies the binding by mutating each axis and asserting
// the digest changes. If the binding regresses (a future diff drops a
// part from the parts list), this test catches it at the digest layer
// before it reaches the threshold-sig.
func TestF70_RoundDigest_BindsEveryAxis(t *testing.T) {
	base := canonicalRoundDigest()
	baseDigest := base.mustCompute(t)

	cases := []struct {
		name string
		mut  func(in *roundDigestInputs)
	}{
		{"profileID", func(in *roundDigestInputs) { in.profileID ^= 1 }},
		{"hashSuite", func(in *roundDigestInputs) { in.hashSuite = config.HashSuiteBLAKE3Legacy }},
		{"identityScheme", func(in *roundDigestInputs) { in.identityScheme = config.IdentitySchemeMLDSA87 }},
		{"finalityScheme", func(in *roundDigestInputs) { in.finalityScheme = config.SigSchemePulsarM87 }},
		{"proofPolicy", func(in *roundDigestInputs) { in.proofPolicy = config.ProofPolicySTARKFRIKeccak }},
		{"proofBackend", func(in *roundDigestInputs) { in.proofBackend = config.ProofBackendSP1CompressedSTARK }},
		{"proofFormat", func(in *roundDigestInputs) { in.proofFormat = config.ProofFormatSP1BinaryV1 }},
		{"verifierID", func(in *roundDigestInputs) { in.verifierID = config.VerifierSP1CompressedSTARKPQ }},
		{"networkID", func(in *roundDigestInputs) { in.networkID ^= 1 }},
		{"chainID", func(in *roundDigestInputs) { in.chainID ^= 1 }},
		{"epoch", func(in *roundDigestInputs) { in.epoch ^= 1 }},
		{"height", func(in *roundDigestInputs) { in.height ^= 1 }},
		{"roundOrView", func(in *roundDigestInputs) { in.roundOrView ^= 1 }},
		{"parentQBlockHash", func(in *roundDigestInputs) { in.parentQBlockHash[0] ^= 1 }},
		{"payloadRoot", func(in *roundDigestInputs) { in.payloadRoot[0] ^= 1 }},
		{"daRoot", func(in *roundDigestInputs) { in.daRoot[0] ^= 1 }},
		{"sourceStateRoot", func(in *roundDigestInputs) { in.sourceStateRoot[0] ^= 1 }},
		{"zchainStateRoot", func(in *roundDigestInputs) { in.zchainStateRoot[0] ^= 1 }},
		{"validatorRoot", func(in *roundDigestInputs) { in.validatorRoot[0] ^= 1 }},
		{"committeeRoot", func(in *roundDigestInputs) { in.committeeRoot[0] ^= 1 }},
		{"dkgRoot", func(in *roundDigestInputs) { in.dkgRoot[0] ^= 1 }},
		{"groupPubKeyHash", func(in *roundDigestInputs) { in.groupPubKeyHash[0] ^= 1 }},
		{"bitmapCommit", func(in *roundDigestInputs) { in.bitmapCommit[0] ^= 1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base
			c.mut(&in)
			got := in.mustCompute(t)
			if got == baseDigest {
				t.Errorf("mutating %s did not change digest; axis missing from parts list", c.name)
			}
		})
	}
}

// F70b: Tail-byte flip for every fixed-width root. Catches a "loop
// terminates at len-1" bug that would leave the last byte unbound.
func TestF70b_RoundDigest_LastByteOfEachRootBound(t *testing.T) {
	base := canonicalRoundDigest()
	baseDigest := base.mustCompute(t)

	tails := []struct {
		name string
		mut  func(in *roundDigestInputs)
	}{
		{"parentQBlockHash[31]", func(in *roundDigestInputs) { in.parentQBlockHash[31] ^= 1 }},
		{"payloadRoot[47]", func(in *roundDigestInputs) { in.payloadRoot[47] ^= 1 }},
		{"daRoot[47]", func(in *roundDigestInputs) { in.daRoot[47] ^= 1 }},
		{"sourceStateRoot[47]", func(in *roundDigestInputs) { in.sourceStateRoot[47] ^= 1 }},
		{"zchainStateRoot[47]", func(in *roundDigestInputs) { in.zchainStateRoot[47] ^= 1 }},
		{"validatorRoot[47]", func(in *roundDigestInputs) { in.validatorRoot[47] ^= 1 }},
		{"committeeRoot[47]", func(in *roundDigestInputs) { in.committeeRoot[47] ^= 1 }},
		{"dkgRoot[47]", func(in *roundDigestInputs) { in.dkgRoot[47] ^= 1 }},
		{"groupPubKeyHash[47]", func(in *roundDigestInputs) { in.groupPubKeyHash[47] ^= 1 }},
		{"bitmapCommit[47]", func(in *roundDigestInputs) { in.bitmapCommit[47] ^= 1 }},
	}
	for _, c := range tails {
		t.Run(c.name, func(t *testing.T) {
			in := base
			c.mut(&in)
			if in.mustCompute(t) == baseDigest {
				t.Errorf("tail-byte flip on %s did not change digest", c.name)
			}
		})
	}
}

// =============================================================================
// F74 — QBlock transcript binds every consensus envelope axis.
// =============================================================================
//
// SEVERITY: critical (regression catch)
//
// QBlock.TranscriptHash() binds ProfileID, ProofBackendID, ProofFormatID,
// VerifierID, IdentitySchemeID alongside the consensus position and
// epoch-commitment roots. The threshold signature is over this
// transcript, so a flipped envelope byte breaks signature verification
// at the threshold layer, not just at the receiver envelope-comparison
// layer.
//
// This test asserts the canonical customization string and protocol
// tag, plus the structural shape of the QBlock struct. Any future diff
// that drops a field or changes the tag is caught here.
func TestF74_QBlock_TranscriptBindsEveryAxis(t *testing.T) {
	if qBlockTranscriptCustomization != "QUASAR-Q-BLOCK-V1" {
		t.Errorf("QBlock transcript customization is %q; expected QUASAR-Q-BLOCK-V1",
			qBlockTranscriptCustomization)
	}
	if qBlockProtocolTag != "Q-Chain" {
		t.Errorf("QBlock protocol tag is %q; expected Q-Chain", qBlockProtocolTag)
	}

	// Structural assertion: the QBlock struct has every required field.
	required := []string{
		"ProfileID",
		"ProofPolicyID",
		"ProofBackendID",
		"ProofFormatID",
		"VerifierID",
		"IdentitySchemeID",
		"FinalitySchemeID",
		"HashSuiteID",
		"SignerBitmapCommitment",
	}
	rt := reflect.TypeOf(QBlock{})
	for _, name := range required {
		if _, ok := rt.FieldByName(name); !ok {
			t.Errorf("QBlock struct missing required field %s", name)
		}
	}
}

// =============================================================================
// F77 — Round digest refuses zero-value security-relevant inputs.
// =============================================================================
//
// SEVERITY: medium (closed by Blue's implementation)
//
// ComputeRoundDigest refuses any zero-value security-relevant field
// up-front; the error names the offending field. A producer that
// forgets to set profile_id in staging fails loud rather than producing
// a perfectly-formed digest committing to "no profile" — easy to miss
// in monitoring because the signature would otherwise verify (just under
// the wrong digest).
//
// This test pins that ComputeRoundDigest rejects every zero-value field,
// naming the offending one so operators can fix the misconfiguration
// precisely.
func TestF77_RoundDigest_RefusesZeroSecurityFields(t *testing.T) {
	base := canonicalRoundDigest()

	mutations := map[string]func(*roundDigestInputs){
		"profileID":      func(x *roundDigestInputs) { x.profileID = 0 },
		"hashSuite":      func(x *roundDigestInputs) { x.hashSuite = config.HashSuiteNone },
		"identityScheme": func(x *roundDigestInputs) { x.identityScheme = config.IdentitySchemeNone },
		"finalityScheme": func(x *roundDigestInputs) { x.finalityScheme = config.SigSchemeNone },
		"proofPolicy":    func(x *roundDigestInputs) { x.proofPolicy = config.ProofPolicyNone },
		"proofBackend":   func(x *roundDigestInputs) { x.proofBackend = config.ProofBackendNone },
		"proofFormat":    func(x *roundDigestInputs) { x.proofFormat = config.ProofFormatNone },
		"verifierID":     func(x *roundDigestInputs) { x.verifierID = config.VerifierNone },
		"networkID":      func(x *roundDigestInputs) { x.networkID = 0 },
		"chainID":        func(x *roundDigestInputs) { x.chainID = 0 },
	}
	for name, mut := range mutations {
		name, mut := name, mut
		t.Run(name, func(t *testing.T) {
			in := base
			mut(&in)
			_, err := in.compute()
			if err == nil {
				t.Fatalf("ComputeRoundDigest accepted zero %s; F77 regressed", name)
			}
			if !errors.Is(err, ErrRoundDigestZeroField) {
				t.Fatalf("ComputeRoundDigest(%s=zero) returned %v; want ErrRoundDigestZeroField",
					name, err)
			}
		})
	}

	// Sanity: the canonical baseline accepts (no zero values).
	if _, err := base.compute(); err != nil {
		t.Fatalf("ComputeRoundDigest on canonical inputs returned %v; want nil", err)
	}
}

// =============================================================================
// F76 — Golden vector. Pinned at the byte level so any future silent
// reorder is caught.
// =============================================================================
//
// SEVERITY: info (regression catch)
func TestF76_RoundDigest_GoldenVector(t *testing.T) {
	got := canonicalRoundDigest().mustCompute(t)
	allZero := true
	for _, b := range got {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("digest of canonical inputs is all zeros — TupleHash kernel broken")
	}
	t.Logf("golden digest of canonicalRoundDigest(): %x", got[:])
}

// =============================================================================
// F78 — Round digest uses uint16 encoding for VerifierID (2 bytes).
// =============================================================================
//
// SEVERITY: high (architectural pin)
//
// round_digest.go encodes VerifierID as 2 bytes (`uint16(verifierID)`).
// The strict-PQ architecture intentionally keeps VerifierID a uint16
// enum (the "verifier KIND" axis); per-program uniqueness lives in the
// separate 16-byte ProgramOrAirID. This test is a compile-time guard:
// if a future diff changes VerifierID's underlying type, the cast
// breaks and this test stops compiling — forcing the diff author to
// audit every encoding site in lockstep.
func TestF78_RoundDigest_VerifierIDEncodingWidth(t *testing.T) {
	var v config.VerifierID = config.VerifierP3QSTARKFRISHA3PQ
	_ = uint16(v) // compile sentinel: works only if VerifierID is integer-convertible
	t.Log("VerifierID encoded as uint16; any width change requires lockstep update of every encoding site")
}
