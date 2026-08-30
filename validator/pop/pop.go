// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package pop is the proof a registrant gives that it holds the key it is
// registering — AND that the key names the node it is registering it for.
//
// THE PUBKEY-ONLY PROOF IS NOT ENOUGH. The IETF proof of possession signs the
// public key under its own ciphersuite; it proves someone holds the secret, and
// nothing else. It is a statement about a key, so it travels with the key: an
// honest validator publishes its key and its proof, and anyone can re-register
// that pair under a SECOND node identity, because the proof never mentions the
// first. Counting signers then counts identities the key holder does not have,
// and a floor written to require many distinct signers is cleared by one.
//
// Binding the node closes it. The registrant signs its own identity followed by
// its own key, so a proof is only valid for the one pair it was made for. Lifting
// the key to another identity needs a proof over that identity, which needs the
// secret.
//
// THE MESSAGE, byte for byte:
//
//	offset  0 .. 19   node    — ids.NodeID,          20 bytes
//	offset 20 .. 67   key     — compressed G1 pubkey, 48 bytes
//	                  total                           68 bytes
//
// In that order, with no separator and no length prefix. Both fields are fixed
// width, so the concatenation reads back apart into exactly the pair that made
// it — the ambiguity that lengths exist to prevent (see the FPC seed preimage,
// where the fields are variable width) cannot arise here.
//
// WHY 20 BYTES OF NODE. The registered identity of a Lux validator IS its
// ids.NodeID: 20 bytes, the value AddValidatorTx names, the value a cert's votes
// carry, the value slashing evidence keys on, and the value the validator set
// maps. A proof that bound anything else would bind something the chain does not
// use to decide who signed. The consensus Id is 32 bytes and names a block or a
// chain, never a node; there is no canonical NodeID -> Id direction to borrow
// (ids.ID.ShortID() truncates 32 down to 20, and nothing defines the inverse), so
// widening to 32 would mean inventing a second identity for a node that already
// has one. Two names for one thing is what forks. The raw 20 bytes are bound —
// not the typed form (ids.TypedNodeID, scheme byte ‖ 20), because the scheme byte
// is a transport annotation and the 20 bytes are what the set and the certs carry.
//
// THE CIPHERSUITE is BLS12-381 min_pk with the proof-of-possession domain:
//
//	DST     BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_
//	pubkey  compressed G1, 48 bytes
//	proof   compressed G2, 96 bytes
//
// The _POP_ tag, never the vote's ..._NUL_. Separate domains are what keep a vote
// from being replayed as a proof and a proof from being replayed as a vote: the
// two hash the same bytes onto different points, so a signature under one is
// noise under the other. Verify enforces the order the standard fixes —
// encoding, then possession — and the caller enforces uniqueness after it.
//
// This package is the oracle: Rust and C++ reproduce these bytes, and
// vectors/pop.json in luxfi/conformance freezes them.
package pop

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

const (
	// NodeLen is the width of the node identity in the message: ids.NodeID.
	NodeLen = ids.NodeIDLen

	// KeyLen is the width of the public key in the message: BLS12-381 min_pk,
	// compressed G1.
	KeyLen = bls.PublicKeyLen

	// ProofLen is the width of the proof itself: BLS12-381 min_pk, compressed G2.
	ProofLen = bls.SignatureLen

	// MessageLen is the whole preimage: node ‖ key, both fixed width.
	MessageLen = NodeLen + KeyLen

	// DST is the domain separation tag the proof is signed under: the IETF
	// proof-of-possession ciphersuite, distinct from the vote's ..._NUL_. Named
	// here so the standard is readable at the place it is enforced; asserted
	// against the library's own constant by the tests, and frozen by the golden
	// proof in vectors/pop.json, which moves if the domain ever does.
	DST = "BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_"
)

var (
	// ErrKey is returned when the public key is not a canonical compressed
	// BLS12-381 G1 point: wrong width, non-canonical encoding, off-curve, outside
	// the prime-order subgroup, or the identity.
	ErrKey = errors.New("pop: public key is not a canonical compressed BLS12-381 G1 point")

	// ErrProof is returned when the proof is not a canonical compressed
	// BLS12-381 G2 point, by the same measures.
	ErrProof = errors.New("pop: proof is not a canonical compressed BLS12-381 G2 point")

	// ErrPossession is returned when the bytes decode but the proof does not
	// verify: it was made for a different node, for a different key, under a
	// different domain, or by someone who does not hold the secret. All four are
	// one answer — this key is not admitted under this identity.
	ErrPossession = errors.New("pop: proof does not bind this node to this key")

	// ErrSecretKey is returned when there is no secret key to prove with.
	ErrSecretKey = errors.New("pop: no secret key")
)

// Message returns the exact bytes a node-bound proof signs: node ‖ key, 68 bytes.
// The key must already be a canonical compressed G1 encoding; Message does not
// decode it, because it is called on the signing side from a key this node just
// serialized and on the verifying side from a key Verify has already decoded.
func Message(node ids.NodeID, key []byte) ([]byte, error) {
	if len(key) != KeyLen {
		return nil, fmt.Errorf("%w: got %d bytes, want %d", ErrKey, len(key), KeyLen)
	}
	msg := make([]byte, 0, MessageLen)
	msg = append(msg, node[:]...)
	msg = append(msg, key...)
	return msg, nil
}

// Sign produces this node's proof for its own key: the 96-byte compressed G2
// signature over Message(node, key) under the proof-of-possession domain. BLS
// signing is deterministic, so one (secret key, node) pair has exactly one proof
// — which is what lets a vector freeze it.
func Sign(sk *bls.SecretKey, node ids.NodeID) ([]byte, error) {
	if sk == nil {
		return nil, ErrSecretKey
	}
	key := bls.PublicKeyToCompressedBytes(sk.PublicKey())
	msg, err := Message(node, key)
	if err != nil {
		return nil, err
	}
	sig, err := sk.SignProofOfPossession(msg)
	if err != nil {
		return nil, fmt.Errorf("pop: sign: %w", err)
	}
	return bls.SignatureToBytes(sig), nil
}

// Verify admits (node, key) if and only if proof is a node-bound proof of
// possession for exactly that pair.
//
// The order is the standard's: ENCODING first — both points are decoded, which
// rejects wrong widths, non-canonical encodings, off-curve points, points outside
// the prime-order subgroup, and the identity — and only then POSSESSION, the
// pairing check. Nothing is admitted on a point that was never a point, and no
// pairing is computed on one either.
//
// The key is required to be the canonical compressed encoding of the point it
// decodes to, so the 48 bytes that go into the message are not a choice the
// registrant gets to make. Uniqueness — one key, one node — is the caller's
// check, because it is a property of the set and not of this pair.
func Verify(node ids.NodeID, key, proof []byte) error {
	if len(key) != KeyLen {
		return fmt.Errorf("%w: got %d bytes, want %d", ErrKey, len(key), KeyLen)
	}
	if len(proof) != ProofLen {
		return fmt.Errorf("%w: got %d bytes, want %d", ErrProof, len(proof), ProofLen)
	}
	pk, err := bls.PublicKeyFromCompressedBytes(key)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrKey, err)
	}
	// One point, one encoding. A decoder that accepted a second spelling of the
	// same key would let a registrant choose which 48 bytes the message carries,
	// and a proof over one spelling would not verify against the other.
	if canonical := bls.PublicKeyToCompressedBytes(pk); !bytes.Equal(canonical, key) {
		return fmt.Errorf("%w: encoding is not canonical", ErrKey)
	}
	sig, err := bls.SignatureFromBytes(proof)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrProof, err)
	}
	msg, err := Message(node, key)
	if err != nil {
		return err
	}
	if !bls.VerifyProofOfPossession(pk, sig, msg) {
		return ErrPossession
	}
	return nil
}
