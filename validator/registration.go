// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// registration.go — who is admitted to the validator set, and on what evidence.
//
// A certificate is only as sound as the set it is checked against. Verification
// can be perfect and still be checked against a set that counts one key holder
// as many signers, so two rules are enforced here, in this order:
//
//	ENCODING     the key is a canonical compressed BLS12-381 G1 point;
//	POSSESSION   a node-bound proof (validator/pop) binds this key to this node;
//	UNIQUENESS   the key is registered to no other node.
//
// Then, and only then, weight is counted. The order is not decoration: a pairing
// check on bytes that are not a point is undefined, and weight counted before
// uniqueness is weight counted twice.
//
// UNIQUENESS IS WHY THE MERGE IS GONE. FlattenValidatorSet used to fold two
// validators that shared a public key into ONE entry carrying both node ids and
// their summed weight. That fold is the only thing that ever made
// many-nodes-one-key representable, so a floor written to require many DISTINCT
// signers was satisfied by one holder registering one key under several ids. The
// state does not exist to normalize any more: a repeated key is refused, and a
// canonical validator carries exactly one node id.

package validators

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/luxfi/consensus/validator/pop"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/math"
	"github.com/luxfi/validators"
)

var (
	// ErrDuplicateKey is returned when two node ids present the same public key.
	// One key, one node: counting distinct voters has to count distinct signers.
	ErrDuplicateKey = errors.New("validators: public key is registered to more than one node")

	// ErrDuplicateNode is returned when one node id is registered twice. One
	// node, one key: a slice can name a node twice where a map could not, and
	// each entry is a separate signer index and a separate share of the weight.
	ErrDuplicateNode = errors.New("validators: node is registered more than once")

	// ErrZeroWeight is returned when a validator is admitted with no stake — a
	// signer that counts toward the number of signers but not toward the weight.
	ErrZeroWeight = errors.New("validators: validator has zero weight")

	// ErrNoKey is returned when a registration carries no public key. A validator
	// with no key cannot sign, so it cannot be admitted through the proof path.
	ErrNoKey = errors.New("validators: registration carries no public key")

	// ErrPossession is returned when a registration's proof does not bind its key
	// to its node. It wraps the reason from validator/pop.
	ErrPossession = errors.New("validators: registration is not proven")
)

// Registration is one validator asking to be counted: the identity it claims,
// the key it will sign under, the proof binding the two, and the weight staked
// behind it.
type Registration struct {
	NodeID ids.NodeID
	// Key is the BLS12-381 min_pk public key, compressed G1, 48 bytes.
	Key []byte
	// Proof is the node-bound proof of possession over NodeID ‖ Key, compressed
	// G2, 96 bytes. See validator/pop for the exact preimage and domain.
	Proof  []byte
	Weight uint64
}

// Register admits a registration set and returns it in canonical order.
//
// This is the admission rule of the standard, written once. Every registration
// is checked in the fixed order — encoding, possession, uniqueness — and the set
// is admitted whole or not at all: a single unproven or duplicated key fails the
// call rather than being dropped from an otherwise-good set, because a set that
// silently loses a signer is a set whose weight no longer describes it.
//
// Ordering and PublicKeyBytes match FlattenValidatorSet exactly, so an admitted
// set and a flattened one are the same set.
func Register(rs []Registration) (CanonicalValidatorSet, error) {
	// Deterministic order in, deterministic error out: which duplicate a set is
	// refused on must not depend on the order a caller happened to build a slice.
	sorted := slices.Clone(rs)
	slices.SortStableFunc(sorted, func(a, b Registration) int {
		return bytes.Compare(a.NodeID[:], b.NodeID[:])
	})

	var (
		out         = make([]*CanonicalValidator, 0, len(sorted))
		byKey       = make(map[string]ids.NodeID, len(sorted))
		byNode      = make(map[ids.NodeID]struct{}, len(sorted))
		totalWeight uint64
	)
	for _, r := range sorted {
		if len(r.Key) == 0 {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s", ErrNoKey, r.NodeID)
		}
		// A keyed validator with no stake is a phantom signer: it raises the
		// signer count without raising the weight, which is the same disagreement
		// between "how many signed" and "how much signed" that BuildWeightedValidatorSet
		// refuses for the same reason.
		if r.Weight == 0 {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s", ErrZeroWeight, r.NodeID)
		}
		// ENCODING, then POSSESSION — both inside pop.Verify, in that order.
		if err := pop.Verify(r.NodeID, r.Key, r.Proof); err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s: %w", ErrPossession, r.NodeID, err)
		}
		pk, err := bls.PublicKeyFromCompressedBytes(r.Key)
		if err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%s: %w", r.NodeID, err)
		}
		// The set has to be a set on BOTH axes, and neither implies the other.
		// UNIQUENESS OF KEY, keyed on the canonical compressed encoding decoded
		// from the point — never the caller's input bytes, which pop.Verify read
		// but which a caller could mutate between reads.
		canonical := string(bls.PublicKeyToCompressedBytes(pk))
		if first, dup := byKey[canonical]; dup {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s and %s", ErrDuplicateKey, first, r.NodeID)
		}
		byKey[canonical] = r.NodeID
		// UNIQUENESS OF NODE. A slice can name one node twice under two keys,
		// each with a genuine proof; possession does not catch it, and one
		// operator in N canonical slots is N signer indices and N times its
		// weight. `FlattenValidatorSet`'s map input made this unrepresentable;
		// a slice does not, so it is refused here.
		if _, dup := byNode[r.NodeID]; dup {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s", ErrDuplicateNode, r.NodeID)
		}
		byNode[r.NodeID] = struct{}{}

		// WEIGHT, last.
		sum, err := math.Add64(totalWeight, r.Weight)
		if err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %w", ErrWeightOverflow, err)
		}
		totalWeight = sum
		out = append(out, &CanonicalValidator{
			PublicKey: pk,
			// Compressed, always 48 bytes — the sort key `Compare` reads, so the
			// canonical ORDER does not depend on which crypto build produced it.
			// Upstream's uncompressed bytes are 96 under cgo and 48 under purego,
			// which orders the same set two ways; the oracle must have one order.
			PublicKeyBytes: []byte(canonical),
			Weight:         r.Weight,
			NodeIDs:        []ids.NodeID{r.NodeID},
		})
	}
	slices.SortFunc(out, (*CanonicalValidator).Compare)
	return CanonicalValidatorSet{Validators: out, TotalWeight: totalWeight}, nil
}

// FlattenValidatorSet converts vdrSet into canonical order and returns the total
// weight, which — as upstream — includes validators that carry no public key and
// therefore cannot sign.
//
// It replaces the re-export of validators.FlattenValidatorSet for one reason: the
// upstream MERGES two node ids that share a public key into a single validator
// with a summed weight, and that state is retired. A repeated key is now an
// error, so every returned CanonicalValidator carries exactly one node id.
//
// It does NOT check possession, because this input has no proof to check: it is
// a set the chain already admitted, and the proof lives at admission. Register is
// where the full order runs. Until the P-chain's registration proof becomes
// node-bound, uniqueness is the half of the rule this side can hold — and it is
// the half that decides whether counting signers counts holders.
//
// An undecodable key is skipped rather than refused, exactly as upstream: this
// function reads a set it did not admit, and refusing the whole set on one bad
// key would trade a signer for a halt. Strictness belongs at admission.
//
// A key with NO STAKE behind it is refused, not skipped, and that is not the
// same trade. Skipping a bad key costs a signer the set was going to lose
// anyway; a keyed seat carrying zero weight is a signer the set gains for free
// — it counts toward the distinct-signer floor and contributes nothing to the
// stake floor, which is precisely the disagreement between "how many signed" and
// "how much signed" that the two floors exist to prevent. Register refuses it
// with the same ErrZeroWeight, so the two doors agree on the clause they share.
func FlattenValidatorSet(vdrSet map[ids.NodeID]*GetValidatorOutput) (CanonicalValidatorSet, error) {
	// Iterate in node-id order. The upstream ranged over the map and sorted at the
	// end, which is fine for a total but not for an error: which duplicate pair a
	// set is refused on must be the same on every node.
	nodeIDs := make([]ids.NodeID, 0, len(vdrSet))
	for nodeID := range vdrSet {
		nodeIDs = append(nodeIDs, nodeID)
	}
	slices.SortFunc(nodeIDs, func(a, b ids.NodeID) int { return bytes.Compare(a[:], b[:]) })

	var (
		out         = make([]*CanonicalValidator, 0, len(nodeIDs))
		byKey       = make(map[string]ids.NodeID, len(nodeIDs))
		totalWeight uint64
		err         error
	)
	for _, nodeID := range nodeIDs {
		vdr := vdrSet[nodeID]
		if vdr == nil {
			continue
		}
		totalWeight, err = math.Add64(totalWeight, vdr.Weight)
		if err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %w", ErrWeightOverflow, err)
		}
		if len(vdr.PublicKey) == 0 {
			continue
		}
		// ZERO WEIGHT, on a member that carries a key. The same clause Register
		// runs, at the same point in the order — after the key is known to be
		// present and before it is decoded — and the justification is that
		// AGREEMENT, not the phantom argument on its own.
		//
		// The phantom argument covers most of the ground: an entry that raises
		// the count of distinct signers a floor is read against without raising
		// the weight is exactly the disagreement between how many signed and how
		// much signed that the count floors exist to prevent. But it does not
		// cover all of it, and the gap is worth naming rather than glossing. A
		// seat whose key is present and does NOT decode can never sign, so it is
		// no phantom — and it is refused here anyway, because the weight is read
		// before the key is. Deferring the weight clause until after the decode
		// would close that gap and open a worse one: whether a set is admitted
		// would then depend on which keys THIS node's crypto build could parse,
		// and two nodes reading one set would disagree about whether it exists.
		//
		// So the clause is placed where Register places it and refuses what
		// Register refuses. Both doors admit exactly the same sets, and the
		// floors mean one thing at registration and the same thing here. That
		// agreement is the property; the phantom is why the property is worth
		// having.
		//
		// A KEYLESS member with no stake is not refused: it was skipped above, so
		// it never becomes a signer, and it neither signs nor weighs. That is the
		// one leniency this door keeps, and it is deliberate — it is the door for
		// a set the chain already admitted, and a member this node holds no key
		// for is a fact about the node's view, not a defect in the set. A key with
		// no stake behind it is a defect.
		if vdr.Weight == 0 {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s", ErrZeroWeight, nodeID)
		}
		pk, err := bls.PublicKeyFromCompressedBytes(vdr.PublicKey)
		if err != nil {
			continue
		}
		// Compressed, so uniqueness and the canonical order are the same on every
		// crypto build — see Register.
		pkBytes := bls.PublicKeyToCompressedBytes(pk)
		if first, dup := byKey[string(pkBytes)]; dup {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s and %s", ErrDuplicateKey, first, nodeID)
		}
		byKey[string(pkBytes)] = nodeID
		out = append(out, &CanonicalValidator{
			PublicKey:      pk,
			PublicKeyBytes: pkBytes,
			Weight:         vdr.Weight,
			NodeIDs:        []ids.NodeID{nodeID},
		})
	}
	slices.SortFunc(out, (*CanonicalValidator).Compare)
	return CanonicalValidatorSet{Validators: out, TotalWeight: totalWeight}, nil
}

// The upstream flatten is kept referenced so a version bump that changes it is a
// visible diff here rather than a silent divergence: this package now owns the
// rule, and the assertion is that it still owns the same signature.
var _ func(map[ids.NodeID]*GetValidatorOutput) (validators.CanonicalValidatorSet, error) = validators.FlattenValidatorSet
