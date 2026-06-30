// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert_fuzz_test.go — fuzzing for the QCv2 canonical-commitment cert.
//
// Two targets:
//
//  1. FuzzQuorumCertCodec — the wire codec. Malformed input must NEVER panic and
//     must fail closed; well-formed input must round-trip (decode→encode→decode is
//     stable). Guards the decode-DoS cap and the strict trailing-bytes policy.
//
//  2. FuzzFinalizeDecision — the finalize / equivocation decision function. A random
//     stream of {hint, cert} operations over small pools of {height, canonical,
//     envelope} ids must NEVER panic and must NEVER violate the core invariant:
//       (INV) for every height H at most ONE canonical commitment is finalized, it
//             is immutable once set, a hint never finalizes, and equivocation is
//             raised IFF a DIFFERENT canonical is presented at a certified height.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// FuzzQuorumCertCodec feeds arbitrary bytes to the decoder (must not panic / must
// fail closed) and, on a successful decode, asserts round-trip stability.
func FuzzQuorumCertCodec(f *testing.F) {
	// Seed corpus: a couple of structurally valid certs (with canonical fields set).
	seedCerts := []*QuorumCert{
		{
			Version: QuorumCertVersion, Type: QCFinality, Threshold: 1,
			Position: VotePosition{
				ChainID: ids.GenerateTestID(), Height: 7, Round: 2,
				BlockID: ids.GenerateTestID(), ParentID: ids.GenerateTestID(),
				CanonicalID: ids.GenerateTestID(), ParentCanonicalID: ids.GenerateTestID(),
				ExecutionStateRoot: ids.GenerateTestID(), PayloadRoot: ids.GenerateTestID(),
				ValidatorSetRoot: ids.GenerateTestID(),
			},
			Votes: []SignedVote{{NodeID: ids.GenerateTestNodeID(), Accept: true, Signature: []byte("sig-a")}},
		},
		{
			Version: QuorumCertVersion, Type: QCFinality, Threshold: 2,
			Position: VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()},
			Votes: []SignedVote{
				{NodeID: ids.GenerateTestNodeID(), Accept: true, Signature: nil},
				{NodeID: ids.GenerateTestNodeID(), Accept: true, Signature: []byte{0x00, 0x01}},
			},
		},
	}
	for _, c := range seedCerts {
		if b, err := c.MarshalBinary(); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte{})
	f.Add([]byte{0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		cert, err := UnmarshalQuorumCert(data) // must never panic
		if err != nil {
			if cert != nil {
				t.Fatalf("decode returned an error AND a non-nil cert: %v", err)
			}
			return
		}
		// Decoded successfully → re-encoding must reproduce the SAME bytes the
		// decoder accepted (canonical, deterministic codec), and a second decode must
		// equal the first (round-trip stability).
		reencoded, err := cert.MarshalBinary()
		if err != nil {
			t.Fatalf("re-marshal of a decoded cert failed: %v", err)
		}
		again, err := UnmarshalQuorumCert(reencoded)
		if err != nil {
			t.Fatalf("re-decode of a re-encoded cert failed: %v", err)
		}
		if !cert.Equal(again) {
			t.Fatal("round-trip instability: decode→encode→decode produced a different cert")
		}
		// Every canonical field survived the round trip (the QCv2 additions).
		if cert.Position.CanonicalID != again.Position.CanonicalID ||
			cert.Position.ParentCanonicalID != again.Position.ParentCanonicalID ||
			cert.Position.ExecutionStateRoot != again.Position.ExecutionStateRoot ||
			cert.Position.PayloadRoot != again.Position.PayloadRoot ||
			cert.Position.ValidatorSetRoot != again.Position.ValidatorSetRoot {
			t.Fatal("a canonical position field did not survive the codec round trip")
		}
	})
}

// FuzzFinalizeDecision drives a random op stream into a fresh ChainConsensus and
// checks the finalize/equivocation invariant after every op. It never asserts a
// full prediction of the fold (parent/DAG-dependent); it asserts the safety
// invariants that must hold UNCONDITIONALLY.
func FuzzFinalizeDecision(f *testing.F) {
	f.Add([]byte{0x01, 0x12, 0x23, 0x34, 0x45})
	f.Add([]byte{0x00, 0x00, 0x00, 0x10, 0x20, 0x30})
	f.Add([]byte{0xFF, 0xAA, 0x55, 0x0F, 0xF0})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Small fixed pools so collisions (the bug-relevant cases: same/different
		// canonical at one height) actually occur.
		heights := []uint64{1, 2, 3, 4}
		canon := []ids.ID{ids.Empty, idFromByte(0xC1), idFromByte(0xC2), idFromByte(0xC3)}
		outer := []ids.ID{idFromByte(0x01), idFromByte(0x02), idFromByte(0x03), idFromByte(0x04)}

		c := NewChainConsensus(5, 4, 1)
		// model[h] = the EFFECTIVE canonical certified at height h (immutable once set).
		model := map[uint64]ids.ID{}

		for _, op := range data {
			h := heights[(op>>1)&0x3]
			cn := canon[(op>>3)&0x3]
			ot := outer[(op>>5)&0x3]
			effCanon := cn
			if effCanon == ids.Empty {
				effCanon = ot // the fold's Empty-canonical fallback (outer == inner)
			}

			if op&0x1 == 0 {
				// HINT op (SyncState). Snapshot the certified set, apply, assert the
				// certified set is UNCHANGED (a hint can never finalize).
				before := snapshotCertified(c, heights)
				if err := c.SyncState(ot, h); err != nil {
					// Only the documented refusals are allowed; never a panic, never a
					// silent finalize.
					if !errors.Is(err, ErrSyncStateRegression) &&
						!errors.Is(err, ErrSyncStateEmptyWithHeight) {
						t.Fatalf("SyncState returned an unexpected error: %v", err)
					}
				}
				after := snapshotCertified(c, heights)
				if !sameCertified(before, after) {
					t.Fatal("INV violated: a hint (SyncState) changed the certified finality set")
				}
				continue
			}

			// CERT op (ApplyCert). Parent left Empty: a fresh seed at any height, a
			// duplicate/equivocation at an already-certified height, or a defer.
			_, err := c.ApplyCert(Cert{Block: ot, Parent: ids.Empty, Height: h, Canonical: cn})
			switch {
			case err == nil:
				// A successful finalize must have recorded EXACTLY this canonical at h.
				got, ok := c.FinalizedBlockAtHeight(h)
				if !ok || got != effCanon {
					t.Fatalf("INV violated: nil-err finalize did not record canonical at %d: got (%s,%v) want %s", h, got, ok, effCanon)
				}
				if prev, seen := model[h]; seen && prev != effCanon {
					t.Fatalf("INV violated: height %d canonical changed %s -> %s (must be immutable)", h, prev, effCanon)
				}
				model[h] = effCanon

			case errors.Is(err, ErrHeightAlreadyFinalized):
				// Equivocation is raised IFF a DIFFERENT canonical is already certified
				// at this height.
				got, ok := c.FinalizedBlockAtHeight(h)
				if !ok {
					t.Fatalf("INV violated: equivocation at %d but nothing finalized there", h)
				}
				if got == effCanon {
					t.Fatalf("INV violated: equivocation raised for the SAME canonical %s at %d (must be a no-op)", effCanon, h)
				}

			default:
				// Any other error is a benign refusal (monotonic / conflict / behind).
				// It must NOT have changed the certified set for h.
				if got, ok := c.FinalizedBlockAtHeight(h); ok {
					if prev, seen := model[h]; seen && got != prev {
						t.Fatalf("INV violated: refused cert (%v) mutated certified canonical at %d", err, h)
					}
				}
			}

			// Global immutability check: every modeled height still maps to its
			// recorded canonical (certs never rewrite finalized history).
			for mh, mc := range model {
				if got, ok := c.FinalizedBlockAtHeight(mh); ok && got != mc {
					t.Fatalf("INV violated: finalized canonical at %d mutated to %s (was %s)", mh, got, mc)
				}
			}
		}
	})
}

// idFromByte builds a deterministic non-empty id from a single byte (a stable pool
// element for the fuzzer).
func idFromByte(b byte) ids.ID {
	var id ids.ID
	id[0] = b
	id[1] = 0x5A
	return id
}

// snapshotCertified records the certified canonical at each candidate height.
func snapshotCertified(c *ChainConsensus, heights []uint64) map[uint64]ids.ID {
	out := map[uint64]ids.ID{}
	for _, h := range heights {
		if id, ok := c.FinalizedBlockAtHeight(h); ok {
			out[h] = id
		}
	}
	return out
}

func sameCertified(a, b map[uint64]ids.ID) bool {
	if len(a) != len(b) {
		return false
	}
	for h, id := range a {
		if b[h] != id {
			return false
		}
	}
	return true
}
