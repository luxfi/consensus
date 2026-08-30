// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luxfi/consensus/validator/pop"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/validators"
)

func secret(t *testing.T, seed byte) *bls.SecretKey {
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

func nodeID(b byte) ids.NodeID {
	var n ids.NodeID
	for i := range n {
		n[i] = b
	}
	return n
}

func registration(t *testing.T, seed, id byte, weight uint64) Registration {
	t.Helper()
	sk := secret(t, seed)
	n := nodeID(id)
	proof, err := pop.Sign(sk, n)
	if err != nil {
		t.Fatalf("pop.Sign: %v", err)
	}
	return Registration{
		NodeID: n,
		Key:    bls.PublicKeyToCompressedBytes(sk.PublicKey()),
		Proof:  proof,
		Weight: weight,
	}
}

// TestRegisterAdmitsProvenValidators is the happy path, and it pins the shape
// the retirement of the merge guarantees: one node id per validator, always.
func TestRegisterAdmitsProvenValidators(t *testing.T) {
	set, err := Register([]Registration{
		registration(t, 1, 0x01, 100),
		registration(t, 2, 0x02, 200),
		registration(t, 3, 0x03, 300),
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(set.Validators) != 3 {
		t.Fatalf("admitted %d validators, want 3", len(set.Validators))
	}
	if set.TotalWeight != 600 {
		t.Fatalf("total weight %d, want 600", set.TotalWeight)
	}
	for _, v := range set.Validators {
		if len(v.NodeIDs) != 1 {
			t.Fatalf("a validator carries %d node ids — the merge is back", len(v.NodeIDs))
		}
	}
	for i := 1; i < len(set.Validators); i++ {
		if bytes.Compare(set.Validators[i-1].PublicKeyBytes, set.Validators[i].PublicKeyBytes) >= 0 {
			t.Fatal("the admitted set is not in canonical order")
		}
	}
}

// TestOneKeyOneNode is the second rule, and the reason possession alone is not
// enough: the holder of a key can mint a VALID node-bound proof for any identity
// it likes, so both registrations below are individually sound. What refuses them
// is uniqueness — counting distinct voters has to count distinct signers.
func TestOneKeyOneNode(t *testing.T) {
	sk := secret(t, 5)
	key := bls.PublicKeyToCompressedBytes(sk.PublicKey())

	one, two := nodeID(0x10), nodeID(0x20)
	proofOne, err := pop.Sign(sk, one)
	if err != nil {
		t.Fatalf("pop.Sign: %v", err)
	}
	proofTwo, err := pop.Sign(sk, two)
	if err != nil {
		t.Fatalf("pop.Sign: %v", err)
	}
	// Both proofs are real: possession holds for each pair on its own.
	if err := pop.Verify(one, key, proofOne); err != nil {
		t.Fatalf("first proof is not sound: %v", err)
	}
	if err := pop.Verify(two, key, proofTwo); err != nil {
		t.Fatalf("second proof is not sound: %v", err)
	}

	_, err = Register([]Registration{
		{NodeID: one, Key: key, Proof: proofOne, Weight: 1},
		{NodeID: two, Key: key, Proof: proofTwo, Weight: 1},
	})
	if !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("one key admitted under two nodes: %v", err)
	}
}

// TestOneNodeOneKey is the other axis: one node id, two keys, each with a genuine
// proof. Possession does not catch it — both proofs are sound — so only a node
// uniqueness rule refuses it, and it must, or one operator holds several signer
// indices and several shares of the weight under one identity.
func TestOneNodeOneKey(t *testing.T) {
	node := nodeID(0x40)
	skA, skB := secret(t, 40), secret(t, 41)
	keyA := bls.PublicKeyToCompressedBytes(skA.PublicKey())
	keyB := bls.PublicKeyToCompressedBytes(skB.PublicKey())
	proofA, err := pop.Sign(skA, node)
	if err != nil {
		t.Fatalf("pop.Sign A: %v", err)
	}
	proofB, err := pop.Sign(skB, node)
	if err != nil {
		t.Fatalf("pop.Sign B: %v", err)
	}
	// Both proofs are real: possession holds for each pair on its own.
	if err := pop.Verify(node, keyA, proofA); err != nil {
		t.Fatalf("first proof is not sound: %v", err)
	}
	if err := pop.Verify(node, keyB, proofB); err != nil {
		t.Fatalf("second proof is not sound: %v", err)
	}

	set, err := Register([]Registration{
		{NodeID: node, Key: keyA, Proof: proofA, Weight: 100},
		{NodeID: node, Key: keyB, Proof: proofB, Weight: 100},
	})
	if !errors.Is(err, ErrDuplicateNode) {
		t.Fatalf("one node admitted under two keys: err=%v set=%d validators", err, len(set.Validators))
	}
}

// TestRegisterRefusesTheUnproven walks the possession gate.
func TestRegisterRefusesTheUnproven(t *testing.T) {
	good := registration(t, 6, 0x30, 1)

	lifted := registration(t, 6, 0x31, 1) // same key, different node…
	lifted.Proof = good.Proof             // …carrying the first node's proof

	pubkeyOnly := registration(t, 7, 0x32, 1)
	sig, err := secret(t, 7).SignProofOfPossession(pubkeyOnly.Key)
	if err != nil {
		t.Fatalf("SignProofOfPossession: %v", err)
	}
	pubkeyOnly.Proof = bls.SignatureToBytes(sig)

	noProof := registration(t, 8, 0x33, 1)
	noProof.Proof = nil

	noKey := registration(t, 9, 0x34, 1)
	noKey.Key = nil

	for _, c := range []struct {
		name string
		r    Registration
		want error
	}{
		{"a proof lifted from another node", lifted, ErrPossession},
		{"the pubkey-only proof", pubkeyOnly, ErrPossession},
		{"no proof at all", noProof, ErrPossession},
		{"no key at all", noKey, ErrNoKey},
	} {
		if _, err := Register([]Registration{c.r}); !errors.Is(err, c.want) {
			t.Fatalf("%s: got %v, want %v", c.name, err, c.want)
		}
		// And it must not slip in beside a good one either.
		if _, err := Register([]Registration{good, c.r}); !errors.Is(err, c.want) {
			t.Fatalf("%s beside a good registration: got %v, want %v", c.name, err, c.want)
		}
	}
}

// TestRegisterRefusesTheSetWhole: a bad registration fails the call rather than
// being dropped, so an admitted set's weight always describes the set.
func TestRegisterRefusesTheSetWhole(t *testing.T) {
	bad := registration(t, 12, 0x40, 5)
	bad.Proof = make([]byte, pop.ProofLen)
	set, err := Register([]Registration{registration(t, 13, 0x41, 7), bad})
	if err == nil {
		t.Fatal("an unproven registration was dropped instead of refused")
	}
	if len(set.Validators) != 0 || set.TotalWeight != 0 {
		t.Fatal("a refused call returned a partial set")
	}
}

// TestRegisterIsOrderIndependent: which duplicate a set is refused on must be
// the same on every node, whatever order the caller built the slice in.
func TestRegisterIsOrderIndependent(t *testing.T) {
	sk := secret(t, 15)
	key := bls.PublicKeyToCompressedBytes(sk.PublicKey())
	one, two := nodeID(0x50), nodeID(0x51)
	p1, _ := pop.Sign(sk, one)
	p2, _ := pop.Sign(sk, two)
	a := Registration{NodeID: one, Key: key, Proof: p1, Weight: 1}
	b := Registration{NodeID: two, Key: key, Proof: p2, Weight: 1}

	_, forward := Register([]Registration{a, b})
	_, backward := Register([]Registration{b, a})
	if forward == nil || backward == nil {
		t.Fatal("a duplicate key was admitted")
	}
	if forward.Error() != backward.Error() {
		t.Fatalf("the refusal depends on input order:\n %q\n %q", forward, backward)
	}
}

// -----------------------------------------------------------------------------
// FlattenValidatorSet
// -----------------------------------------------------------------------------

func output(id byte, sk *bls.SecretKey, weight uint64) (ids.NodeID, *GetValidatorOutput) {
	n := nodeID(id)
	var key []byte
	if sk != nil {
		key = bls.PublicKeyToCompressedBytes(sk.PublicKey())
	}
	return n, &GetValidatorOutput{NodeID: n, PublicKey: key, Weight: weight, Light: weight}
}

// TestFlattenRefusesTheMerge is the retirement itself. Upstream folded two node
// ids that shared a key into ONE validator carrying both ids and their summed
// weight; that fold is the only thing that ever made many-nodes-one-key
// representable, and a signer floor written to require many holders was cleared
// by one. The state is now an error.
func TestFlattenRefusesTheMerge(t *testing.T) {
	sk := secret(t, 17)
	in := map[ids.NodeID]*GetValidatorOutput{}
	for _, id := range []byte{0x60, 0x61} {
		n, o := output(id, sk, 10)
		in[n] = o
	}

	// What the upstream does with it, recorded so the change is visible: one
	// validator, two node ids, twenty weight.
	merged, err := validators.FlattenValidatorSet(in)
	if err != nil {
		t.Fatalf("upstream flatten: %v", err)
	}
	if len(merged.Validators) != 1 || len(merged.Validators[0].NodeIDs) != 2 {
		t.Fatalf("upstream no longer merges — re-read this test: %+v", merged.Validators)
	}

	if _, err := FlattenValidatorSet(in); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("the merge is still reachable: %v", err)
	}
}

// TestFlattenMatchesUpstreamWhenKeysAreDistinct is the compatibility proof: on
// every set that already obeys one-key-one-node — which is every set the chain
// holds today — this returns exactly what the network has always computed,
// including the canonical ORDER that decides warp signature bit indices.
func TestFlattenMatchesUpstreamWhenKeysAreDistinct(t *testing.T) {
	in := map[ids.NodeID]*GetValidatorOutput{}
	for i, seed := range []byte{31, 32, 33, 34, 35} {
		n, o := output(byte(0x70+i), secret(t, seed), uint64(100*(i+1)))
		in[n] = o
	}
	// A validator with no key at all: it cannot sign, and its weight still counts.
	n, o := output(0x7f, nil, 55)
	in[n] = o

	mine, err := FlattenValidatorSet(in)
	if err != nil {
		t.Fatalf("FlattenValidatorSet: %v", err)
	}
	theirs, err := validators.FlattenValidatorSet(in)
	if err != nil {
		t.Fatalf("upstream flatten: %v", err)
	}
	if mine.TotalWeight != theirs.TotalWeight {
		t.Fatalf("total weight diverged: %d vs %d", mine.TotalWeight, theirs.TotalWeight)
	}
	if mine.TotalWeight != 1500+55 {
		t.Fatalf("a keyless validator's weight stopped counting: %d", mine.TotalWeight)
	}
	if len(mine.Validators) != len(theirs.Validators) {
		t.Fatalf("validator count diverged: %d vs %d", len(mine.Validators), len(theirs.Validators))
	}

	// Same CONTENT, compared as a set keyed by node id, not index by index:
	// upstream sorts by uncompressed bytes, which are 96 under cgo and 48 under
	// purego, so its ORDER is build-dependent. Ours sorts by the compressed bytes
	// and is the same on every build; that the two orders can differ under cgo is
	// the divergence this branch removes, not a parity failure.
	theirByNode := map[ids.NodeID]*CanonicalValidator{}
	for _, v := range theirs.Validators {
		theirByNode[v.NodeIDs[0]] = v
	}
	for _, a := range mine.Validators {
		if len(a.NodeIDs) != 1 {
			t.Fatalf("a validator carries %d node ids, want 1", len(a.NodeIDs))
		}
		b, ok := theirByNode[a.NodeIDs[0]]
		if !ok {
			t.Fatalf("node %s is in mine but not upstream", a.NodeIDs[0])
		}
		if a.Weight != b.Weight {
			t.Fatalf("weight diverged for %s: %d vs %d", a.NodeIDs[0], a.Weight, b.Weight)
		}
		if !bytes.Equal(a.PublicKeyBytes, bls.PublicKeyToCompressedBytes(b.PublicKey)) {
			t.Fatalf("key diverged for %s", a.NodeIDs[0])
		}
	}

	// And ours is deterministically ordered by the compressed key, on any build.
	for i := 1; i < len(mine.Validators); i++ {
		if bytes.Compare(mine.Validators[i-1].PublicKeyBytes, mine.Validators[i].PublicKeyBytes) >= 0 {
			t.Fatalf("mine is not sorted ascending by compressed key at %d", i)
		}
	}
}

// TestFlattenRefusesTheSameDuplicateEveryTime: map iteration order must not
// decide which pair a set is refused on, or two nodes disagree about a set.
func TestFlattenRefusesTheSameDuplicateEveryTime(t *testing.T) {
	sk := secret(t, 41)
	in := map[ids.NodeID]*GetValidatorOutput{}
	for _, id := range []byte{0x91, 0x92, 0x93, 0x94} {
		n, o := output(id, sk, 1)
		in[n] = o
	}
	_, first := FlattenValidatorSet(in)
	if first == nil {
		t.Fatal("four nodes sharing one key were admitted")
	}
	for i := 0; i < 64; i++ {
		if _, err := FlattenValidatorSet(in); err == nil || err.Error() != first.Error() {
			t.Fatalf("the refusal moved with map order: %q then %q", first, err)
		}
	}
}

// TestFlattenStillSkipsAnUndecodableKey: this function reads a set it did not
// admit, so one unreadable key costs a signer, never the whole set. Strictness
// lives at Register.
func TestFlattenStillSkipsAnUndecodableKey(t *testing.T) {
	in := map[ids.NodeID]*GetValidatorOutput{}
	n, o := output(0xA1, secret(t, 51), 10)
	in[n] = o
	bad := nodeID(0xA2)
	in[bad] = &GetValidatorOutput{NodeID: bad, PublicKey: bytes.Repeat([]byte{0xEE}, bls.PublicKeyLen), Weight: 90}

	set, err := FlattenValidatorSet(in)
	if err != nil {
		t.Fatalf("an undecodable key failed the whole set: %v", err)
	}
	if len(set.Validators) != 1 {
		t.Fatalf("admitted %d validators, want 1", len(set.Validators))
	}
	if set.TotalWeight != 100 {
		t.Fatalf("total weight %d, want 100 — the skipped validator's stake still counts", set.TotalWeight)
	}
}

// TestFlattenAndRegisterAgree: two doors, one set. An admitted set and a
// flattened one must be the same bytes in the same order.
func TestFlattenAndRegisterAgree(t *testing.T) {
	rs := []Registration{
		registration(t, 61, 0xB1, 3),
		registration(t, 62, 0xB2, 5),
		registration(t, 63, 0xB3, 7),
	}
	in := map[ids.NodeID]*GetValidatorOutput{}
	for _, r := range rs {
		in[r.NodeID] = &GetValidatorOutput{NodeID: r.NodeID, PublicKey: r.Key, Weight: r.Weight, Light: r.Weight}
	}

	admitted, err := Register(rs)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	flattened, err := FlattenValidatorSet(in)
	if err != nil {
		t.Fatalf("FlattenValidatorSet: %v", err)
	}
	if admitted.TotalWeight != flattened.TotalWeight {
		t.Fatalf("weights disagree: %d vs %d", admitted.TotalWeight, flattened.TotalWeight)
	}
	for i := range admitted.Validators {
		if !bytes.Equal(admitted.Validators[i].PublicKeyBytes, flattened.Validators[i].PublicKeyBytes) {
			t.Fatalf("the two doors order the set differently at %d", i)
		}
		if admitted.Validators[i].NodeIDs[0] != flattened.Validators[i].NodeIDs[0] {
			t.Fatalf("the two doors name different nodes at %d", i)
		}
	}
}
