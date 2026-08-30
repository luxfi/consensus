// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bench_trilang_test.go — the Go leg of the three-language CPU comparison.
//
// These benchmarks time the operations the Rust (pkg/rust criterion) and C++
// (consensus2 bench_trilang) legs time, on the same inputs, so a number here and
// a number there are about the same work:
//
//   sign          one BLS12-381 signature over CanonicalVoteMessage (226 bytes)
//   verifyOne     one signature verified against one resolved key
//   certVerify    QuorumCert.Verify — the finality predicate, O(n) pairings
//   aggregate     n signatures summed into one
//   fastAggVerify the O(1) aggregate predicate C++ verify_cert uses
//   voteMessage   the canonical message construction itself
//
// The committee is deterministic: key i is KeyGen(be64(i+1) padded to 32), the
// same derivation the Rust bench uses, so a run here is comparable to a run
// there rather than to a different set of keys.
//
// certVerify and fastAggVerify are DIFFERENT ALGORITHMS at the same n — one
// pairing per signature against one pairing total. Both are timed here so the
// C++ figure has a Go counterpart doing the same work, and so the cost of the
// choice is visible rather than inferred.

package chain

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// benchCommitteeSizes are the committee sizes every leg reports. 41 is the size
// the quorum ruling names (n=41 ⇒ 28).
var benchCommitteeSizes = []int{1, 4, 21, 41, 100}

// benchPosition is the fixed position every benchmark signs and verifies over.
// Every axis is non-zero so no field is accidentally free.
func benchPosition() VotePosition {
	fill := func(b byte) ids.ID {
		var id ids.ID
		for i := range id {
			id[i] = b
		}
		return id
	}
	return VotePosition{
		ChainID:            fill(0x11),
		Height:             0x0102030405060708,
		Round:              0x0A0B0C0D,
		BlockID:            fill(0x22),
		ParentID:           fill(0x33),
		CanonicalID:        fill(0x44),
		ParentCanonicalID:  fill(0x55),
		ExecutionStateRoot: fill(0x66),
		PayloadRoot:        fill(0x77),
		ValidatorSetRoot:   fill(0x88),
	}
}

// benchKey derives validator i's key the way the Rust bench does: the 32-byte
// IKM is be64(i+1) followed by zeros.
func benchKey(i int) *bls.SecretKey {
	var ikm [32]byte
	binary.BigEndian.PutUint64(ikm[:8], uint64(i)+1)
	sk, err := bls.SecretKeyFromSeed(ikm[:])
	if err != nil {
		panic(err)
	}
	return sk
}

// benchNode is validator i's node id: be64(i) followed by zeros, so ids are
// strictly increasing in i and the cert is already in canonical order.
func benchNode(i int) ids.NodeID {
	var n ids.NodeID
	binary.BigEndian.PutUint64(n[:8], uint64(i))
	return n
}

// benchRegistry resolves a node to its DECOMPRESSED public key. Decompressing
// once at registration rather than once per verification is what the Rust
// Registry does, so the per-verify work matches.
type benchRegistry map[ids.NodeID]*bls.PublicKey

// VerifyVote implements VoteVerifier: resolve the key, parse the signature,
// verify. An unresolved node is a refusal, never a skip.
func (r benchRegistry) VerifyVote(node ids.NodeID, msg []byte, sig []byte, _ uint64) bool {
	pk, ok := r[node]
	if !ok {
		return false
	}
	s, err := bls.SignatureFromBytes(sig)
	if err != nil {
		return false
	}
	return bls.Verify(pk, s, msg)
}

// benchCommittee builds n validators, the message they all sign, their
// signatures, and a cert carrying all n votes.
func benchCommittee(tb testing.TB, n int) (
	pos VotePosition,
	msg []byte,
	reg benchRegistry,
	sigs []*bls.Signature,
	pks []*bls.PublicKey,
	cert *QuorumCert,
) {
	tb.Helper()
	pos = benchPosition()
	msg = CanonicalVoteMessage(pos)
	reg = make(benchRegistry, n)
	votes := make([]SignedVote, 0, n)

	for i := 0; i < n; i++ {
		sk := benchKey(i)
		pk := sk.PublicKey()
		sig, err := sk.Sign(msg)
		if err != nil {
			tb.Fatalf("sign %d: %v", i, err)
		}
		node := benchNode(i)
		reg[node] = pk
		pks = append(pks, pk)
		sigs = append(sigs, sig)
		votes = append(votes, SignedVote{
			NodeID:    node,
			Accept:    true,
			Signature: bls.SignatureToBytes(sig),
		})
	}

	cert = &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Tier:      Quasar,
		Position:  pos,
		Threshold: uint32(n),
		Votes:     votes,
	}
	// A benchmark of a predicate that refuses measures the refusal path.
	if err := cert.Verify(reg, 0); err != nil {
		tb.Fatalf("committee of %d does not verify: %v", n, err)
	}
	return pos, msg, reg, sigs, pks, cert
}

func BenchmarkTriLangVoteMessage(b *testing.B) {
	pos := benchPosition()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = CanonicalVoteMessage(pos)
	}
}

func BenchmarkTriLangSign(b *testing.B) {
	_, msg, _, _, _, _ := benchCommittee(b, 1)
	sk := benchKey(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := sk.Sign(msg)
		if err != nil {
			b.Fatal(err)
		}
		sigSink = s
	}
}

func BenchmarkTriLangVerifyOne(b *testing.B) {
	_, msg, reg, _, _, cert := benchCommittee(b, 1)
	v := cert.Votes[0]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !reg.VerifyVote(v.NodeID, msg, v.Signature, 0) {
			b.Fatal("verify failed")
		}
	}
}

// BenchmarkTriLangCertVerify is the finality predicate itself: clauses 1-7 with
// one pairing per vote.
func BenchmarkTriLangCertVerify(b *testing.B) {
	for _, n := range benchCommitteeSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			_, _, reg, _, _, cert := benchCommittee(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := cert.Verify(reg, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkTriLangAggregate sums n signatures into one.
func BenchmarkTriLangAggregate(b *testing.B) {
	for _, n := range benchCommitteeSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			_, _, _, sigs, _, _ := benchCommittee(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				agg, err := bls.AggregateSignatures(sigs)
				if err != nil {
					b.Fatal(err)
				}
				sigSink = agg
			}
		})
	}
}

// BenchmarkTriLangFastAggVerify is the O(1) predicate: sum the n public keys,
// verify the aggregate signature once. This is what C++ verify_cert does, and
// it is the number the C++ figure is comparable to — not CertVerify.
func BenchmarkTriLangFastAggVerify(b *testing.B) {
	for _, n := range benchCommitteeSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			_, msg, _, sigs, pks, _ := benchCommittee(b, n)
			agg, err := bls.AggregateSignatures(sigs)
			if err != nil {
				b.Fatal(err)
			}
			aggPK, err := bls.AggregatePublicKeys(pks)
			if err != nil {
				b.Fatal(err)
			}
			if !bls.Verify(aggPK, agg, msg) {
				b.Fatal("aggregate does not verify")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a, err := bls.AggregatePublicKeys(pks)
				if err != nil {
					b.Fatal(err)
				}
				if !bls.Verify(a, agg, msg) {
					b.Fatal("verify failed")
				}
			}
		})
	}
}

// sink / sigSink keep the compiler from removing the work being timed.
var (
	sink    []byte
	sigSink *bls.Signature
)
