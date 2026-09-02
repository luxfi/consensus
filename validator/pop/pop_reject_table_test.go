// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pop_reject_table_test.go — the refusals, each named by the class it belongs
// to, mirroring the frozen corpus.
//
// The existing tests hold that a malformed proof is refused. This file holds the
// stronger thing the standard actually fixes: WHICH refusal. The order is
// encoding, then possession, and the two classes are distinguishable precisely so
// nothing is admitted on a point that was never a point and no pairing is spent
// on one either. A table that only asserted "err != nil" would pass with the
// order reversed and with a pairing computed over garbage — the two properties
// the order exists to give.
//
// The rows are the cases in vectors/pop.json, which Rust and C++ are checked
// against, written here as unit assertions because Go is the oracle those bytes
// were generated from. Two of them cannot be built from a keypair — an identity
// encoding and an on-curve point outside the prime-order subgroup — so those
// carry the corpus's own bytes.
package pop

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// mustHex is the corpus's spelling of a fixed byte string.
func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("corpus bytes are not hex: %v", err)
	}
	return b
}

// theseAreTheCorpusBytes are the two shapes no keypair produces.
const (
	// identityG1 is the compressed encoding of the G1 point at infinity: valid
	// bytes, a real encoding, and never a validator. A set that admitted it would
	// hold a seat whose signature is the empty product.
	identityG1 = "c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	// identityG2 is the same point one group up: the compressed G2 infinity, which
	// as a "proof" verifies against nothing and must not be read as a proof.
	identityG2 = "c000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	// subgroupG1 is on the curve and outside the prime-order subgroup — the point
	// the subgroup check exists for. Decoding it and skipping that check is the
	// classic small-subgroup opening, so it must be an encoding refusal.
	subgroupG1 = "800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000004"
)

// TestVerifyRefusalClasses walks every refusal, naming the class. The valid row
// comes first: a table of refusals over a fixture that never verified proves
// only that the fixture is broken.
func TestVerifyRefusalClasses(t *testing.T) {
	mine, other := key(t, 101), key(t, 102)
	me, them := node(0xA1), node(0xA2)

	myKey := bls.PublicKeyToCompressedBytes(mine.PublicKey())
	otherKey := bls.PublicKeyToCompressedBytes(other.PublicKey())

	myProof, err := Sign(mine, me)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	otherProof, err := Sign(other, them)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(me, myKey, myProof); err != nil {
		t.Fatalf("the control pair must verify, or no row below says anything: %v", err)
	}

	// A signature over the SAME 68 bytes under the vote domain rather than the
	// proof domain. It is a real signature by the real key over the real message,
	// and it is not a proof — which is the whole of domain separation.
	msg, err := Message(me, myKey)
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	voteSig, err := mine.Sign(msg)
	if err != nil {
		t.Fatalf("Sign under the vote domain: %v", err)
	}
	voteDomainProof := bls.SignatureToBytes(voteSig)

	garbageProof := make([]byte, ProofLen)
	for i := range garbageProof {
		garbageProof[i] = 0xAB
	}

	for _, row := range []struct {
		holds      string
		node       ids.NodeID
		key, proof []byte
		want       error
	}{
		// POSSESSION — the bytes all decode; the pairing is what refuses them.
		{
			holds: "a proof made for one node does not admit that key under another",
			node:  them, key: myKey, proof: myProof, want: ErrPossession,
		},
		{
			holds: "a proof made by one key does not admit another key",
			node:  me, key: otherKey, proof: myProof, want: ErrPossession,
		},
		{
			holds: "another pair's proof is not this pair's proof",
			node:  me, key: myKey, proof: otherProof, want: ErrPossession,
		},
		{
			holds: "a vote over the proof's own message is not a proof — the domains do not cross",
			node:  me, key: myKey, proof: voteDomainProof, want: ErrPossession,
		},
		// ENCODING, key side — refused before any pairing.
		{
			holds: "an absent key is an encoding refusal, not a possession one",
			node:  me, key: nil, proof: myProof, want: ErrKey,
		},
		{
			holds: "a key one byte short is a key that was never a point",
			node:  me, key: myKey[:KeyLen-1], proof: myProof, want: ErrKey,
		},
		{
			holds: "a key one byte long is refused rather than truncated to fit",
			node:  me, key: append(append([]byte{}, myKey...), 0), proof: myProof, want: ErrKey,
		},
		{
			holds: "the identity is a well-formed encoding and never a validator",
			node:  me, key: mustHex(t, identityG1), proof: myProof, want: ErrKey,
		},
		{
			holds: "an on-curve point outside the prime-order subgroup is refused at the encoding gate",
			node:  me, key: mustHex(t, subgroupG1), proof: myProof, want: ErrKey,
		},
		{
			holds: "the infinity-bit-set compression-bit-clear trap is an error, never a panic",
			node:  me, key: append([]byte{0x40}, make([]byte, KeyLen-1)...), proof: myProof, want: ErrKey,
		},
		{
			holds: "48 zero bytes are not a point",
			node:  me, key: make([]byte, KeyLen), proof: myProof, want: ErrKey,
		},
		// ENCODING, proof side.
		{
			holds: "an absent proof is an encoding refusal",
			node:  me, key: myKey, proof: nil, want: ErrProof,
		},
		{
			holds: "a proof one byte short is refused",
			node:  me, key: myKey, proof: myProof[:ProofLen-1], want: ErrProof,
		},
		{
			holds: "a proof one byte long is refused rather than truncated",
			node:  me, key: myKey, proof: append(append([]byte{}, myProof...), 0), want: ErrProof,
		},
		{
			holds: "96 bytes of noise are not a G2 point",
			node:  me, key: myKey, proof: garbageProof, want: ErrProof,
		},
		{
			holds: "the G2 identity is not a proof",
			node:  me, key: myKey, proof: mustHex(t, identityG2), want: ErrProof,
		},
		{
			holds: "the G2 infinity-bit trap is an error, never a panic",
			node:  me, key: myKey, proof: append([]byte{0x40}, make([]byte, ProofLen-1)...), want: ErrProof,
		},
		{
			holds: "96 zero bytes are not a proof",
			node:  me, key: myKey, proof: make([]byte, ProofLen), want: ErrProof,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			err := Verify(row.node, row.key, row.proof)
			if !errors.Is(err, row.want) {
				t.Fatalf("want %v, got %v", row.want, err)
			}
		})
	}
}

// TestTheKeyIsReadBeforeTheProof holds the ORDER the standard fixes. Both
// arguments are malformed, and the answer names the key — so a caller cannot
// learn anything about the proof from a call whose key was never a point, and no
// pairing is reached on either.
func TestTheKeyIsReadBeforeTheProof(t *testing.T) {
	err := Verify(node(0xB1), make([]byte, KeyLen), make([]byte, ProofLen))
	if !errors.Is(err, ErrKey) {
		t.Fatalf("a malformed key and a malformed proof must answer for the key, got %v", err)
	}
	if errors.Is(err, ErrProof) {
		t.Fatal("the refusal names both classes — they must be distinguishable")
	}
}

// TestPossessionIsOneAnswer records that the four ways a decoded proof can fail
// to bind are deliberately not told apart. Made for another node, made for
// another key, made under another domain, made by someone without the secret —
// all one sentence, because the caller's decision is the same and a finer answer
// would be an oracle about which half of the pair a registrant got wrong.
func TestPossessionIsOneAnswer(t *testing.T) {
	mine, other := key(t, 111), key(t, 112)
	me, them := node(0xC1), node(0xC2)
	myKey := bls.PublicKeyToCompressedBytes(mine.PublicKey())

	myProof, err := Sign(mine, me)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	theirProof, err := Sign(other, them)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	wrongNode := Verify(them, myKey, myProof)
	wrongPair := Verify(me, myKey, theirProof)
	if !errors.Is(wrongNode, ErrPossession) || !errors.Is(wrongPair, ErrPossession) {
		t.Fatalf("both must be possession refusals: %v / %v", wrongNode, wrongPair)
	}
	if wrongNode.Error() != wrongPair.Error() {
		t.Fatalf("the two refusals differ:\n %q\n %q", wrongNode, wrongPair)
	}
}

// TestSignFailsClosedOnAnUnusableSecret covers the second shape an absent secret
// arrives in. A nil pointer is the obvious one; a pointer to a zero SecretKey is
// the one a caller reaches by holding a struct field it never populated, and it
// gets past the nil check. Its public key is absent, so the preimage would be 20
// bytes of node and nothing else — a well-formed signature over the wrong
// statement. Sign must return the refusal instead.
func TestSignFailsClosedOnAnUnusableSecret(t *testing.T) {
	proof, err := Sign(&bls.SecretKey{}, node(0xE1))
	if !errors.Is(err, ErrKey) {
		t.Fatalf("want the encoding refusal, got %v", err)
	}
	if proof != nil {
		t.Fatal("a refused signing returned a proof")
	}
}

// TestMessageRefusesAKeyOfTheWrongWidth is the preimage's own guard. The layout
// is fixed-width with no separator, so a key of any other length would shift the
// boundary between the two fields and make the 68 bytes read back apart into a
// different pair — which is exactly the ambiguity fixed widths exist to prevent.
func TestMessageRefusesAKeyOfTheWrongWidth(t *testing.T) {
	n := node(0xD1)
	full := bls.PublicKeyToCompressedBytes(key(t, 121).PublicKey())

	for _, row := range []struct {
		holds string
		key   []byte
	}{
		{"no key at all", nil},
		{"an empty key", []byte{}},
		{"a key one byte short", full[:KeyLen-1]},
		{"a key one byte long", append(append([]byte{}, full...), 0)},
	} {
		t.Run(row.holds, func(t *testing.T) {
			msg, err := Message(n, row.key)
			if !errors.Is(err, ErrKey) {
				t.Fatalf("want ErrKey, got %v", err)
			}
			if msg != nil {
				t.Fatal("a refused message must return no bytes — a caller must not sign what was rejected")
			}
		})
	}
}
