// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bench_pq_policy_tiers_test.go — REAL measured per-lane and composed
// policy-tier benchmarks for the Quasar PQ trustless finality stack.
//
// This is the evidence behind the operator's policy-tier + deployment
// decision: for each finality posture it measures, with the PRODUCTION verify
// functions over REAL crypto (no mocks, no stubs):
//
//   - per-lane VERIFY latency (ns/op)        — the receiver cost per block
//   - per-lane CERT SIZE (bytes)             — the gossip / storage cost
//   - per-lane SIGN / produce cost           — the proposer-side cost
//
// and composes them into the five named policy tiers (AND-mode = the receiver
// runs every required leg's verify, so composed verify = Σ lane verifies and
// composed cert = Σ lane bytes):
//
//	Tier 1  BLS                         = Beam                              (BLS_FAST)
//	Tier 2  BLS + Corona                = Beam ∧ Corona                     (lattice diversity, no named posture)
//	Tier 3  BLS + Pulsar                = Beam ∧ Pulsar-sampled             (HYBRID_PQ, the default)
//	Tier 4  BLS + Corona + Pulsar       = Beam ∧ Pulsar ∧ Corona            (STRICT_QUASAR / STRICT_DUAL_PQ)
//	Tier 5  BLS + Corona + Pulsar
//	          + Magnetar (+ P3Q)        = POLARIS_MAX (+ P3Q fallback lane)
//
// Each lane is the REAL one:
//   - Beam      : luxfi/crypto/bls aggregate verify (one aggregate signature)
//   - Pulsar    : (a) TALUS compact threshold sig = ONE FIPS-204 ML-DSA-65 verify
//                 (b) Sampled cert = r dealerless-committee ML-DSA-65 verifies
//                     over the unbiasable-sortition plan (the default HYBRID_PQ,
//                     n=8,t=7,m=12,r=8 = PulsarHybridPQv1)
//   - Corona    : luxfi/threshold/protocols/corona Ring-LWE threshold verify
//                 (O(1): one group signature, committee size only affects SIGN)
//   - Magnetar  : VerifyMagnetarQuorum = N independent FIPS-205 SLH-DSA-192s
//                 verifies (Track-A trustless rollup, O(N))
//   - P3Q       : VerifyP3QRollupLeg Direct = N independent FIPS-204 ML-DSA-65
//                 verifies (O(N) fallback rollup)
//
// Run:
//
//	cd consensus
//	export SDKROOT="$(xcrun --show-sdk-path)"; export GOWORK=off
//	go test ./protocol/quasar/ -run '^$' -bench 'BenchmarkPQ' -benchmem -benchtime=20x -timeout 30m
//
// The composed tier table is printed by BenchmarkPQPolicyTiers_zzSummary (sorts
// last so every lane fixture is warm). The SLH-DSA-192s sign path is ~1.5s per
// validator, so the Magnetar fixtures are bounded (magnetarBenchN) and built
// once via a lazy cache; the table notes the linear extrapolation to a mainnet
// quorum.
package quasar

import (
	"crypto/rand"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/crypto/bls"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// ---------------------------------------------------------------------------
// Bench parameters (owner-specified realistic committee shapes).
// ---------------------------------------------------------------------------

// magnetarBenchN is the per-validator SLH-DSA quorum size the Magnetar / P3Q
// O(N) lanes are measured at. SLH-DSA-192s sign is ~1.5s/validator, so the
// fixture build is N×1.5s; 16 keeps it tractable while showing the O(N) shape.
// The summary extrapolates verify linearly to a mainnet quorum (the per-signer
// verify is the measured invariant).
const magnetarBenchN = 16

// p3qBenchN is the ML-DSA quorum size the P3Q lane is measured at (ML-DSA sign
// is ~0.5ms, so larger N is cheap to build).
var p3qBenchNs = []int{4, 16, 64}

// coronaBenchN is the Corona committee the 2-round SIGN ceremony is measured
// at. Corona VERIFY is O(1) (one group sig) regardless of committee size; only
// SIGN scales with n. t = ceil(2n/3) BFT. Kept small because a full ceremony
// is on the critical path of fixture build.
const (
	coronaBenchN = 3
	coronaBenchT = 2
)

// ---------------------------------------------------------------------------
// Lazily-built shared fixtures (the expensive SLH-DSA certs are built once).
// ---------------------------------------------------------------------------

type laneFixtures struct {
	msg []byte

	// Beam (BLS).
	blsAggSig []byte
	blsAggPK  *bls.PublicKey
	blsMsg    []byte

	// Pulsar — TALUS compact threshold sig (one ML-DSA-65).
	pulsarSig []byte
	pulsarGK  []byte
	pulsarMsg []byte

	// Pulsar — sampled cert (r-of-m committees).
	sampled *sampledFixture

	// Corona — Ring-LWE threshold sig.
	coronaSig *coronaThreshold.Signature
	coronaGK  *coronaThreshold.GroupKey
	coronaMsg []byte

	// Magnetar — N independent SLH-DSA (built at magnetarBenchN).
	magnetar *benchMagnetarFx

	// P3Q — N independent ML-DSA, keyed by N.
	p3q map[int]*benchP3QFx
}

var (
	laneOnce sync.Once
	laneFx   *laneFixtures
)

func fixtures(tb testing.TB) *laneFixtures {
	laneOnce.Do(func() {
		f := &laneFixtures{p3q: map[int]*benchP3QFx{}}

		// Beam (BLS): aggregate of 21 validator signatures over a 32-byte block
		// hash (mainnet-shape committee; BLS verify is O(1) in the aggregate).
		f.blsMsg = randMsg()
		f.blsAggSig, f.blsAggPK = benchBLSAggregate(tb, f.blsMsg, 21)

		// Pulsar TALUS: one real FIPS-204 ML-DSA-65 group signature.
		f.pulsarMsg = randMsg()
		f.pulsarSig, f.pulsarGK = signPulsarLegFIPS204(tb, f.pulsarMsg)

		// Pulsar sampled: the production default n=8,t=7,m=12,r=8 with exactly r
		// valid dealerless committees.
		f.sampled = newSampledFixture(tb, PulsarHybridPQv1, int(PulsarHybridPQv1.R))

		// Corona: a real Ring-LWE t-of-n threshold signature.
		f.coronaMsg = randMsg()
		f.coronaSig, f.coronaGK, _ = signCoronaLegMultiNode(tb, coronaBenchT, coronaBenchT, coronaBenchN, f.coronaMsg)

		// Magnetar: N independent SLH-DSA-192s certs (the slow build).
		f.magnetar = buildBenchMagnetar(tb, magnetarBenchN)

		// P3Q: N independent ML-DSA-65 certs for each N.
		for _, n := range p3qBenchNs {
			f.p3q[n] = buildBenchP3Q(tb, n)
		}

		laneFx = f
	})
	return laneFx
}

func randMsg() []byte {
	m := make([]byte, 32)
	if _, err := rand.Read(m); err != nil {
		panic(err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Beam (BLS) lane.
// ---------------------------------------------------------------------------

func benchBLSAggregate(tb testing.TB, msg []byte, n int) ([]byte, *bls.PublicKey) {
	pubs := make([]*bls.PublicKey, 0, n)
	sigs := make([]*bls.Signature, 0, n)
	for i := 0; i < n; i++ {
		sk, err := bls.NewSecretKey()
		if err != nil {
			tb.Fatalf("bls keygen: %v", err)
		}
		sig, err := sk.Sign(msg)
		if err != nil {
			tb.Fatalf("bls sign: %v", err)
		}
		pubs = append(pubs, sk.PublicKey())
		sigs = append(sigs, sig)
	}
	apk, err := bls.AggregatePublicKeys(pubs)
	if err != nil {
		tb.Fatalf("aggregate pubkeys: %v", err)
	}
	asig, err := bls.AggregateSignatures(sigs)
	if err != nil {
		tb.Fatalf("aggregate sigs: %v", err)
	}
	return bls.SignatureToBytes(asig), apk
}

// beamCertBytes is the Beam leg wire cost: the aggregate signature (48 B
// compressed BLS12-381 G1) is what the cert carries; the aggregate public key
// is resolved from the validator-set era, not carried per block.
func (f *laneFixtures) beamCertBytes() int { return len(f.blsAggSig) }

func (f *laneFixtures) verifyBeam(tb testing.TB) {
	sig, err := bls.SignatureFromBytes(f.blsAggSig)
	if err != nil {
		tb.Fatalf("bls SignatureFromBytes: %v", err)
	}
	if !bls.Verify(f.blsAggPK, sig, f.blsMsg) {
		tb.Fatal("beam BLS verify failed")
	}
}

// ---------------------------------------------------------------------------
// Pulsar lanes.
// ---------------------------------------------------------------------------

// verifyPulsarTALUS is the compact O(1) Pulsar path: ONE FIPS-204 ML-DSA-65
// group-signature verify (EvidencePulsarThresholdMLDSA).
func (f *laneFixtures) verifyPulsarTALUS(tb testing.TB) {
	if !verifyPulsarLeg(f.pulsarMsg, f.pulsarGK, f.pulsarSig) {
		tb.Fatal("pulsar TALUS verify failed")
	}
}

func (f *laneFixtures) pulsarTALUSCertBytes() int { return len(f.pulsarSig) }

// verifyPulsarSampled is the DEFAULT HYBRID_PQ path: re-derive the unbiasable
// committee plan + subject M and stock-verify r dealerless committee ML-DSA
// signatures.
func (f *laneFixtures) verifyPulsarSampled(tb testing.TB) {
	if _, err := VerifyPulsarSampled(f.sampled.req); err != nil {
		tb.Fatalf("pulsar sampled verify failed: %v", err)
	}
}

// pulsarSampledCertBytes sums the wire fields of the r committee certs + the
// plan hash + the (r,m) header (no MarshalBinary exists; this is the honest
// field-sum estimate). Per committee: CommitteeID(32) + KeyEraID(8) +
// Generation(8) + PubKeyHash(len) + Signature(len ML-DSA-65 ≈ 3309).
func (f *laneFixtures) pulsarSampledCertBytes() int {
	total := len(f.sampled.cert.PlanHash) + 4 // PlanHash + r,m (uint16 each)
	for i := range f.sampled.cert.CommitteeCerts {
		cc := &f.sampled.cert.CommitteeCerts[i]
		total += 32 + 8 + 8 + len(cc.PubKeyHash) + len(cc.Signature)
	}
	return total
}

// ---------------------------------------------------------------------------
// Corona lane.
// ---------------------------------------------------------------------------

func (f *laneFixtures) verifyCorona(tb testing.TB) {
	if !coronaThreshold.Verify(f.coronaGK, string(f.coronaMsg), f.coronaSig) {
		tb.Fatal("corona verify failed")
	}
}

func (f *laneFixtures) coronaCertBytes() int { return len(EncodeCoronaSig(f.coronaSig)) }

// ---------------------------------------------------------------------------
// Magnetar-Quorum lane (N independent SLH-DSA-192s).
// ---------------------------------------------------------------------------

type benchMagnetarFx struct {
	n          int
	mc         *MagnetarQuorumCert
	policy     *envPolicy
	validators *envValidators
	cert       *ConsensusCert
	msg        []byte
}

func buildBenchMagnetar(tb testing.TB, n int) *benchMagnetarFx {
	signers := make([]*testSigner, n)
	for i := 0; i < n; i++ {
		signers[i] = newSLHDSASigner(tb, byte(i+1), 100)
	}
	sc := buildScenario(tb, signers, uint64(n*100), nil)

	mc, err := BuildMagnetarQuorumCert(magnetarDirectSuite, sc.cert)
	if err != nil {
		tb.Fatalf("BuildMagnetarQuorumCert: %v", err)
	}
	magLeg := LegSpec{Kind: LegMagnetarSLHDSA, ParamSetID: slhParam}
	policy := &envPolicy{
		required:        []LegSpec{magLeg},
		allow:           map[legModeParam]bool{{LegMagnetarSLHDSA, EvidenceMagnetarRollup, slhParam}: true},
		thresholdWeight: sc.cert.QuorumThreshold,
		classical:       map[ClassicalScheme]bool{},
	}
	validators := &envValidators{
		root: sc.cert.ValidatorSetRoot, epoch: sc.env.Epoch,
		cfg: sc.cfg, env: sc.env, classKeys: map[ClassicalScheme][]byte{},
	}
	cert := newCertForBH(policy, sc.cert.ChainID, sc.cert.Epoch, sc.cert.Height, sc.cert.Round,
		sc.cert.ValueHash, sc.cert.ValidatorSetRoot,
		[]LegEvidence{{Leg: magLeg, Mode: EvidenceMagnetarRollup, Payload: mc.Encode()}})
	cert.AggregateWeight = sc.cert.AggregateWeight
	return &benchMagnetarFx{
		n: n, mc: mc, policy: policy, validators: validators, cert: cert,
		msg: consensusCertMessage(cert, HashRequiredLegs(policy.RequiredLegs())),
	}
}

func (x *benchMagnetarFx) verify(tb testing.TB) {
	if err := VerifyMagnetarQuorum(x.policy, x.validators, x.cert, x.msg, x.mc); err != nil {
		tb.Fatalf("VerifyMagnetarQuorum: %v", err)
	}
}

func (x *benchMagnetarFx) certBytes() int { return len(x.mc.Encode()) }

// ---------------------------------------------------------------------------
// P3Q rollup lane (N independent ML-DSA-65).
// ---------------------------------------------------------------------------

type benchP3QFx struct {
	n          int
	leg        LegEvidence
	policy     *envPolicy
	validators *envValidators
	cert       *ConsensusCert
	msg        []byte
}

func buildBenchP3Q(tb testing.TB, n int) *benchP3QFx {
	signers := make([]*testSigner, n)
	for i := 0; i < n; i++ {
		signers[i] = newMLDSASigner(tb, byte(i+1), 100)
	}
	sc := buildScenario(tb, signers, uint64(n*100), nil)
	innerWire, err := sc.cert.MarshalBinary()
	if err != nil {
		tb.Fatalf("marshal inner cert: %v", err)
	}
	leg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	policy := &envPolicy{
		required:        []LegSpec{leg},
		allow:           map[legModeParam]bool{{LegPulsarMLDSA, EvidenceP3QRollup, pqParam}: true},
		thresholdWeight: sc.cert.QuorumThreshold,
		classical:       map[ClassicalScheme]bool{},
	}
	validators := &envValidators{
		root: sc.cert.ValidatorSetRoot, epoch: sc.env.Epoch,
		cfg: sc.cfg, env: sc.env, classKeys: map[ClassicalScheme][]byte{},
	}
	p3qLeg := LegEvidence{
		Leg:  leg,
		Mode: EvidenceP3QRollup,
		Payload: encodeP3QRollupPayload(&P3QRollupPayload{
			SuiteID:    p3qSuiteDirect,
			RollupRoot: P3QRollupRoot(p3qSuiteDirect, innerWire),
			CertSet:    innerWire,
		}),
	}
	cert := newCertForBH(policy, sc.cert.ChainID, sc.cert.Epoch, sc.cert.Height, sc.cert.Round,
		sc.cert.ValueHash, sc.cert.ValidatorSetRoot, []LegEvidence{p3qLeg})
	cert.AggregateWeight = sc.cert.AggregateWeight
	return &benchP3QFx{
		n: n, leg: p3qLeg, policy: policy, validators: validators, cert: cert,
		msg: consensusCertMessage(cert, HashRequiredLegs(policy.RequiredLegs())),
	}
}

func (x *benchP3QFx) verify(tb testing.TB) {
	if err := VerifyP3QRollupLeg(x.policy, x.validators, x.cert, x.msg, x.leg); err != nil {
		tb.Fatalf("VerifyP3QRollupLeg: %v", err)
	}
}

func (x *benchP3QFx) certBytes() int { return len(x.leg.Payload) }

// ===========================================================================
// Per-lane VERIFY benchmarks (real ns/op from the framework).
// ===========================================================================

func BenchmarkPQVerifyLane(b *testing.B) {
	f := fixtures(b)

	b.Run("Beam-BLS", func(b *testing.B) {
		b.ReportMetric(float64(f.beamCertBytes()), "cert_bytes")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f.verifyBeam(b)
		}
	})
	b.Run("Pulsar-TALUS-compact", func(b *testing.B) {
		b.ReportMetric(float64(f.pulsarTALUSCertBytes()), "cert_bytes")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f.verifyPulsarTALUS(b)
		}
	})
	b.Run("Pulsar-Sampled-r8", func(b *testing.B) {
		b.ReportMetric(float64(f.pulsarSampledCertBytes()), "cert_bytes")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f.verifyPulsarSampled(b)
		}
	})
	b.Run("Corona-Ringtail", func(b *testing.B) {
		b.ReportMetric(float64(f.coronaCertBytes()), "cert_bytes")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f.verifyCorona(b)
		}
	})
	b.Run(fmt.Sprintf("Magnetar-Quorum-N%d", f.magnetar.n), func(b *testing.B) {
		b.ReportMetric(float64(f.magnetar.certBytes()), "cert_bytes")
		b.ReportMetric(float64(f.magnetar.n), "signers")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f.magnetar.verify(b)
		}
	})
	for _, n := range p3qBenchNs {
		x := f.p3q[n]
		b.Run(fmt.Sprintf("P3Q-Direct-N%d", n), func(b *testing.B) {
			b.ReportMetric(float64(x.certBytes()), "cert_bytes")
			b.ReportMetric(float64(x.n), "signers")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				x.verify(b)
			}
		})
	}
}

// ===========================================================================
// Per-lane SIGN / produce benchmarks (proposer-side cost).
// ===========================================================================

func BenchmarkPQSignLane(b *testing.B) {
	msg := randMsg()

	// One BLS validator signature (the Beam per-validator contribution).
	b.Run("BLS-sign", func(b *testing.B) {
		sk, err := bls.NewSecretKey()
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := sk.Sign(msg); err != nil {
				b.Fatal(err)
			}
		}
	})

	// One ML-DSA-65 signature = the per-validator Pulsar-sampled committee sign
	// AND the per-validator P3Q sign.
	b.Run("MLDSA65-sign", func(b *testing.B) {
		s := newMLDSASigner(b, 0x01, 100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = s.sign(b, msg, nil)
		}
	})

	// One SLH-DSA-192s signature = the per-validator Magnetar-Quorum sign (the
	// hot-path gate: each validator signs independently, in parallel).
	b.Run("SLHDSA192s-sign", func(b *testing.B) {
		s := newSLHDSASigner(b, 0x01, 100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = s.sign(b, msg, nil)
		}
	})

	// Full Corona 2-round threshold ceremony (Round1 + Round2 + Finalize) over a
	// t-of-n committee = the Corona produce cost.
	b.Run(fmt.Sprintf("Corona-2round-t%d-n%d", coronaBenchT, coronaBenchN), func(b *testing.B) {
		cmsg := randMsg()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _ = signCoronaLegMultiNode(b, coronaBenchT, coronaBenchT, coronaBenchN, cmsg)
		}
	})
}

// ===========================================================================
// Composed policy-tier summary (sorts last; prints the decision table).
// ===========================================================================

// laneCost is one lane's measured verify latency + cert size.
type laneCost struct {
	verifyNs  int64
	certBytes int
}

// measureNs runs fn iters times and returns the median-of-3 per-call ns. A
// fixed-iteration timer (not b.N) so every lane is measured identically and the
// composed tier numbers are self-consistent.
func measureNs(iters int, fn func()) int64 {
	if iters < 1 {
		iters = 1
	}
	samples := make([]int64, 3)
	for s := range samples {
		start := time.Now()
		for i := 0; i < iters; i++ {
			fn()
		}
		samples[s] = time.Since(start).Nanoseconds() / int64(iters)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[1]
}

func BenchmarkPQPolicyTiers_zzSummary(b *testing.B) {
	f := fixtures(b)

	// Iteration counts tuned per-lane: cheap lanes get many iters, O(N)
	// SLH-DSA the fewest (each verify is ~N×1.35ms).
	beam := laneCost{measureNs(2000, func() { f.verifyBeam(b) }), f.beamCertBytes()}
	pulsarTALUS := laneCost{measureNs(2000, func() { f.verifyPulsarTALUS(b) }), f.pulsarTALUSCertBytes()}
	pulsarSampled := laneCost{measureNs(300, func() { f.verifyPulsarSampled(b) }), f.pulsarSampledCertBytes()}
	corona := laneCost{measureNs(2000, func() { f.verifyCorona(b) }), f.coronaCertBytes()}
	magnetar := laneCost{measureNs(20, func() { f.magnetar.verify(b) }), f.magnetar.certBytes()}

	p3q := map[int]laneCost{}
	for _, n := range p3qBenchNs {
		x := f.p3q[n]
		iters := 200
		if n >= 64 {
			iters = 40
		}
		p3q[n] = laneCost{measureNs(iters, func() { x.verify(b) }), x.certBytes()}
	}
	p3qTier := p3q[magnetarBenchN] // P3Q at the same N as Magnetar for the max tier

	// Compose the five tiers. Pulsar lane in the tiers = the sampled cert (the
	// default HYBRID_PQ); the TALUS compact alternative is reported separately.
	type tier struct {
		name  string
		cost  laneCost
		legs  string
	}
	sum := func(parts ...laneCost) laneCost {
		var out laneCost
		for _, p := range parts {
			out.verifyNs += p.verifyNs
			out.certBytes += p.certBytes
		}
		return out
	}
	tiers := []tier{
		{"1. BLS (BLS_FAST)", beam, "Beam"},
		{"2. BLS+Corona", sum(beam, corona), "Beam∧Corona"},
		{"3. BLS+Pulsar (HYBRID_PQ)", sum(beam, pulsarSampled), "Beam∧Pulsar-sampled"},
		{"4. BLS+Corona+Pulsar (STRICT_DUAL_PQ)", sum(beam, corona, pulsarSampled), "Beam∧Pulsar∧Corona"},
		{fmt.Sprintf("5. POLARIS_MAX (+Magnetar/N%d)", magnetarBenchN), sum(beam, corona, pulsarSampled, magnetar), "Beam∧Pulsar∧Corona∧Magnetar"},
		{fmt.Sprintf("5b. POLARIS_MAX+P3Q (+P3Q/N%d)", magnetarBenchN), sum(beam, corona, pulsarSampled, magnetar, p3qTier), "…∧Magnetar∧P3Q(fallback)"},
	}

	usf := func(ns int64) float64 { return float64(ns) / 1000.0 }
	base := beam

	// Printed via fmt.Printf (stdout): go test TRUNCATES large b.Log output in
	// benchmarks ("... [output truncated]"); stdout is passed through verbatim.
	p := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	p("\n================= PER-LANE MEASURED COST (real go test -bench, M1 Max) =================")
	p("%-26s %14s %14s", "Lane", "Verify (us)", "Cert (bytes)")
	p("%s", dashes(56))
	laneRow := func(name string, c laneCost) { p("%-26s %14.2f %14d", name, usf(c.verifyNs), c.certBytes) }
	laneRow("Beam (BLS aggregate)", beam)
	laneRow("Pulsar TALUS (1 ML-DSA)", pulsarTALUS)
	laneRow("Pulsar sampled (r=8)", pulsarSampled)
	laneRow("Corona (Ring-LWE)", corona)
	laneRow(fmt.Sprintf("Magnetar (N=%d SLH-DSA)", magnetarBenchN), magnetar)
	for _, n := range p3qBenchNs {
		laneRow(fmt.Sprintf("P3Q direct (N=%d ML-DSA)", n), p3q[n])
	}

	p("\n================= COMPOSED POLICY TIERS (AND-mode = Σ lanes) =================")
	p("%-40s %12s %12s %10s %10s", "Tier", "Verify(us)", "Cert(KB)", "xVerify", "xCert")
	p("%s", dashes(88))
	for _, t := range tiers {
		p("%-40s %12.2f %12.2f %9.1fx %9.1fx",
			t.name,
			usf(t.cost.verifyNs),
			float64(t.cost.certBytes)/1024.0,
			float64(t.cost.verifyNs)/float64(base.verifyNs),
			float64(t.cost.certBytes)/float64(base.certBytes),
		)
	}

	p("\nNotes:")
	p("  - Pulsar lane in tiers = sampled cert (default HYBRID_PQ, n=8,t=7,m=12,r=8).")
	p("  - Pulsar TALUS compact (1 ML-DSA verify, %d B) is the O(1) alternative to the sampled cert.", pulsarTALUS.certBytes)
	p("  - Corona verify is O(1) in committee size; only SIGN (2-round) scales with n.")
	p("  - Magnetar/P3Q verify are O(N); table N=%d. Mainnet quorum N=64 ~= 4x the Magnetar row.", magnetarBenchN)
	p("  - POLARIS_MAX shipped policy = Beam^Pulsar^Corona^Magnetar; P3Q is the Pulsar-OR fallback (RECOVERY_MODE), shown stacked in 5b for completeness.")

	// Keep the harness happy (this benchmark is a one-shot table printer).
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
	}
}

func dashes(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = '-'
	}
	return string(out)
}
