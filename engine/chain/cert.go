// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert.go — the engine-level finality witness for chain consensus.
//
// THE FINALITY RULE (one rule, one place):
//
//	A value block FINALIZES only after α distinct validators have each produced
//	a correctly-signed ACCEPT vote over the SAME consensus position
//	(chain, height, round, block_id, parent_id). The proof of that fact is a
//	QuorumCert. No node — not even the proposer — may finalize a value block
//	without holding (or having verified) a QuorumCert for it.
//
// This closes the proposer-self-finality hole: previously the proposer force-
// accepted its own block on its LONE self-vote (consensus.ForceAccept), and a
// peer's REJECT was counted as ACCEPT for the proposer's own block
// (effectiveAccept = ... || IsOwnProposal). Both are deleted. The α-of-K
// Snowball counting in consensus.go is now the sole finality authority, and a
// QuorumCert is its portable, verifiable witness.
//
// DECOMPLECTION — the rule vs. the witness's cryptography are SEPARATE:
//
//   - The RULE ("α distinct validators accepted this exact value") lives here
//     and is identical on every chain (P/X/C/D) and in every deployment.
//   - The per-vote signature CRYPTOGRAPHY is pluggable via VoteVerifier. The
//     engine never invents a signature scheme; the node injects one (BLS,
//     ML-DSA, secp256k1) backed by a proven library. A QuorumCert over signed
//     votes is the full-node-verifiable witness at THIS abstraction level.
//   - protocol/quasar.WeightedQuorumCert is the SAME relation expressed with
//     the heavyweight PQ apparatus (per-signer FIPS 204/205 records + weighted
//     validator-set Merkle root + epoch). When a chain has its validator ML-DSA
//     key material and weighted-set root plumbed through the node layer, a
//     QuorumCert UPGRADES to carry a quasar.WeightedQuorumCert as its crypto
//     witness with NO change to the finality rule (see CryptoWitness / the
//     quasar bridge in quasar.go). One rule; the witness format is
//     orthogonal and forward-compatible.
//
// This is a quorum CERTIFICATE, not threshold signing: nothing is aggregated,
// no secret share is combined. Building a cert needs no secrets — any node that
// has collected α distinct signed ACCEPT votes assembles the identical cert
// (leaderless, permissionless). Verification is the direct predicate below.
package chain

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// QuorumCertVersion is the wire/struct version of the engine-level quorum
// certificate. Bound into the canonical vote message and the cert digest so a
// version bump is non-malleable.
//
// Wire v2 added VotePosition.ValidatorSetRoot (the MEDIUM epoch-binding fix).
//
// Wire v3 is the design's "QCv2" — the CANONICAL-COMMITMENT cert (incident
// 1082814 durable fix). It makes the inner execution commitment the PRIMARY
// consensus object: the signed message binds {canonical_block_id,
// parent_canonical_id, execution_state_root, payload_root} and DROPS the outer
// proposervm envelope id from the signature entirely. The outer id is demoted to
// a non-authoritative transport cache key (carried in the wire for block lookup,
// excluded from the signed message and from every finality/equivocation
// decision). Two outer envelopes that wrap the SAME inner block therefore produce
// IDENTICAL signed messages — their votes interoperate and their certs are
// duplicates (harmless aliases), never a fork.
//
// This is a deliberate forward-only break: a v2 signature (outer-id finality) and
// a v3 signature (canonical finality) are over different messages and carry
// different versions, so a mixed-version cert fails clause-1 (version) and
// clause-6 (signature) loudly rather than silently mis-parsing. The whole
// validator set upgrades together in one atomic roll (the collapsed-timeline
// directive — all 5 validators per net roll together); there is no partial
// QCv1→QCv2 interop and no read-only legacy window.
const QuorumCertVersion uint16 = 3

// QCType names a certificate's semantic role so a signature gathered for one
// role can never be replayed as another. The chain engine finalizes blocks, so
// the only role here is QCFinality; the type is bound into every signed vote
// message and into the cert, mirroring quasar's QCType axis for a clean future
// bridge.
type QCType uint8

const (
	// QCFinality witnesses that α-of-K validators ACCEPTED a value block at a
	// position — the proof required before VM.Accept may run for that block.
	QCFinality QCType = 1
)

// Typed verification errors. Each maps 1:1 to one predicate clause so a caller
// or test can name the exact failure. Every one is a CLEAN rejection (the cert
// or vote is invalid); none is a panic and none does unbounded work — an
// adversarial cert yields an error, never a node crash.
var (
	ErrQCNil                     = errors.New("chain: nil quorum cert")
	ErrQCVersion                 = errors.New("chain: quorum cert version mismatch")
	ErrQCType                    = errors.New("chain: quorum cert type mismatch")
	ErrQCNoVotes                 = errors.New("chain: quorum cert has no votes")
	ErrQCThresholdZero           = errors.New("chain: quorum cert threshold (alpha) is zero")
	ErrQCNotStrictlyIncreasing   = errors.New("chain: cert voters are not strictly increasing (duplicate or unsorted node id)")
	ErrQCBelowThreshold          = errors.New("chain: distinct accept votes below quorum threshold (alpha)")
	ErrQCVoteNotAccept           = errors.New("chain: cert carries a non-accept vote")
	ErrQCVotePosition            = errors.New("chain: cert vote position does not match cert position")
	ErrQCSigInvalid              = errors.New("chain: cert vote signature failed verification")
	ErrQCVerifierNil             = errors.New("chain: vote verifier is nil; cannot verify a cert's signatures — fail closed")
	ErrQCStakeBelowSupermajority = errors.New("chain: cert voters' stake below 2/3 of total stake (count quorum reached but not stake-weighted supermajority)")
)

// SignedVote is one validator's signed ACCEPT decision over a consensus
// position. It is the atom a QuorumCert is assembled from. The signature is
// over CanonicalVoteMessage(position) under a scheme the engine does not need
// to know — VoteVerifier resolves the (NodeID, message, signature) triple.
//
// Only ACCEPT votes are certifiable: a finality cert proves a value was
// accepted, so Accept must be true and is bound into the canonical message.
// (Reject votes drive the rejection path in consensus.go; they are never put
// in a finality cert.)
type SignedVote struct {
	// NodeID is the signing validator's identifier. Votes in a cert are sorted
	// strictly-increasing by this field (distinctness / anti-double-count).
	NodeID ids.NodeID
	// Accept is the validator's decision. For a finality cert this MUST be true.
	Accept bool
	// Signature is the validator's signature over CanonicalVoteMessage of the
	// cert's position. Verified by a VoteVerifier; the engine is scheme-agnostic.
	Signature []byte
}

// VotePosition is the consensus position a vote (and a cert) binds to.
//
// THE CANONICAL/TRANSPORT SPLIT (the incident-1082814 durable fix). The position
// carries TWO identities for the block:
//
//   - the CANONICAL execution identity — {CanonicalID, ParentCanonicalID,
//     ExecutionStateRoot, PayloadRoot}. This is the PRIMARY consensus object: the
//     inner execution block commitment that finality, equivocation, ancestry, and
//     idempotency are ALL defined over. It is folded into the canonical signed
//     message, so a signature is bound to the exact execution result.
//   - the TRANSPORT/envelope identity — {BlockID, ParentID}, the outer proposervm
//     wrapper ids. These are a CACHE KEY for block lookup/gossip only. They are
//     DELIBERATELY EXCLUDED from the signed message (CanonicalVoteMessage) and from
//     every finality decision, so two different outer envelopes wrapping the SAME
//     inner block sign the SAME message and are duplicates, never a fork.
//
// Every CANONICAL axis (plus chain/height/round/set-root) is folded into the
// signed message; a signature for one canonical position can never be replayed at
// another. The transport ids are not signed — they are non-authoritative.
type VotePosition struct {
	ChainID ids.ID
	Height  uint64
	Round   uint32

	// BlockID / ParentID are the OUTER proposervm envelope ids — the TRANSPORT
	// cache keys (block lookup, gossip, DAG tracking). NON-AUTHORITATIVE: excluded
	// from the signed message and from finality/equivocation. For a block that is
	// not proposervm-wrapped at the engine boundary, BlockID == CanonicalID (the
	// scheme degrades to outer==canonical and behaves exactly as before this split).
	BlockID  ids.ID
	ParentID ids.ID

	// CanonicalID is the inner EXECUTION block commitment — THE primary consensus
	// object. Finality certifies THIS; the per-height equivocation index keys on
	// THIS; two certs at one height conflict iff THIS differs. It is bound into the
	// signed message. For a non-wrapped block it equals BlockID.
	CanonicalID ids.ID
	// ParentCanonicalID is the inner execution commitment of the parent — binds the
	// certified block into the canonical ancestry. Bound into the signed message.
	ParentCanonicalID ids.ID
	// ExecutionStateRoot is the post-execution state root the block commits to.
	// Bound into the signed message so a cert pins the exact execution result (a
	// block claiming the same canonical id but a different state root would be a
	// distinct signed message). ids.Empty when the VM does not expose one.
	ExecutionStateRoot ids.ID
	// PayloadRoot is the transaction/payload root (tx_root) the block commits to.
	// Bound into the signed message. ids.Empty when the VM does not expose one.
	PayloadRoot ids.ID
	// ValidatorSetRoot binds the cert to the EXACT weighted validator set the
	// vote was cast under (the MEDIUM fix). It is a commitment to the active
	// set+weights at this position's height/epoch — mirroring quasar's
	// ConsensusCert.ValidatorSetRoot, which already binds the same axis
	// (consensus_cert_legs.go). Because it is folded into the canonical signed
	// message, a cert assembled from votes cast under set-root R cannot be
	// re-presented as certifying under a different set-root R': every signature
	// was over R, so reconstructing the message with R' fails clause-6 verify.
	//
	// This turns the stake-weighted finality predicate ("⅔-by-stake at the
	// cert-position epoch") from an assumption into an ENFORCED invariant: a
	// cross-epoch stake change cannot retroactively flip an already-correct cert
	// (the cert's identity is pinned to R, not to "current" stake), and a cert
	// gathered under one epoch's set cannot be laundered into another epoch.
	//
	// ids.Empty means "no validator-set epoch bound" — a chain that does not
	// wire a set-root source signs and verifies with Empty consistently, so the
	// behavior is byte-identical to before this field existed (backward-safe).
	ValidatorSetRoot ids.ID
}

// CanonicalVoteMessage is the exact byte string a validator signs to vote
// ACCEPT on a position. It is the message a QuorumCert's signatures are bound
// to (a finality cert proves ACCEPT). Deterministic and domain-separated: a
// signature is bound to (version, qc_type, chain, height, round, block, parent,
// accept=1) so it cannot be lifted to a different role, position, or decision.
func CanonicalVoteMessage(pos VotePosition) []byte {
	return canonicalVoteMessageFor(pos, true)
}

// canonicalVoteMessageFor builds the domain-separated vote message for a
// position AND a decision. The accept byte is bound, so an ACCEPT signature
// (accept=1, what a cert carries) and a REJECT signature (accept=0) over the
// same position are DISTINCT messages — a reject signature can never be
// presented as an accept (and vice-versa). The engine verifies accept votes
// against (pos,true) and reject votes against (pos,false); the cert only ever
// uses (pos,true).
//
// THE MESSAGE BINDS THE CANONICAL EXECUTION IDENTITY, NOT THE OUTER ENVELOPE
// (the incident-1082814 fix). The outer proposervm ids (pos.BlockID/ParentID)
// are NOT in this message — they are transport cache keys. The signed identity is
// the inner execution commitment {canonical_id, parent_canonical_id,
// execution_state_root, payload_root}. Consequence: two validators that executed
// the SAME inner block sign byte-identical messages even if they received
// different outer envelopes, so their votes interoperate and a cert assembled
// from them verifies on every node. A locally-derived wrapper id can NEVER reach
// a signature.
//
// Layout (big-endian, fixed-width, length-free because every field is fixed):
//
//	"LUX/chain/vote/v2\x00"   domain tag (NUL-terminated; v2 == canonical-commitment)
//	version:2  qc_type:1
//	chain_id:32  height:8  round:4
//	canonical_block_id:32      <- PRIMARY consensus object (inner execution commitment)
//	parent_canonical_id:32
//	execution_state_root:32
//	payload_root:32            <- tx_root
//	validator_set_root:32      (epoch/weighted-set commitment; Empty = unbound)
//	accept:1   (0x01 accept | 0x00 reject)
//
// validator_set_root is bound BEFORE the accept byte so a vote is committed to
// the exact weighted validator set it was cast under (the MEDIUM fix): a cert
// gathered under set-root R cannot be re-verified as certifying under a
// different set R'. The domain tag is bumped v1→v2 so a canonical-commitment
// signature can never be confused with a legacy outer-id signature.
func canonicalVoteMessageFor(pos VotePosition, accept bool) []byte {
	const tag = "LUX/chain/vote/v2\x00"
	buf := make([]byte, 0, len(tag)+2+1+32+8+4+32+32+32+32+32+1)
	buf = append(buf, tag...)
	var u16 [2]byte
	binary.BigEndian.PutUint16(u16[:], QuorumCertVersion)
	buf = append(buf, u16[:]...)
	buf = append(buf, byte(QCFinality))
	buf = append(buf, pos.ChainID[:]...)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], pos.Height)
	buf = append(buf, u64[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], pos.Round)
	buf = append(buf, u32[:]...)
	// CANONICAL execution identity — the signed primary object. Outer ids omitted.
	// FALLBACK (the non-wrapped degrade, in ONE place): a position whose canonical
	// fields are unset (a bare/in-process VM block with no inner/outer split, or a
	// fixed-set chain) binds its OUTER id under the canonical slot — byte-identical to
	// the pre-canonical message for that block. A proposervm-wrapped block carries a
	// distinct CanonicalID and binds THAT. Resolving the fallback here (not at the
	// position-build sites) guarantees every producer of a position — engine or test —
	// signs/verifies the SAME bytes for the same block.
	canonicalID := pos.CanonicalID
	if canonicalID == ids.Empty {
		canonicalID = pos.BlockID
	}
	parentCanonicalID := pos.ParentCanonicalID
	if parentCanonicalID == ids.Empty {
		parentCanonicalID = pos.ParentID
	}
	buf = append(buf, canonicalID[:]...)
	buf = append(buf, parentCanonicalID[:]...)
	buf = append(buf, pos.ExecutionStateRoot[:]...)
	buf = append(buf, pos.PayloadRoot[:]...)
	buf = append(buf, pos.ValidatorSetRoot[:]...)
	if accept {
		buf = append(buf, 0x01)
	} else {
		buf = append(buf, 0x00)
	}
	return buf
}

// VoteVerifier verifies one validator's signature over the canonical vote
// message. This is the engine's sole crypto dependency for finality: the node
// supplies an implementation backed by a proven library (BLS, ML-DSA via
// quasar, or secp256k1). The engine defines the QUORUM RULE; the verifier
// supplies the SIGNATURE CHECK. Decomplected.
//
// VerifyVote MUST be deterministic and side-effect free, return true iff sig is
// a valid signature by nodeID over message, and NEVER panic on adversarial
// input (return false). Implementations that consult a validator set MUST treat
// an unknown nodeID as "false", not an error — a cert with an out-of-set voter
// is simply invalid.
//
// epochHeight is the P-CHAIN height the block's weighted validator set is pinned
// to (MEDIUM-1 / RESIDUAL-B). An implementation that resolves the voter's public
// key from a validator set MUST resolve it from the set IN FORCE AT epochHeight —
// the SAME height the set-root and the ⅔-by-stake tally are read at — NOT from
// the current validator map. Resolving from the current map drops the legitimate
// vote of a validator that has since left the current set but was a member at
// epochHeight (an async-skew window during a staking change), stalling finality
// for that block. The four reads — membership, pubkey, set-root, stake — all key
// off epochHeight so a cert is internally consistent at exactly one epoch.
type VoteVerifier interface {
	VerifyVote(nodeID ids.NodeID, message []byte, sig []byte, epochHeight uint64) bool
}

// VoteVerifierFunc adapts a function to a VoteVerifier.
type VoteVerifierFunc func(nodeID ids.NodeID, message []byte, sig []byte, epochHeight uint64) bool

// VerifyVote implements VoteVerifier.
func (f VoteVerifierFunc) VerifyVote(nodeID ids.NodeID, message []byte, sig []byte, epochHeight uint64) bool {
	return f(nodeID, message, sig, epochHeight)
}

// StakeSource supplies validator voting weights so finality can be checked as a
// STAKE-WEIGHTED supermajority rather than a raw voter COUNT. This is the HIGH-3
// fix: on a PoS chain with unequal stake, "α distinct voters" is NOT the same as
// "≥⅔ of stake" — a coalition of many low-stake validators could reach the count
// while controlling a minority of stake. When a value/PoS chain wires a
// StakeSource, a cert finalizes only if BOTH hold: (count ≥ α) AND (Σ voter
// stake ≥ ⅔ Σ total stake).
//
// Determinism + fail-closed: Weight MUST be deterministic for a given
// (nodeID, epoch) and return 0 for an unknown/out-of-set voter (an out-of-set
// voter contributes no stake — it cannot inflate the numerator). TotalStake is
// the epoch's total active stake; if it is 0 the source is unusable and the
// caller treats the cert as unverifiable (fail closed). The engine binds the
// source to the cert's position height so weights are read at the right epoch.
type StakeSource interface {
	// Weight returns the voting weight (stake) of nodeID at the given height, or
	// 0 if nodeID is not an active validator at that height.
	Weight(nodeID ids.NodeID, height uint64) uint64
	// TotalStake returns the total active validator stake at the given height.
	TotalStake(height uint64) uint64
}

// ValidatorSetRootSource computes the commitment to the active weighted
// validator set at a given height — the value bound into a VotePosition's
// ValidatorSetRoot. The node supplies it from the chain's validator set; the
// engine stamps it into every position it signs/assembles so a cert is
// cryptographically pinned to the exact set+weights it was certified under
// (the MEDIUM fix). It MUST be deterministic for a given height across all
// honest nodes (every node computing the root for height H must agree, or their
// signatures over the same block would not be mutually verifiable).
//
// Returning ids.Empty is the explicit "no epoch bound" answer — a chain that
// does not commit to a set-root signs and verifies with Empty consistently
// (behavior identical to before set-root binding existed).
type ValidatorSetRootSource interface {
	// ValidatorSetRoot returns the deterministic commitment to the active
	// weighted validator set at height.
	ValidatorSetRoot(height uint64) ids.ID
}

// ValidatorSetRootFunc adapts a function to a ValidatorSetRootSource.
type ValidatorSetRootFunc func(height uint64) ids.ID

// ValidatorSetRoot implements ValidatorSetRootSource.
func (f ValidatorSetRootFunc) ValidatorSetRoot(height uint64) ids.ID { return f(height) }

// VerifyWeighted verifies the cert under `verifier` (the full count predicate of
// Verify) AND additionally requires that the summed stake of the cert's voters
// is at least two-thirds of the total stake at the cert's position height. It is
// the stake-aware finality predicate for value/PoS chains.
//
// A nil stake source means "no stake model wired" — the caller MUST instead use
// Verify and is responsible for the equal-stake admission invariant (documented
// on the engine). VerifyWeighted with a nil source returns ErrQCVerifierNil's
// sibling fail-closed error to make a wiring mistake loud rather than silently
// count-only.
func (c *QuorumCert) VerifyWeighted(verifier VoteVerifier, stake StakeSource, epochHeight uint64) error {
	if err := c.Verify(verifier, epochHeight); err != nil {
		return err
	}
	if stake == nil {
		return fmt.Errorf("%w: stake source nil", ErrQCStakeBelowSupermajority)
	}
	// The ⅔-by-stake tally is read at the block's P-CHAIN EPOCH height — the SAME
	// height the cert's set-root and the per-voter pubkeys are read at (MEDIUM-1)
	// — NOT c.Position.Height (the value-chain height, which platformvm would
	// reject as unfinalized and which races ahead of the P-chain epoch). Reading
	// the tally at the same epoch the signatures were cast under guarantees a
	// validator whose vote is in the cert also contributes its epoch weight.
	total := stake.TotalStake(epochHeight)
	if total == 0 {
		// No known stake at this epoch — cannot assert a supermajority. Fail closed.
		return fmt.Errorf("%w: total stake is zero at epoch height %d (value-height %d)", ErrQCStakeBelowSupermajority, epochHeight, c.Position.Height)
	}
	var voted uint64
	for i := range c.Votes {
		voted += stake.Weight(c.Votes[i].NodeID, epochHeight)
	}
	// STRICT supermajority by stake: accept iff voted > floor(2·total/3)
	// (Tendermint +⅔). The floor is the SINGLE definition in config.TwoThirdsStakeFloor
	// — the SAME function the live-set parameter sizer derives α from
	// (config.WeightedSupermajorityThreshold), so the count threshold the node
	// sizes to can never drift from the stake predicate enforced here.
	twoThirdsFloor := config.TwoThirdsStakeFloor(total)
	if voted <= twoThirdsFloor {
		return fmt.Errorf("%w: voted=%d total=%d (need > floor(2/3·total)=%d) at height %d",
			ErrQCStakeBelowSupermajority, voted, total, twoThirdsFloor, c.Position.Height)
	}
	return nil
}

// QuorumCert is the engine-level finality witness: α distinct validators each
// signed ACCEPT over Position. It is portable (gossipable), verifiable by any
// node holding the VoteVerifier, and deterministic to assemble.
//
// It is NOT a signature — there is no aggregate field, because nothing is
// aggregated. The cert carries the per-voter signed records; verification is
// the predicate in Verify.
type QuorumCert struct {
	// Version pins the cert format.
	Version uint16
	// Type names the cert's role (QCFinality). Bound into every vote message.
	Type QCType
	// Position is the consensus position every vote binds to.
	Position VotePosition
	// Threshold (alpha) is the minimum number of distinct ACCEPT voters required
	// for the cert to be valid — the chain's α-of-K quorum floor.
	Threshold uint32
	// Votes are the per-voter signed ACCEPT records, sorted strictly-increasing
	// by NodeID. len(Votes) >= Threshold for a valid cert.
	Votes []SignedVote
}

// AssembleQuorumCert builds a finality cert from collected signed ACCEPT votes.
// Permissionless and deterministic: NO secrets, NO randomness. Given the same
// votes and position the same cert comes out.
//
// It sorts votes strictly-increasing by NodeID (dropping duplicate NodeIDs —
// last-writer-wins is not used; a duplicate NodeID is an error so a cert can
// never double-count), and rejects a structurally impossible cert (no votes,
// zero threshold, a non-accept vote). It does NOT verify signatures —
// assembly is orthogonal to verification (a relaying node assembles; verifiers
// verify). A subsequent Verify is the gate.
//
// Returns ErrQCBelowThreshold if fewer than `threshold` distinct accept votes
// are supplied: assembly only succeeds once the quorum is actually present, so
// a cert can never claim a quorum it does not hold.
func AssembleQuorumCert(pos VotePosition, threshold uint32, votes []SignedVote) (*QuorumCert, error) {
	if threshold == 0 {
		return nil, ErrQCThresholdZero
	}
	if len(votes) == 0 {
		return nil, ErrQCNoVotes
	}

	// Defensive copy; dedup by NodeID (reject duplicates), require ACCEPT.
	sorted := make([]SignedVote, 0, len(votes))
	seen := make(map[ids.NodeID]struct{}, len(votes))
	for i := range votes {
		v := votes[i]
		if !v.Accept {
			return nil, fmt.Errorf("%w: voter %s", ErrQCVoteNotAccept, v.NodeID)
		}
		if _, dup := seen[v.NodeID]; dup {
			return nil, fmt.Errorf("%w: voter %s", ErrQCNotStrictlyIncreasing, v.NodeID)
		}
		seen[v.NodeID] = struct{}{}
		cp := v
		cp.Signature = append([]byte(nil), v.Signature...)
		sorted = append(sorted, cp)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i].NodeID[:], sorted[j].NodeID[:]) < 0
	})

	if uint32(len(sorted)) < threshold {
		return nil, fmt.Errorf("%w: have %d need %d", ErrQCBelowThreshold, len(sorted), threshold)
	}

	return &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Position:  pos,
		Threshold: threshold,
		Votes:     sorted,
	}, nil
}

// Verify checks the cert against verifier. Returns nil iff every predicate
// clause holds; otherwise a typed error naming the FIRST failure. NEVER panics
// and NEVER does unbounded work — an adversarial cert (duplicate voter, bad
// signature, sub-threshold, wrong position) yields a clean error.
//
// Predicate (the engine-level projection of quasar's weighted-quorum predicate):
//
//	(0) verifier is non-nil                          (fail-closed)
//	(1) version + type match
//	(2) threshold (alpha) > 0
//	(3) at least one vote
//	for each vote, in order:
//	  (4) node ids are STRICTLY INCREASING            (distinct, anti-double-count)
//	  (5) vote is ACCEPT
//	  (6) signature verifies under verifier over CanonicalVoteMessage(Position)
//	then
//	  (7) count of distinct valid accept votes >= threshold
//
// fail-closed: a nil verifier is an error, never a pass — a cert may not be
// trusted without the ability to check its signatures.
func (c *QuorumCert) Verify(verifier VoteVerifier, epochHeight uint64) error {
	if c == nil {
		return ErrQCNil
	}
	if verifier == nil {
		return ErrQCVerifierNil
	}
	if c.Version != QuorumCertVersion {
		return fmt.Errorf("%w: got %d want %d", ErrQCVersion, c.Version, QuorumCertVersion)
	}
	if c.Type != QCFinality {
		return fmt.Errorf("%w: got %d want %d", ErrQCType, c.Type, QCFinality)
	}
	if c.Threshold == 0 {
		return ErrQCThresholdZero
	}
	if len(c.Votes) == 0 {
		return ErrQCNoVotes
	}

	message := CanonicalVoteMessage(c.Position)

	var count uint32
	var prev ids.NodeID
	havePrev := false
	for i := range c.Votes {
		v := &c.Votes[i]

		// Clause (4): strictly increasing node ids — distinct + canonical order;
		// closes duplicate-voter double counting and cert re-ordering malleability.
		if havePrev && bytes.Compare(prev[:], v.NodeID[:]) >= 0 {
			return fmt.Errorf("%w: vote %d", ErrQCNotStrictlyIncreasing, i)
		}
		prev = v.NodeID
		havePrev = true

		// Clause (5): finality certs carry ACCEPT votes only.
		if !v.Accept {
			return fmt.Errorf("%w: vote %d voter %s", ErrQCVoteNotAccept, i, v.NodeID)
		}

		// Clause (6): signature verifies over the cert's own position, with the
		// voter's pubkey resolved at the block's P-CHAIN EPOCH height (the same
		// height the set-root in this position commits to). The verifier rebuilds
		// nothing from the vote — the message is derived from the CERT position,
		// so a vote that signed a different position fails.
		if !verifier.VerifyVote(v.NodeID, message, v.Signature, epochHeight) {
			return fmt.Errorf("%w: vote %d voter %s", ErrQCSigInvalid, i, v.NodeID)
		}

		count++
	}

	// Clause (7): distinct valid accept votes meet the quorum floor.
	if count < c.Threshold {
		return fmt.Errorf("%w: have %d need %d", ErrQCBelowThreshold, count, c.Threshold)
	}
	return nil
}

// VoterCount returns the number of distinct voters the cert carries. After a
// successful Verify this equals the number of distinct valid accept votes.
func (c *QuorumCert) VoterCount() int {
	if c == nil {
		return 0
	}
	return len(c.Votes)
}

// Equal reports structural equality of two certs (used in round-trip tests).
func (c *QuorumCert) Equal(o *QuorumCert) bool {
	if c == nil || o == nil {
		return c == o
	}
	if c.Version != o.Version || c.Type != o.Type || c.Position != o.Position ||
		c.Threshold != o.Threshold || len(c.Votes) != len(o.Votes) {
		return false
	}
	for i := range c.Votes {
		if c.Votes[i].NodeID != o.Votes[i].NodeID ||
			c.Votes[i].Accept != o.Votes[i].Accept ||
			!bytes.Equal(c.Votes[i].Signature, o.Votes[i].Signature) {
			return false
		}
	}
	return true
}
