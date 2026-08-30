// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package fpc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

// The seed decides α, the count of votes a round accepts on. Two triples that
// reach one seed are two chains, or two epochs, running one threshold schedule.
// These tests hold the preimage to the only property that rules that out: the
// bytes can be read back apart into the inputs that produced them.

// readPreimage parses a preimage back into the three inputs it was built from.
// It exists only here: production never decodes a seed preimage. Its job is to
// state injectivity as something a machine checks rather than something a
// comment claims — a preimage that parses back to one triple was produced by
// one triple.
func readPreimage(b []byte) (epoch uint64, chainID, prev []byte, err error) {
	rest := b
	take := func(n int) ([]byte, error) {
		if len(rest) < n {
			return nil, fmt.Errorf("want %d bytes, have %d", n, len(rest))
		}
		out := rest[:n]
		rest = rest[n:]
		return out, nil
	}
	dom, err := take(len(seedDomain))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("domain: %w", err)
	}
	if string(dom) != seedDomain {
		return 0, nil, nil, fmt.Errorf("domain is %q, want %q", dom, seedDomain)
	}
	u64 := func() (uint64, error) {
		raw, err := take(8)
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint64(raw), nil
	}
	if epoch, err = u64(); err != nil {
		return 0, nil, nil, fmt.Errorf("epoch: %w", err)
	}
	field := func(name string) ([]byte, error) {
		n, err := u64()
		if err != nil {
			return nil, fmt.Errorf("%s length: %w", name, err)
		}
		if n > uint64(len(rest)) {
			return nil, fmt.Errorf("%s claims %d bytes, %d remain", name, n, len(rest))
		}
		return take(int(n))
	}
	if chainID, err = field("chain id"); err != nil {
		return 0, nil, nil, err
	}
	if prev, err = field("prev block hash"); err != nil {
		return 0, nil, nil, err
	}
	if len(rest) != 0 {
		return 0, nil, nil, errors.New("trailing bytes after the last field")
	}
	return epoch, chainID, prev, nil
}

// triples covers the shapes that make a concatenation ambiguous: empty fields,
// fields that are prefixes of each other, and a field that carries the bytes of
// the field beside it.
func triples() []struct {
	note   string
	epoch  uint64
	chain  []byte
	parent []byte
} {
	parent := []byte("0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecaf")
	return []struct {
		note   string
		epoch  uint64
		chain  []byte
		parent []byte
	}{
		{"every input at its floor", 0, nil, nil},
		{"an empty chain, a real parent", 0, nil, parent},
		{"a real chain, no parent", 1, []byte("chain-A"), nil},
		{"the chain, one byte shorter", 1, []byte("chain-"), []byte("A")},
		{"the same bytes, split the other way", 1, []byte("chain"), []byte("-A")},
		{"the next epoch", 2, []byte("chain-A"), nil},
		{"another chain", 1, []byte("chain-B"), nil},
		{"a real chain and a real parent", 7, []byte("lux-mainnet"), parent},
		{"the chain that swallows the parent", 7, append(append([]byte{}, []byte("lux-mainnet")...), parent...), nil},
		{"the top of the epoch counter", ^uint64(0), []byte("lux-mainnet"), parent},
	}
}

// TestTheChainCannotSwallowTheParent is the attack the lengths exist for.
//
// A chain that names itself lux-mainnet||H binds no parent and still derives
// what lux-mainnet derives at parent H. Under a concatenated preimage those are
// one seed, so the parent — the input nobody can know before the previous epoch
// finalizes — is traded for a name the chain chooses for itself.
func TestTheChainCannotSwallowTheParent(t *testing.T) {
	chain := []byte("lux-mainnet")
	parent := []byte("0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecaf")
	swallowed := append(append([]byte{}, chain...), parent...)

	honest := DeriveEpochSeed(7, chain, parent)
	forged := DeriveEpochSeed(7, swallowed, nil)

	if bytes.Equal(honest, forged) {
		t.Fatalf("a chain naming itself %q derives the seed %q derives at its parent: "+
			"the parent has stopped being an input\n  both = %x", swallowed, chain, honest)
	}
	t.Logf("honest %x\nforged %x", honest, forged)
}

// TestTheFieldBoundaryDoesNotSlide covers the general case: moving a byte from
// the end of the chain id to the front of the parent must move the seed.
func TestTheFieldBoundaryDoesNotSlide(t *testing.T) {
	for _, c := range []struct{ chain, parent []byte }{
		{[]byte("chain-A"), nil},
		{[]byte("chain-"), []byte("A")},
		{[]byte("chain"), []byte("-A")},
		{[]byte("chai"), []byte("n-A")},
	} {
		t.Logf("chain=%-9q parent=%-5q seed=%x", c.chain, c.parent, DeriveEpochSeed(1, c.chain, c.parent))
	}
	a := DeriveEpochSeed(1, []byte("chain-A"), nil)
	b := DeriveEpochSeed(1, []byte("chain-"), []byte("A"))
	if bytes.Equal(a, b) {
		t.Fatalf("the boundary between the chain id and the parent slides: both = %x", a)
	}
}

// TestOneTripleReachesOnePreimage states injectivity the strong way. A preimage
// that parses back to the inputs that built it cannot have been built by any
// other inputs, so no sweep of examples is needed to rule the rest out.
func TestOneTripleReachesOnePreimage(t *testing.T) {
	for _, c := range triples() {
		epoch, chain, parent, err := readPreimage(EpochSeedPreimage(c.epoch, c.chain, c.parent))
		if err != nil {
			t.Fatalf("%s: the preimage does not read back apart: %v", c.note, err)
		}
		if epoch != c.epoch || !bytes.Equal(chain, c.chain) || !bytes.Equal(parent, c.parent) {
			t.Fatalf("%s: read back (%d, %q, %q), built from (%d, %q, %q)",
				c.note, epoch, chain, parent, c.epoch, c.chain, c.parent)
		}
	}
}

// TestNoTwoTriplesShareASeed is the empirical backstop to the parse-back proof:
// distinct inputs, distinct preimages, distinct seeds, checked pairwise.
func TestNoTwoTriplesShareASeed(t *testing.T) {
	seen := map[string]string{}
	for _, c := range triples() {
		seed := fmt.Sprintf("%x", DeriveEpochSeed(c.epoch, c.chain, c.parent))
		if prev, ok := seen[seed]; ok {
			t.Fatalf("%q and %q reach one seed %s: two epochs run one threshold schedule", prev, c.note, seed)
		}
		seen[seed] = c.note
	}
	if len(seen) != len(triples()) {
		t.Fatalf("%d triples reached %d seeds", len(triples()), len(seen))
	}
	t.Logf("%d distinct triples, %d distinct seeds", len(triples()), len(seen))
}

// TestTheDigestIsTakenOverThePreimage keeps the two exported functions from
// drifting: the corpus records the preimage and checks the seed, which only
// means anything while the seed is that preimage's digest.
func TestTheDigestIsTakenOverThePreimage(t *testing.T) {
	for _, c := range triples() {
		pre := EpochSeedPreimage(c.epoch, c.chain, c.parent)
		if !bytes.HasPrefix(pre, []byte(seedDomain)) {
			t.Fatalf("%s: the preimage does not open with the domain", c.note)
		}
		want := sha256Of(pre)
		if got := fmt.Sprintf("%x", DeriveEpochSeed(c.epoch, c.chain, c.parent)); got != want {
			t.Fatalf("%s: seed is %s, sha256 of the preimage is %s", c.note, got, want)
		}
	}
}

func sha256Of(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}
