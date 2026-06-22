// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_cert.go — the verifiable weighted quorum certificate.
//
// A WeightedQuorumCert proves: a quorum-WEIGHT subset of the epoch
// validator set each produced a valid INDEPENDENT FIPS 204 ML-DSA /
// FIPS 205 SLH-DSA signature over the same domain-separated consensus
// message. The proof is the certificate itself — there is no succinct
// (STARK/SNARK) layer here; verification is direct (full-node-verifiable
// mode):
//
//	for each signer record:
//	  (a) the record's leaf is included under validator_set_root (Merkle)
//	  (b) the record's parameter set is in the allowed set
//	  (c) the record's signature verifies under the stock FIPS verifier
//	  (d) validator_ids are STRICTLY INCREASING (distinct, canonical,
//	      anti-double-count)
//	accumulate voting weight; then
//	  Σ weight ≥ quorum_threshold
//	  Σ weight == aggregate_weight
//	  commit(signer_ids) == signer_commitment
//
// Security posture (NIST Class N1): soundness rests ONLY on (i) unmodified
// FIPS 204/205 verification — each signer's signature is byte-identical to
// what cloudflare/circl, the NIST reference, or openssl-pq accept; and (ii)
// the weighted-validator-set Merkle commitment's second-preimage resistance
// (weighted_merkle.go). No private key material is shared, reconstructed,
// or combined. This is threshold CERTIFICATION, NOT threshold ML-DSA /
// SLH-DSA signing: there is no DKG, no seed / WOTS+ / FORS / secret share
// ever combined or exposed. A STARK compression layer over the SAME
// relation is a SEPARATE future backend (ProofBackendID 0x10/0x20 block);
// it is out of scope here.
//
// Permissionless: building a cert needs NO secrets — a prover collects
// already-produced signer records, sorts them, and assembles. Leaderless:
// any node with the records can build the identical cert.
package quasar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
)

// QuorumCertVersion is the wire/struct version of the weighted quorum
// certificate. Bound into the signer commitment and the encodings so a
// version bump is non-malleable.
const QuorumCertVersion uint16 = 1

// signerCommitmentCustomization is the cSHAKE256 customization tag for the
// signer-set commitment. Wire-stable cryptographic constant.
const signerCommitmentCustomization = "QUASAR-WQC-SIGNERS-V1"

// signerCommitmentProtocolTag is the in-band redundant protocol tag for the
// signer-set commitment.
const signerCommitmentProtocolTag = "Quasar/WQC/Signers"

// QuorumSignerRecord is one signer's contribution to a weighted quorum
// certificate: their identity, the public key + accountability fields that
// MUST match the validator-set leaf, the Merkle path proving that leaf is
// committed, and their independent signature over the consensus message.
//
// Every field except MerklePath and Signature is also a validator-set leaf
// field; the verifier reconstructs the leaf from these and checks the path.
// A record whose embedded fields do not match a committed leaf fails the
// Merkle check — that is how an unknown validator / pubkey-mismatch /
// out-of-set record is rejected (cleanly, never a panic).
type QuorumSignerRecord struct {
	// ValidatorID is the signer's canonical 32-byte identifier. Records are
	// sorted strictly-increasing by this field.
	ValidatorID [32]byte

	// PublicKey is the signer's canonical public-key bytes. Bound into the
	// leaf AND used as the verification key — both must agree.
	PublicKey []byte

	// VotingWeight is the signer's stake weight. Bound into the leaf and
	// accumulated toward the quorum threshold.
	VotingWeight uint64

	// Scheme names the signature scheme this record was produced under
	// (ML-DSA / SLH-DSA per FIPS 204 / 205). The verifier dispatches to the
	// stock FIPS verifier for this scheme.
	Scheme QuorumSchemeID

	// ParamSetID is the parameter-set byte bound into the validator-set
	// leaf. It MUST equal byte(Scheme) — the leaf commits to the parameter
	// set, so a record cannot claim one scheme while the leaf pins another.
	ParamSetID uint8

	// KeyVersion is the signer's key-rotation counter, bound into the leaf.
	KeyVersion uint32

	// MerklePath proves leaf(ValidatorID, PublicKey, VotingWeight,
	// ParamSetID, KeyVersion) is included under validator_set_root.
	MerklePath *WeightedInclusionProof

	// Signature is the signer's independent signature over the consensus
	// message. Verified by the stock FIPS verifier for Scheme.
	Signature []byte
}

// leaf reconstructs the validator-set leaf this record claims.
func (r *QuorumSignerRecord) leaf() WeightedValidatorLeaf {
	return WeightedValidatorLeaf{
		ValidatorID:    r.ValidatorID,
		PublicKey:      r.PublicKey,
		VotingWeight:   r.VotingWeight,
		ParameterSetID: r.ParamSetID,
		KeyVersion:     r.KeyVersion,
	}
}

// WeightedQuorumCert is the certificate. It is NOT a signature: there is no
// "aggregate_signature" field, because nothing is aggregated — the cert
// carries (in full mode) the per-signer records, or (in compact mode) just
// the commitment to them. Verification is the predicate documented at the
// top of this file.
type WeightedQuorumCert struct {
	// Version pins the cert format.
	Version uint16

	// Consensus position — the message-binding axes. These MUST equal the
	// values the signers' message was derived from (quorum_message.go).
	ChainID uint32
	Epoch   uint64
	Height  uint64
	Round   uint32

	// ValueHash is the block_id / value hash this quorum finalises (the
	// payload anchor of the consensus message).
	ValueHash [32]byte

	// QCType names the certificate's semantic role (prepare / commit /
	// finality / …) so a signature for one role cannot be replayed as
	// another. Bound into the consensus message.
	QCType uint8

	// ValidatorSetRoot is the weighted-validator-set commitment every
	// signer's leaf is proven against (weighted_merkle.go). Equals the
	// epoch's committed root.
	ValidatorSetRoot [48]byte

	// QuorumThreshold is the minimum total voting weight required for the
	// cert to be valid. Σ signer weight must meet or exceed it.
	QuorumThreshold uint64

	// AggregateWeight is the prover's claimed total signer weight. The
	// verifier recomputes Σ weight and asserts equality — a tampered
	// aggregate is rejected.
	AggregateWeight uint64

	// SignerCommitment is the cSHAKE256 commitment to the ordered signer-id
	// set (+ version + position). The verifier recomputes it from the
	// records and asserts equality — closes signer-set malleability.
	SignerCommitment [48]byte

	// SignerCount is the number of signer records (the cardinality the
	// compact encoding still commits to even when records are omitted).
	SignerCount uint32

	// Signers is the per-signer records. Populated in FULL mode (records in
	// the certificate); EMPTY in COMPACT mode (records live in the DA layer,
	// the header carries only the commitment). A compact cert is verified
	// by re-attaching records from DA and calling Verify.
	Signers []QuorumSignerRecord
}

// Typed verification errors. Each maps 1:1 to one predicate clause so a
// caller / test can name the exact failure. Every one is a CLEAN rejection
// (the cert is invalid); none is a panic or an unbounded-work DoS.
var (
	ErrQCNil                   = errors.New("quasar: nil weighted quorum cert")
	ErrQCVersion               = errors.New("quasar: weighted quorum cert version mismatch")
	ErrQCNoSigners             = errors.New("quasar: weighted quorum cert has no signer records")
	ErrQCCompactNoRecords      = errors.New("quasar: compact cert carries no records; re-attach from DA before Verify")
	ErrQCSignerCountMismatch   = errors.New("quasar: signer record count does not match SignerCount")
	ErrQCNotStrictlyIncreasing = errors.New("quasar: signer validator_ids are not strictly increasing (duplicate or unsorted)")
	ErrQCSchemeNotAllowed      = errors.New("quasar: signer scheme not in allowed set")
	ErrQCParamSetMismatch      = errors.New("quasar: signer ParamSetID does not equal scheme byte")
	ErrQCPubKeyLen             = errors.New("quasar: signer public key length wrong for its scheme")
	ErrQCMerkleInclusion       = errors.New("quasar: signer leaf not included under validator_set_root")
	ErrQCSigInvalid            = errors.New("quasar: signer signature failed FIPS verification")
	ErrQCBelowThreshold        = errors.New("quasar: total signer weight below quorum threshold")
	ErrQCAggregateWeight       = errors.New("quasar: recomputed signer weight != AggregateWeight")
	ErrQCSignerCommitment      = errors.New("quasar: recomputed signer commitment != SignerCommitment")
	ErrQCWeightOverflow        = errors.New("quasar: signer weight sum overflows uint64")
	ErrQCThresholdZero         = errors.New("quasar: quorum threshold is zero")
	ErrQCMinThresholdUnset     = errors.New("quasar: verifier MinThreshold is unset (zero); a cert may not assert its own security parameter — fail closed")
	ErrQCThresholdBelowFloor   = errors.New("quasar: cert QuorumThreshold is below the chain BFT quorum floor (MinThreshold)")
	ErrQCProofBackendMismatch  = errors.New("quasar: envelope proof backend does not match the cert's ProofBackendID")
)

// QuorumVerifierConfig pins the policy axes the verifier enforces. Kept
// separate from the cert so verification policy (what's allowed) and the
// cert (what's claimed) are not braided.
type QuorumVerifierConfig struct {
	// AllowedSchemes is the set of per-signer schemes admissible on this
	// chain. A record whose scheme is absent fails (ErrQCSchemeNotAllowed).
	// Empty means "every scheme the registry knows" — convenient for tests;
	// production callers pin an explicit set via the chain profile.
	AllowedSchemes map[QuorumSchemeID]bool

	// Context is the FIPS context string passed to every per-signer verify.
	// It MUST match what signers bound at sign time. Length ≤ 255.
	//
	// Applied per-scheme: ML-DSA (FIPS 204) records verify under this context;
	// SLH-DSA (FIPS 205) records ALWAYS verify under the empty context,
	// because the only SLH-DSA sign path here (magnetar.ValidatorSign) binds
	// the empty FIPS-205 context and the round-digest message already carries
	// the full domain binding. See contextForScheme.
	Context []byte

	// MinThreshold is the chain's BFT quorum floor — the minimum voting
	// weight any valid cert on this chain MUST assert. Verify FAILS CLOSED if
	// MinThreshold is zero (unset): a certificate may not be the sole source
	// of its own security parameter, so the verifier MUST pin the floor from
	// the chain profile. A cert whose QuorumThreshold is below this floor is
	// rejected (ErrQCThresholdBelowFloor) regardless of how many signatures it
	// carries — this is the mandatory defence against sub-quorum finality
	// forgery, independent of (and in addition to) the threshold's binding
	// into the signed message.
	MinThreshold uint64
}

// schemeAllowed reports whether scheme is admissible under this config.
func (c *QuorumVerifierConfig) schemeAllowed(scheme QuorumSchemeID) bool {
	if len(c.AllowedSchemes) == 0 {
		_, known := defaultQuorumVerifiers[scheme]
		return known
	}
	return c.AllowedSchemes[scheme]
}

// Verify checks the certificate against the chain envelope under cfg. Returns
// nil iff every predicate clause holds; otherwise a typed error naming the
// first failure. NEVER panics and NEVER does unbounded work — an adversarial
// cert (unknown validator, bad signature, tampered weight, forged threshold)
// yields a clean error, not a node crash.
//
// envelope carries the chain's pinned posture axes (profile, hash suite,
// schemes, proof backend/format/verifier, network). Verify rebuilds the
// domain-separated signing message ITSELF from the envelope plus the cert's
// own consensus-position fields (via QuorumMessageForCert) — it NEVER trusts
// a caller-supplied opaque message. This closes the role-replay / position
// mismatch surface: a cert whose signers signed a different
// (chain_id, epoch, height, round, value_hash, qc_type, validator_set_root,
// quorum_threshold) fails the per-signer FIPS check for every record, and a
// cert that lies about any of those fails because the rebuilt message no
// longer matches the signatures.
//
// Verify additionally asserts the envelope's proof-backend axis equals the
// cert's own ProofBackendID(), so a cert produced under the direct
// weighted-quorum backend cannot be verified under a different backend's
// envelope (and vice versa).
func (c *WeightedQuorumCert) Verify(envelope QuorumMessageEnvelope, cfg QuorumVerifierConfig) error {
	if c == nil {
		return ErrQCNil
	}
	// Bind the verification envelope to the cert's trust model: the caller's
	// envelope axis MUST name the same proof backend this cert is produced
	// under. Folds the round-digest proof-backend axis check into Verify so a
	// cross-backend envelope is rejected before any message is built.
	if envelope.ProofBackend != c.ProofBackendID() {
		return fmt.Errorf("%w: envelope=0x%02x cert=0x%02x",
			ErrQCProofBackendMismatch, uint8(envelope.ProofBackend), uint8(c.ProofBackendID()))
	}
	// Rebuild the signing message from the envelope + the cert's own position
	// fields. The verifier owns message construction — it does not trust a
	// caller-supplied message.
	message, err := QuorumMessageForCert(envelope, c)
	if err != nil {
		return err
	}
	return c.verifyWithMessage(message, cfg)
}

// verifyWithMessage is the core weighted-quorum predicate over an already-
// built domain-separated message. It is INTERNAL: production callers MUST use
// Verify, which constructs the message itself from the chain envelope so a
// caller can never substitute an opaque message that skips the position /
// threshold / proof-backend binding. This helper carries only the predicate
// clauses + the mandatory threshold floor; it is exercised directly by tests
// that need to drive a deliberately-mismatched message.
func (c *WeightedQuorumCert) verifyWithMessage(message []byte, cfg QuorumVerifierConfig) error {
	if c == nil {
		return ErrQCNil
	}
	if c.Version != QuorumCertVersion {
		return fmt.Errorf("%w: got %d want %d", ErrQCVersion, c.Version, QuorumCertVersion)
	}
	// Mandatory threshold floor (fail-closed). A cert may not be the sole
	// source of its own security parameter: the verifier MUST pin the chain's
	// BFT quorum floor, and a cert asserting a threshold below it is rejected
	// regardless of signature count — the defence-in-depth that closes
	// sub-quorum finality forgery even if a signed-message binding were
	// somehow bypassed.
	if cfg.MinThreshold == 0 {
		return ErrQCMinThresholdUnset
	}
	if c.QuorumThreshold == 0 {
		return ErrQCThresholdZero
	}
	if c.QuorumThreshold < cfg.MinThreshold {
		return fmt.Errorf("%w: threshold %d floor %d",
			ErrQCThresholdBelowFloor, c.QuorumThreshold, cfg.MinThreshold)
	}
	if c.SignerCount == 0 {
		return ErrQCNoSigners
	}
	if len(c.Signers) == 0 {
		// Compact cert: records were stripped to the DA layer. Cannot verify
		// the predicate without them. Caller must re-attach records first.
		return ErrQCCompactNoRecords
	}
	if uint32(len(c.Signers)) != c.SignerCount {
		return fmt.Errorf("%w: records=%d SignerCount=%d",
			ErrQCSignerCountMismatch, len(c.Signers), c.SignerCount)
	}

	var total uint64
	var prev [32]byte
	havePrev := false

	for i := range c.Signers {
		rec := &c.Signers[i]

		// Clause (d): strictly increasing validator_ids. Distinct +
		// canonical order; closes duplicate-signer double counting and
		// signer-set malleability (re-ordering a valid cert).
		if havePrev && !bytesLess(prev[:], rec.ValidatorID[:]) {
			return fmt.Errorf("%w: record %d", ErrQCNotStrictlyIncreasing, i)
		}
		prev = rec.ValidatorID
		havePrev = true

		// Clause (b): scheme in the allowed set.
		if !cfg.schemeAllowed(rec.Scheme) {
			return fmt.Errorf("%w: record %d scheme %s", ErrQCSchemeNotAllowed, i, rec.Scheme)
		}

		// ParamSetID must equal the scheme byte — the leaf commits to the
		// parameter set, so a record cannot claim ML-DSA-65 while its leaf
		// pins ML-DSA-44 (cross-parameter confusion).
		if rec.ParamSetID != uint8(rec.Scheme) {
			return fmt.Errorf("%w: record %d param=0x%02x scheme=0x%02x",
				ErrQCParamSetMismatch, i, rec.ParamSetID, uint8(rec.Scheme))
		}

		// Public-key length must match the scheme before any dispatch.
		wantLen, known := expectedPubKeyLen(rec.Scheme)
		if !known {
			return fmt.Errorf("%w: record %d scheme %s", ErrQCSchemeNotAllowed, i, rec.Scheme)
		}
		if len(rec.PublicKey) != wantLen {
			return fmt.Errorf("%w: record %d scheme %s have=%d want=%d",
				ErrQCPubKeyLen, i, rec.Scheme, len(rec.PublicKey), wantLen)
		}

		// Clause (a): Merkle inclusion under validator_set_root. This is
		// where an unknown validator / pubkey-mismatch / out-of-set record
		// is rejected — its reconstructed leaf is not in the committed tree.
		if !VerifyWeightedInclusion(c.ValidatorSetRoot, c.Epoch, rec.leaf(), rec.MerklePath) {
			return fmt.Errorf("%w: record %d", ErrQCMerkleInclusion, i)
		}

		// Clause (c): unmodified FIPS verify. The dispatch is a thin call
		// into the stock FIPS 204 / 205 verifier for the scheme. The FIPS
		// context is selected PER SCHEME (contextForScheme): ML-DSA records
		// verify under cfg.Context; SLH-DSA records verify under the empty
		// FIPS-205 context, matching magnetar.ValidatorSign's empty-ctx sign.
		// Without this, a mixed ML-DSA ∧ SLH-DSA cert would only verify when
		// cfg.Context is nil (the SLH-DSA leg would reject a non-empty ctx).
		verify := defaultQuorumVerifiers[rec.Scheme]
		if verify == nil || !verify(rec.PublicKey, message, contextForScheme(cfg, rec.Scheme), rec.Signature) {
			return fmt.Errorf("%w: record %d scheme %s", ErrQCSigInvalid, i, rec.Scheme)
		}

		// Accumulate weight (overflow-checked).
		if total+rec.VotingWeight < total {
			return ErrQCWeightOverflow
		}
		total += rec.VotingWeight
	}

	// Σ weight ≥ quorum_threshold.
	if total < c.QuorumThreshold {
		return fmt.Errorf("%w: have %d need %d", ErrQCBelowThreshold, total, c.QuorumThreshold)
	}

	// Σ weight == aggregate_weight (tamper check).
	if total != c.AggregateWeight {
		return fmt.Errorf("%w: recomputed %d claimed %d", ErrQCAggregateWeight, total, c.AggregateWeight)
	}

	// commit(signer_ids) == signer_commitment (signer-set malleability).
	want := c.computeSignerCommitment()
	if want != c.SignerCommitment {
		return ErrQCSignerCommitment
	}

	return nil
}

// computeSignerCommitment recomputes the cSHAKE256 commitment over the
// ordered signer set plus the cert's version and consensus position.
// Binding the position fields here means a cert that lies about its own
// (chain_id, epoch, height, round, value_hash, qc_type, validator_set_root,
// quorum_threshold) fails the commitment check even before the per-signer
// message binding catches it — defence-in-depth against header tampering.
//
// Per signer it binds validator_id AND the Merkle (leaf_index, leaf_count).
// The PRIMARY canonicality guarantee for (leaf_index, leaf_count) is enforced
// cryptographically in the weighted-Merkle commitment itself: leaf_count is
// folded into every leaf digest (computeWeightedLeafHash), so a record that
// relabels its count within a shape-class (e.g. (0,3)→(0,4), both shape "RR")
// recomputes a different leaf digest and FAILS the Merkle inclusion clause
// against validator_set_root — exactly one (leaf_index, leaf_count) verifies
// per committed tree (QUASAR-C5). Binding the pair here as well is
// defence-in-depth: it makes the commitment field itself total and tamper-
// evident over the Merkle position, so any position malleation is caught at
// the commitment check too, not only at the inclusion clause.
//
// Layout (TupleHash256, customization "QUASAR-WQC-SIGNERS-V1"):
//
//	parts[0]  = "Quasar/WQC/Signers"
//	parts[1]  = version            (2 BE)
//	parts[2]  = chain_id           (4 BE)
//	parts[3]  = epoch              (8 BE)
//	parts[4]  = height             (8 BE)
//	parts[5]  = round              (4 BE)
//	parts[6]  = value_hash         ([32]byte)
//	parts[7]  = qc_type            (1)
//	parts[8]  = validator_set_root ([48]byte)
//	parts[9]  = quorum_threshold   (8 BE)
//	parts[10] = signer_count       (4 BE)
//	then, per record i in order, three parts:
//	  validator_id[i]  ([32]byte)
//	  leaf_index[i]    (4 BE)
//	  leaf_count[i]    (4 BE)
func (c *WeightedQuorumCert) computeSignerCommitment() [48]byte {
	var u16 [2]byte
	var u32 [4]byte
	var u64 [8]byte

	binary.BigEndian.PutUint16(u16[:], c.Version)
	verBytes := append([]byte(nil), u16[:]...)
	binary.BigEndian.PutUint32(u32[:], c.ChainID)
	chainBytes := append([]byte(nil), u32[:]...)
	binary.BigEndian.PutUint64(u64[:], c.Epoch)
	epochBytes := append([]byte(nil), u64[:]...)
	binary.BigEndian.PutUint64(u64[:], c.Height)
	heightBytes := append([]byte(nil), u64[:]...)
	binary.BigEndian.PutUint32(u32[:], c.Round)
	roundBytes := append([]byte(nil), u32[:]...)
	binary.BigEndian.PutUint64(u64[:], c.QuorumThreshold)
	threshBytes := append([]byte(nil), u64[:]...)
	binary.BigEndian.PutUint32(u32[:], c.SignerCount)
	countBytes := append([]byte(nil), u32[:]...)

	parts := make([][]byte, 0, 11+3*len(c.Signers))
	parts = append(parts,
		[]byte(signerCommitmentProtocolTag),
		verBytes,
		chainBytes,
		epochBytes,
		heightBytes,
		roundBytes,
		c.ValueHash[:],
		[]byte{c.QCType},
		c.ValidatorSetRoot[:],
		threshBytes,
		countBytes,
	)
	for i := range c.Signers {
		// Bind the Merkle position (leaf_index, leaf_count). A nil path (a
		// structurally-incomplete record that will fail the Merkle clause
		// anyway) binds the (0,0) sentinel so the commitment is still total.
		var leafIdx, leafCount uint32
		if p := c.Signers[i].MerklePath; p != nil {
			leafIdx, leafCount = p.LeafIndex, p.LeafCount
		}
		var idxBytes, cntBytes [4]byte
		binary.BigEndian.PutUint32(idxBytes[:], leafIdx)
		binary.BigEndian.PutUint32(cntBytes[:], leafCount)
		parts = append(parts,
			c.Signers[i].ValidatorID[:],
			append([]byte(nil), idxBytes[:]...),
			append([]byte(nil), cntBytes[:]...),
		)
	}

	out := tupleHash256RoundDigest(parts, 48, signerCommitmentCustomization)
	var h [48]byte
	copy(h[:], out)
	return h
}

// ----------------------------------------------------------------------------
// Prover (permissionless — needs NO secrets)
// ----------------------------------------------------------------------------

// QuorumCertParams names the consensus position + policy a cert is built
// for. Separate from the records so the prover's two inputs (what we're
// certifying / who signed) are not braided.
type QuorumCertParams struct {
	ChainID          uint32
	Epoch            uint64
	Height           uint64
	Round            uint32
	ValueHash        [32]byte
	QCType           uint8
	ValidatorSetRoot [48]byte
	QuorumThreshold  uint64
}

// BuildWeightedQuorumCert assembles a FULL (records-included) certificate
// from already-produced signer records. It is permissionless and
// deterministic: NO secrets, NO randomness, NO hidden state. Given the same
// records and params, the same cert bytes come out.
//
// The prover sorts records strictly-increasing by validator_id (rejecting
// duplicates), computes aggregate_weight, and computes the signer
// commitment. It does NOT verify the records' signatures — assembly is
// orthogonal to verification (a relaying node assembles; verifiers verify).
// It DOES reject a structurally impossible cert (no records, duplicate ids,
// zero threshold) so a malformed cert is never produced.
func BuildWeightedQuorumCert(params QuorumCertParams, records []QuorumSignerRecord) (*WeightedQuorumCert, error) {
	if params.QuorumThreshold == 0 {
		return nil, ErrQCThresholdZero
	}
	if len(records) == 0 {
		return nil, ErrQCNoSigners
	}

	sorted := make([]QuorumSignerRecord, len(records))
	copy(sorted, records)
	// Defensive copy of variable-length fields so a later caller mutation
	// cannot move the assembled cert underneath us.
	for i := range sorted {
		sorted[i].PublicKey = append([]byte(nil), sorted[i].PublicKey...)
		sorted[i].Signature = append([]byte(nil), sorted[i].Signature...)
	}
	sortSignerRecords(sorted)

	var total uint64
	for i := range sorted {
		if i > 0 && sorted[i].ValidatorID == sorted[i-1].ValidatorID {
			return nil, fmt.Errorf("%w: %x", ErrWVSetDuplicateID, sorted[i].ValidatorID[:])
		}
		if total+sorted[i].VotingWeight < total {
			return nil, ErrQCWeightOverflow
		}
		total += sorted[i].VotingWeight
	}

	cert := &WeightedQuorumCert{
		Version:          QuorumCertVersion,
		ChainID:          params.ChainID,
		Epoch:            params.Epoch,
		Height:           params.Height,
		Round:            params.Round,
		ValueHash:        params.ValueHash,
		QCType:           params.QCType,
		ValidatorSetRoot: params.ValidatorSetRoot,
		QuorumThreshold:  params.QuorumThreshold,
		AggregateWeight:  total,
		SignerCount:      uint32(len(sorted)),
		Signers:          sorted,
	}
	cert.SignerCommitment = cert.computeSignerCommitment()
	return cert, nil
}

// sortSignerRecords sorts records strictly by validator_id ascending.
func sortSignerRecords(recs []QuorumSignerRecord) {
	// insertion-free stable sort via the std sort on a closure would alloc;
	// validator IDs are public so a simple comparison sort is fine.
	for i := 1; i < len(recs); i++ {
		for j := i; j > 0 && bytesLess(recs[j].ValidatorID[:], recs[j-1].ValidatorID[:]); j-- {
			recs[j], recs[j-1] = recs[j-1], recs[j]
		}
	}
}

// ----------------------------------------------------------------------------
// Compact ↔ full (DA hybrid mode)
// ----------------------------------------------------------------------------

// Compact returns a copy of the cert with the per-signer records stripped —
// the commitment-only form a block header carries. The raw signer records
// live in the data-availability layer; a verifier re-attaches them with
// WithRecords before calling Verify. The SignerCommitment + SignerCount are
// retained, so the header still binds exactly which signer set the cert
// claims (a DA layer cannot substitute a different record set without
// failing the commitment check on re-attach).
func (c *WeightedQuorumCert) Compact() *WeightedQuorumCert {
	cp := *c
	cp.Signers = nil
	return &cp
}

// IsCompact reports whether the cert is in commitment-only form (no
// records). A compact cert cannot be verified until records are re-attached.
func (c *WeightedQuorumCert) IsCompact() bool {
	return len(c.Signers) == 0 && c.SignerCount > 0
}

// WithRecords re-attaches signer records to a compact cert (records fetched
// from the DA layer). It does NOT trust the records: WithRecords binds them
// in, and a subsequent Verify recomputes the signer commitment — if the DA
// layer served a different record set than the header committed to, Verify
// returns ErrQCSignerCommitment. Returns a copy; the receiver is unmodified.
func (c *WeightedQuorumCert) WithRecords(records []QuorumSignerRecord) *WeightedQuorumCert {
	cp := *c
	cp.Signers = make([]QuorumSignerRecord, len(records))
	copy(cp.Signers, records)
	return &cp
}

// ----------------------------------------------------------------------------
// Wire codec — full and compact encodings
// ----------------------------------------------------------------------------
//
// Compact header layout (deterministic, big-endian):
//
//	kind:1            = wqcKindCompact (0x00) | wqcKindFull (0x01)
//	version:2
//	chain_id:4
//	epoch:8
//	height:8
//	round:4
//	value_hash:32
//	qc_type:1
//	validator_set_root:48
//	quorum_threshold:8
//	aggregate_weight:8
//	signer_commitment:48
//	signer_count:4
//
// Full layout = compact header (kind=0x01) followed by signer_count signer
// records, each:
//
//	validator_id:32
//	scheme:1
//	param_set_id:1
//	key_version:4
//	voting_weight:8
//	pubkey_len:4   pubkey:pubkey_len
//	sig_len:4      sig:sig_len
//	merkle_leaf_index:4
//	merkle_leaf_count:4
//	merkle_step_count:4
//	step[i]: flags:1 (bit0=promoted, bit1=sibling_is_right) [+ sibling:48 if !promoted]

const (
	wqcKindCompact byte = 0x00
	wqcKindFull    byte = 0x01
)

// wqcHeaderSize is the fixed compact-header byte length.
const wqcHeaderSize = 1 + 2 + 4 + 8 + 8 + 4 + 32 + 1 + 48 + 8 + 8 + 48 + 4

// wqcMinRecordBytes is the SMALLEST possible on-wire size of one signer
// record: the fixed fields with a zero-length public key, a zero-length
// signature, and a zero-step Merkle path. Used to cap an attacker-controlled
// signer_count against the remaining buffer BEFORE any allocation, so a
// header claiming signer_count = 0xFFFFFFFF cannot force a multi-hundred-GB
// reservation (decode-DoS). Layout, per the full-record comment above:
//
//	validator_id:32 scheme:1 param_set_id:1 key_version:4 voting_weight:8
//	pubkey_len:4 sig_len:4 merkle_leaf_index:4 merkle_leaf_count:4
//	merkle_step_count:4
const wqcMinRecordBytes = 32 + 1 + 1 + 4 + 8 + 4 + 4 + 4 + 4 + 4 // = 66

// ErrQCWireCorrupt is returned by the decoders on any structural defect.
var ErrQCWireCorrupt = errors.New("quasar: weighted quorum cert wire corrupt")

// MarshalBinary encodes the cert. If records are present it emits the FULL
// form; otherwise the COMPACT form. Deterministic: equal certs encode to
// equal bytes.
func (c *WeightedQuorumCert) MarshalBinary() ([]byte, error) {
	if c == nil {
		return nil, ErrQCNil
	}
	full := len(c.Signers) > 0
	kind := wqcKindCompact
	if full {
		kind = wqcKindFull
	}

	buf := make([]byte, 0, wqcHeaderSize)
	buf = append(buf, kind)
	buf = appendU16(buf, c.Version)
	buf = appendU32(buf, c.ChainID)
	buf = appendU64(buf, c.Epoch)
	buf = appendU64(buf, c.Height)
	buf = appendU32(buf, c.Round)
	buf = append(buf, c.ValueHash[:]...)
	buf = append(buf, c.QCType)
	buf = append(buf, c.ValidatorSetRoot[:]...)
	buf = appendU64(buf, c.QuorumThreshold)
	buf = appendU64(buf, c.AggregateWeight)
	buf = append(buf, c.SignerCommitment[:]...)
	buf = appendU32(buf, c.SignerCount)

	if !full {
		return buf, nil
	}

	for i := range c.Signers {
		rec := &c.Signers[i]
		buf = append(buf, rec.ValidatorID[:]...)
		buf = append(buf, byte(rec.Scheme))
		buf = append(buf, rec.ParamSetID)
		buf = appendU32(buf, rec.KeyVersion)
		buf = appendU64(buf, rec.VotingWeight)
		buf = appendU32(buf, uint32(len(rec.PublicKey)))
		buf = append(buf, rec.PublicKey...)
		buf = appendU32(buf, uint32(len(rec.Signature)))
		buf = append(buf, rec.Signature...)

		if rec.MerklePath == nil {
			return nil, fmt.Errorf("%w: record %d nil merkle path", ErrQCWireCorrupt, i)
		}
		buf = appendU32(buf, rec.MerklePath.LeafIndex)
		buf = appendU32(buf, rec.MerklePath.LeafCount)
		buf = appendU32(buf, uint32(len(rec.MerklePath.Steps)))
		for _, st := range rec.MerklePath.Steps {
			var flags byte
			if st.Promoted {
				flags |= 0x01
			}
			if st.SiblingIsRight {
				flags |= 0x02
			}
			buf = append(buf, flags)
			if !st.Promoted {
				buf = append(buf, st.Sibling[:]...)
			}
		}
	}
	return buf, nil
}

// UnmarshalWeightedQuorumCert is the inverse of MarshalBinary. Strict
// trailing-bytes policy: any byte after the declared frame rejects the
// input. Fail-closed on every short read.
func UnmarshalWeightedQuorumCert(data []byte) (*WeightedQuorumCert, error) {
	r := &qcReader{buf: data}
	kind, err := r.u8()
	if err != nil {
		return nil, ErrQCWireCorrupt
	}
	if kind != wqcKindCompact && kind != wqcKindFull {
		return nil, fmt.Errorf("%w: bad kind 0x%02x", ErrQCWireCorrupt, kind)
	}

	c := &WeightedQuorumCert{}
	if c.Version, err = r.u16(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.ChainID, err = r.u32(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.Epoch, err = r.u64(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.Height, err = r.u64(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.Round, err = r.u32(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if err = r.read32(&c.ValueHash); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.QCType, err = r.u8(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if err = r.read48(&c.ValidatorSetRoot); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.QuorumThreshold, err = r.u64(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.AggregateWeight, err = r.u64(); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if err = r.read48(&c.SignerCommitment); err != nil {
		return nil, ErrQCWireCorrupt
	}
	if c.SignerCount, err = r.u32(); err != nil {
		return nil, ErrQCWireCorrupt
	}

	if kind == wqcKindCompact {
		if len(r.buf) != 0 {
			return nil, fmt.Errorf("%w: %d trailing bytes (compact)", ErrQCWireCorrupt, len(r.buf))
		}
		return c, nil
	}

	// Full: decode signer_count records. Cap signer_count against the
	// remaining buffer BEFORE reserving capacity: each record occupies at
	// least wqcMinRecordBytes, so a count whose minimum footprint exceeds the
	// bytes that remain is structurally impossible. This rejects an
	// adversarial header (signer_count = 0xFFFFFFFF → ~446 GB reservation) in
	// O(1) with no allocation — mirrors the step_count cap below.
	if uint64(c.SignerCount)*wqcMinRecordBytes > uint64(len(r.buf)) {
		return nil, fmt.Errorf("%w: signer_count %d exceeds remaining buffer (%d bytes)",
			ErrQCWireCorrupt, c.SignerCount, len(r.buf))
	}
	c.Signers = make([]QuorumSignerRecord, 0, c.SignerCount)
	for i := uint32(0); i < c.SignerCount; i++ {
		var rec QuorumSignerRecord
		if err = r.read32(&rec.ValidatorID); err != nil {
			return nil, ErrQCWireCorrupt
		}
		sch, err := r.u8()
		if err != nil {
			return nil, ErrQCWireCorrupt
		}
		rec.Scheme = QuorumSchemeID(sch)
		if rec.ParamSetID, err = r.u8(); err != nil {
			return nil, ErrQCWireCorrupt
		}
		if rec.KeyVersion, err = r.u32(); err != nil {
			return nil, ErrQCWireCorrupt
		}
		if rec.VotingWeight, err = r.u64(); err != nil {
			return nil, ErrQCWireCorrupt
		}
		if rec.PublicKey, err = r.lenPrefixed(); err != nil {
			return nil, ErrQCWireCorrupt
		}
		if rec.Signature, err = r.lenPrefixed(); err != nil {
			return nil, ErrQCWireCorrupt
		}

		var path WeightedInclusionProof
		if path.LeafIndex, err = r.u32(); err != nil {
			return nil, ErrQCWireCorrupt
		}
		if path.LeafCount, err = r.u32(); err != nil {
			return nil, ErrQCWireCorrupt
		}
		stepCount, err := r.u32()
		if err != nil {
			return nil, ErrQCWireCorrupt
		}
		// Bound the step count: a Merkle path over N leaves is ≤ ceil(log2 N)
		// + a small constant; cap at 64 (supports 2^64 leaves) so a malicious
		// header cannot force a huge allocation.
		if stepCount > 64 {
			return nil, fmt.Errorf("%w: step count %d too large", ErrQCWireCorrupt, stepCount)
		}
		path.Steps = make([]WeightedProofStep, 0, stepCount)
		for s := uint32(0); s < stepCount; s++ {
			flags, err := r.u8()
			if err != nil {
				return nil, ErrQCWireCorrupt
			}
			step := WeightedProofStep{
				Promoted:       flags&0x01 != 0,
				SiblingIsRight: flags&0x02 != 0,
			}
			if !step.Promoted {
				if err = r.read48(&step.Sibling); err != nil {
					return nil, ErrQCWireCorrupt
				}
			}
			path.Steps = append(path.Steps, step)
		}
		rec.MerklePath = &path
		c.Signers = append(c.Signers, rec)
	}

	if len(r.buf) != 0 {
		return nil, fmt.Errorf("%w: %d trailing bytes (full)", ErrQCWireCorrupt, len(r.buf))
	}
	return c, nil
}

// qcReader is a bounds-checked sequential reader for the cert codec.
type qcReader struct{ buf []byte }

func (r *qcReader) need(n int) bool { return len(r.buf) >= n }

func (r *qcReader) u8() (uint8, error) {
	if !r.need(1) {
		return 0, ErrQCWireCorrupt
	}
	v := r.buf[0]
	r.buf = r.buf[1:]
	return v, nil
}

func (r *qcReader) u16() (uint16, error) {
	if !r.need(2) {
		return 0, ErrQCWireCorrupt
	}
	v := binary.BigEndian.Uint16(r.buf[:2])
	r.buf = r.buf[2:]
	return v, nil
}

func (r *qcReader) u32() (uint32, error) {
	if !r.need(4) {
		return 0, ErrQCWireCorrupt
	}
	v := binary.BigEndian.Uint32(r.buf[:4])
	r.buf = r.buf[4:]
	return v, nil
}

func (r *qcReader) u64() (uint64, error) {
	if !r.need(8) {
		return 0, ErrQCWireCorrupt
	}
	v := binary.BigEndian.Uint64(r.buf[:8])
	r.buf = r.buf[8:]
	return v, nil
}

func (r *qcReader) read32(dst *[32]byte) error {
	if !r.need(32) {
		return ErrQCWireCorrupt
	}
	copy(dst[:], r.buf[:32])
	r.buf = r.buf[32:]
	return nil
}

func (r *qcReader) read48(dst *[48]byte) error {
	if !r.need(48) {
		return ErrQCWireCorrupt
	}
	copy(dst[:], r.buf[:48])
	r.buf = r.buf[48:]
	return nil
}

func (r *qcReader) lenPrefixed() ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if uint64(n) > uint64(len(r.buf)) {
		return nil, ErrQCWireCorrupt
	}
	out := make([]byte, n)
	copy(out, r.buf[:n])
	r.buf = r.buf[n:]
	return out, nil
}

// Equal reports structural equality of two certs (used in round-trip
// tests). Compares all fields including the per-signer records.
func (c *WeightedQuorumCert) Equal(o *WeightedQuorumCert) bool {
	if c == nil || o == nil {
		return c == o
	}
	if c.Version != o.Version || c.ChainID != o.ChainID || c.Epoch != o.Epoch ||
		c.Height != o.Height || c.Round != o.Round || c.ValueHash != o.ValueHash ||
		c.QCType != o.QCType || c.ValidatorSetRoot != o.ValidatorSetRoot ||
		c.QuorumThreshold != o.QuorumThreshold || c.AggregateWeight != o.AggregateWeight ||
		c.SignerCommitment != o.SignerCommitment || c.SignerCount != o.SignerCount ||
		len(c.Signers) != len(o.Signers) {
		return false
	}
	for i := range c.Signers {
		a, b := &c.Signers[i], &o.Signers[i]
		if a.ValidatorID != b.ValidatorID || a.Scheme != b.Scheme ||
			a.ParamSetID != b.ParamSetID || a.KeyVersion != b.KeyVersion ||
			a.VotingWeight != b.VotingWeight ||
			!bytes.Equal(a.PublicKey, b.PublicKey) || !bytes.Equal(a.Signature, b.Signature) {
			return false
		}
		if !equalProof(a.MerklePath, b.MerklePath) {
			return false
		}
	}
	return true
}

func equalProof(a, b *WeightedInclusionProof) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.LeafIndex != b.LeafIndex || a.LeafCount != b.LeafCount || len(a.Steps) != len(b.Steps) {
		return false
	}
	for i := range a.Steps {
		if a.Steps[i].Promoted != b.Steps[i].Promoted ||
			a.Steps[i].SiblingIsRight != b.Steps[i].SiblingIsRight ||
			a.Steps[i].Sibling != b.Steps[i].Sibling {
			return false
		}
	}
	return true
}

// ProofBackendID returns the canonical wire ProofBackendID for a direct
// weighted quorum certificate. A caller assembling the round-digest
// envelope (quorum_message.go) MUST stamp this backend so the signed
// transcript binds the full-node-verifiable trust model — a cert produced
// under this backend cannot be re-presented under a STARK backend's
// envelope.
func (c *WeightedQuorumCert) ProofBackendID() config.ProofBackendID {
	return config.ProofBackendDirectWeightedQuorum
}

// ProofFormatID returns the canonical wire ProofFormatID for the full
// (records-included) weighted quorum cert encoding.
func (c *WeightedQuorumCert) ProofFormatID() config.ProofFormatID {
	return config.ProofFormatDirectWeightedQuorumV1
}

// VerifierID returns the canonical pinned VerifierID for the direct
// weighted quorum predicate.
func (c *WeightedQuorumCert) VerifierID() config.VerifierID {
	return config.VerifierDirectWeightedQuorumPQ
}
