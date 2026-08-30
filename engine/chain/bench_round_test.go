// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bench_round_test.go — the rows that make the three-language table comparable,
// and the round nobody was timing.
//
// bench_trilang_test.go times what a Go node does. That is the right number for
// Go and the wrong number to divide by a C++ or Rust one, because the three
// implementations were handing their aggregators and verifiers DIFFERENT WORK
// under the same names:
//
//   aggregate   Rust summed decompressed points with the group check off, Go
//               summed decompressed points with it ON, and C++ summed
//               COMPRESSED bytes and paid to decompress each one. Sixtyfold
//               apart, with no language anywhere in it.
//   verify one  Go decompresses the signature and group-checks it TWICE —
//               SignatureFromBytes calls SigValidate, then Verify passes
//               sigGroupcheck=true and blst checks it again. Rust checks the
//               signature once and re-validates the PUBLIC key it already
//               validated at registration. C++ checks each once.
//
// So each operation is timed twice: AS SHIPPED, which is what a Go node really
// pays, and MATCHED, which is the same blst calls in the same order as the
// other two legs. The first is the honest cost; the second is the comparable
// one. Where they differ, the difference is a policy this implementation chose,
// and naming it is the point.
//
// A ROUND is also timed here, which no leg was doing. A round is not a
// predicate — it is the work of turning a position into an admitted
// certificate — and it is split by WHO PAYS, because the three parties pay
// different amounts and only one of them is on the critical path:
//
//   sign     one validator's own cost: build the message, sign it once.
//            Independent of n. A validator does not sign n times.
//   collect  the assembling node: canonical order and the wire. O(n), no curve.
//   admit    a follower: decode the gossiped bytes and run the predicate.
//            O(n) pairings — the finality path itself.

package chain

import (
	"fmt"
	"testing"

	blst "github.com/supranational/blst/bindings/go"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// dstSignature is the consensus vote domain, spelled here for the blst calls
// that take it raw. It is bls.Sign's tag; the conformance corpus pins that a
// signature made under it is the signature Go produces.
var dstSignature = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_")

// benchPoints decompresses a committee's signatures and public keys once, so a
// benchmark of the SUM is a benchmark of the sum and not of the parsing.
func benchPoints(tb testing.TB, n int) (msg []byte, pks []*blst.P1Affine, sigs []*blst.P2Affine) {
	tb.Helper()
	_, msg, _, s, p, _ := benchCommittee(tb, n)
	for i := 0; i < n; i++ {
		var pk blst.P1Affine
		if pk.Uncompress(bls.PublicKeyToCompressedBytes(p[i])) == nil {
			tb.Fatalf("pubkey %d does not uncompress", i)
		}
		var sig blst.P2Affine
		if sig.Uncompress(bls.SignatureToBytes(s[i])) == nil {
			tb.Fatalf("signature %d does not uncompress", i)
		}
		pks = append(pks, &pk)
		sigs = append(sigs, &sig)
	}
	return msg, pks, sigs
}

// BenchmarkTriLangEmpty is the harness floor: what an iteration costs when the
// body does nothing. The nanosecond-scale rows are only readable against it.
func BenchmarkTriLangEmpty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkSink++
	}
}

// ── verify, both ways ───────────────────────────────────────────────────────

// BenchmarkTriLangVerifyMatched is one verification doing exactly what the C++
// and Rust legs do: decompress the signature, group-check it ONCE, and pair.
// The public key is already decompressed and was validated when the validator
// joined the set. The gap to BenchmarkTriLangVerifyOne is the second group
// check Go's shipped path pays on every vote.
func BenchmarkTriLangVerifyMatched(b *testing.B) {
	msg, pks, sigs := benchPoints(b, 1)
	raw := sigs[0].Compress()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sig blst.P2Affine
		if sig.Uncompress(raw) == nil {
			b.Fatal("uncompress")
		}
		if !sig.SigValidate(true) {
			b.Fatal("group check")
		}
		if !sig.Verify(false, pks[0], false, msg, dstSignature) {
			b.Fatal("verify failed")
		}
	}
}

// BenchmarkTriLangCoreVerifyOnly is the pairing alone: hash-to-curve, two
// Miller loops, one final exponentiation, with both points already decompressed
// and both group checks already paid. The floor every leg's verify sits on.
func BenchmarkTriLangCoreVerifyOnly(b *testing.B) {
	msg, pks, sigs := benchPoints(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !sigs[0].Verify(false, pks[0], false, msg, dstSignature) {
			b.Fatal("verify failed")
		}
	}
}

// BenchmarkTriLangGroupCheckSig prices the check Go pays twice.
func BenchmarkTriLangGroupCheckSig(b *testing.B) {
	_, _, sigs := benchPoints(b, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !sigs[0].SigValidate(true) {
			b.Fatal("group check")
		}
	}
}

// ── aggregation, by the work it does ────────────────────────────────────────

// BenchmarkTriLangAggregateFromPoints is the sum ALONE, over points already
// decompressed and already checked — the same work Rust's aggregate(refs,false)
// and the C++ AggregateFromPoints row do.
func BenchmarkTriLangAggregateFromPoints(b *testing.B) {
	for _, n := range benchCommitteeSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			_, _, sigs := benchPoints(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var agg blst.P2Aggregate
				agg.Aggregate(sigs, false)
				benchmarkSink++
			}
		})
	}
}

// BenchmarkTriLangFastAggVerifyFromPoints sums n public keys already held
// decompressed and pairs once — the C++ FastAggVerifyFromPoints row's work.
func BenchmarkTriLangFastAggVerifyFromPoints(b *testing.B) {
	for _, n := range benchCommitteeSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			msg, pks, sigs := benchPoints(b, n)
			var aggSig blst.P2Aggregate
			aggSig.Aggregate(sigs, false)
			sum := aggSig.ToAffine()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var aggPK blst.P1Aggregate
				aggPK.Aggregate(pks, false)
				if !sum.Verify(false, aggPK.ToAffine(), false, msg, dstSignature) {
					b.Fatal("aggregate verify failed")
				}
			}
		})
	}
}

// ── the wire ────────────────────────────────────────────────────────────────

func BenchmarkTriLangCertEncode(b *testing.B) {
	for _, n := range []int{1, 21, 100} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			_, _, _, _, _, cert := benchCommittee(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				wire, err := cert.MarshalBinary()
				if err != nil {
					b.Fatal(err)
				}
				sink = wire
			}
		})
	}
}

func BenchmarkTriLangCertDecode(b *testing.B) {
	for _, n := range []int{1, 21, 100} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			_, _, _, _, _, cert := benchCommittee(b, n)
			wire, err := cert.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, err := UnmarshalQuorumCert(wire)
				if err != nil {
					b.Fatal(err)
				}
				certSink = got
			}
		})
	}
}

// ── a finality round ────────────────────────────────────────────────────────

// BenchmarkTriLangRoundSign is one validator's own cost for a round: build the
// canonical message from the position and sign it. Independent of n.
func BenchmarkTriLangRoundSign(b *testing.B) {
	pos := benchPosition()
	sk := benchKey(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := sk.Sign(CanonicalVoteMessage(pos))
		if err != nil {
			b.Fatal(err)
		}
		sigSink = s
	}
}

// BenchmarkTriLangRoundCollect is the assembling node's cost: put the votes in
// canonical order and put the certificate on the wire. O(n) and no curve
// arithmetic at all. The votes are reversed first so the sort does work rather
// than confirm it — they arrive in whatever order they were gossiped.
func BenchmarkTriLangRoundCollect(b *testing.B) {
	for _, n := range []int{4, 21, 41, 100} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			pos, _, _, _, _, cert := benchCommittee(b, n)
			shuffled := make([]SignedVote, n)
			for i := range cert.Votes {
				shuffled[n-1-i] = cert.Votes[i]
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c, err := AssembleQuorumCert(pos, Quasar, uint32(n), shuffled)
				if err != nil {
					b.Fatal(err)
				}
				wire, err := c.MarshalBinary()
				if err != nil {
					b.Fatal(err)
				}
				sink = wire
			}
		})
	}
}

// BenchmarkTriLangRoundAdmit is a follower's cost: decode the gossiped bytes
// and run the finality predicate. O(n) pairings — the critical path.
func BenchmarkTriLangRoundAdmit(b *testing.B) {
	for _, n := range []int{4, 21, 41, 100} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			_, _, reg, _, _, cert := benchCommittee(b, n)
			wire, err := cert.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, err := UnmarshalQuorumCert(wire)
				if err != nil {
					b.Fatal(err)
				}
				if err := got.Verify(reg, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// benchmarkSink / certSink keep the compiler from removing the work being timed.
var (
	benchmarkSink uint64
	certSink      *QuorumCert
	_             = ids.Empty
)
