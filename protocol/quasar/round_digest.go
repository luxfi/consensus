// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
	"golang.org/x/crypto/sha3"
)

// ----------------------------------------------------------------------------
// HIP-0077 F34 — Quasar round digest
// ----------------------------------------------------------------------------
//
// ComputeRoundDigest binds EVERY consensus envelope axis into the 32-byte
// subject the threshold signature signs over for a Quasar consensus round.
//
// A different DKG transcript, different committee, different group key,
// different proof backend, different proof format, different verifier,
// different identity scheme, or different data-availability root MUST
// produce a different digest, even when (network, chain, epoch, height,
// round, parent_state) are byte-equal.
//
// Concretely, the digest binds:
//
//	profile_id                   security-profile envelope (ChainSecurityProfile.ProfileID)
//	hash_suite_id                hash family bound as DATA (kernel is cSHAKE256)
//	identity_scheme_id           validator identity-signature scheme (FIPS 204 ML-DSA)
//	finality_scheme_id           threshold finality kernel (Pulsar-M-44/65/87)
//	proof_policy_id              policy class (STARK_FRI_SHA3_PQ / STARK_FRI_Keccak)
//	proof_backend_id             implementation (SP1 / RISC0 / P3Q / Stone / Stwo)
//	proof_format_id              wire byte layout of the proof bytes
//	verifier_id                  concrete pinned verifier identity
//	network_id, chain_id         envelope replay axes
//	epoch, height, round_or_view consensus position
//	parent_qblock_hash           chain tip extension
//	payload_root                 block-specific payload anchor
//	da_root                      data-availability root
//	lux_state_root               Lux state commitment for the round
//	zchain_state_root            latest accepted Z-Chain root
//	validator_set_root           validator registry root
//	committee_root               committee Merkle root
//	dkg_transcript_root          Pedersen-DKG transcript root
//	group_public_key_hash        threshold group public-key hash
//	signer_set_or_bitmap_commit  who attested (bitmap or weight-Merkle root)
//
// Hash family: cSHAKE256 / TupleHash256 (FIPS 202 + SP 800-185), independent
// of HashSuiteID — HashSuiteID is bound as DATA, not as the kernel that
// produces the digest. Customization tag pins the layout to "QUASAR-ROUND-
// DIGEST"; the tag string is the schema identity and is itself bound by
// the cSHAKE256 customization stream.
//
// Layout (TupleHash256, customization "QUASAR-ROUND-DIGEST"):
//
//	parts[0]  = "Quasar/RoundDigest"   (in-band protocol tag)
//	parts[1]  = profile_id           (4 BE)
//	parts[2]  = hash_suite_id        (1)
//	parts[3]  = identity_scheme_id   (1)
//	parts[4]  = finality_scheme_id   (1)
//	parts[5]  = proof_policy_id      (1)
//	parts[6]  = proof_backend_id     (1)
//	parts[7]  = proof_format_id      (1)
//	parts[8]  = verifier_id          (2 BE)
//	parts[9]  = network_id           (4 BE)
//	parts[10] = chain_id             (4 BE)
//	parts[11] = epoch                (8 BE)
//	parts[12] = height               (8 BE)
//	parts[13] = round_or_view        (4 BE)
//	parts[14] = parent_qblock_hash   ([32]byte)
//	parts[15] = payload_root         ([48]byte)
//	parts[16] = da_root              ([48]byte)
//	parts[17] = lux_state_root       ([48]byte)
//	parts[18] = zchain_state_root    ([48]byte)
//	parts[19] = validator_set_root   ([48]byte)
//	parts[20] = committee_root       ([48]byte)
//	parts[21] = dkg_transcript_root  ([48]byte)
//	parts[22] = group_public_key_hash ([48]byte)
//	parts[23] = signer_set_or_bitmap_commitment ([48]byte)
//
// Every part is bound via SP 800-185 encode_string framing, so flipping
// any byte of any field yields a different digest. TupleHash injectivity
// is the formal guarantee.
//
// ComputeRoundDigest is the single production entry point. It refuses any
// zero-value security-relevant argument up-front; the error names which
// field was zero so the operator can fix the misconfiguration precisely.
func ComputeRoundDigest(
	profileID uint32,
	hashSuite config.HashSuiteID,
	identityScheme config.IdentitySchemeID,
	finalityScheme config.SigSchemeID,
	proofPolicy config.ProofPolicyID,
	proofBackend config.ProofBackendID,
	proofFormat config.ProofFormatID,
	verifierID config.VerifierID,
	networkID, chainID uint32,
	epoch, height uint64,
	roundOrView uint32,
	parentQBlockHash [32]byte,
	payloadRoot [48]byte,
	daRoot [48]byte,
	sourceStateRoot [48]byte,
	zchainStateRoot [48]byte,
	validatorSetRoot [48]byte,
	committeeRoot [48]byte,
	dkgTranscriptRoot [48]byte,
	groupPublicKeyHash [48]byte,
	signerSetOrBitmapCommitment [48]byte,
) (RoundDigest, error) {
	switch {
	case profileID == 0:
		return RoundDigest{}, fmt.Errorf("%w: profileID", ErrRoundDigestZeroField)
	case hashSuite == config.HashSuiteNone:
		return RoundDigest{}, fmt.Errorf("%w: hashSuite", ErrRoundDigestZeroField)
	case identityScheme == config.IdentitySchemeNone:
		return RoundDigest{}, fmt.Errorf("%w: identityScheme", ErrRoundDigestZeroField)
	case finalityScheme == config.SigSchemeNone:
		return RoundDigest{}, fmt.Errorf("%w: finalityScheme", ErrRoundDigestZeroField)
	case proofPolicy == config.ProofPolicyNone:
		return RoundDigest{}, fmt.Errorf("%w: proofPolicy", ErrRoundDigestZeroField)
	case proofBackend == config.ProofBackendNone:
		return RoundDigest{}, fmt.Errorf("%w: proofBackend", ErrRoundDigestZeroField)
	case proofFormat == config.ProofFormatNone:
		return RoundDigest{}, fmt.Errorf("%w: proofFormat", ErrRoundDigestZeroField)
	case verifierID == config.VerifierNone:
		return RoundDigest{}, fmt.Errorf("%w: verifierID", ErrRoundDigestZeroField)
	case networkID == 0:
		return RoundDigest{}, fmt.Errorf("%w: networkID", ErrRoundDigestZeroField)
	case chainID == 0:
		return RoundDigest{}, fmt.Errorf("%w: chainID", ErrRoundDigestZeroField)
	}

	var u16 [2]byte
	var u32 [4]byte
	var u64 [8]byte

	binary.BigEndian.PutUint32(u32[:], profileID)
	profileBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint16(u16[:], uint16(verifierID))
	verifierBytes := append([]byte(nil), u16[:]...)

	binary.BigEndian.PutUint32(u32[:], networkID)
	netBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint32(u32[:], chainID)
	chainBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint64(u64[:], epoch)
	epochBytes := append([]byte(nil), u64[:]...)

	binary.BigEndian.PutUint64(u64[:], height)
	heightBytes := append([]byte(nil), u64[:]...)

	binary.BigEndian.PutUint32(u32[:], roundOrView)
	roundBytes := append([]byte(nil), u32[:]...)

	parts := [][]byte{
		[]byte(roundDigestProtocolTag),
		profileBytes,
		{byte(hashSuite)},
		{byte(identityScheme)},
		{byte(finalityScheme)},
		{byte(proofPolicy)},
		{byte(proofBackend)},
		{byte(proofFormat)},
		verifierBytes,
		netBytes,
		chainBytes,
		epochBytes,
		heightBytes,
		roundBytes,
		parentQBlockHash[:],
		payloadRoot[:],
		daRoot[:],
		sourceStateRoot[:],
		zchainStateRoot[:],
		validatorSetRoot[:],
		committeeRoot[:],
		dkgTranscriptRoot[:],
		groupPublicKeyHash[:],
		signerSetOrBitmapCommitment[:],
	}

	out := tupleHash256RoundDigest(parts, 32, roundDigestCustomization)
	var digest RoundDigest
	copy(digest[:], out)
	return digest, nil
}

// roundDigestCustomization is the SP 800-185 cSHAKE256 customization tag
// for the canonical round-digest layout. The tag is the schema identity;
// changing it produces a digest stream that does not collide with any
// prior digest under the previous tag.
const roundDigestCustomization = "QUASAR-ROUND-DIGEST"

// roundDigestProtocolTag is the in-band redundant protocol tag bound as
// the first TupleHash part. Defence-in-depth so a cross-customization-
// collision attacker also has to forge the leading TupleHash part.
const roundDigestProtocolTag = "Quasar/RoundDigest"

// ErrRoundDigestZeroField is returned by ComputeRoundDigest when any
// security-relevant input is the zero value. The error names which field
// was zero so the operator can fix the misconfiguration precisely.
//
// Closes HIP-0077 red-review F77.
var ErrRoundDigestZeroField = errors.New(
	"quasar: ComputeRoundDigest refuses zero-value security-relevant input")

// tupleHash256RoundDigest computes TupleHash256(parts, outLen, customization)
// per NIST SP 800-185 §5. The implementation mirrors the FIPS-aligned
// primitive in github.com/luxfi/corona/hash/sp800_185.go; vendored here
// to keep consensus below pulsar in the module dependency graph.
//
// TupleHash256 differs from naïve cSHAKE256(concat(parts)) in that each
// part is length-prefixed via encodeString — so (parts ["ab", "cd"]) and
// (parts ["a", "bcd"]) yield distinct digests. This is the property that
// makes flipping any single field's bytes change the digest, even if a
// neighbouring field could absorb the flipped bytes.
func tupleHash256RoundDigest(parts [][]byte, outLen int, customization string) []byte {
	var x []byte
	for _, p := range parts {
		x = append(x, encodeStringSP800185(p)...)
	}
	x = append(x, rightEncodeSP800185(uint64(outLen)*8)...)

	h := sha3.NewCShake256([]byte("TupleHash"), []byte(customization))
	_, _ = h.Write(x)
	out := make([]byte, outLen)
	_, _ = h.Read(out)
	return out
}

// leftEncodeSP800185 returns the SP 800-185 §2.3 left_encode(x) byte
// string. Operates on the BIT length, not the byte length — every
// caller multiplies by 8 before passing in.
func leftEncodeSP800185(x uint64) []byte {
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

// rightEncodeSP800185 returns the SP 800-185 §2.3 right_encode(x) byte
// string. Used at the tail of TupleHash to bind the requested output
// length into the absorbed stream.
func rightEncodeSP800185(x uint64) []byte {
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

// encodeStringSP800185 returns left_encode(bit_len(s)) || s per
// SP 800-185 §2.3. This is the unambiguous length-prefix framing that
// makes TupleHash injective over its tuple of parts.
func encodeStringSP800185(s []byte) []byte {
	out := leftEncodeSP800185(uint64(len(s)) * 8)
	out = append(out, s...)
	return out
}
