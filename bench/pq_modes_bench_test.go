// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// Benchmark suite comparing the post-quantum consensus modes.
//
// Each PQMode is measured for:
//   - sign latency (per-validator)
//   - aggregate latency (combine N shares into a finality cert)
//   - verify latency (per-validator on receive)
//   - cert wire size (bytes per cert)
//   - storage growth for 10K blocks
//
// Run with:
//
//	cd consensus
//	GOWORK=off go test -bench=BenchmarkPQModes -benchmem -benchtime=10x ./bench/...
//
// The bench prints a summary table at the end. Modes blocked on yet-to-land
// Z-Chain Groth16 wiring (PQModeQuasar) are reported as
// "n/a -- pending implementation" rather than fabricated.
package bench

import (
	"crypto/rand"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// =============================================================================
// Modes & sizes under test
// =============================================================================

// pqModesValidatorCounts is the validator-set sizes the bench sweeps for
// modes whose work is cheap (BLS-only, ML-DSA, Groth16 placeholder). n=21
// is the mainnet shape; n=4 is the smallest BFT, n=64 / n=100 stress the
// per-validator linear paths (notably ML-DSA-65 = 3309 B per validator).
var pqModesValidatorCounts = []int{4, 21, 64, 100}

// pqModesCoronaCounts is the validator-set sizes used for Corona-bearing
// modes. n=4 is excluded because the Ring-LWE rejection sampler is known
// flaky at that group size (see consensus CLAUDE.md). n=100 is excluded
// because a single full 2-round Corona signing pass takes 5+ minutes
// at that group size; the bench instead measures n in {21, 64} which is
// where the shape of the cost curve is informative.
var pqModesCoronaCounts = []int{21, 64}

// coronaMaxRetries bounds how many times we re-run Round1/Round2/Finalize
// when verify fails (Corona rejection-sampling is nondeterministic).
const coronaMaxRetries = 4

// modeMetrics is the per-mode per-n result row aggregated by the suite.
type modeMetrics struct {
	mode      config.PQMode
	n         int
	signNs    int64  // ns per validator-sign call (real measurement)
	aggNs     int64  // ns to aggregate N shares
	verifyNs  int64  // ns per cert verify
	certBytes int    // serialized cert size
	pending   string // non-empty -> mode is not yet implemented
}

// metricsRegistry collects rows across the whole bench run so the final
// "summary table" sub-bench can print them after every PQMode and N has
// run. Keyed by (mode, n) so reruns overwrite cleanly.
var (
	metricsMu       sync.Mutex
	metricsRegistry = map[string]*modeMetrics{}
)

func recordMetric(m *modeMetrics) {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	metricsRegistry[fmt.Sprintf("%s/n%d", m.mode.String(), m.n)] = m
}

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

// blsValidator is a single BLS keypair (used for the legacy aggregate path).
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

// blsAggregateOnce signs `msg` with all `vals`, aggregates, and returns
// (signed_aggregate_bytes, aggregate_pubkey_bytes_len).
func blsAggregateOnce(vals []blsValidator, msg []byte) ([]byte, *bls.PublicKey) {
	sigs := make([]*bls.Signature, len(vals))
	pks := make([]*bls.PublicKey, len(vals))
	for i, v := range vals {
		s, err := v.sk.Sign(msg)
		if err != nil {
			panic(err)
		}
		sigs[i] = s
		pks[i] = v.pk
	}
	agg, err := bls.AggregateSignatures(sigs)
	if err != nil {
		panic(err)
	}
	aggPK, err := bls.AggregatePublicKeys(pks)
	if err != nil {
		panic(err)
	}
	return bls.SignatureToBytes(agg), aggPK
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

// =============================================================================
// Mode 1: BLS-only (classical fast path)
// =============================================================================

func BenchmarkPQModes_BLSOnly(b *testing.B) {
	for _, n := range pqModesValidatorCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			vals := makeBLSValidators(n)
			msg := randMessage()

			// Pre-sign for verify path
			aggBytes, aggPK := blsAggregateOnce(vals, msg)

			// Sign latency = single-validator BLS sign
			signNs := timeIt(b.N, func() {
				_, _ = vals[0].sk.Sign(msg)
			})

			// Aggregate latency = combine N sigs (sigs created fresh inside)
			aggNs := timeIt(b.N, func() {
				_, _ = blsAggregateOnce(vals, msg)
			})

			// Verify latency = bls.Verify of the aggregate
			parsedSig, err := bls.SignatureFromBytes(aggBytes)
			if err != nil {
				b.Fatal(err)
			}
			verifyNs := timeIt(b.N, func() {
				if !bls.Verify(aggPK, parsedSig, msg) {
					b.Fatal("verify failed")
				}
			})

			cert := &quasar.QuasarCert{
				BLS:        aggBytes,
				Epoch:      1,
				Finality:   time.Now(),
				Validators: n,
			}
			certBytes, err := cert.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}

			report(b, &modeMetrics{
				mode:      config.PQModeBLS,
				n:         n,
				signNs:    signNs,
				aggNs:     aggNs,
				verifyNs:  verifyNs,
				certBytes: len(certBytes),
			})
		})
	}
}

// =============================================================================
// Mode 2: BLS + ML-DSA-65 per-validator
// =============================================================================

func BenchmarkPQModes_BLSPlusMLDSA(b *testing.B) {
	for _, n := range pqModesValidatorCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			blsVals := makeBLSValidators(n)
			mlVals := makeMLDSAValidators(n)
			msg := randMessage()

			// Pre-build for aggregate / verify
			mlSigs := make([][]byte, n)
			for i := range mlVals {
				sig, err := mlVals[i].sk.Sign(rand.Reader, msg, nil)
				if err != nil {
					b.Fatal(err)
				}
				mlSigs[i] = sig
			}
			blsAgg, blsAggPK := blsAggregateOnce(blsVals, msg)
			mldsaPayload := quasar.EncodeMLDSASigs(mlSigs)

			// Sign latency: BLS sign + ML-DSA sign in parallel (real validator path)
			signNs := timeIt(b.N, func() {
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

			// Aggregate latency: BLS aggregate + ML-DSA payload concat
			aggNs := timeIt(b.N, func() {
				_, _ = blsAggregateOnce(blsVals, msg)
				_ = quasar.EncodeMLDSASigs(mlSigs)
			})

			// Verify latency: BLS aggregate verify + N ML-DSA verifies
			parsedSig, err := bls.SignatureFromBytes(blsAgg)
			if err != nil {
				b.Fatal(err)
			}
			verifyNs := timeIt(b.N, func() {
				if !bls.Verify(blsAggPK, parsedSig, msg) {
					b.Fatal("BLS verify failed")
				}
				for i := range mlVals {
					if !mlVals[i].pk.Verify(msg, mlSigs[i], nil) {
						b.Fatal("ML-DSA verify failed")
					}
				}
			})

			cert := &quasar.QuasarCert{
				BLS:         blsAgg,
				MLDSARollup: mldsaPayload,
				Epoch:       1,
				Finality:    time.Now(),
				Validators:  n,
			}
			certBytes, err := cert.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}

			report(b, &modeMetrics{
				mode:      config.PQModeMLDSA,
				n:         n,
				signNs:    signNs,
				aggNs:     aggNs,
				verifyNs:  verifyNs,
				certBytes: len(certBytes),
			})
		})
	}
}

// =============================================================================
// Mode 3: BLS + Corona (lattice threshold, O(1) cert in N)
// =============================================================================

// coronaFixture holds a fully-keyed Corona group plus a parallel BLS
// keyset. We re-use the dual_threshold_test.go flow: GenerateDualKeys
// gives us BLS-threshold + Corona-threshold shares for a (t, n) group,
// and we drive Round1 / Round2 / Finalize to produce a real signature.
type coronaFixture struct {
	cfg          *quasar.SignerConfig
	signer       *quasar.Signer
	rtSigners    []*coronaThreshold.Signer
	rtSignerIDs  []int
	validatorIDs []string
	blsVals      []blsValidator
	t            int
	n            int
}

func newCoronaFixture(t, n int) (*coronaFixture, error) {
	cfg, err := quasar.GenerateDualKeys(t, n)
	if err != nil {
		return nil, fmt.Errorf("GenerateDualKeys(%d,%d): %w", t, n, err)
	}
	s, err := quasar.NewSignerWithDualThreshold(*cfg)
	if err != nil {
		return nil, fmt.Errorf("NewSignerWithDualThreshold: %w", err)
	}

	ids := make([]string, n)
	rtSigners := make([]*coronaThreshold.Signer, n)
	rtIDs := make([]int, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("v%d", i)
		ids[i] = id
		share := cfg.CoronaShares[id]
		rtSigners[i] = coronaThreshold.NewSigner(share)
		rtIDs[i] = i
	}

	return &coronaFixture{
		cfg:          cfg,
		signer:       s,
		rtSigners:    rtSigners,
		rtSignerIDs:  rtIDs,
		validatorIDs: ids,
		blsVals:      makeBLSValidators(n),
		t:            t,
		n:            n,
	}, nil
}

// signResult holds one full pass's outputs.
type signResult struct {
	rtSig    *coronaThreshold.Signature
	rtBytes  []byte
	blsAgg   []byte
	blsAggPK *bls.PublicKey
}

// signWithRetry re-runs signOnce until the produced Corona signature
// verifies, up to coronaMaxRetries attempts. Returns (result, attempts,
// error). Used because Corona rejection sampling is nondeterministic.
func (f *coronaFixture) signWithRetry(msg []byte, prfKey []byte, groupKey *coronaThreshold.GroupKey) (*signResult, int, error) {
	var lastErr error
	for i := 0; i < coronaMaxRetries; i++ {
		res, err := f.signOnce(msg, i+1, prfKey)
		if err != nil {
			lastErr = err
			continue
		}
		if coronaThreshold.Verify(groupKey, string(msg), res.rtSig) {
			return res, i + 1, nil
		}
		lastErr = fmt.Errorf("Corona verify failed on attempt %d", i+1)
	}
	return nil, coronaMaxRetries, lastErr
}

// signOnce runs the Corona 2-round flow once and returns the resulting
// signature alongside the BLS aggregate over the same message.
func (f *coronaFixture) signOnce(msg []byte, sessionID int, prfKey []byte) (*signResult, error) {
	round1 := make(map[int]*coronaThreshold.Round1Data, f.n)
	for _, s := range f.rtSigners {
		d, e := s.Round1(sessionID, prfKey, f.rtSignerIDs)
		if e != nil {
			return nil, fmt.Errorf("Round1: %w", e)
		}
		round1[d.PartyID] = d
	}

	round2 := make(map[int]*coronaThreshold.Round2Data, f.n)
	for _, s := range f.rtSigners {
		d, e := s.Round2(sessionID, string(msg), prfKey, f.rtSignerIDs, round1)
		if e != nil {
			return nil, fmt.Errorf("Round2: %w", e)
		}
		round2[d.PartyID] = d
	}

	rtSig, e := f.rtSigners[0].Finalize(round2)
	if e != nil {
		return nil, fmt.Errorf("Finalize: %w", e)
	}

	rtBytes := quasar.EncodeCoronaSig(rtSig)
	if rtBytes == nil {
		return nil, fmt.Errorf("EncodeCoronaSig returned nil")
	}

	blsAgg, blsAggPK := blsAggregateOnce(f.blsVals, msg)
	return &signResult{rtSig: rtSig, rtBytes: rtBytes, blsAgg: blsAgg, blsAggPK: blsAggPK}, nil
}

func BenchmarkPQModes_BLSPlusCorona(b *testing.B) {
	for _, n := range pqModesCoronaCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			t := (2*n + 2) / 3 // ~2/3 threshold (BFT)
			if t < 1 {
				t = 1
			}
			if t >= n {
				t = n - 1
			}
			fix, err := newCoronaFixture(t, n)
			if err != nil {
				b.Skipf("Corona fixture for n=%d failed: %v", n, err)
				return
			}

			msg := randMessage()
			prfKey := []byte("pq-modes-bench-prf-key-32-bytes!")

			// Pre-run with retry to build steady-state sig + BLS agg.
			// This is also our aggregate-latency sample (one full pass).
			aggStart := time.Now()
			res, attempts, err := fix.signWithRetry(msg, prfKey, fix.cfg.CoronaGroupKey)
			aggElapsed := time.Since(aggStart)
			if err != nil {
				b.Skipf("Corona signWithRetry exhausted (n=%d, attempts=%d): %v", n, attempts, err)
				return
			}
			// Per-pass latency = total / attempts (each attempt is one full sig).
			aggNsPerPass := aggElapsed.Nanoseconds() / int64(attempts)

			// Sign latency: per-validator BLS share + Corona Round1 share.
			signNs := timeIt(b.N, func() {
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, _ = fix.blsVals[0].sk.Sign(msg)
				}()
				go func() {
					defer wg.Done()
					_, _ = fix.rtSigners[0].Round1(1, prfKey, fix.rtSignerIDs)
				}()
				wg.Wait()
			})

			// Verify latency: BLS aggregate verify + Corona Verify.
			parsedSig, err := bls.SignatureFromBytes(res.blsAgg)
			if err != nil {
				b.Fatal(err)
			}
			verifyNs := timeIt(b.N, func() {
				if !bls.Verify(res.blsAggPK, parsedSig, msg) {
					b.Fatal("BLS verify failed")
				}
				if !coronaThreshold.Verify(fix.cfg.CoronaGroupKey, string(msg), res.rtSig) {
					b.Fatal("Corona verify failed")
				}
			})

			cert := &quasar.QuasarCert{
				BLS:        res.blsAgg,
				Corona:     res.rtBytes,
				Epoch:      1,
				Finality:   time.Now(),
				Validators: n,
			}
			certBytes, err := cert.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}

			report(b, &modeMetrics{
				mode:      config.PQModeNasua,
				n:         n,
				signNs:    signNs,
				aggNs:     aggNsPerPass,
				verifyNs:  verifyNs,
				certBytes: len(certBytes),
			})
		})
	}
}

// =============================================================================
// Mode 4: BLS + Groth16 ML-DSA rollup (Z-Chain)
// =============================================================================
//
// The Z-Chain Groth16 prover is not in this repo; the rollup proof is
// fixed at 192 bytes (BN254 point pair). We project cert size honestly
// and leave latency rows blank.

func BenchmarkPQModes_BLSPlusGroth16(b *testing.B) {
	for _, n := range pqModesValidatorCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			vals := makeBLSValidators(n)
			msg := randMessage()
			blsAgg, _ := blsAggregateOnce(vals, msg)

			// Cert wire size: BLS aggregate (48 B) + 192 B Groth16 placeholder.
			groth16Placeholder := make([]byte, 192)
			cert := &quasar.QuasarCert{
				BLS:         blsAgg,
				MLDSARollup: groth16Placeholder,
				Epoch:       1,
				Finality:    time.Now(),
				Validators:  n,
			}
			certBytes, err := cert.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}

			report(b, &modeMetrics{
				mode:      config.PQModeQuasar,
				n:         n,
				certBytes: len(certBytes),
				pending:   "Z-Chain Groth16 prover not wired",
			})
		})
	}
}

// =============================================================================
// Mode 5: Triple Quantum (BLS + Corona + per-validator ML-DSA)
// =============================================================================

func BenchmarkPQModes_TripleQuantum(b *testing.B) {
	for _, n := range pqModesCoronaCounts {
		n := n
		b.Run(fmt.Sprintf("n%d", n), func(b *testing.B) {
			t := (2*n + 2) / 3
			if t < 1 {
				t = 1
			}
			if t >= n {
				t = n - 1
			}
			fix, err := newCoronaFixture(t, n)
			if err != nil {
				b.Skipf("Corona fixture for n=%d failed: %v", n, err)
				return
			}
			mlVals := makeMLDSAValidators(n)
			msg := randMessage()
			prfKey := []byte("pq-modes-bench-prf-key-32-bytes!")

			// Pre-build steady-state for verify (with retry)
			aggStart := time.Now()
			res, attempts, err := fix.signWithRetry(msg, prfKey, fix.cfg.CoronaGroupKey)
			rtElapsed := time.Since(aggStart)
			if err != nil {
				b.Skipf("Corona signWithRetry exhausted (n=%d, attempts=%d): %v", n, attempts, err)
				return
			}
			mlSigs := make([][]byte, n)
			for i := range mlVals {
				s, err := mlVals[i].sk.Sign(rand.Reader, msg, nil)
				if err != nil {
					b.Fatal(err)
				}
				mlSigs[i] = s
			}
			mldsaPayload := quasar.EncodeMLDSASigs(mlSigs)
			// Triple aggregate latency = Corona-per-pass + ML-DSA payload encode.
			// ML-DSA encode is ns-cheap; Corona dominates.
			aggNs := rtElapsed.Nanoseconds() / int64(attempts)

			// Sign latency: BLS share + Corona Round1 + ML-DSA sign in parallel.
			signNs := timeIt(b.N, func() {
				var wg sync.WaitGroup
				wg.Add(3)
				go func() {
					defer wg.Done()
					_, _ = fix.blsVals[0].sk.Sign(msg)
				}()
				go func() {
					defer wg.Done()
					_, _ = fix.rtSigners[0].Round1(1, prfKey, fix.rtSignerIDs)
				}()
				go func() {
					defer wg.Done()
					_, _ = mlVals[0].sk.Sign(rand.Reader, msg, nil)
				}()
				wg.Wait()
			})

			// Verify latency: BLS + Corona + N ML-DSA verifies
			parsedSig, err := bls.SignatureFromBytes(res.blsAgg)
			if err != nil {
				b.Fatal(err)
			}
			verifyNs := timeIt(b.N, func() {
				if !bls.Verify(res.blsAggPK, parsedSig, msg) {
					b.Fatal("BLS verify failed")
				}
				if !coronaThreshold.Verify(fix.cfg.CoronaGroupKey, string(msg), res.rtSig) {
					b.Fatal("Corona verify failed")
				}
				for i := range mlVals {
					if !mlVals[i].pk.Verify(msg, mlSigs[i], nil) {
						b.Fatal("ML-DSA verify failed")
					}
				}
			})

			cert := &quasar.QuasarCert{
				BLS:         res.blsAgg,
				Corona:      res.rtBytes,
				MLDSARollup: mldsaPayload,
				Epoch:       1,
				Finality:    time.Now(),
				Validators:  n,
			}
			certBytes, err := cert.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}

			report(b, &modeMetrics{
				mode:      config.PQModeQuasar,
				n:         n,
				signNs:    signNs,
				aggNs:     aggNs,
				verifyNs:  verifyNs,
				certBytes: len(certBytes),
			})
		})
	}
}

// =============================================================================
// Summary table (run last)
// =============================================================================

const blocksFor10K = 10_000

func BenchmarkPQModes_zSummary(b *testing.B) {
	// Force the table even if -benchtime=10x: we don't need iterations
	// here, we just print collected metrics from the prior sub-benches.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
	}

	metricsMu.Lock()
	defer metricsMu.Unlock()

	if len(metricsRegistry) == 0 {
		b.Log("no metrics collected -- run all PQ-mode sub-benches first")
		return
	}

	// Flat sorted view: by mode, then by n.
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

	b.Logf("\n=== PQ-Mode Comparison Table ===")
	b.Logf("%-22s %-4s %-12s %-12s %-12s %-10s %-18s",
		"Mode", "n", "Sign (us)", "Agg (us)", "Verify (us)", "Cert (B)", "Storage 10K (MB)")
	b.Logf("%s", linef(22+1+4+1+12+1+12+1+12+1+10+1+18, '-'))
	for _, r := range rows {
		mb := float64(r.certBytes) * float64(blocksFor10K) / (1024.0 * 1024.0)
		if r.pending != "" {
			b.Logf("%-22s %-4d %-12s %-12s %-12s %-10d %-18s -- %s",
				r.mode.String(), r.n,
				"n/a", "n/a", "n/a",
				r.certBytes, fmtMB(mb), r.pending)
			continue
		}
		b.Logf("%-22s %-4d %-12.2f %-12.2f %-12.2f %-10d %-18s",
			r.mode.String(), r.n,
			float64(r.signNs)/1000.0,
			float64(r.aggNs)/1000.0,
			float64(r.verifyNs)/1000.0,
			r.certBytes,
			fmtMB(mb),
		)
	}
}

// =============================================================================
// Bench reporting helpers
// =============================================================================

// report records a row to the registry and emits the standard `go test`
// metrics so `-benchmem` style reports show real numbers per sub-bench.
func report(b *testing.B, m *modeMetrics) {
	recordMetric(m)
	if m.pending != "" {
		b.ReportMetric(0, "ns/sign")
		b.ReportMetric(0, "ns/agg")
		b.ReportMetric(0, "ns/verify")
		b.ReportMetric(float64(m.certBytes), "cert_bytes")
		b.ReportMetric(float64(m.certBytes)*float64(blocksFor10K), "storage_10k_blocks")
		b.Logf("%s n=%d: %s", m.mode.String(), m.n, m.pending)
		return
	}
	b.ReportMetric(float64(m.signNs), "ns/sign")
	b.ReportMetric(float64(m.aggNs), "ns/agg")
	b.ReportMetric(float64(m.verifyNs), "ns/verify")
	b.ReportMetric(float64(m.certBytes), "cert_bytes")
	b.ReportMetric(float64(m.certBytes)*float64(blocksFor10K), "storage_10k_blocks")
}

// timeIt runs `fn` `iters` times and returns the average wall-clock ns.
// Pure timer-based; we don't use b.N because we want stable per-mode
// comparisons across sub-benches without each one re-tuning iterations.
func timeIt(iters int, fn func()) int64 {
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
