// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_cert_quasar.go — the forward bridge from the engine-level QuorumCert
// to protocol/quasar.WeightedQuorumCert, the canonical PQ finality witness.
//
// WHY A BRIDGE AND NOT A DIRECT WIRING:
//
// The engine-level QuorumCert (quorum_cert.go) and quasar's WeightedQuorumCert
// witness the SAME relation — "α-of-K validators accepted this value" — at two
// abstraction levels:
//
//   - engine QuorumCert: voters are ids.NodeID, each carrying ONE signature
//     over the canonical vote message. Self-contained in engine/chain; needs
//     only a pluggable VoteVerifier. This is what every chain (P/X/C/D) runs
//     today, on every deployment, with no extra plumbing.
//
//   - quasar.WeightedQuorumCert: signers are weighted validator-set LEAVES,
//     each carrying a FIPS 204/205 record + a Merkle inclusion proof against
//     the epoch's weighted-validator-set ROOT. It is the heavyweight,
//     post-quantum, full-node-verifiable witness — but it requires three
//     pieces of node-layer state the engine does not own: (1) the epoch's
//     weighted validator-set Merkle root, (2) each validator's ML-DSA/SLH-DSA
//     public-key leaf + voting weight, and (3) per-validator Merkle paths.
//
// The finality RULE is identical; only the witness CRYPTOGRAPHY differs. This
// bridge expresses that: a CryptoWitnessSource supplied by the node layer turns
// the engine cert's distinct signed voters into quasar signer records and
// assembles a WeightedQuorumCert that quasar.Verify accepts. When a chain has
// not (yet) plumbed its weighted PQ validator set, the source is nil and the
// engine cert remains the witness — same rule, lighter witness. NO crypto is
// ever faked: absent the real validator-set material, the bridge FAILS CLOSED
// (ErrQuasarWitnessUnavailable) rather than fabricate records.
//
// This is the "one way" wiring of protocol/quasar into engine/chain: the engine
// owns the quorum rule and the vote-collection topology; quasar owns the PQ
// certificate format; the bridge composes them without braiding.
package chain

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/ids"
)

// ErrQuasarWitnessUnavailable is returned when a quasar.WeightedQuorumCert is
// requested but the node has not supplied the weighted-validator-set material
// required to build one. Fail-closed: the caller falls back to the engine-level
// QuorumCert witness, never a fabricated PQ cert.
var ErrQuasarWitnessUnavailable = errors.New("chain: quasar weighted-quorum witness unavailable (no validator-set material plumbed)")

// ErrQuasarVoterNotInSet is returned when a cert voter has no corresponding
// weighted-validator-set leaf in the source — an out-of-set voter cannot be
// represented as a quasar signer record.
var ErrQuasarVoterNotInSet = errors.New("chain: cert voter has no weighted-validator-set leaf")

// CryptoWitnessSource is the node-layer hook that supplies the weighted
// validator-set material needed to upgrade an engine QuorumCert into a
// quasar.WeightedQuorumCert. The node implements this once it has plumbed its
// PQ validator key set; until then engine/chain operates on the engine cert
// alone and this is nil.
//
// All three methods describe the epoch the cert finalizes in. They are pure
// reads of committed validator-set state — no secrets, no signing.
type CryptoWitnessSource interface {
	// Epoch returns the consensus epoch for the given block height — the epoch
	// whose validator-set root the resulting cert proves against.
	Epoch(height uint64) uint64

	// ValidatorSetRoot returns the weighted-validator-set Merkle root committed
	// for the epoch (weighted_merkle.go ROOT). Every signer record's leaf is
	// proven against this.
	ValidatorSetRoot(epoch uint64) ([48]byte, bool)

	// QuorumThreshold returns the BFT quorum WEIGHT floor for the epoch — the
	// minimum total signer weight a valid quasar cert must assert. This is the
	// weighted analogue of the engine cert's count threshold (alpha).
	QuorumThreshold(epoch uint64) (uint64, bool)

	// SignerRecord turns one engine voter into its quasar signer record: the
	// validator's weighted-set leaf fields + a Merkle inclusion proof against
	// ValidatorSetRoot + the validator's FIPS signature over the SAME consensus
	// message the engine vote signed. Returns ok=false for an out-of-set voter.
	//
	// The node owns this mapping because it owns the validator key set; the
	// engine supplies the voter identity, the canonical message bytes, and the
	// engine signature, and the node resolves the leaf/path/record.
	SignerRecord(nodeID ids.NodeID, message []byte, engineSig []byte) (quasar.QuorumSignerRecord, bool)
}

// quasarChainID projects a 32-byte chain ids.ID onto the uint32 chain-id axis
// quasar's envelope binds. The high 4 big-endian bytes of the chain ID are the
// canonical projection: chain IDs in this engine are derived hashes, so the
// leading word is a stable, collision-resistant discriminator for the envelope
// axis (the full 32-byte block id is ALSO bound, via ValueHash, so this
// projection only scopes the chain, it does not weaken value binding).
func quasarChainID(chainID ids.ID) uint32 {
	return uint32(chainID[0])<<24 | uint32(chainID[1])<<16 | uint32(chainID[2])<<8 | uint32(chainID[3])
}

// ToQuasarCert upgrades a verified engine QuorumCert into a
// quasar.WeightedQuorumCert using node-supplied validator-set material. The
// engine cert MUST already have passed Verify (its voters are distinct and
// correctly signed) — this method maps, it does not re-decide quorum.
//
// Fail-closed: a nil source, a missing validator-set root / threshold, or an
// out-of-set voter yields an error and NO cert — the caller keeps the engine
// witness. NO record is ever fabricated.
//
// The resulting cert is verified with quasar's own Verify by the caller (a
// follower receiving it), against the chain's QuorumMessageEnvelope and
// QuorumVerifierConfig — that is where the PQ FIPS checks and the weighted
// Merkle inclusion run. This method only ASSEMBLES.
func (c *QuorumCert) ToQuasarCert(src CryptoWitnessSource) (*quasar.WeightedQuorumCert, error) {
	if c == nil {
		return nil, ErrQCNil
	}
	if src == nil {
		return nil, ErrQuasarWitnessUnavailable
	}

	epoch := src.Epoch(c.Position.Height)
	root, ok := src.ValidatorSetRoot(epoch)
	if !ok {
		return nil, fmt.Errorf("%w: epoch %d root", ErrQuasarWitnessUnavailable, epoch)
	}
	threshold, ok := src.QuorumThreshold(epoch)
	if !ok {
		return nil, fmt.Errorf("%w: epoch %d threshold", ErrQuasarWitnessUnavailable, epoch)
	}

	message := CanonicalVoteMessage(c.Position)

	records := make([]quasar.QuorumSignerRecord, 0, len(c.Votes))
	for i := range c.Votes {
		v := &c.Votes[i]
		rec, ok := src.SignerRecord(v.NodeID, message, v.Signature)
		if !ok {
			return nil, fmt.Errorf("%w: voter %s", ErrQuasarVoterNotInSet, v.NodeID)
		}
		records = append(records, rec)
	}

	params := quasar.QuorumCertParams{
		ChainID:          quasarChainID(c.Position.ChainID),
		Epoch:            epoch,
		Height:           c.Position.Height,
		Round:            c.Position.Round,
		ValueHash:        idTo32(c.Position.BlockID),
		QCType:           uint8(c.Type),
		ValidatorSetRoot: root,
		QuorumThreshold:  threshold,
	}
	return quasar.BuildWeightedQuorumCert(params, records)
}

// idTo32 copies an ids.ID (32 bytes) into a [32]byte for quasar's value-hash
// axis.
func idTo32(id ids.ID) [32]byte {
	var out [32]byte
	copy(out[:], id[:])
	return out
}
