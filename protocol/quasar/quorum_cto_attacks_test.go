// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_cto_attacks_test.go — the supplemental adversarial corpus the
// weighted-quorum design must withstand beyond the F1-F5 envelope findings.
// Each test reproduces a specific attack from the consensus-threshold-cert
// directive and asserts a CLEAN rejection (typed error, never a panic, never
// unbounded work). These close the gaps the F-series did not name explicitly:
//
//	weight overflow            Σ voting weight wrapping uint64
//	wrong validator_set_root   a cert whose signers are committed under a
//	                           different epoch set than its envelope pins
//	malformed cert / no panic  a fuzz + table sweep over arbitrary wire bytes
//
// All are threshold-CERTIFICATION attacks: the adversary controls cert bytes
// but cannot break a stock FIPS 204/205 verifier or the weighted-Merkle
// commitment, and the verifier owns message construction and the policy axes.
package quasar

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/luxfi/crypto/mldsa"
)

// ----------------------------------------------------------------------------
// Weight overflow — Σ voting weight must never wrap.
// ----------------------------------------------------------------------------

// TestCTO_WeightOverflowRejected drives a cert whose per-signer weights, summed
// in order, overflow uint64. The verifier's checked accumulation must reject at
// the overflow guard (ErrQCWeightOverflow) rather than wrapping to a small
// total that silently satisfies (or silently fails) the threshold. A wrapped
// accumulator is a finality-forgery primitive: an attacker who could make Σ
// wrap to exactly the threshold would forge a quorum out of phantom weight.
//
// BuildWeightedQuorumCert ALSO guards overflow at assembly, so to exercise the
// VERIFIER's own guard we build a valid two-signer cert with modest weights,
// then mutate the SECOND record's in-cert weight (and the aggregate) to values
// whose running sum wraps — the per-signer Merkle leaf still authenticates
// because we rebuild the committed set with the mutated weight, and the
// commitment is recomputed, so the rejection is provably the overflow clause.
func TestCTO_WeightOverflowRejected(t *testing.T) {
	// First signer carries MaxUint64; second carries a positive weight, so the
	// running sum overflows on the SECOND accumulation. Commit the set with
	// exactly these weights so each leaf authenticates. Kept as runtime vars so
	// the wrapping sum is computed at runtime (a constant w1+w2 would be a
	// compile-time overflow error).
	var w1 uint64 = math.MaxUint64
	var w2 uint64 = 100

	s1 := newMLDSASigner(t, 0x01, w1)
	s2 := newMLDSASigner(t, 0x02, w2)

	// Build the committed set + message + records by hand (buildScenario would
	// route through BuildWeightedQuorumCert's assembly overflow guard).
	env := testEnv()
	var vh [32]byte
	for i := range vh {
		vh[i] = 0xAB
	}
	env.ValueHash = vh
	// Threshold below the wrapped value so the floor/threshold clauses are not
	// what trips — isolate the overflow guard. Σ would be w1+w2 (overflow); the
	// honest semantics is "huge", so any small threshold is met IF no overflow.
	env.QuorumThreshold = 50

	leaves := []WeightedValidatorLeaf{
		{ValidatorID: s1.id, PublicKey: s1.pubBytes, VotingWeight: w1, ParameterSetID: uint8(s1.scheme), KeyVersion: s1.keyVer},
		{ValidatorID: s2.id, PublicKey: s2.pubBytes, VotingWeight: w2, ParameterSetID: uint8(s2.scheme), KeyVersion: s2.keyVer},
	}
	set, err := BuildWeightedValidatorSet(env.Epoch, leaves)
	if err != nil {
		t.Fatalf("build set: %v", err)
	}
	env.ValidatorSetRoot = set.Root()
	msg, err := QuorumConsensusMessage(env)
	if err != nil {
		t.Fatalf("build message: %v", err)
	}
	sorted := set.Leaves()
	idxOf := func(id [32]byte) int {
		for i := range sorted {
			if sorted[i].ValidatorID == id {
				return i
			}
		}
		t.Fatal("id not in set")
		return -1
	}
	records := make([]QuorumSignerRecord, 0, 2)
	for _, s := range []*testSigner{s1, s2} {
		proof, err := set.InclusionProof(idxOf(s.id))
		if err != nil {
			t.Fatalf("inclusion proof: %v", err)
		}
		records = append(records, QuorumSignerRecord{
			ValidatorID:  s.id,
			PublicKey:    s.pubBytes,
			VotingWeight: s.weight,
			Scheme:       s.scheme,
			ParamSetID:   uint8(s.scheme),
			KeyVersion:   s.keyVer,
			MerklePath:   proof,
			Signature:    s.sign(t, msg, nil),
		})
	}

	// Assemble the cert WITHOUT the prover's overflow guard (hand-build the
	// struct). AggregateWeight is set to the wrapped value an attacker claims.
	cert := &WeightedQuorumCert{
		Version:          QuorumCertVersion,
		ChainID:          env.ChainID,
		Epoch:            env.Epoch,
		Height:           env.Height,
		Round:            env.Round,
		ValueHash:        env.ValueHash,
		QCType:           env.QCType,
		ValidatorSetRoot: set.Root(),
		QuorumThreshold:  env.QuorumThreshold,
		AggregateWeight:  w1 + w2, // wraps at runtime; the value the attacker claims
		SignerCount:      2,
		Signers:          records,
	}
	cert.SignerCommitment = cert.computeSignerCommitment()

	cfg := QuorumVerifierConfig{MinThreshold: env.QuorumThreshold}
	err = cert.Verify(env, cfg)
	if !errors.Is(err, ErrQCWeightOverflow) {
		t.Fatalf("overflowing weight sum err = %v, want ErrQCWeightOverflow", err)
	}
}

// TestCTO_ProverRejectsWeightOverflow proves the prover (assembly) ALSO
// refuses to build a cert whose weights overflow — defence in depth so a
// malformed overflowing cert is never even produced by an honest relayer.
func TestCTO_ProverRejectsWeightOverflow(t *testing.T) {
	s1 := newMLDSASigner(t, 0x01, math.MaxUint64)
	s2 := newMLDSASigner(t, 0x02, 1)
	rec := func(s *testSigner) QuorumSignerRecord {
		return QuorumSignerRecord{
			ValidatorID:  s.id,
			PublicKey:    s.pubBytes,
			VotingWeight: s.weight,
			Scheme:       s.scheme,
			ParamSetID:   uint8(s.scheme),
			KeyVersion:   s.keyVer,
			MerklePath:   &WeightedInclusionProof{LeafCount: 2},
			Signature:    []byte{0x00},
		}
	}
	_, err := BuildWeightedQuorumCert(QuorumCertParams{
		ChainID: 1, Epoch: 1, QuorumThreshold: 1, ValidatorSetRoot: [48]byte{1},
	}, []QuorumSignerRecord{rec(s1), rec(s2)})
	if !errors.Is(err, ErrQCWeightOverflow) {
		t.Fatalf("prover overflow err = %v, want ErrQCWeightOverflow", err)
	}
}

// ----------------------------------------------------------------------------
// Wrong validator_set_root — signers committed under a different set.
// ----------------------------------------------------------------------------

// TestCTO_WrongValidatorSetRootRejected reproduces a cross-set attack: a cert
// whose signers were committed under set A is re-presented with its
// ValidatorSetRoot swapped to an unrelated set B's root. Because the root is
// (a) bound into the signed message (so the original signatures no longer
// verify) AND (b) the per-signer Merkle paths were authenticated against set
// A's root, the swapped root cannot reproduce inclusion. The verifier must
// reject — a quorum certified over one validator set may not be laundered as a
// quorum over a different set. We recompute the signer commitment so the
// rejection is provably the Merkle clause / signature binding, not the
// commitment field.
func TestCTO_WrongValidatorSetRootRejected(t *testing.T) {
	sc := fourMLDSA(t)

	// An unrelated validator set B (different ids/keys) → a different root.
	other := []*testSigner{
		newMLDSASigner(t, 0xA1, 25),
		newMLDSASigner(t, 0xA2, 25),
		newMLDSASigner(t, 0xA3, 25),
		newMLDSASigner(t, 0xA4, 25),
	}
	leavesB := make([]WeightedValidatorLeaf, len(other))
	for i, s := range other {
		leavesB[i] = WeightedValidatorLeaf{
			ValidatorID:    s.id,
			PublicKey:      s.pubBytes,
			VotingWeight:   s.weight,
			ParameterSetID: uint8(s.scheme),
			KeyVersion:     s.keyVer,
		}
	}
	setB, err := BuildWeightedValidatorSet(sc.env.Epoch, leavesB)
	if err != nil {
		t.Fatalf("build set B: %v", err)
	}
	if setB.Root() == sc.cert.ValidatorSetRoot {
		t.Fatal("precondition: set B root collides with set A root")
	}

	// Swap the cert's committed root to set B's root and recompute the
	// commitment (which binds the root) so the commitment clause passes and the
	// failure is the per-signer Merkle inclusion / rebuilt-message signature.
	sc.cert.ValidatorSetRoot = setB.Root()
	sc.cert.SignerCommitment = sc.cert.computeSignerCommitment()

	err = sc.cert.Verify(sc.env, sc.cfg)
	// The verifier rebuilds the message with the swapped root (signatures fail)
	// AND the original set-A Merkle paths cannot authenticate against set B's
	// root — either clean clause rejection is acceptable; both are typed.
	if !errors.Is(err, ErrQCMerkleInclusion) && !errors.Is(err, ErrQCSigInvalid) {
		t.Fatalf("wrong validator_set_root err = %v, want Merkle or signature rejection", err)
	}
}

// ----------------------------------------------------------------------------
// Malformed cert bytes — typed error, NEVER a panic / unbounded work.
// ----------------------------------------------------------------------------

// FuzzUnmarshalWeightedQuorumCert asserts the decoder never panics on
// arbitrary input and, when it does decode, the result re-encodes to bytes the
// decoder accepts again (idempotent on the accepted-language). The seed corpus
// includes a real full cert, a real compact cert, and pathological headers.
func FuzzUnmarshalWeightedQuorumCert(f *testing.F) {
	// Seeds: a real full + compact cert, plus adversarial fragments.
	full := buildFuzzSeedCert(f)
	if wire, err := full.MarshalBinary(); err == nil {
		f.Add(wire)
	}
	if wire, err := full.Compact().MarshalBinary(); err == nil {
		f.Add(wire)
	}
	f.Add([]byte{})                   // empty
	f.Add([]byte{wqcKindFull})        // kind only, truncated
	f.Add([]byte{wqcKindCompact})     // compact kind only, truncated
	f.Add([]byte{0x99})               // bad kind
	// A full header claiming an enormous signer_count with no records.
	hdr := make([]byte, wqcHeaderSize)
	hdr[0] = wqcKindFull
	binary.BigEndian.PutUint32(hdr[wqcHeaderSize-4:], 0xFFFFFFFF)
	f.Add(hdr)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic, never hang. A decode either errors or yields a cert.
		cert, err := UnmarshalWeightedQuorumCert(data)
		if err != nil {
			if cert != nil {
				t.Fatalf("decoder returned a cert alongside error %v", err)
			}
			return
		}
		// Accepted: re-encoding must succeed and round-trip (the decoder accepts
		// a canonical sub-language; a cert it produced must re-encode to bytes it
		// accepts and that Equal the cert).
		wire, merr := cert.MarshalBinary()
		if merr != nil {
			t.Fatalf("re-marshal of accepted cert failed: %v", merr)
		}
		again, aerr := UnmarshalWeightedQuorumCert(wire)
		if aerr != nil {
			t.Fatalf("re-decode of re-marshaled accepted cert failed: %v", aerr)
		}
		if !cert.Equal(again) {
			t.Fatal("decode∘encode∘decode not stable on an accepted cert")
		}
	})
}

// TestCTO_MalformedNeverPanics is the deterministic companion to the fuzzer: a
// table of hostile byte strings that each must yield a typed error and never a
// panic. Runs in normal `go test` (no -fuzz needed) so CI always exercises it.
func TestCTO_MalformedNeverPanics(t *testing.T) {
	full := buildFuzzSeedCert(t)
	good, err := full.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}

	cases := map[string][]byte{
		"nil":                    nil,
		"empty":                  {},
		"kind_full_only":         {wqcKindFull},
		"kind_compact_only":      {wqcKindCompact},
		"bad_kind":               {0x99, 0x00, 0x01},
		"single_zero":            {0x00},
		"header_minus_one":       good[:wqcHeaderSize-1],
		"good_plus_trailer":      append(append([]byte(nil), good...), 0xDE, 0xAD),
		"all_0xFF_short":         bytesRepeat(0xFF, 16),
		"all_0xFF_headerlen":     bytesRepeat(0xFF, wqcHeaderSize),
		"all_0x00_headerlen":     bytesRepeat(0x00, wqcHeaderSize),
	}
	// Plus every truncation prefix of a valid full cert.
	for n := 0; n < len(good); n++ {
		cases[truncName(n)] = good[:n]
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			// The only requirement: no panic. We do not assert error-ness for the
			// full-length valid prefix (n == len(good)) since that is the cert
			// itself; every shorter/garbled input must error, and none may panic.
			cert, err := UnmarshalWeightedQuorumCert(data)
			if err == nil && !bytesEq(data, good) {
				// Accepted something other than the exact valid cert — only OK if
				// it genuinely round-trips (the decoder's accepted language).
				wire, merr := cert.MarshalBinary()
				if merr != nil || !bytesEq(wire, data) {
					// A non-canonical accept would be a malleability bug; surface it.
					// (The strict trailing-byte policy should prevent this.)
					if !bytesEq(data, good) {
						t.Fatalf("accepted non-canonical input %q without round-trip", name)
					}
				}
			}
		})
	}
}

// ----------------------------------------------------------------------------
// local helpers (kept here so this file is self-contained alongside the
// shared scenario harness in quorum_cert_test.go)
// ----------------------------------------------------------------------------

// buildFuzzSeedCert builds a small real cert usable as a fuzz/table seed. It
// takes testing.TB so both Fuzz (*testing.F) and Test (*testing.T) can use it.
// It builds two real ML-DSA-65 signers inline (the shared scenario harness in
// quorum_cert_test.go is *testing.T-only and cannot serve the fuzzer).
func buildFuzzSeedCert(tb testing.TB) *WeightedQuorumCert {
	tb.Helper()

	type kp struct {
		id   [32]byte
		priv *mldsa.PrivateKey
		pub  []byte
	}
	mk := func(idByte byte) kp {
		priv, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
		if err != nil {
			tb.Fatalf("seed keygen: %v", err)
		}
		var id [32]byte
		for i := range id {
			id[i] = idByte
		}
		return kp{id: id, priv: priv, pub: priv.Public().(*mldsa.PublicKey).Bytes()}
	}
	k1, k2 := mk(0x01), mk(0x02)

	env := testEnv()
	var vh [32]byte
	for i := range vh {
		vh[i] = 0xAB
	}
	env.ValueHash = vh
	env.QuorumThreshold = 100

	leaves := []WeightedValidatorLeaf{
		{ValidatorID: k1.id, PublicKey: k1.pub, VotingWeight: 50, ParameterSetID: uint8(QuorumSchemeMLDSA65), KeyVersion: 1},
		{ValidatorID: k2.id, PublicKey: k2.pub, VotingWeight: 50, ParameterSetID: uint8(QuorumSchemeMLDSA65), KeyVersion: 1},
	}
	set, err := BuildWeightedValidatorSet(env.Epoch, leaves)
	if err != nil {
		tb.Fatalf("seed build set: %v", err)
	}
	env.ValidatorSetRoot = set.Root()
	msg, err := QuorumConsensusMessage(env)
	if err != nil {
		tb.Fatalf("seed message: %v", err)
	}
	sorted := set.Leaves()
	idxOf := func(id [32]byte) int {
		for i := range sorted {
			if sorted[i].ValidatorID == id {
				return i
			}
		}
		tb.Fatal("seed id not in set")
		return -1
	}
	recs := make([]QuorumSignerRecord, 0, 2)
	for _, k := range []kp{k1, k2} {
		proof, err := set.InclusionProof(idxOf(k.id))
		if err != nil {
			tb.Fatalf("seed proof: %v", err)
		}
		sig, err := k.priv.SignCtx(rand.Reader, msg, nil)
		if err != nil {
			tb.Fatalf("seed sign: %v", err)
		}
		recs = append(recs, QuorumSignerRecord{
			ValidatorID:  k.id,
			PublicKey:    k.pub,
			VotingWeight: 50,
			Scheme:       QuorumSchemeMLDSA65,
			ParamSetID:   uint8(QuorumSchemeMLDSA65),
			KeyVersion:   1,
			MerklePath:   proof,
			Signature:    sig,
		})
	}
	cert, err := BuildWeightedQuorumCert(QuorumCertParams{
		ChainID:          env.ChainID,
		Epoch:            env.Epoch,
		Height:           env.Height,
		Round:            env.Round,
		ValueHash:        env.ValueHash,
		QCType:           env.QCType,
		ValidatorSetRoot: set.Root(),
		QuorumThreshold:  100,
	}, recs)
	if err != nil {
		tb.Fatalf("seed build cert: %v", err)
	}
	return cert
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func truncName(n int) string {
	return "trunc_" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
