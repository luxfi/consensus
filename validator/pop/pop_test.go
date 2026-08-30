// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pop

import (
	"bytes"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// key returns a deterministic secret key, so a failure is reproducible and the
// bytes a test asserts are the bytes a vector freezes.
func key(t *testing.T, seed byte) *bls.SecretKey {
	t.Helper()
	s := make([]byte, 32)
	for i := range s {
		s[i] = seed
	}
	sk, err := bls.SecretKeyFromSeed(s)
	if err != nil {
		t.Fatalf("SecretKeyFromSeed(%d): %v", seed, err)
	}
	return sk
}

func node(b byte) ids.NodeID {
	var n ids.NodeID
	for i := range n {
		n[i] = b
	}
	return n
}

// TestTheMessageIsNodeThenKey pins the layout the standard fixes: 20 bytes of
// node then 48 bytes of key, in that order, nothing between them and nothing
// around them.
func TestTheMessageIsNodeThenKey(t *testing.T) {
	n := node(0x11)
	pk := bls.PublicKeyToCompressedBytes(key(t, 1).PublicKey())

	msg, err := Message(n, pk)
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if len(msg) != MessageLen || MessageLen != 68 {
		t.Fatalf("message is %d bytes (MessageLen=%d), want 68", len(msg), MessageLen)
	}
	if NodeLen != 20 || KeyLen != 48 || ProofLen != 96 {
		t.Fatalf("widths moved: node=%d key=%d proof=%d, want 20/48/96", NodeLen, KeyLen, ProofLen)
	}
	if !bytes.Equal(msg[:NodeLen], n[:]) {
		t.Fatal("bytes 0..19 are not the node identity")
	}
	if !bytes.Equal(msg[NodeLen:], pk) {
		t.Fatal("bytes 20..67 are not the compressed public key")
	}
}

// TestTheDomainIsTheProofSuite holds the DST this package names to the one the
// crypto library calls the proof-of-possession ciphersuite. Naming it twice is
// how a domain drifts; this is the assertion that keeps the two the same string.
func TestTheDomainIsTheProofSuite(t *testing.T) {
	if got := bls.CiphersuiteProofOfPossession.String(); got != DST {
		t.Fatalf("DST is not the library's proof suite:\n got %q\nwant %q", got, DST)
	}
	if DST != "BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_" {
		t.Fatalf("the proof domain moved: %q", DST)
	}
}

// TestAProofVerifiesForItsOwnPair is the round trip, and the determinism the
// vector depends on: BLS signing carries no nonce, so one (key, node) pair has
// exactly one proof.
func TestAProofVerifiesForItsOwnPair(t *testing.T) {
	sk := key(t, 7)
	n := node(0x22)
	pk := bls.PublicKeyToCompressedBytes(sk.PublicKey())

	proof, err := Sign(sk, n)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(proof) != ProofLen {
		t.Fatalf("proof is %d bytes, want %d", len(proof), ProofLen)
	}
	if err := Verify(n, pk, proof); err != nil {
		t.Fatalf("a proof does not verify for the pair it was made for: %v", err)
	}
	again, err := Sign(sk, n)
	if err != nil {
		t.Fatalf("Sign again: %v", err)
	}
	if !bytes.Equal(proof, again) {
		t.Fatal("signing is not deterministic — a vector could not freeze this")
	}
}

// TestAProofCannotBeLifted is the whole reason the node is in the message. The
// pubkey-only proof is a statement about a key, so it travels with the key and
// re-registers under any identity. This one does not.
func TestAProofCannotBeLifted(t *testing.T) {
	sk := key(t, 9)
	pk := bls.PublicKeyToCompressedBytes(sk.PublicKey())
	mine, theirs := node(0x01), node(0x02)

	proof, err := Sign(sk, mine)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(mine, pk, proof); err != nil {
		t.Fatalf("own pair: %v", err)
	}
	if err := Verify(theirs, pk, proof); err == nil {
		t.Fatal("a proof made for one node verified under another — the key is not bound")
	}
	// One bit of the node is enough.
	near := mine
	near[NodeLen-1] ^= 0x01
	if err := Verify(near, pk, proof); err == nil {
		t.Fatal("a one-bit change to the node identity still verified")
	}
}

// TestThePubkeyOnlyProofIsRefused records what this supersedes: the IETF proof
// signs the public key alone, and that is exactly the proof an attacker already
// holds for every honest validator's published key.
func TestThePubkeyOnlyProofIsRefused(t *testing.T) {
	sk := key(t, 11)
	n := node(0x33)
	pk := bls.PublicKeyToCompressedBytes(sk.PublicKey())

	sig, err := sk.SignProofOfPossession(pk) // the old message: the key, and nothing else
	if err != nil {
		t.Fatalf("SignProofOfPossession: %v", err)
	}
	if err := Verify(n, pk, bls.SignatureToBytes(sig)); err == nil {
		t.Fatal("a pubkey-only proof was accepted as node-bound")
	}
}

// TestTheDomainsDoNotCross is domain separation, checked in both directions: a
// vote over the proof's bytes is not a proof, and a proof is not a vote.
func TestTheDomainsDoNotCross(t *testing.T) {
	sk := key(t, 13)
	n := node(0x44)
	pk := bls.PublicKeyToCompressedBytes(sk.PublicKey())
	msg, err := Message(n, pk)
	if err != nil {
		t.Fatalf("Message: %v", err)
	}

	vote, err := sk.Sign(msg) // the vote domain, ..._NUL_
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(n, pk, bls.SignatureToBytes(vote)); err == nil {
		t.Fatal("a vote over the proof's message was accepted as a proof")
	}

	proof, err := Sign(sk, n)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	sig, err := bls.SignatureFromBytes(proof)
	if err != nil {
		t.Fatalf("SignatureFromBytes: %v", err)
	}
	pub, err := bls.PublicKeyFromCompressedBytes(pk)
	if err != nil {
		t.Fatalf("PublicKeyFromCompressedBytes: %v", err)
	}
	if bls.Verify(pub, sig, msg) {
		t.Fatal("a proof verified as a vote — the two domains are not separated")
	}
}

// TestAProofIsNotAKeysToo: a proof made by one key must not admit another.
func TestAProofDoesNotTravelToAnotherKey(t *testing.T) {
	mine, other := key(t, 21), key(t, 22)
	n := node(0x55)

	proof, err := Sign(mine, n)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	otherPK := bls.PublicKeyToCompressedBytes(other.PublicKey())
	if err := Verify(n, otherPK, proof); err == nil {
		t.Fatal("a proof made by one key admitted another")
	}
}

// TestMalformedInputIsRefusedBeforeAnyPairing walks the encoding gate: every
// shape that is not a canonical point is an ErrKey/ErrProof, not a pairing.
func TestMalformedInputIsRefusedBeforeAnyPairing(t *testing.T) {
	sk := key(t, 31)
	n := node(0x66)
	pk := bls.PublicKeyToCompressedBytes(sk.PublicKey())
	proof, err := Sign(sk, n)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	garbageKey := make([]byte, KeyLen)
	for i := range garbageKey {
		garbageKey[i] = 0xAB
	}
	garbageProof := make([]byte, ProofLen)
	for i := range garbageProof {
		garbageProof[i] = 0xCD
	}
	// The compressed encoding of the identity: valid bytes, and never a validator.
	identityKey := make([]byte, KeyLen)
	identityKey[0] = 0xC0
	identityProof := make([]byte, ProofLen)
	identityProof[0] = 0xC0
	// The malformed "infinity set, compression clear" encoding, which is a panic
	// in a raw decoder and must be an error here.
	trapKey := make([]byte, KeyLen)
	trapKey[0] = 0x40
	trapProof := make([]byte, ProofLen)
	trapProof[0] = 0x40

	for _, c := range []struct {
		name       string
		key, proof []byte
	}{
		{"no key", nil, proof},
		{"short key", pk[:KeyLen-1], proof},
		{"long key", append(append([]byte{}, pk...), 0), proof},
		{"garbage key", garbageKey, proof},
		{"identity key", identityKey, proof},
		{"infinity-bit trap key", trapKey, proof},
		{"no proof", pk, nil},
		{"empty proof", pk, []byte{}},
		{"short proof", pk, proof[:ProofLen-1]},
		{"long proof", pk, append(append([]byte{}, proof...), 0)},
		{"garbage proof", pk, garbageProof},
		{"identity proof", pk, identityProof},
		{"infinity-bit trap proof", pk, trapProof},
		{"zero proof", pk, make([]byte, ProofLen)},
		{"zero key", make([]byte, KeyLen), proof},
	} {
		if err := Verify(n, c.key, c.proof); err == nil {
			t.Fatalf("%s was admitted", c.name)
		}
	}
}

// TestOneBitOfTheProofIsEnough: the proof is not a checksum.
func TestFlippingTheProofRefusesIt(t *testing.T) {
	sk := key(t, 41)
	n := node(0x77)
	pk := bls.PublicKeyToCompressedBytes(sk.PublicKey())
	proof, err := Sign(sk, n)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	for _, i := range []int{ProofLen - 1, ProofLen / 2} {
		bad := append([]byte{}, proof...)
		bad[i] ^= 0x01
		if err := Verify(n, pk, bad); err == nil {
			t.Fatalf("a proof with byte %d flipped was admitted", i)
		}
	}
}

// TestSignRefusesWithoutASecret keeps the nil path off the happy path.
func TestSignRefusesWithoutASecret(t *testing.T) {
	if _, err := Sign(nil, node(0x88)); err == nil {
		t.Fatal("signing with no secret key returned a proof")
	}
}
