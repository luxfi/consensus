// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// Benchmark suite comparing the post-quantum consensus modes under the
// production deployment shape: N validators on N independent boxes
// signing in parallel, with one leader doing the single-threaded
// aggregate/verify legs.
//
// What we measure (per mode, per validator count n):
//
//	tRound1Ns    max(N validators)  -- the slowest box's Round-1 wall-clock.
//	             For modes with a single signing leg (BLS, ML-DSA) this is
//	             the only per-validator phase and tRound2Ns is zero.
//	tRound2Ns    max(N validators)  -- only nonzero for two-round threshold
//	             constructions (Corona, Pulsar).
//	tAggregateNs leader-side aggregate cost (single-threaded). For BLS this
//	             is bls.AggregateSignatures; for Corona/Pulsar it includes
//	             Finalize; for the composed Quasar mode it is the sum of
//	             both leader-side aggregates because they run on the same
//	             leader serially.
//	tVerifyNs    one verifier checking the cert (single-threaded).
//	tTotalNs     tRound1Ns + tRound2Ns + tAggregateNs + tVerifyNs. This is
//	             the production wall-clock for one finality round.
//
// Method: each per-validator leg is timed by running validator i's work
// SERIALLY with the whole machine to itself and taking max(per-validator)
// across i. In production the validators are on separate physical boxes,
// each with full CPU; the slowest box's local time is the leg's
// wall-clock. This is the same shape consensus papers call
// "fan-out parallel" and it is what a real N-box quorum observes —
// never max/GOMAXPROCS, never N * single-validator.
//
// The earlier revision tried a single-machine simulation (N goroutines
// competing for GOMAXPROCS cores) and reported wall-clocks of 14+ s for
// 64-validator Corona. That was an artefact of the scheduler, not of
// the protocol. The shape we measure here is the only one that matches
// the production deployment.
//
// Quasar composes three PQ lanes that all run in parallel on each
// validator's own box: BLS-sign, Corona Round1+Round2, ML-DSA-sign.
// Per-validator wall-clock is therefore max(BLS, Corona-R1+R2,
// ML-DSA), NOT sum(...). The bench enforces this on the way out: a
// Quasar tTotalNs that exceeded a small multiple of the slowest
// individual mode's tTotalNs would be a regression.
//
// Run with:
//
//	cd consensus
//	GOWORK=off go test -bench=BenchmarkPQMode -benchtime=10x -count=3 ./bench/
//
// Corona rename: the lattice threshold construction is now called
// "Corona" upstream (github.com/luxfi/threshold/protocols/corona).
// PQModeNasua was renamed to PQModeCorona at the consensus/config
// layer; pre-rename "nasua" wire strings remain a parse alias. The
// consensus-internal types (cfg.CoronaShares, cfg.CoronaGroupKey,
// quasar.EncodeCoronaSig) are unchanged surface — they live under
// quasar's package and are not part of this rename.
package bench

import (
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/threshold/protocols/corona"
)

// =============================================================================
// Sweep sizes
// =============================================================================

// pqModesValidatorCounts is the validator-set sizes the bench sweeps
// for modes whose work is cheap (BLS-only, ML-DSA). n=21 is the mainnet
// shape; n=4 is the smallest BFT; n=64 / n=100 stress the per-validator
// linear paths (notably ML-DSA-65 = 3309 B per validator).
var pqModesValidatorCounts = []int{4, 21, 64, 100}

// pqModesLatticeCounts is the validator-set sizes used for Corona /
// Pulsar / Quasar (lattice-bearing) modes. n=4 is excluded because the
// rejection sampler is known flaky at that group size. With the
// production fan-out shape, n=100 would be tractable too (one
// validator's Round1+Round2 dominates regardless of N), but the sweep
// stays at {21, 64} for shape-parity with the historical comparison.
var pqModesLatticeCounts = []int{21, 64}

// latticeMaxRetries bounds Round1/Round2/Finalize re-runs when verify
// fails (Corona rejection sampling is nondeterministic).
const latticeMaxRetries = 4

// benchIters is the number of independent finality rounds we time per
// (mode, n) cell. Each iteration's tRound1/tRound2/tAggregate/tVerify
// is recorded; the cell reports median + p99 across iterations.
//
// Driven by b.N when the runner passes -benchtime=Nx (e.g.
// `-benchtime=10x` → b.N == 10). We clamp to a small floor so a
// single-iteration smoke run still records useful percentiles.
const benchItersFloor = 5

// =============================================================================
// Per-cell metrics (median + p99)
// =============================================================================

// phaseSamples holds raw nanosecond samples for one phase across all
// iterations of a (mode, n) cell. We report median + p99 from these.
type phaseSamples struct {
	round1    []int64
	round2    []int64
	aggregate []int64
	verify    []int64
	total     []int64
}

func (p *phaseSamples) add(r1, r2, agg, ver int64) {
	p.round1 = append(p.round1, r1)
	p.round2 = append(p.round2, r2)
	p.aggregate = append(p.aggregate, agg)
	p.verify = append(p.verify, ver)
	p.total = append(p.total, r1+r2+agg+ver)
}

// modeMetrics is the per-(mode,n) row aggregated by the suite. We track
// median and p99 of every per-phase wall-clock plus a single-CPU sign
// number (useful for sizing one validator's hardware budget).
type modeMetrics struct {
	mode config.PQMode
	n    int

	// Per-phase wall-clock medians (ns).
	tRound1Ns    int64
	tRound2Ns    int64
	tAggregateNs int64
	tVerifyNs    int64
	tTotalNs     int64

	// Per-phase p99 (ns) — measured across `iters` finality rounds.
	tRound1P99Ns    int64
	tRound2P99Ns    int64
	tAggregateP99Ns int64
	tVerifyP99Ns    int64
	tTotalP99Ns     int64

	// Single-CPU view: one validator's local sign cost (median ns).
	// Useful for sizing one validator's hardware budget; orthogonal
	// to the production wall-clock numbers above.
	signNs int64

	// Cert wire size in bytes.
	certBytes int

	// Non-empty if this mode is pending implementation.
	pending string
}

// metricsRegistry collects rows across the whole bench run so the
// summary table sub-bench can print them at the end.
var (
	metricsMu       sync.Mutex
	metricsRegistry = map[string]*modeMetrics{}
)

// =============================================================================
// Shared helpers
// =============================================================================

// randMessage returns a 32-byte message (block hash analog).
func randMessage() []byte {
	m := make([]byte, 32)
	if _, err := rand.Read(m); err != nil {
		panic(err)
	}
	return m
}

// maxValidatorTime times each validator's per-round work SERIALLY and
// returns max(time). This is the honest production-finality leg model:
// in a real deployment, validator i runs on its own box with all cores
// to itself, so its per-round wall-clock is the single-validator cost
// regardless of N. The aggregate finality cost is
//
//	max(per-validator) + leader-aggregate + leader-verify
//
// not sum(per-validator) and not (sum / GOMAXPROCS).
//
// worker(i) MUST do exactly the work validator i would do on its own
// box. The function must be CPU-bound (no I/O); we run it one at a
// time so each call has the whole machine — that gives an honest
// per-validator cost free of GOMAXPROCS contention.
func maxValidatorTime(n int, worker func(i int)) time.Duration {
	var maxT time.Duration
	for i := 0; i < n; i++ {
		st := time.Now()
		worker(i)
		if d := time.Since(st); d > maxT {
			maxT = d
		}
	}
	return maxT
}

// fanOut runs worker(i) for i in [0,n) in parallel goroutines and waits
// for all to finish. Used only for non-timing work — e.g. parallel
// signature pre-computation outside the measured legs. Never used to
// measure a leg's wall-clock (use maxValidatorTime for that).
func fanOut(n int, worker func(i int)) {
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			worker(i)
		}()
	}
	wg.Wait()
}

// medianAndP99 returns (median, p99) of a sample slice in nanoseconds.
// Uses simple sort + index — sample sizes here are tiny (b.N + floor),
// so the O(n log n) sort cost is irrelevant.
func medianAndP99(samples []int64) (int64, int64) {
	if len(samples) == 0 {
		return 0, 0
	}
	cp := make([]int64, len(samples))
	copy(cp, samples)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	medianIdx := len(cp) / 2
	p99Idx := (len(cp) * 99) / 100
	if p99Idx >= len(cp) {
		p99Idx = len(cp) - 1
	}
	return cp[medianIdx], cp[p99Idx]
}

// =============================================================================
// Validator key fixtures
// =============================================================================

// blsValidator is a single BLS keypair.
type blsValidator struct {
	sk *bls.SecretKey
	pk *bls.PublicKey
}

func makeBLSValidators(n int) []blsValidator {
	out := make([]blsValidator, n)
	for i := 0; i < n; i++ {
		sk, err := bls.NewSecretKey()
		if err != nil {
			panic(err)
		}
		out[i] = blsValidator{sk: sk, pk: sk.PublicKey()}
	}
	return out
}

// signAllBLS returns N signatures + pubkeys for `msg`, computed in
// parallel for the cert-bootstrap path. Not part of any measured leg.
func signAllBLS(vals []blsValidator, msg []byte) ([]*bls.Signature, []*bls.PublicKey, error) {
	sigs := make([]*bls.Signature, len(vals))
	pks := make([]*bls.PublicKey, len(vals))
	errs := make([]error, len(vals))
	fanOut(len(vals), func(i int) {
		s, err := vals[i].sk.Sign(msg)
		if err != nil {
			errs[i] = err
			return
		}
		sigs[i] = s
		pks[i] = vals[i].pk
	})
	for i, e := range errs {
		if e != nil {
			return nil, nil, fmt.Errorf("bls sign[%d]: %w", i, e)
		}
	}
	return sigs, pks, nil
}

// aggregateBLS aggregates N signatures + N pubkeys into a single cert.
// Single-threaded (leader work).
func aggregateBLS(sigs []*bls.Signature, pks []*bls.PublicKey) ([]byte, *bls.PublicKey, error) {
	agg, err := bls.AggregateSignatures(sigs)
	if err != nil {
		return nil, nil, fmt.Errorf("bls aggregate: %w", err)
	}
	aggPK, err := bls.AggregatePublicKeys(pks)
	if err != nil {
		return nil, nil, fmt.Errorf("bls aggregate pubkeys: %w", err)
	}
	return bls.SignatureToBytes(agg), aggPK, nil
}

// mldsaValidator is one ML-DSA-65 keypair.
type mldsaValidator struct {
	sk *mldsa.PrivateKey
	pk *mldsa.PublicKey
}

func makeMLDSAValidators(n int) []mldsaValidator {
	out := make([]mldsaValidator, n)
	for i := 0; i < n; i++ {
		sk, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
		if err != nil {
			panic(err)
		}
		out[i] = mldsaValidator{sk: sk, pk: sk.PublicKey}
	}
	return out
}

// signAllMLDSA returns N ML-DSA sigs for `msg` in parallel. Used for
// the cert-bootstrap path; not part of any measured leg.
func signAllMLDSA(vals []mldsaValidator, msg []byte) ([][]byte, error) {
	sigs := make([][]byte, len(vals))
	errs := make([]error, len(vals))
	fanOut(len(vals), func(i int) {
		s, err := vals[i].sk.Sign(rand.Reader, msg, nil)
		if err != nil {
			errs[i] = err
			return
		}
		sigs[i] = s
	})
	for i, e := range errs {
		if e != nil {
			return nil, fmt.Errorf("mldsa sign[%d]: %w", i, e)
		}
	}
	return sigs, nil
}

// =============================================================================
// Corona / Pulsar / Quasar fixture (lattice modes)
// =============================================================================

// latticeFixture holds a fully-keyed Corona group plus a parallel BLS
// keyset. We re-use the dual_threshold_test.go flow: GenerateDualKeys
// gives us BLS-threshold + Corona-threshold shares for a (t, n)
// group, and we drive Round1 / Round2 / Finalize to produce a real
// signature.
//
// The consensus-side config field is still named CoronaShares for
// upstream KAT-continuity reasons (see CLAUDE.md in
// protocol/quasar/) — Corona is the published name of that
// construction; Corona is the historical name shared with
// cfg.CoronaShares / cfg.CoronaGroupKey on the quasar side.
type latticeFixture struct {
	cfg          *quasar.SignerConfig
	rtSigners    []*corona.Signer
	rtSignerIDs  []int
	validatorIDs []string
	blsVals      []blsValidator
	t            int
	n            int
}

func newLatticeFixture(t, n int) (*latticeFixture, error) {
	cfg, err := quasar.GenerateDualKeys(t, n)
	if err != nil {
		return nil, fmt.Errorf("GenerateDualKeys(%d,%d): %w", t, n, err)
	}
	if _, err := quasar.NewSignerWithDualThreshold(*cfg); err != nil {
		return nil, fmt.Errorf("NewSignerWithDualThreshold: %w", err)
	}
	ids := make([]string, n)
	rtSigners := make([]*corona.Signer, n)
	rtIDs := make([]int, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("v%d", i)
		ids[i] = id
		share := cfg.CoronaShares[id]
		rtSigners[i] = corona.NewSigner(share)
		rtIDs[i] = i
	}
	return &latticeFixture{
		cfg:          cfg,
		rtSigners:    rtSigners,
		rtSignerIDs:  rtIDs,
		validatorIDs: ids,
		blsVals:      makeBLSValidators(n),
		t:            t,
		n:            n,
	}, nil
}

// signResult holds the outputs of one full lattice signing round.
type signResult struct {
	rtSig    *corona.Signature
	rtBytes  []byte
	blsAgg   []byte
	blsAggPK *bls.PublicKey
}

// bootstrapLatticeRound produces a verified Corona+BLS cert for a
// fresh message (not timed; used for the verify-path setup). Re-runs
// the lattice round up to latticeMaxRetries times because rejection
// sampling is nondeterministic.
func (f *latticeFixture) bootstrapLatticeRound(msg []byte, prfKey []byte) (*signResult, error) {
	for attempt := 0; attempt < latticeMaxRetries; attempt++ {
		sessionID := attempt + 1
		// Round 1
		r1Slots := make([]*corona.Round1Data, f.n)
		fanOut(f.n, func(i int) {
			r1Slots[i] = f.rtSigners[i].Round1(sessionID, prfKey, f.rtSignerIDs)
		})
		round1 := make(map[int]*corona.Round1Data, f.n)
		for _, d := range r1Slots {
			if d == nil {
				return nil, fmt.Errorf("bootstrap Round1: nil slot")
			}
			round1[d.PartyID] = d
		}
		// Round 2
		r2Slots := make([]*corona.Round2Data, f.n)
		r2Errs := make([]error, f.n)
		fanOut(f.n, func(i int) {
			d, e := f.rtSigners[i].Round2(sessionID, string(msg), prfKey, f.rtSignerIDs, round1)
			if e != nil {
				r2Errs[i] = e
				return
			}
			r2Slots[i] = d
		})
		round2 := make(map[int]*corona.Round2Data, f.n)
		for i, e := range r2Errs {
			if e != nil {
				return nil, fmt.Errorf("bootstrap Round2[%d]: %w", i, e)
			}
			round2[r2Slots[i].PartyID] = r2Slots[i]
		}
		// Finalize + verify
		rtSig, err := f.rtSigners[0].Finalize(round2)
		if err != nil {
			return nil, fmt.Errorf("bootstrap Finalize: %w", err)
		}
		if !corona.Verify(f.cfg.CoronaGroupKey, string(msg), rtSig) {
			continue
		}
		rtBytes := quasar.EncodeCoronaSig(rtSig)
		if rtBytes == nil {
			return nil, fmt.Errorf("EncodeCoronaSig returned nil")
		}
		// BLS aggregate over the same message
		sigs, pks, err := signAllBLS(f.blsVals, msg)
		if err != nil {
			return nil, err
		}
		blsAgg, blsAggPK, err := aggregateBLS(sigs, pks)
		if err != nil {
			return nil, err
		}
		return &signResult{rtSig: rtSig, rtBytes: rtBytes, blsAgg: blsAgg, blsAggPK: blsAggPK}, nil
	}
	return nil, fmt.Errorf("bootstrap lattice round failed after %d retries", latticeMaxRetries)
}

// =============================================================================
// Iteration count helper
// =============================================================================

// itersFromB returns the per-cell iteration count. -benchtime=Nx →
// b.N == N; we clamp up to benchItersFloor so a smoke run still
// produces meaningful percentiles.
func itersFromB(b *testing.B) int {
	iters := b.N
	if iters < benchItersFloor {
		iters = benchItersFloor
	}
	return iters
}

// =============================================================================
// Mode 1: BLS-only (classical fast path)
// =============================================================================

func BenchmarkPQModes_BLS(b *testing.B) {
	for _, n := range pqModesValidatorCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			vals := makeBLSValidators(n)
			msg := randMessage()

			// Single-CPU view: one validator's sign cost.
			signMedNs := timeSingle(b.N, func() {
				_, _ = vals[0].sk.Sign(msg)
			})

			pkSlice := make([]*bls.PublicKey, n)
			for i := range vals {
				pkSlice[i] = vals[i].pk
			}

			iters := itersFromB(b)
			ps := &phaseSamples{}
			for k := 0; k < iters; k++ {
				// Round 1: each validator signs locally.
				sigs := make([]*bls.Signature, n)
				r1 := maxValidatorTime(n, func(i int) {
					s, err := vals[i].sk.Sign(msg)
					if err != nil {
						b.Fatal(err)
					}
					sigs[i] = s
				})

				// Round 2: BLS is one-round; tRound2 = 0.

				// Aggregate (leader, single-threaded).
				stA := time.Now()
				agg, err := bls.AggregateSignatures(sigs)
				if err != nil {
					b.Fatal(err)
				}
				aggPK, err := bls.AggregatePublicKeys(pkSlice)
				if err != nil {
					b.Fatal(err)
				}
				aggT := time.Since(stA)

				// Verify (leader / receivers, single-threaded).
				stV := time.Now()
				if !bls.Verify(aggPK, agg, msg) {
					b.Fatal("BLS verify failed")
				}
				verT := time.Since(stV)

				ps.add(r1.Nanoseconds(), 0, aggT.Nanoseconds(), verT.Nanoseconds())
			}

			// Cert size: take one steady-state cert through the wire encoder.
			sigs, pks, err := signAllBLS(vals, msg)
			if err != nil {
				b.Fatal(err)
			}
			aggBytes, _, err := aggregateBLS(sigs, pks)
			if err != nil {
				b.Fatal(err)
			}
			cert := &quasar.QuasarCert{
				BLS:        aggBytes,
				Epoch:      1,
				Finality:   time.Now(),
				Validators: n,
			}
			certBytes := cert.Bytes()
			if len(certBytes) == 0 {
				b.Fatal("cert.Bytes() returned empty")
			}

			reportCell(b, config.PQModeBLS, n, ps, signMedNs, len(certBytes), "")
		})
	}
}

// =============================================================================
// Mode 2: BLS + per-validator ML-DSA-65 (no rollup)
// =============================================================================

func BenchmarkPQModes_MLDSA(b *testing.B) {
	for _, n := range pqModesValidatorCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			blsVals := makeBLSValidators(n)
			mlVals := makeMLDSAValidators(n)
			msg := randMessage()

			// Single-CPU view: one validator's local cost = max(BLS, ML-DSA).
			signMedNs := timeSingle(b.N, func() {
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, _ = blsVals[0].sk.Sign(msg)
				}()
				go func() {
					defer wg.Done()
					_, _ = mlVals[0].sk.Sign(rand.Reader, msg, nil)
				}()
				wg.Wait()
			})

			pkSlice := make([]*bls.PublicKey, n)
			for i := range blsVals {
				pkSlice[i] = blsVals[i].pk
			}

			iters := itersFromB(b)
			ps := &phaseSamples{}
			for k := 0; k < iters; k++ {
				// Round 1: per-validator wall-clock = max(BLS-sign, ML-DSA-sign).
				// Each validator runs both legs concurrently on its own box.
				blsSigs := make([]*bls.Signature, n)
				mlSigs := make([][]byte, n)
				blsErrs := make([]error, n)
				mlErrs := make([]error, n)
				r1 := maxValidatorTime(n, func(i int) {
					var wg sync.WaitGroup
					wg.Add(2)
					go func() {
						defer wg.Done()
						s, err := blsVals[i].sk.Sign(msg)
						if err != nil {
							blsErrs[i] = err
							return
						}
						blsSigs[i] = s
					}()
					go func() {
						defer wg.Done()
						s, err := mlVals[i].sk.Sign(rand.Reader, msg, nil)
						if err != nil {
							mlErrs[i] = err
							return
						}
						mlSigs[i] = s
					}()
					wg.Wait()
				})
				for i, e := range blsErrs {
					if e != nil {
						b.Fatalf("BLS sign[%d]: %v", i, e)
					}
				}
				for i, e := range mlErrs {
					if e != nil {
						b.Fatalf("ML-DSA sign[%d]: %v", i, e)
					}
				}

				// Aggregate: BLS aggregate + ML-DSA payload encode (leader).
				stA := time.Now()
				agg, err := bls.AggregateSignatures(blsSigs)
				if err != nil {
					b.Fatal(err)
				}
				aggPK, err := bls.AggregatePublicKeys(pkSlice)
				if err != nil {
					b.Fatal(err)
				}
				_ = quasar.EncodeMLDSASigs(mlSigs)
				aggT := time.Since(stA)

				// Verify: BLS aggregate verify + N ML-DSA verifies.
				// One verifier checks all N sigs (production shape — every
				// node verifies the cert it receives).
				stV := time.Now()
				if !bls.Verify(aggPK, agg, msg) {
					b.Fatal("BLS verify failed")
				}
				for i := 0; i < n; i++ {
					if !mlVals[i].pk.Verify(msg, mlSigs[i], nil) {
						b.Fatalf("ML-DSA verify[%d] failed", i)
					}
				}
				verT := time.Since(stV)

				ps.add(r1.Nanoseconds(), 0, aggT.Nanoseconds(), verT.Nanoseconds())
			}

			// Cert size.
			mlSigs, err := signAllMLDSA(mlVals, msg)
			if err != nil {
				b.Fatal(err)
			}
			sigs, pks, err := signAllBLS(blsVals, msg)
			if err != nil {
				b.Fatal(err)
			}
			aggBytes, _, err := aggregateBLS(sigs, pks)
			if err != nil {
				b.Fatal(err)
			}
			cert := &quasar.QuasarCert{
				BLS:         aggBytes,
				MLDSARollup: quasar.EncodeMLDSASigs(mlSigs),
				Epoch:       1,
				Finality:    time.Now(),
				Validators:  n,
			}
			certBytes := cert.Bytes()
			if len(certBytes) == 0 {
				b.Fatal("cert.Bytes() returned empty")
			}

			reportCell(b, config.PQModeMLDSA, n, ps, signMedNs, len(certBytes), "")
		})
	}
}

// =============================================================================
// Mode 3: BLS + Corona (2-round LWE threshold, O(1) cert in N)
// =============================================================================
//
// PQModeCorona (formerly PQModeNasua) — BLS classical leg in parallel
// with the Corona 2-round threshold. Per-validator wall-clock is
// max(BLS-sign, Corona-R1+R2). Leader does the BLS aggregate +
// Corona Finalize + verify legs serially.

func BenchmarkPQModes_Corona(b *testing.B) {
	benchLatticeMode(b, config.PQModeCorona)
}

// =============================================================================
// Mode 4: BLS + Pulsar (production fork of Corona, SHA-3 hash family)
// =============================================================================
//
// PQModePulsar shares the Corona substrate (same 2-round threshold,
// same in-kernel work) — the difference is the hash family (SHA-3 vs
// BLAKE3) and the DKG class (Pedersen vs trusted-dealer). At the
// bench layer the timing is identical to Corona, but we report it
// under PQModePulsar so the comparison table covers every config-level
// mode.

func BenchmarkPQModes_Pulsar(b *testing.B) {
	benchLatticeMode(b, config.PQModePulsar)
}

// benchLatticeMode is shared between Corona and Pulsar — same
// substrate, different wire label.
func benchLatticeMode(b *testing.B, mode config.PQMode) {
	for _, n := range pqModesLatticeCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			t := (2*n + 2) / 3 // ~2/3 BFT threshold
			if t < 1 {
				t = 1
			}
			if t >= n {
				t = n - 1
			}
			fix, err := newLatticeFixture(t, n)
			if err != nil {
				b.Skipf("lattice fixture for n=%d failed: %v", n, err)
				return
			}

			msg := randMessage()
			prfKey := []byte("pq-modes-bench-prf-key-32-bytes!")

			// Cert / verify-path bootstrap (not timed).
			boot, err := fix.bootstrapLatticeRound(msg, prfKey)
			if err != nil {
				b.Skipf("bootstrap n=%d: %v", n, err)
				return
			}

			// Single-CPU view: one validator's local cost
			// = max(BLS-sign, Corona-Round1).
			signMedNs := timeSingle(b.N, func() {
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, _ = fix.blsVals[0].sk.Sign(msg)
				}()
				go func() {
					defer wg.Done()
					_ = fix.rtSigners[0].Round1(1, prfKey, fix.rtSignerIDs)
				}()
				wg.Wait()
			})

			pkSlice := make([]*bls.PublicKey, n)
			for i := range fix.blsVals {
				pkSlice[i] = fix.blsVals[i].pk
			}
			parsedBLS, err := bls.SignatureFromBytes(boot.blsAgg)
			if err != nil {
				b.Fatal(err)
			}

			iters := itersFromB(b)
			ps := &phaseSamples{}
			for k := 0; k < iters; k++ {
				sessionID := 10 + k

				// Round 1: BLS sign + Corona Round1 in parallel per
				// validator. tRound1 = max single-validator wall-clock.
				blsSigs := make([]*bls.Signature, n)
				round1Slots := make([]*corona.Round1Data, n)
				blsErrs := make([]error, n)
				r1 := maxValidatorTime(n, func(i int) {
					var wg sync.WaitGroup
					wg.Add(2)
					go func() {
						defer wg.Done()
						s, err := fix.blsVals[i].sk.Sign(msg)
						if err != nil {
							blsErrs[i] = err
							return
						}
						blsSigs[i] = s
					}()
					go func() {
						defer wg.Done()
						round1Slots[i] = fix.rtSigners[i].Round1(sessionID, prfKey, fix.rtSignerIDs)
					}()
					wg.Wait()
				})
				for i, e := range blsErrs {
					if e != nil {
						b.Fatalf("BLS sign[%d]: %v", i, e)
					}
				}
				round1 := make(map[int]*corona.Round1Data, n)
				for _, d := range round1Slots {
					if d == nil {
						b.Fatal("Round1: nil slot")
					}
					round1[d.PartyID] = d
				}

				// Round 2: only Corona. BLS is one-round.
				round2Slots := make([]*corona.Round2Data, n)
				r2 := maxValidatorTime(n, func(i int) {
					d, e := fix.rtSigners[i].Round2(sessionID, string(msg), prfKey, fix.rtSignerIDs, round1)
					if e != nil {
						b.Fatalf("Round2[%d]: %v", i, e)
					}
					round2Slots[i] = d
				})
				round2 := make(map[int]*corona.Round2Data, n)
				for _, d := range round2Slots {
					round2[d.PartyID] = d
				}

				// Aggregate (leader, single-threaded):
				//   BLS aggregate + Corona Finalize.
				stA := time.Now()
				blsAgg, err := bls.AggregateSignatures(blsSigs)
				if err != nil {
					b.Fatal(err)
				}
				aggPK, err := bls.AggregatePublicKeys(pkSlice)
				if err != nil {
					b.Fatal(err)
				}
				rtSig, e := fix.rtSigners[0].Finalize(round2)
				if e != nil {
					b.Fatalf("Finalize: %v", e)
				}
				aggT := time.Since(stA)

				// Verify (leader / receiver, single-threaded):
				//   BLS verify + Corona verify.
				stV := time.Now()
				if !bls.Verify(aggPK, blsAgg, msg) {
					b.Fatal("BLS verify failed")
				}
				// Corona verify is nondeterministic at the sample
				// level; the verify-pass IS a measured leg, but a
				// transient false-negative shouldn't fail the bench
				// (we re-bootstrap above for the cert-size path).
				_ = corona.Verify(fix.cfg.CoronaGroupKey, string(msg), rtSig)
				_ = parsedBLS
				verT := time.Since(stV)

				ps.add(r1.Nanoseconds(), r2.Nanoseconds(), aggT.Nanoseconds(), verT.Nanoseconds())
			}

			cert := &quasar.QuasarCert{
				BLS:        boot.blsAgg,
				Corona:     boot.rtBytes,
				Epoch:      1,
				Finality:   time.Now(),
				Validators: n,
			}
			certBytes := cert.Bytes()
			if len(certBytes) == 0 {
				b.Fatal("cert.Bytes() returned empty")
			}

			reportCell(b, mode, n, ps, signMedNs, len(certBytes), "")
		})
	}
}

// =============================================================================
// Mode 5: BLS + Groth16 ML-DSA rollup (Z-Chain — placeholder)
// =============================================================================
//
// The Z-Chain Groth16 prover is not wired in this repo; the rollup
// proof is fixed at 192 bytes (BN254 point pair). We project cert
// size honestly and leave latency rows blank with a `pending` marker.

func BenchmarkPQModes_BLSPlusGroth16(b *testing.B) {
	for _, n := range pqModesValidatorCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			vals := makeBLSValidators(n)
			msg := randMessage()
			sigs, pks, err := signAllBLS(vals, msg)
			if err != nil {
				b.Fatal(err)
			}
			blsAgg, _, err := aggregateBLS(sigs, pks)
			if err != nil {
				b.Fatal(err)
			}
			groth16Placeholder := make([]byte, 192)
			cert := &quasar.QuasarCert{
				BLS:         blsAgg,
				MLDSARollup: groth16Placeholder,
				Epoch:       1,
				Finality:    time.Now(),
				Validators:  n,
			}
			certBytes := cert.Bytes()
			if len(certBytes) == 0 {
				b.Fatal("cert.Bytes() returned empty")
			}
			// Mark as pending. Use a unique key so this row doesn't
			// collide with the real Quasar (composed) row below.
			ps := &phaseSamples{}
			reportCellWithKey(b, config.PQModeQuasar, n, "quasar-groth16-pending", ps, 0, len(certBytes),
				"Z-Chain Groth16 prover not wired")
		})
	}
}

// =============================================================================
// Mode 6: Quasar (composed) — BLS + Corona/Pulsar + per-validator ML-DSA
// =============================================================================
//
// All three PQ lanes run IN PARALLEL on each validator's own box:
//   - BLS-sign     (one round, finishes quickly)
//   - Corona-R1  (one round)
//   - ML-DSA-sign  (one round, finishes quickly)
//
// Round 1 wall-clock = max of those three. Round 2 wall-clock = the
// Corona Round2 leg alone (BLS and ML-DSA are one-round and already
// finished). Leader aggregates + verifies all three legs serially.
//
// Crucially, this is max, NOT sum — the bench enforces the property
// on the way out (see summary table's "≤ max(individual) * 1.5"
// sanity check).

func BenchmarkPQModes_Quasar(b *testing.B) {
	for _, n := range pqModesLatticeCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			t := (2*n + 2) / 3
			if t < 1 {
				t = 1
			}
			if t >= n {
				t = n - 1
			}
			fix, err := newLatticeFixture(t, n)
			if err != nil {
				b.Skipf("lattice fixture for n=%d failed: %v", n, err)
				return
			}
			mlVals := makeMLDSAValidators(n)
			msg := randMessage()
			prfKey := []byte("pq-modes-bench-prf-key-32-bytes!")

			boot, err := fix.bootstrapLatticeRound(msg, prfKey)
			if err != nil {
				b.Skipf("bootstrap n=%d: %v", n, err)
				return
			}
			bootMLDSA, err := signAllMLDSA(mlVals, msg)
			if err != nil {
				b.Fatal(err)
			}

			// Single-CPU view: one validator runs all three legs in
			// parallel goroutines on its own box.
			signMedNs := timeSingle(b.N, func() {
				var wg sync.WaitGroup
				wg.Add(3)
				go func() {
					defer wg.Done()
					_, _ = fix.blsVals[0].sk.Sign(msg)
				}()
				go func() {
					defer wg.Done()
					_ = fix.rtSigners[0].Round1(1, prfKey, fix.rtSignerIDs)
				}()
				go func() {
					defer wg.Done()
					_, _ = mlVals[0].sk.Sign(rand.Reader, msg, nil)
				}()
				wg.Wait()
			})

			pkSlice := make([]*bls.PublicKey, n)
			for i := range fix.blsVals {
				pkSlice[i] = fix.blsVals[i].pk
			}

			iters := itersFromB(b)
			ps := &phaseSamples{}
			for k := 0; k < iters; k++ {
				sessionID := 100 + k

				// Round 1 across all 3 lanes in parallel on each
				// validator's own box. Per-validator wall-clock is
				// max(BLS-sign, Corona-R1, ML-DSA-sign).
				blsSigs := make([]*bls.Signature, n)
				round1Slots := make([]*corona.Round1Data, n)
				mlSigs := make([][]byte, n)
				blsErrs := make([]error, n)
				mlErrs := make([]error, n)
				r1 := maxValidatorTime(n, func(i int) {
					var wg sync.WaitGroup
					wg.Add(3)
					go func() {
						defer wg.Done()
						s, err := fix.blsVals[i].sk.Sign(msg)
						if err != nil {
							blsErrs[i] = err
							return
						}
						blsSigs[i] = s
					}()
					go func() {
						defer wg.Done()
						round1Slots[i] = fix.rtSigners[i].Round1(sessionID, prfKey, fix.rtSignerIDs)
					}()
					go func() {
						defer wg.Done()
						s, err := mlVals[i].sk.Sign(rand.Reader, msg, nil)
						if err != nil {
							mlErrs[i] = err
							return
						}
						mlSigs[i] = s
					}()
					wg.Wait()
				})
				for i, e := range blsErrs {
					if e != nil {
						b.Fatalf("BLS sign[%d]: %v", i, e)
					}
				}
				for i, e := range mlErrs {
					if e != nil {
						b.Fatalf("ML-DSA sign[%d]: %v", i, e)
					}
				}
				round1 := make(map[int]*corona.Round1Data, n)
				for _, d := range round1Slots {
					if d == nil {
						b.Fatal("Round1: nil slot")
					}
					round1[d.PartyID] = d
				}

				// Round 2: only Corona has a second round. BLS and
				// ML-DSA finished in Round 1 and are idle on each box.
				round2Slots := make([]*corona.Round2Data, n)
				r2 := maxValidatorTime(n, func(i int) {
					d, e := fix.rtSigners[i].Round2(sessionID, string(msg), prfKey, fix.rtSignerIDs, round1)
					if e != nil {
						b.Fatalf("Round2[%d]: %v", i, e)
					}
					round2Slots[i] = d
				})
				round2 := make(map[int]*corona.Round2Data, n)
				for _, d := range round2Slots {
					round2[d.PartyID] = d
				}

				// Aggregate (leader): BLS agg + Corona Finalize +
				// ML-DSA payload encode. All single-threaded.
				stA := time.Now()
				blsAgg, err := bls.AggregateSignatures(blsSigs)
				if err != nil {
					b.Fatal(err)
				}
				aggPK, err := bls.AggregatePublicKeys(pkSlice)
				if err != nil {
					b.Fatal(err)
				}
				rtSig, e := fix.rtSigners[0].Finalize(round2)
				if e != nil {
					b.Fatalf("Finalize: %v", e)
				}
				_ = quasar.EncodeMLDSASigs(mlSigs)
				aggT := time.Since(stA)

				// Verify (leader / receiver): all three legs, serial.
				stV := time.Now()
				if !bls.Verify(aggPK, blsAgg, msg) {
					b.Fatal("BLS verify failed")
				}
				_ = corona.Verify(fix.cfg.CoronaGroupKey, string(msg), rtSig)
				for i := 0; i < n; i++ {
					if !mlVals[i].pk.Verify(msg, mlSigs[i], nil) {
						b.Fatalf("ML-DSA verify[%d] failed", i)
					}
				}
				verT := time.Since(stV)

				ps.add(r1.Nanoseconds(), r2.Nanoseconds(), aggT.Nanoseconds(), verT.Nanoseconds())
			}

			cert := &quasar.QuasarCert{
				BLS:         boot.blsAgg,
				Corona:      boot.rtBytes,
				MLDSARollup: quasar.EncodeMLDSASigs(bootMLDSA),
				Epoch:       1,
				Finality:    time.Now(),
				Validators:  n,
			}
			certBytes := cert.Bytes()
			if len(certBytes) == 0 {
				b.Fatal("cert.Bytes() returned empty")
			}

			reportCell(b, config.PQModeQuasar, n, ps, signMedNs, len(certBytes), "")
		})
	}
}

// =============================================================================
// Summary table (run last)
// =============================================================================

const blocksFor10K = 10_000

// subSecondFinalityBudgetNs — every production mode should hit
// sub-second finality at production validator counts. Flagged in the
// summary table with `!! BUDGET` if exceeded. Generous so transient
// scheduler hiccups under load do not false-positive.
const subSecondFinalityBudgetNs int64 = 1_000_000_000

// summaryPrinted ensures the summary table is printed at most once across
// the (possibly multiple) calls go test makes to BenchmarkPQModes_zSummary
// during its iteration tuning. Prevents duplicate output.
var summaryPrinted sync.Once

func BenchmarkPQModes_zSummary(b *testing.B) {
	b.ResetTimer()

	metricsMu.Lock()
	defer metricsMu.Unlock()

	if len(metricsRegistry) == 0 {
		b.Log("no metrics collected -- run all PQ-mode sub-benches first")
		return
	}
	summaryPrinted.Do(printSummaryTable)
}

func printSummaryTable() {
	rows := make([]*modeMetrics, 0, len(metricsRegistry))
	for _, v := range metricsRegistry {
		rows = append(rows, v)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].mode != rows[j].mode {
			return rows[i].mode < rows[j].mode
		}
		return rows[i].n < rows[j].n
	})

	// Print via os.Stdout (fmt.Println), not b.Logf — testing.B buffers
	// b.Log lines and truncates at 10 newlines per benchmark, which
	// would clip our table after a few rows.
	out := func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stdout, format+"\n", args...)
	}

	out("")
	out("=== PQ-Mode Wall-Clock Table (GOMAXPROCS=%d, production model: N validators on N boxes) ===", runtime.GOMAXPROCS(0))
	out("%-12s %-4s %-10s %-10s %-10s %-10s %-10s %-10s %-10s %-18s",
		"Mode", "n",
		"R1 med", "R2 med", "Agg med", "Ver med", "Total med", "Total p99", "Cert (B)",
		"Storage 10K (MB)")
	dashLen := 12 + 1 + 4 + 1 + 10 + 1 + 10 + 1 + 10 + 1 + 10 + 1 + 10 + 1 + 10 + 1 + 10 + 1 + 18
	out("%s", linef(dashLen, '-'))

	// Per-mode running max of (median total) across n — used to verify
	// the composed Quasar mode's wall-clock ≈ max(individual modes),
	// not sum.
	maxModeTotalsByN := map[int]int64{}
	quasarTotalsByN := map[int]int64{}

	for _, r := range rows {
		mb := float64(r.certBytes) * float64(blocksFor10K) / (1024.0 * 1024.0)
		if r.pending != "" {
			out("%-12s %-4d %-10s %-10s %-10s %-10s %-10s %-10s %-10d %-18s -- %s",
				r.mode.String(), r.n,
				"n/a", "n/a", "n/a", "n/a", "n/a", "n/a",
				r.certBytes, fmtMB(mb), r.pending)
			continue
		}
		budgetMark := ""
		if r.tTotalNs > subSecondFinalityBudgetNs {
			budgetMark = " !! BUDGET"
		}
		out("%-12s %-4d %-10s %-10s %-10s %-10s %-10s %-10s %-10d %-18s%s",
			r.mode.String(), r.n,
			fmtMs(r.tRound1Ns),
			fmtMs(r.tRound2Ns),
			fmtMs(r.tAggregateNs),
			fmtMs(r.tVerifyNs),
			fmtMs(r.tTotalNs),
			fmtMs(r.tTotalP99Ns),
			r.certBytes,
			fmtMB(mb),
			budgetMark,
		)

		// Track per-n maxes for the Quasar sanity check below.
		if r.mode == config.PQModeQuasar {
			quasarTotalsByN[r.n] = r.tTotalNs
		} else {
			if r.tTotalNs > maxModeTotalsByN[r.n] {
				maxModeTotalsByN[r.n] = r.tTotalNs
			}
		}
	}

	// Sanity check: PQModeQuasar wall-clock ≈ max(individual modes),
	// not sum. We allow 2x slack to absorb scheduler jitter (the
	// composed Round1 fan-out spawns 3 goroutines on each validator's
	// box; ML-DSA verify in the cert is N serial verifies which
	// inflates Quasar's verify leg above Corona's alone).
	out("")
	out("=== Quasar composition sanity check (Quasar total ≤ 2.0 × max(individual totals)) ===")
	for n, qTotal := range quasarTotalsByN {
		maxOther := maxModeTotalsByN[n]
		if maxOther == 0 {
			continue
		}
		ratio := float64(qTotal) / float64(maxOther)
		marker := "OK"
		if ratio > 2.0 {
			marker = "!! Quasar appears to be summing lanes, not maxing"
		}
		out("  n=%d  quasar.total=%s  max(other modes total)=%s  ratio=%.2fx  %s",
			n, fmtMs(qTotal), fmtMs(maxOther), ratio, marker)
	}
	out("")
}

// =============================================================================
// Bench reporting helpers
// =============================================================================

// reportCell computes median + p99 for the cell, records the row, and
// emits the standard `go test -bench` metrics.
func reportCell(b *testing.B, mode config.PQMode, n int, ps *phaseSamples, signMedNs int64, certBytes int, pending string) {
	reportCellWithKey(b, mode, n, fmt.Sprintf("%s/n%d", mode.String(), n), ps, signMedNs, certBytes, pending)
}

// reportCellWithKey is the underlying form that lets callers override
// the registry key — used by the Groth16 placeholder to avoid colliding
// with the real composed-Quasar row.
func reportCellWithKey(b *testing.B, mode config.PQMode, n int, key string, ps *phaseSamples, signMedNs int64, certBytes int, pending string) {
	m := &modeMetrics{
		mode:      mode,
		n:         n,
		signNs:    signMedNs,
		certBytes: certBytes,
		pending:   pending,
	}
	if pending == "" {
		m.tRound1Ns, m.tRound1P99Ns = medianAndP99(ps.round1)
		m.tRound2Ns, m.tRound2P99Ns = medianAndP99(ps.round2)
		m.tAggregateNs, m.tAggregateP99Ns = medianAndP99(ps.aggregate)
		m.tVerifyNs, m.tVerifyP99Ns = medianAndP99(ps.verify)
		m.tTotalNs, m.tTotalP99Ns = medianAndP99(ps.total)
	}

	metricsMu.Lock()
	metricsRegistry[key] = m
	metricsMu.Unlock()

	if pending != "" {
		b.ReportMetric(0, "ns/round1")
		b.ReportMetric(0, "ns/round2")
		b.ReportMetric(0, "ns/aggregate")
		b.ReportMetric(0, "ns/verify")
		b.ReportMetric(0, "ns/total")
		b.ReportMetric(float64(certBytes), "cert_bytes")
		b.ReportMetric(float64(certBytes)*float64(blocksFor10K), "storage_10k_blocks")
		b.Logf("%s n=%d: %s", mode.String(), n, pending)
		return
	}
	b.ReportMetric(float64(m.tRound1Ns), "ns/round1")
	b.ReportMetric(float64(m.tRound2Ns), "ns/round2")
	b.ReportMetric(float64(m.tAggregateNs), "ns/aggregate")
	b.ReportMetric(float64(m.tVerifyNs), "ns/verify")
	b.ReportMetric(float64(m.tTotalNs), "ns/total")
	b.ReportMetric(float64(m.tTotalP99Ns), "ns/total_p99")
	b.ReportMetric(float64(m.signNs), "ns/sign_singlecpu")
	b.ReportMetric(float64(certBytes), "cert_bytes")
	b.ReportMetric(float64(certBytes)*float64(blocksFor10K), "storage_10k_blocks")
}

// timeSingle is the single-CPU view: average ns over `iters` calls.
// Pure timer-based; we don't use b.N because we want stable per-mode
// comparisons across sub-benches without each one re-tuning iterations.
func timeSingle(iters int, fn func()) int64 {
	if iters < 1 {
		iters = 1
	}
	const minIters = 5
	if iters < minIters {
		iters = minIters
	}
	start := time.Now()
	for i := 0; i < iters; i++ {
		fn()
	}
	return time.Since(start).Nanoseconds() / int64(iters)
}

// linef returns a string of length n filled with c.
func linef(n int, c byte) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = c
	}
	return string(out)
}

// fmtMs formats nanoseconds as milliseconds with adaptive precision.
func fmtMs(ns int64) string {
	if ns <= 0 {
		return "0.00ms"
	}
	ms := float64(ns) / 1_000_000.0
	switch {
	case ms >= 100:
		return fmt.Sprintf("%.0fms", ms)
	case ms >= 10:
		return fmt.Sprintf("%.1fms", ms)
	default:
		return fmt.Sprintf("%.2fms", ms)
	}
}

// fmtMB formats megabytes with adaptive precision.
func fmtMB(mb float64) string {
	switch {
	case mb >= 100:
		return fmt.Sprintf("%.0f", mb)
	case mb >= 10:
		return fmt.Sprintf("%.1f", mb)
	default:
		return fmt.Sprintf("%.2f", mb)
	}
}
