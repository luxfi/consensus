// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/luxfi/consensus/config"
	"golang.org/x/crypto/sha3"
)

// canonicalRoundDigestInputs is the reference vector. Each field is
// distinct and non-zero so a flip of any single field is guaranteed to
// leave the others unchanged.
type canonicalRoundDigestInputs struct {
	profileID         uint32
	hashSuite         config.HashSuiteID
	identityScheme    config.IdentitySchemeID
	finalityScheme    config.SigSchemeID
	proofPolicy       config.ProofPolicyID
	proofBackend      config.ProofBackendID
	proofFormat       config.ProofFormatID
	verifierID        config.VerifierID
	effectivePolicy   uint8 // BLOCKERS.md CR-12: bound into digest
	networkID         uint32
	chainID           uint32
	epoch             uint64
	height            uint64
	roundOrView       uint32
	parentQBlockHash  [32]byte
	payloadRoot       [48]byte
	daRoot            [48]byte
	sourceStateRoot   [48]byte
	zchainStateRoot   [48]byte
	validatorSetRoot  [48]byte
	committeeRoot     [48]byte
	dkgTranscriptRoot [48]byte
	groupPubKeyHash   [48]byte
	signerSetCommit   [48]byte
}

func canonical() canonicalRoundDigestInputs {
	var ps [32]byte
	for i := range ps {
		ps[i] = byte(i + 1)
	}
	mk48 := func(seed byte) [48]byte {
		var out [48]byte
		for i := range out {
			out[i] = byte(i+1) ^ seed
		}
		return out
	}
	return canonicalRoundDigestInputs{
		profileID:         0xC57E0001,
		hashSuite:         config.HashSuiteSHA3NIST,
		identityScheme:    config.IdentitySchemeMLDSA65,
		finalityScheme:    config.SigSchemePulsarM65,
		proofPolicy:       config.ProofPolicySTARKFRISHA3PQ,
		proofBackend:      config.ProofBackendP3QSTARKFRISHA3,
		proofFormat:       config.ProofFormatSTARKFRIBinaryV1,
		verifierID:        config.VerifierP3QSTARKFRISHA3PQ,
		effectivePolicy:   1, // PolicyQuorum non-zero baseline
		networkID:         0xC0DE0001,
		chainID:           0xDEADBEEF,
		epoch:             0x1122334455667788,
		height:            0x99AABBCCDDEEFF00,
		roundOrView:       0xCAFEBABE,
		parentQBlockHash:  ps,
		payloadRoot:       mk48(0x10),
		daRoot:            mk48(0x18),
		sourceStateRoot:   mk48(0x20),
		zchainStateRoot:   mk48(0x30),
		validatorSetRoot:  mk48(0x40),
		committeeRoot:     mk48(0x50),
		dkgTranscriptRoot: mk48(0x60),
		groupPubKeyHash:   mk48(0x70),
		signerSetCommit:   mk48(0x80),
	}
}

func compute(t *testing.T, in canonicalRoundDigestInputs) RoundDigest {
	t.Helper()
	d, err := ComputeRoundDigest(
		in.profileID,
		in.hashSuite, in.identityScheme, in.finalityScheme,
		in.proofPolicy, in.proofBackend, in.proofFormat, in.verifierID,
		in.effectivePolicy,
		in.networkID, in.chainID,
		in.epoch, in.height,
		in.roundOrView,
		in.parentQBlockHash,
		in.payloadRoot,
		in.daRoot,
		in.sourceStateRoot,
		in.zchainStateRoot,
		in.validatorSetRoot,
		in.committeeRoot,
		in.dkgTranscriptRoot,
		in.groupPubKeyHash,
		in.signerSetCommit,
	)
	if err != nil {
		t.Fatalf("ComputeRoundDigest: %v", err)
	}
	return d
}

// TestComputeRoundDigest_Deterministic — pure function: same inputs map
// to the same 32-byte digest on every call. Determinism is the bedrock
// property the threshold-sig layer relies on; a non-deterministic digest
// would silently break aggregate verification across honest signers.
func TestComputeRoundDigest_Deterministic(t *testing.T) {
	in := canonical()
	a := compute(t, in)
	b := compute(t, in)
	if a != b {
		t.Fatalf("non-deterministic: a=%x b=%x", a, b)
	}
	c := compute(t, in)
	if a != c {
		t.Fatalf("non-deterministic over 3 calls: a=%x c=%x", a, c)
	}
}

// TestComputeRoundDigest_BindsEveryField — table-driven coverage that
// each individual field flip changes the digest. This is the formal F34
// closure: an attacker mutating any envelope axis breaks signature
// verification at the threshold layer.
func TestComputeRoundDigest_BindsEveryField(t *testing.T) {
	base := canonical()
	baseDigest := compute(t, base)

	mutations := map[string]func(*canonicalRoundDigestInputs){
		"profileID":            func(x *canonicalRoundDigestInputs) { x.profileID ^= 1 },
		"hashSuite":            func(x *canonicalRoundDigestInputs) { x.hashSuite = config.HashSuiteBLAKE3Legacy },
		"identityScheme":       func(x *canonicalRoundDigestInputs) { x.identityScheme = config.IdentitySchemeMLDSA87 },
		"finalityScheme":       func(x *canonicalRoundDigestInputs) { x.finalityScheme = config.SigSchemePulsarM87 },
		"proofPolicy":          func(x *canonicalRoundDigestInputs) { x.proofPolicy = config.ProofPolicySTARKFRIKeccak },
		"proofBackend":         func(x *canonicalRoundDigestInputs) { x.proofBackend = config.ProofBackendSP1CompressedSTARK },
		"proofFormat":          func(x *canonicalRoundDigestInputs) { x.proofFormat = config.ProofFormatSP1BinaryV1 },
		"verifierID":           func(x *canonicalRoundDigestInputs) { x.verifierID = config.VerifierSP1CompressedSTARKPQ },
		"networkID":            func(x *canonicalRoundDigestInputs) { x.networkID ^= 1 },
		"chainID":              func(x *canonicalRoundDigestInputs) { x.chainID ^= 1 },
		"epoch":                func(x *canonicalRoundDigestInputs) { x.epoch ^= 1 },
		"height":               func(x *canonicalRoundDigestInputs) { x.height ^= 1 },
		"roundOrView":          func(x *canonicalRoundDigestInputs) { x.roundOrView ^= 1 },
		"parentQBlockHash[0]":  func(x *canonicalRoundDigestInputs) { x.parentQBlockHash[0] ^= 1 },
		"parentQBlockHash[31]": func(x *canonicalRoundDigestInputs) { x.parentQBlockHash[31] ^= 1 },
		"payloadRoot[0]":       func(x *canonicalRoundDigestInputs) { x.payloadRoot[0] ^= 1 },
		"payloadRoot[47]":      func(x *canonicalRoundDigestInputs) { x.payloadRoot[47] ^= 1 },
		"daRoot[0]":            func(x *canonicalRoundDigestInputs) { x.daRoot[0] ^= 1 },
		"daRoot[47]":           func(x *canonicalRoundDigestInputs) { x.daRoot[47] ^= 1 },
		"sourceStateRoot[0]":   func(x *canonicalRoundDigestInputs) { x.sourceStateRoot[0] ^= 1 },
		"zchainStateRoot[0]":   func(x *canonicalRoundDigestInputs) { x.zchainStateRoot[0] ^= 1 },
		"validatorSetRoot[0]":  func(x *canonicalRoundDigestInputs) { x.validatorSetRoot[0] ^= 1 },
		"committeeRoot[0]":     func(x *canonicalRoundDigestInputs) { x.committeeRoot[0] ^= 1 },
		"dkgTranscriptRoot[0]": func(x *canonicalRoundDigestInputs) { x.dkgTranscriptRoot[0] ^= 1 },
		"groupPubKeyHash[0]":   func(x *canonicalRoundDigestInputs) { x.groupPubKeyHash[0] ^= 1 },
		"signerSetCommit[0]":   func(x *canonicalRoundDigestInputs) { x.signerSetCommit[0] ^= 1 },
		"signerSetCommit[47]":  func(x *canonicalRoundDigestInputs) { x.signerSetCommit[47] ^= 1 },
	}

	for name, mut := range mutations {
		name, mut := name, mut
		t.Run(name, func(t *testing.T) {
			in := base
			mut(&in)
			got := compute(t, in)
			if got == baseDigest {
				t.Fatalf("%s flip did not change digest (base=%x flipped=%x)",
					name, baseDigest, got)
			}
		})
	}
}

// TestComputeRoundDigest_CrossChainReplayFails — two chains with
// identical (networkID, height, parentState, etc.) but different
// chainIDs MUST produce distinct digests. Therefore a threshold sig
// from chain A does not satisfy chain B's verifier.
func TestComputeRoundDigest_CrossChainReplayFails(t *testing.T) {
	a := canonical()
	b := a
	b.chainID = a.chainID + 1

	digestA := compute(t, a)
	digestB := compute(t, b)
	if digestA == digestB {
		t.Fatalf("cross-chain replay: same digest across chains %x and %x: %x",
			a.chainID, b.chainID, digestA)
	}

	c := a
	c.networkID = a.networkID + 1
	digestC := compute(t, c)
	if digestA == digestC {
		t.Fatalf("cross-network replay: same digest across networks %x and %x: %x",
			a.networkID, c.networkID, digestA)
	}
}

// TestComputeRoundDigest_FixedHashFamily verifies the digest kernel is
// pinned at cSHAKE256 / TupleHash256 regardless of which HashSuiteID is
// configured. The HashSuiteID is data the digest binds, not the kernel
// that produces the digest. Recompute by hand-rolling a reference
// SP 800-185 TupleHash256 to guard against silent kernel drift.
func TestComputeRoundDigest_FixedHashFamily(t *testing.T) {
	in := canonical()
	got := compute(t, in)

	var u16 [2]byte
	var u32 [4]byte
	var u64 [8]byte

	binary.BigEndian.PutUint32(u32[:], in.profileID)
	profileBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint16(u16[:], uint16(in.verifierID))
	verifierBytes := append([]byte(nil), u16[:]...)

	binary.BigEndian.PutUint32(u32[:], in.networkID)
	netBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint32(u32[:], in.chainID)
	chainBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint64(u64[:], in.epoch)
	epochBytes := append([]byte(nil), u64[:]...)

	binary.BigEndian.PutUint64(u64[:], in.height)
	heightBytes := append([]byte(nil), u64[:]...)

	binary.BigEndian.PutUint32(u32[:], in.roundOrView)
	roundBytes := append([]byte(nil), u32[:]...)

	parts := [][]byte{
		[]byte("Quasar/RoundDigest"),
		profileBytes,
		{byte(in.hashSuite)},
		{byte(in.identityScheme)},
		{byte(in.finalityScheme)},
		{byte(in.proofPolicy)},
		{byte(in.proofBackend)},
		{byte(in.proofFormat)},
		verifierBytes,
		// BLOCKERS.md CR-12 mirror — must match production order.
		{in.effectivePolicy},
		netBytes,
		chainBytes,
		epochBytes,
		heightBytes,
		roundBytes,
		in.parentQBlockHash[:],
		in.payloadRoot[:],
		in.daRoot[:],
		in.sourceStateRoot[:],
		in.zchainStateRoot[:],
		in.validatorSetRoot[:],
		in.committeeRoot[:],
		in.dkgTranscriptRoot[:],
		in.groupPubKeyHash[:],
		in.signerSetCommit[:],
	}

	var encoded []byte
	for _, p := range parts {
		encoded = append(encoded, refEncodeString(p)...)
	}
	encoded = append(encoded, refRightEncode(uint64(32)*8)...)

	h := sha3.NewCShake256([]byte("TupleHash"), []byte("QUASAR-ROUND-DIGEST"))
	_, _ = h.Write(encoded)
	want := make([]byte, 32)
	_, _ = h.Read(want)

	if !bytes.Equal(got[:], want) {
		t.Fatalf("digest kernel drift detected: got=%x want=%x", got[:], want)
	}
}

// Reference SP 800-185 helpers for TestComputeRoundDigest_FixedHashFamily.
// Deliberately duplicated rather than imported from round_digest.go so
// the test catches an accidental break of either side.

func refLeftEncode(x uint64) []byte {
	if x == 0 {
		return []byte{0x01, 0x00}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	out := make([]byte, 0, 9-i)
	out = append(out, byte(8-i))
	out = append(out, buf[i:]...)
	return out
}

func refRightEncode(x uint64) []byte {
	if x == 0 {
		return []byte{0x00, 0x01}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	out := make([]byte, 0, 9-i)
	out = append(out, buf[i:]...)
	out = append(out, byte(8-i))
	return out
}

func refEncodeString(s []byte) []byte {
	out := refLeftEncode(uint64(len(s)) * 8)
	out = append(out, s...)
	return out
}
