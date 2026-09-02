// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert.go — the engine-level finality witness for chain consensus.
//
// The finality rule, stated once:
//
//	A value block finalizes only after α distinct validators have each produced
//	a correctly-signed ACCEPT vote over the same consensus position
//	(chain, height, round, canonical block, canonical parent). The proof of that
//	fact is a QuorumCert. No node — not even the proposer — may finalize a value
//	block without holding, or having verified, a QuorumCert for it.
//
// A proposer's own vote is one vote and buys its own proposal no credit, and a
// peer's REJECT is never read as an ACCEPT. The α-of-K counting in consensus.go
// is the sole finality authority; a QuorumCert is its portable, verifiable
// witness.
//
// The rule and the witness's cryptography are separate concerns:
//
//   - The rule ("α distinct validators accepted this exact value") lives here
//     and is identical on every chain (P/X/C/D) and in every deployment.
//   - The per-vote signature cryptography is pluggable via VoteVerifier. The
//     engine never invents a signature scheme; the node injects one (BLS,
//     ML-DSA, secp256k1) backed by a proven library. A QuorumCert over signed
//     votes is the full-node-verifiable witness at this abstraction level.
//   - protocol/quasar.WeightedQuorumCert is the same relation expressed with
//     the heavyweight PQ apparatus (per-signer FIPS 204/205 records + weighted
//     validator-set Merkle root + epoch). When a chain has its validator ML-DSA
//     key material and weighted-set root plumbed through the node layer, a
//     QuorumCert upgrades to carry a quasar.WeightedQuorumCert as its crypto
//     witness with no change to the finality rule (see CryptoWitness and the
//     quasar bridge in quasar.go). One rule; the witness format is orthogonal
//     and forward-compatible.
//
// This is a quorum certificate, not threshold signing: nothing is aggregated,
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
	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
	"github.com/luxfi/math"
)

// QuorumCertVersion is the wire/struct version of the engine-level quorum
// certificate. It is bound into the canonical vote message and into the cert
// digest, so a version bump is non-malleable: a signature produced under one
// version cannot be presented under another.
//
// Version 3 makes the inner execution commitment the primary consensus object.
// The signed message binds {canonical_block_id, parent_canonical_id,
// execution_state_root, payload_root} and omits the outer proposervm envelope id
// from the signature entirely. That outer id is a non-authoritative transport
// cache key: carried on the wire for block lookup, absent from the signed message
// and from every finality and equivocation decision. Two outer envelopes wrapping
// the same inner block therefore produce identical signed messages — their votes
// interoperate and their certs are duplicate aliases, never a fork.
//
// Versions do not interoperate, by construction. Because the version is folded
// into the signed message, a cert signed under one version fails a verifier of
// another on both the version clause and the signature clause, loudly rather than
// by silent mis-parse. There is no partial interop and no read-only legacy window.
//
// That is fail-safe but not fail-live across a version boundary: while fewer than
// α validators run a given version, no α-quorum of that version can form, so
// finality stalls on both sides of the skew. A version change is therefore a single
// coordinated cut of at least α of the validator set, not a staged migration — a
// trickle that straddles α on each side halts finality on both.
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
// or test can name the exact failure. Every one is a clean rejection (the cert
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
	ErrQCStakeBelowMajority      = errors.New("chain: nova cert voters' stake is not a strict majority of total stake at the epoch")
	ErrQCUnknownTier             = errors.New("chain: quorum cert finality tier is not Nova or Quasar (a cert attests exactly one of the two accept/export tiers)")
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
	// Accept is the validator's decision. For a finality cert it is always true.
	Accept bool
	// Signature is the validator's signature over CanonicalVoteMessage of the
	// cert's position. Verified by a VoteVerifier; the engine is scheme-agnostic.
	Signature []byte
}

// VotePosition is the consensus position a vote (and a cert) binds to.
//
// The canonical/transport split. The position carries two identities for the
// block:
//
//   - the canonical execution identity — {CanonicalID, ParentCanonicalID,
//     ExecutionStateRoot, PayloadRoot}. This is the primary consensus object: the
//     inner execution block commitment that finality, equivocation, ancestry and
//     idempotency are all defined over. It is folded into the canonical signed
//     message, so a signature is bound to the exact execution result.
//   - the transport identity — {BlockID, ParentID}, the outer proposervm wrapper
//     ids. These are a cache key for block lookup and gossip only. They are left
//     out of the signed message (CanonicalVoteMessage) and out of every finality
//     decision, so two different outer envelopes wrapping the same inner block
//     sign the same message and are duplicates, never a fork.
//
// Every canonical axis (plus chain/height/round/set-root) is folded into the
// signed message; a signature for one canonical position can never be replayed at
// another. The transport ids are not signed — they are non-authoritative.
type VotePosition struct {
	ChainID ids.ID
	Height  uint64
	Round   uint32

	// BlockID / ParentID are the outer proposervm envelope ids — the transport
	// cache keys (block lookup, gossip, DAG tracking). Non-authoritative: excluded
	// from the signed message and from finality and equivocation. For a block that
	// is not proposervm-wrapped at the engine boundary, BlockID == CanonicalID and
	// the scheme degrades to outer == canonical.
	BlockID  ids.ID
	ParentID ids.ID

	// CanonicalID is the inner execution block commitment — the primary consensus
	// object. Finality certifies it; the per-height equivocation index keys on it;
	// two certs at one height conflict iff it differs. It is bound into the signed
	// message. For a non-wrapped block it equals BlockID.
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
	// ValidatorSetRoot binds the cert to the exact weighted validator set the
	// vote was cast under. It is a commitment to the active set and weights at
	// this position's height/epoch — the same axis quasar's
	// ConsensusCert.ValidatorSetRoot binds (consensus_cert_legs.go). Because it is
	// folded into the canonical signed message, a cert assembled from votes cast
	// under set-root R cannot be re-presented as certifying under a different
	// set-root R': every signature was over R, so reconstructing the message with
	// R' fails clause-6 verify.
	//
	// This turns the stake-weighted finality predicate ("⅔-by-stake at the
	// cert-position epoch") from an assumption into an enforced invariant: a
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

// CanonicalRejectMessage is the exact byte string a validator signs to vote
// REJECT on a position — the accept=0 leg of the same layout. It is not a
// certificate message: a finality cert proves ACCEPT and carries only accept
// signatures. It exists because the accept byte is bound, so the two decisions
// are distinct messages, and proving that is what the conformance corpus does:
// a reject signature presented as an accept must fail, and a port that folded
// the decision out of the message would pass every accept-only vector.
func CanonicalRejectMessage(pos VotePosition) []byte {
	return canonicalVoteMessageFor(pos, false)
}

// canonicalVoteMessageFor builds the domain-separated vote message for a
// position AND a decision. The accept byte is bound, so an ACCEPT signature
// (accept=1, what a cert carries) and a REJECT signature (accept=0) over the
// same position are DISTINCT messages — a reject signature can never be
// presented as an accept (and vice-versa). The engine verifies accept votes
// against (pos,true) and reject votes against (pos,false); the cert only ever
// uses (pos,true).
//
// The message binds the canonical execution identity, not the outer envelope.
// The outer proposervm ids (pos.BlockID/ParentID) are absent from this message —
// they are transport cache keys. The signed identity is the inner execution
// commitment {canonical_id, parent_canonical_id, execution_state_root,
// payload_root}. Consequence: two validators that executed the same inner block
// sign byte-identical messages even if they received different outer envelopes,
// so their votes interoperate and a cert assembled from them verifies on every
// node. A locally-derived wrapper id can never reach a signature.
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
// validator_set_root is bound before the accept byte so a vote is committed to
// the exact weighted validator set it was cast under: a cert gathered under
// set-root R cannot be re-verified as certifying under a different set R'. The
// domain tag carries its own version so a canonical-commitment signature can
// never be confused with an outer-id signature.
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
	// Canonical execution identity — the signed primary object. Outer ids omitted.
	// The non-wrapped degrade lives here and only here: a position whose canonical
	// fields are unset (a bare in-process VM block with no inner/outer split, or a
	// fixed-set chain) binds its outer id under the canonical slot. A
	// proposervm-wrapped block carries a distinct CanonicalID and binds that.
	// Resolving the degrade here rather than at the position-build sites is what
	// guarantees every producer of a position — engine or test — signs and verifies
	// the same bytes for the same block.
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
// VerifyVote is deterministic and side-effect free, returns true iff sig is a
// valid signature by nodeID over message, and does not panic on adversarial input
// (it returns false). An implementation that consults a validator set treats an
// unknown nodeID as "false", not an error — a cert with an out-of-set voter is
// simply invalid.
//
// epochHeight is the P-chain height the block's weighted validator set is pinned
// to. An implementation that resolves the voter's public key from a validator set
// resolves it from the set in force at epochHeight — the same height the set-root
// and the ⅔-by-stake tally are read at — never from the current validator map.
// Resolving from the current map drops the legitimate vote of a validator that has
// since left the current set but was a member at epochHeight (the async-skew window
// during a staking change), stalling finality for that block. The four reads —
// membership, pubkey, set-root, stake — all key off epochHeight, so a cert is
// internally consistent at exactly one epoch.
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
// stake-weighted supermajority rather than a raw voter count. On a PoS chain with
// unequal stake the two are not the same predicate: a coalition of many low-stake
// validators can reach "α distinct voters" while controlling a minority of stake.
// When a value/PoS chain supplies a StakeSource, a cert finalizes only if both
// hold: (count ≥ α) and (Σ voter stake ≥ ⅔ Σ total stake).
//
// Determinism and fail-closed: Weight is deterministic for a given (nodeID, epoch)
// and returns 0 for an unknown or out-of-set voter, so such a voter contributes no
// stake and cannot inflate the numerator. TotalStake is the epoch's total active
// stake; if it is 0 the source is unusable and the caller treats the cert as
// unverifiable. The engine binds the source to the cert's position height so
// weights are read at the right epoch.
type StakeSource interface {
	// Weight returns the voting weight (stake) of nodeID at the given height, or
	// 0 if nodeID is not an active validator at that height.
	Weight(nodeID ids.NodeID, height uint64) uint64
	// TotalStake returns the total active validator stake at the given height.
	TotalStake(height uint64) uint64
	// ValidatorCount returns the number of distinct active validators at the given
	// height — the round-scoped view-change's BFT committee size n. Its POL and
	// precommit both count distinct validators, so n is the live set, not the sample
	// K. It is read from the same height-indexed set as Weight and TotalStake, so the
	// count-quorum and the stake-quorum are over one identical set. 0 for an
	// unresolved or empty set; the view-change then keeps the configured committee,
	// guarded by the 2α−n>f bound.
	ValidatorCount(height uint64) int
}

// ValidatorSetRootSource computes the commitment to the active weighted
// validator set at a given height — the value bound into a VotePosition's
// ValidatorSetRoot. The node supplies it from the chain's validator set; the
// engine stamps it into every position it signs or assembles, so a cert is
// cryptographically pinned to the exact set and weights it was certified under.
// It is deterministic for a given height across all honest nodes: every node
// computing the root for height H has to agree, or their signatures over the same
// block would not be mutually verifiable.
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

// VerifyWeighted is the tier-selected finality predicate. It first runs Verify (the
// tier-agnostic structural and signature predicate) and then enforces the threshold for
// the cert's own tier:
//
//   - Nova → a strict majority of total stake by distinct signers (config.HalfStakeFloor)
//     plus a NovaSignerFloor count, and deliberately not a supermajority. This is the
//     local-execution rung: it has to ignite at a bare majority, so a ⅔ threshold here
//     would defeat its purpose. Crash-fault-safe by majority intersection, and not
//     Byzantine-safe — that is Quasar's job.
//   - Quasar → a strict >⅔ of total stake by distinct signers (config.TwoThirdsStakeFloor)
//     plus a config.TwoThirdsCount(n) count of them — the same supermajority read in
//     stake and in seats, and both must hold. This is the export rung, the Byzantine-safe
//     finality bridges, DEX settlement, cross-chain messages and validator-set transitions
//     consume, so "how much stake agreed" is not enough: it must also be how MANY.
//
// The tier is read from the cert, but the threshold is re-derived from the authoritative
// validator set rather than taken from the cert's self-declared Threshold, so a cert
// cannot forge its tier upward: a Nova set of votes relabeled Quasar fails the ⅔-by-stake
// check, and a Quasar cert relabeled Nova merely under-claims. An unknown tier fails
// closed.
//
// A nil stake source means no stake model is supplied — the caller uses Verify instead
// and is responsible for the equal-stake admission invariant documented on the engine.
// VerifyWeighted with a nil source fails closed, so a mis-connected caller is loud rather
// than silently count-only.
func (c *QuorumCert) VerifyWeighted(verifier VoteVerifier, stake StakeSource, epochHeight uint64) error {
	if err := c.Verify(verifier, epochHeight); err != nil {
		return err
	}
	if stake == nil {
		return fmt.Errorf("%w: stake source nil", ErrQCStakeBelowSupermajority)
	}
	switch c.Tier {
	case Nova:
		return c.verifyNovaMajority(stake, epochHeight)
	case Quasar:
		return c.verifyQuasarSupermajority(stake, epochHeight)
	default:
		return fmt.Errorf("%w: tier=%s", ErrQCUnknownTier, c.Tier)
	}
}

// verifyNovaMajority enforces the Nova local-execution threshold: the cert's distinct
// voters hold a strict majority of stake at the epoch (config.HalfStakeFloor), and there
// are at least NovaSignerFloor(n) of them. Both are recomputed from the authoritative
// live set, so a cert can never self-declare a below-majority Nova threshold. An
// unresolved set (n<1, or zero total stake) fails closed: a majority of an unknown set
// cannot be asserted, and NovaQuorum(0)=1 would otherwise let a lone node self-accept
// while its view of the validator set is transiently empty.
//
// Stake, not head-count. Nova is still a bare majority and still deliberately below the
// Quasar ⅔ export threshold — it is only read in the unit the chain actually weighs votes
// in. On an equal-stake set the two readings are identical (⌊n/2⌋+1 signers either way),
// so a uniform fleet sees no difference. They diverge when weights do, and a head-count is
// wrong in both directions there. It lets a registration at the minimum stake raise the
// bar: one more entry takes ⌊n/2⌋+1 from 3 to 4, so a set of five validators plus a
// minimum-stake sixth needs four of the five to agree and tolerates one loss instead of
// two. And it lets that same minimum stake cast a vote toward the majority that decides
// what the chain is. Registration is open at minValidatorStake, so both are purchasable.
// A majority of stake is neither.
//
// The signer floor is the guard the stake predicate cannot give: a single validator
// holding a stake majority would otherwise self-ignite. Stake majority and the floor are
// two independent predicates, neither sufficient alone.
func (c *QuorumCert) verifyNovaMajority(stake StakeSource, epochHeight uint64) error {
	n := stake.ValidatorCount(epochHeight)
	if n < 1 {
		return fmt.Errorf("%w: nova tier over an unresolved validator set (n=%d) at epoch %d",
			ErrQCBelowThreshold, n, epochHeight)
	}
	if floor := NovaSignerFloor(n); c.VoterCount() < floor {
		return fmt.Errorf("%w: nova cert has %d distinct voters, need at least %d of %d at epoch %d",
			ErrQCBelowThreshold, c.VoterCount(), floor, n, epochHeight)
	}
	total := stake.TotalStake(epochHeight)
	if total == 0 {
		return fmt.Errorf("%w: total stake is zero at epoch height %d (value-height %d)",
			ErrQCStakeBelowMajority, epochHeight, c.Position.Height)
	}
	voted, err := c.votedStake(stake, epochHeight)
	if err != nil {
		return err
	}
	halfFloor := config.HalfStakeFloor(total)
	if voted <= halfFloor {
		return fmt.Errorf("%w: nova voted=%d total=%d (need > floor(total/2)=%d) at epoch %d",
			ErrQCStakeBelowMajority, voted, total, halfFloor, epochHeight)
	}
	return nil
}

// verifyQuasarSupermajority enforces the Quasar EXPORT threshold: the summed stake of the
// cert's distinct voters STRICTLY exceeds two-thirds of the total stake at the epoch, AND
// there are at least config.TwoThirdsCount(n) of them.
//
// TWO INDEPENDENT PREDICATES, neither sufficient alone — the same shape as Nova, one rung
// up. Stake alone is not export-grade finality: a single validator holding two thirds of the
// stake clears the stake floor on its OWN signature, and a cert with one signer on it is a
// cert one operator can mint, one key can forge and one compromise can move. Byzantine
// safety is an argument about how many INDEPENDENT parties agreed; a threshold read only in
// stake makes that number one wherever stake is concentrated, which is exactly where the
// argument is needed. The count is the guard the stake predicate cannot give.
//
// The count is the stake floor read in SEATS: config.TwoThirdsCount(n) = ⌊2n/3⌋+1, derived
// from the same config.TwoThirdsStakeFloor arithmetic, so the two halves of the export rule
// have one definition between them and cannot drift. It is not a configured parameter — an
// operator flag that could set it to 1 would put the whole guard back in the hands of the
// party it constrains.
//
// ORDER: stake first, then the count. Both must pass, so the order decides nothing except
// which clause a refusal is NAMED by, and stake is the more informative name here. On an
// equal-stake set the two floors are the same bar in two units and bind together, and a
// refusal that says "the stake was short" says how far short a quorum was; the count clause
// can only ever bind ALONE where the weights are lopsided, so reaching it means the stake was
// there and the signers were not — which is the whale, stated precisely. Nova orders the
// other way for a substantive reason: its count floor is a LOWER, saturating, stake-
// independent bar (NovaSignerFloor caps at 3), so it is a different question asked first,
// not the same question in another unit.
func (c *QuorumCert) verifyQuasarSupermajority(stake StakeSource, epochHeight uint64) error {
	// The ⅔-by-stake tally is read at the block's P-chain epoch height — the same
	// height the cert's set-root and the per-voter pubkeys are read at — rather than
	// at c.Position.Height, the value-chain height, which platformvm would reject as
	// unfinalized and which races ahead of the P-chain epoch. Reading the tally at
	// the same epoch the signatures were cast under is what guarantees a validator
	// whose vote is in the cert also contributes its epoch weight.
	total := stake.TotalStake(epochHeight)
	if total == 0 {
		// No known stake at this epoch — cannot assert a supermajority. Fail closed.
		return fmt.Errorf("%w: total stake is zero at epoch height %d (value-height %d)", ErrQCStakeBelowSupermajority, epochHeight, c.Position.Height)
	}
	voted, err := c.votedStake(stake, epochHeight)
	if err != nil {
		return err
	}
	// Strict supermajority by stake: accept iff voted > floor(2·total/3). The floor
	// has one definition, config.TwoThirdsStakeFloor — the same function the live-set
	// parameter sizer derives α from (config.WeightedSupermajorityThreshold), so the
	// count threshold the node sizes to can never drift from the stake predicate
	// enforced here.
	twoThirdsFloor := config.TwoThirdsStakeFloor(total)
	if voted <= twoThirdsFloor {
		return fmt.Errorf("%w: voted=%d total=%d (need > floor(2/3·total)=%d) at height %d",
			ErrQCStakeBelowSupermajority, voted, total, twoThirdsFloor, c.Position.Height)
	}
	// The distinct-signer half of the export rule. Read from the same authoritative set
	// the tally was read against, so a cert can no more declare its own count floor than
	// its own stake floor.
	//
	// An unresolved set fails closed and does so HERE rather than at the top: a source
	// reporting n<1 alongside a zero total is already refused by the stake clause above
	// with the reason it has always been refused for, and only a source that reports no
	// validators while reporting stake reaches this line. Two thirds of no set is not a
	// number, and TwoThirdsCount(0)=1 would hand a lone signer a floor of one.
	n := stake.ValidatorCount(epochHeight)
	if n < 1 {
		return fmt.Errorf("%w: quasar tier over an unresolved validator set (n=%d) at epoch %d",
			ErrQCBelowThreshold, n, epochHeight)
	}
	if floor := config.TwoThirdsCount(n); c.VoterCount() < floor {
		return fmt.Errorf("%w: quasar cert has %d distinct voters, need at least %d of %d at epoch %d",
			ErrQCBelowThreshold, c.VoterCount(), floor, n, epochHeight)
	}
	return nil
}

// votedStake is the summed stake of the certificate's voters — checked, and
// refused rather than clamped.
//
// One function, because both rungs read the same tally and a floor is only as
// honest as the number it is read against. The voters are a subset of the
// members, so against any set Register or FlattenValidatorSet admitted this sum
// is bounded by a total those doors already proved representable, and the
// overflow is unreachable. Reaching it is therefore evidence about the
// StakeSource, not about the votes: it says the source is reporting weights no
// admitted set could hold, and a source that cannot state its own total is not
// one a threshold can be read against.
//
// It must not wrap, and wrapping is what an unchecked loop does: Go's + is
// modular, so a sum past 2^64 does not fail — it returns a DIFFERENT number and
// the comparison proceeds as if nothing happened. The wrapped value can land
// anywhere, above the floor included. Three voters at 2^63 wrap to 2^63, which
// clears floor(total/2) for a total of 2^64−1 by one, and the certificate is
// accepted on arithmetic rather than on votes. Monotone is not the property
// that matters here; being the sum it claims to be is. Rust holds the same
// clause in Cert::voted_stake, with checked_add over the same votes.
func (c *QuorumCert) votedStake(stake StakeSource, epochHeight uint64) (uint64, error) {
	var voted uint64
	for i := range c.Votes {
		sum, err := math.Add64(voted, stake.Weight(c.Votes[i].NodeID, epochHeight))
		if err != nil {
			return 0, fmt.Errorf("%w: cert tally at epoch %d: %w",
				validators.ErrWeightOverflow, epochHeight, err)
		}
		voted = sum
	}
	return voted, nil
}

// AuthorizesExport reports whether this cert is export-grade finality (Quasar or
// brighter) — the safety boundary a bridge, DEX settlement or cross-chain consumer
// admits on. A Nova cert (local execution) returns false even though it is a valid,
// signature-verified majority cert. This is the cert-level projection of
// Finality.AuthorizesExport, so a raw cert in hand can be gated without re-deriving its
// tier from context. A nil cert is not export-grade.
func (c *QuorumCert) AuthorizesExport() bool {
	return c != nil && c.Tier.AuthorizesExport()
}

// QuorumCert is the engine-level finality witness: α distinct validators each
// signed ACCEPT over Position. It is portable (gossipable), verifiable by any
// node holding the VoteVerifier, and deterministic to assemble.
//
// It is not a signature — there is no aggregate field, because nothing is
// aggregated. The cert carries the per-voter signed records; verification is
// the predicate in Verify.
type QuorumCert struct {
	// Version pins the cert format.
	Version uint16
	// Type names the cert's role (QCFinality). Bound into every vote message.
	Type QCType
	// Tier is the finality rung this cert attests — Nova (a bare-majority
	// local-execution cert) or Quasar (a strict ⅔-by-stake export cert). It selects
	// which threshold VerifyWeighted enforces, so one cert type carries both accept
	// (Nova) and export (Quasar) finality with no ambiguity about what a given cert
	// proves. It is deliberately not bound into the per-vote signed message: a vote is
	// tier-agnostic ("I accept this block"), and one accept vote counts toward both a
	// Nova majority and a Quasar supermajority. The tier therefore cannot be forged
	// upward — VerifyWeighted re-derives the real threshold from the authoritative
	// validator set, so relabeling a Nova cert as Quasar is rejected (it will not clear
	// ⅔-by-stake) and relabeling a Quasar cert as Nova only under-claims. Only Nova and
	// Quasar are valid here; Horizon is the PQ seal layer, not a QuorumCert.
	Tier Finality
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
// Permissionless and deterministic: no secrets, no randomness. Given the same
// votes and position the same cert comes out.
//
// It sorts votes strictly-increasing by NodeID (dropping duplicate NodeIDs —
// last-writer-wins is not used; a duplicate NodeID is an error so a cert can
// never double-count), and rejects a structurally impossible cert (no votes,
// zero threshold, a non-accept vote). It does not verify signatures —
// assembly is orthogonal to verification (a relaying node assembles; verifiers
// verify). A subsequent Verify is the gate.
//
// Returns ErrQCBelowThreshold if fewer than `threshold` distinct accept votes
// are supplied: assembly only succeeds once the quorum is actually present, so
// a cert can never claim a quorum it does not hold.
//
// tier names what the cert attests — Nova (local-execution majority) or Quasar
// (export ⅔-by-stake). It must be exactly one of those two; any other rung
// (Photon/Wave/Horizon) is rejected, because a QuorumCert is only ever an accept
// (Nova) or an export (Quasar) witness — Horizon is the separate PQ seal layer.
func AssembleQuorumCert(pos VotePosition, tier Finality, threshold uint32, votes []SignedVote) (*QuorumCert, error) {
	if tier != Nova && tier != Quasar {
		return nil, fmt.Errorf("%w: %s", ErrQCUnknownTier, tier)
	}
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
		Tier:      tier,
		Position:  pos,
		Threshold: threshold,
		Votes:     sorted,
	}, nil
}

// Verify checks the cert against verifier. Returns nil iff every predicate
// clause holds; otherwise a typed error naming the first failure. It does not
// panic and does no unbounded work — an adversarial cert (duplicate voter, bad
// signature, sub-threshold, wrong position) yields a clean error.
//
// Predicate (the engine-level projection of quasar's weighted-quorum predicate):
//
//	(0) verifier is non-nil                          (fail-closed)
//	(1) version + type match
//	(2) threshold (alpha) > 0
//	(3) at least one vote
//	for each vote, in order:
//	  (4) node ids are strictly increasing            (distinct, anti-double-count)
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
	// A cert attests exactly one accept/export tier. Reject an out-of-range tier
	// (a wire-decoded cert with a bogus/zero tier byte) before any signature work —
	// so the tier-agnostic count-only Verify path (equal-stake chains) also fails
	// closed on a garbage tier, not just the tier-selected VerifyWeighted.
	if c.Tier != Nova && c.Tier != Quasar {
		return fmt.Errorf("%w: tier=%s", ErrQCUnknownTier, c.Tier)
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
	if c.Version != o.Version || c.Type != o.Type || c.Tier != o.Tier || c.Position != o.Position ||
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
