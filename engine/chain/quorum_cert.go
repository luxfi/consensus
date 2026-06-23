// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_cert.go — the engine-level finality witness for chain consensus.
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
//     quasar bridge in quorum_cert_quasar.go). One rule; the witness format is
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

	"github.com/luxfi/ids"
)

// QuorumCertVersion is the wire/struct version of the engine-level quorum
// certificate. Bound into the canonical vote message and the cert digest so a
// version bump is non-malleable.
const QuorumCertVersion uint16 = 1

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

// VotePosition is the consensus position a vote (and a cert) binds to. Every
// axis here is folded into the canonical signed message, so a signature for one
// position can never be replayed at another (height/round/block/parent/chain).
type VotePosition struct {
	ChainID  ids.ID
	Height   uint64
	Round    uint32
	BlockID  ids.ID
	ParentID ids.ID
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
// Layout (big-endian, fixed-width, length-free because every field is fixed):
//
//	"LUX/chain/vote/v1\x00"   domain tag (NUL-terminated, role separator)
//	version:2  qc_type:1
//	chain_id:32  height:8  round:4  block_id:32  parent_id:32
//	accept:1   (0x01 accept | 0x00 reject)
func canonicalVoteMessageFor(pos VotePosition, accept bool) []byte {
	const tag = "LUX/chain/vote/v1\x00"
	buf := make([]byte, 0, len(tag)+2+1+32+8+4+32+32+1)
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
	buf = append(buf, pos.BlockID[:]...)
	buf = append(buf, pos.ParentID[:]...)
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
type VoteVerifier interface {
	VerifyVote(nodeID ids.NodeID, message []byte, sig []byte) bool
}

// VoteVerifierFunc adapts a function to a VoteVerifier.
type VoteVerifierFunc func(nodeID ids.NodeID, message []byte, sig []byte) bool

// VerifyVote implements VoteVerifier.
func (f VoteVerifierFunc) VerifyVote(nodeID ids.NodeID, message []byte, sig []byte) bool {
	return f(nodeID, message, sig)
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
func (c *QuorumCert) VerifyWeighted(verifier VoteVerifier, stake StakeSource) error {
	if err := c.Verify(verifier); err != nil {
		return err
	}
	if stake == nil {
		return fmt.Errorf("%w: stake source nil", ErrQCStakeBelowSupermajority)
	}
	total := stake.TotalStake(c.Position.Height)
	if total == 0 {
		// No known stake at this height — cannot assert a supermajority. Fail closed.
		return fmt.Errorf("%w: total stake is zero at height %d", ErrQCStakeBelowSupermajority, c.Position.Height)
	}
	var voted uint64
	for i := range c.Votes {
		voted += stake.Weight(c.Votes[i].NodeID, c.Position.Height)
	}
	// STRICT supermajority by stake: accept iff voted > 2·total/3 (Tendermint
	// +⅔). Computed overflow-safely from total alone (3·voted and 2·total would
	// overflow for large total): floor(2·total/3) = 2·(total/3) + adjustment for
	// the remainder, then accept iff voted strictly exceeds that floor.
	q, r := total/3, total%3
	twoThirdsFloor := 2 * q
	if r == 2 {
		twoThirdsFloor++ // r∈{0,1}→+0, r==2→+1 (floor(2r/3))
	}
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
func (c *QuorumCert) Verify(verifier VoteVerifier) error {
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

		// Clause (6): signature verifies over the cert's own position. The
		// verifier rebuilds nothing from the vote — the message is derived from
		// the CERT position, so a vote that signed a different position fails.
		if !verifier.VerifyVote(v.NodeID, message, v.Signature) {
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
