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
		totalWeight uint64
	)
	for _, r := range sorted {
		if len(r.Key) == 0 {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s", ErrNoKey, r.NodeID)
		}
		// ENCODING, then POSSESSION — both inside pop.Verify, in that order.
		if err := pop.Verify(r.NodeID, r.Key, r.Proof); err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s: %w", ErrPossession, r.NodeID, err)
		}
		pk, err := bls.PublicKeyFromCompressedBytes(r.Key)
		if err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%s: %w", r.NodeID, err)
		}
		// UNIQUENESS. Keyed on the canonical compressed encoding, which pop.Verify
		// has already established is the only spelling of this point.
		if first, dup := byKey[string(r.Key)]; dup {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %s and %s", ErrDuplicateKey, first, r.NodeID)
		}
		byKey[string(r.Key)] = r.NodeID

		// WEIGHT, last.
		sum, err := math.Add64(totalWeight, r.Weight)
		if err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %w", ErrWeightOverflow, err)
		}
		totalWeight = sum
		out = append(out, &CanonicalValidator{
			PublicKey:      pk,
			PublicKeyBytes: bls.PublicKeyToUncompressedBytes(pk),
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
		pk, err := bls.PublicKeyFromCompressedBytes(vdr.PublicKey)
		if err != nil {
			continue
		}
		pkBytes := bls.PublicKeyToUncompressedBytes(pk)
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
