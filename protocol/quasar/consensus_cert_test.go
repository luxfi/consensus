// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// consensus_cert_test.go — the ConsensusCert ENVELOPE attack corpus. Every
// named test asserts an ATTACK IS REJECTED (a typed error, never a panic). The
// envelope sits on the already-green WeightedQuorumCert foundation; these tests
// pin the 13 envelope invariants at the envelope level, reusing the foundation's
// real-key signer harness (quorum_cert_test.go) for the weighted-sig-set leg.
package quasar

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luxfi/crypto/bls"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// ----------------------------------------------------------------------------
// Envelope test harness — real policy / validator-set / cert over the
// foundation's real-key WeightedQuorumCert.
// ----------------------------------------------------------------------------

// envPolicy is a concrete ConsensusCertPolicy for tests. The required-leg set,
// the allowed (kind, mode, param) triples, the threshold weight, and the
// classical-scheme allow-list are all explicit and POLICY-owned.
type envPolicy struct {
	required        []LegSpec
	allow           map[legModeParam]bool
	thresholdWeight uint64
	classical       map[ClassicalScheme]bool
}

type legModeParam struct {
	kind  LegKind
	mode  EvidenceMode
	param uint8
}

func (p *envPolicy) RequiredLegs() []LegSpec { return p.required }
func (p *envPolicy) Allows(leg LegSpec, mode EvidenceMode, paramSet uint8) bool {
	return p.allow[legModeParam{leg.Kind, mode, paramSet}]
}
func (p *envPolicy) ThresholdWeight() uint64 { return p.thresholdWeight }
func (p *envPolicy) AllowsClassicalScheme(scheme ClassicalScheme) bool {
	return p.classical[scheme]
}

// envStore resolves a single fixed policy (the store is keyed in production by
// (chain, epoch, policy-id); tests pin one policy and assert the lookup args
// flow through).
type envStore struct {
	policy ConsensusCertPolicy
	err    error
}

func (s *envStore) Policy(chainID uint32, epoch uint64, policyID uint32) (ConsensusCertPolicy, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.policy, nil
}

// envValidators wraps the foundation scenario as a ConsensusValidatorSet.
type envValidators struct {
	root      [48]byte
	epoch     uint64
	cfg       QuorumVerifierConfig
	env       QuorumMessageEnvelope
	pulsarKey []byte
	coronaKey *coronaThreshold.GroupKey
	classKeys map[ClassicalScheme][]byte
}

func (v *envValidators) Root() [48]byte                  { return v.root }
func (v *envValidators) Epoch() uint64                   { return v.epoch }
func (v *envValidators) WeightedConfig() QuorumVerifierConfig { return v.cfg }
func (v *envValidators) WeightedEnvelope() QuorumMessageEnvelope { return v.env }
func (v *envValidators) ThresholdGroupKey(kind LegKind) (ThresholdGroupKey, bool) {
	switch kind {
	case LegPulsarMLDSA:
		if len(v.pulsarKey) == 0 {
			return ThresholdGroupKey{}, false
		}
		return ThresholdGroupKey{Kind: LegPulsarMLDSA, PulsarGroupKey: v.pulsarKey}, true
	case LegCoronaLattice:
		if v.coronaKey == nil {
			return ThresholdGroupKey{}, false
		}
		return ThresholdGroupKey{Kind: LegCoronaLattice, CoronaGroupKey: v.coronaKey}, true
	default:
		return ThresholdGroupKey{}, false
	}
}
func (v *envValidators) ClassicalAggregateKey(scheme ClassicalScheme) ([]byte, bool) {
	k, ok := v.classKeys[scheme]
	return k, ok
}

// envFixture is a complete, VALID ConsensusCert over a single weighted-sig-set
// leg, plus the policy + validator set that verify it. The starting point for
// every attack test (mutate one thing, assert rejection).
type envFixture struct {
	store      *envStore
	validators *envValidators
	policy     *envPolicy
	cert       *ConsensusCert
}

// pqParam is the ML-DSA-65 parameter byte the weighted-sig-set leg is built
// under (matches QuorumSchemeMLDSA65 = 0x42).
const pqParam = uint8(QuorumSchemeMLDSA65)

// buildEnvFixture assembles a valid single-leg (Pulsar/weighted-sig-set)
// ConsensusCert over the foundation's real 4-signer ML-DSA scenario.
func buildEnvFixture(t *testing.T) envFixture {
	t.Helper()
	sc := fourMLDSA(t) // real inner WeightedQuorumCert, Σweight=100, threshold=100

	innerWire, err := sc.cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal inner cert: %v", err)
	}

	requiredLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	policy := &envPolicy{
		required: []LegSpec{requiredLeg},
		allow: map[legModeParam]bool{
			{LegPulsarMLDSA, EvidenceWeightedSigSet, pqParam}: true,
		},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}

	validators := &envValidators{
		root:      sc.cert.ValidatorSetRoot,
		epoch:     sc.env.Epoch,
		cfg:       sc.cfg, // MinThreshold gets re-pinned by the leg
		env:       sc.env,
		classKeys: map[ClassicalScheme][]byte{},
	}

	cert := newCertForBH(policy, sc.cert.ChainID, sc.cert.Epoch, sc.cert.Height, sc.cert.Round,
		sc.cert.ValueHash, sc.cert.ValidatorSetRoot,
		[]LegEvidence{{
			Leg:     requiredLeg,
			Mode:    EvidenceWeightedSigSet,
			Payload: innerWire,
		}})
	cert.AggregateWeight = sc.cert.AggregateWeight

	return envFixture{store: store, validators: validators, policy: policy, cert: cert}
}

// newCertForBH is the cert constructor: explicit [32]byte block hash, a
// correctly-derived RequiredLegsRoot (from policy), and a fixed signer root.
// Tests that attack the required-legs root / signer root override them after
// construction.
func newCertForBH(policy ConsensusCertPolicy, chainID uint32, epoch, height uint64, round uint32,
	blockHash [32]byte, vsetRoot [48]byte, evidence []LegEvidence) *ConsensusCert {
	var signerRoot [32]byte
	for i := range signerRoot {
		signerRoot[i] = 0x5A
	}
	return &ConsensusCert{
		Version:          consensusCertVersion,
		Profile:          1,
		ChainID:          chainID,
		Epoch:            epoch,
		Height:           height,
		Round:            round,
		BlockHash:        blockHash,
		ValidatorSetRoot: vsetRoot,
		PolicyID:         7,
		RequiredLegsRoot: HashRequiredLegs(policy.RequiredLegs()),
		SignerRoot:       signerRoot,
		Evidence:         evidence,
	}
}

// bindBlockHash sets the envelope block hash to the inner cert's ValueHash (the
// weighted-sig-set leg cross-checks them). fourMLDSA's inner ValueHash is the
// fixed 0xAB.. [32]byte.
func (f *envFixture) bindBlockHash(t *testing.T) {
	t.Helper()
	inner := scenarioFromFixture(t, f)
	f.cert.BlockHash = inner.ValueHash
}

// scenarioFromFixture re-decodes the inner cert from the fixture's evidence so
// tests can read the inner cert's fields (e.g. its ValueHash).
func scenarioFromFixture(t *testing.T, f *envFixture) *WeightedQuorumCert {
	t.Helper()
	for _, ev := range f.cert.Evidence {
		if ev.Mode == EvidenceWeightedSigSet {
			inner, err := UnmarshalWeightedQuorumCert(ev.Payload)
			if err != nil {
				t.Fatalf("decode inner: %v", err)
			}
			return inner
		}
	}
	t.Fatal("no weighted-sig-set evidence in fixture")
	return nil
}

// validFixture builds a fixture whose envelope block hash is bound to the inner
// cert's value hash — i.e. a cert that VERIFIES. The base for mutate-one tests.
func validFixture(t *testing.T) envFixture {
	t.Helper()
	f := buildEnvFixture(t)
	f.bindBlockHash(t)
	return f
}

// ----------------------------------------------------------------------------
// Sanity: the valid fixture verifies (so a rejection in an attack test is the
// attack, not a broken fixture).
// ----------------------------------------------------------------------------

func TestConsensusCert_HappyPath(t *testing.T) {
	f := validFixture(t)
	if err := VerifyConsensusCert(f.store, f.validators, f.cert); err != nil {
		t.Fatalf("valid consensus cert rejected: %v", err)
	}
}

// ----------------------------------------------------------------------------
// I1/I2 — required legs are POLICY-derived, never cert-derived.
// ----------------------------------------------------------------------------

// TestRejectsCertSuppliedRequiredLegs proves the cert cannot weaken itself by
// claiming a SMALLER required-leg set. We shrink the cert's RequiredLegsRoot to
// the commitment of an empty set; the verifier recomputes the root from POLICY
// and rejects.
func TestRejectsCertSuppliedRequiredLegs(t *testing.T) {
	f := validFixture(t)
	// Forge: claim a required-legs root for a DIFFERENT (empty) leg set — the
	// cert trying to assert its own (weaker) requirement.
	f.cert.RequiredLegsRoot = HashRequiredLegs(nil)
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrRequiredLegsRootMismatch) {
		t.Fatalf("cert-supplied required legs err = %v, want ErrRequiredLegsRootMismatch", err)
	}
}

// TestRejectsRequiredLegsRootMismatch is the direct mismatch: any tamper of the
// RequiredLegsRoot away from HashRequiredLegs(policy.RequiredLegs()) is rejected.
func TestRejectsRequiredLegsRootMismatch(t *testing.T) {
	f := validFixture(t)
	f.cert.RequiredLegsRoot[0] ^= 0xFF
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrRequiredLegsRootMismatch) {
		t.Fatalf("required-legs-root tamper err = %v, want ErrRequiredLegsRootMismatch", err)
	}
}

// ----------------------------------------------------------------------------
// I5/I12 — every required leg must have evidence.
// ----------------------------------------------------------------------------

// TestRejectsMissingRequiredPQLeg: the policy requires a Pulsar PQ leg but the
// cert carries no evidence for it. The verifier rejects (the required-legs root
// still matches because the requirement is policy-derived; the evidence is what
// is missing).
func TestRejectsMissingRequiredPQLeg(t *testing.T) {
	f := validFixture(t)
	// Strip the evidence but keep the (policy-derived) required-legs root intact.
	f.cert.Evidence = nil
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrMissingRequiredLeg) {
		t.Fatalf("missing required PQ leg err = %v, want ErrMissingRequiredLeg", err)
	}
}

// ----------------------------------------------------------------------------
// I11 — classical-only is forbidden under a PQ policy.
// ----------------------------------------------------------------------------

// TestRejectsClassicalOnlyUnderPQPolicy: the policy requires a Pulsar PQ leg;
// the cert offers ONLY a classical aggregate (no PQ evidence). The envelope must
// reject — classical cannot stand in for a required PQ leg.
func TestRejectsClassicalOnlyUnderPQPolicy(t *testing.T) {
	f := validFixture(t)
	// Replace the PQ evidence with a classical aggregate under a Classical leg
	// kind. The policy STILL requires LegPulsarMLDSA, so the Pulsar leg has no
	// evidence → ErrMissingRequiredLeg (the envelope-level classical-only guard:
	// a required PQ leg is unsatisfied).
	f.cert.Evidence = []LegEvidence{{
		Leg:  LegSpec{Kind: LegClassical, ParamSetID: uint8(ClassicalSchemeBLS12381)},
		Mode: EvidenceClassicalAggregate,
		Payload: encodeClassicalAggregatePayload(&ClassicalAggregatePayload{
			Scheme: ClassicalSchemeBLS12381, Payload: []byte{0x01},
		}),
	}}
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrMissingRequiredLeg) {
		t.Fatalf("classical-only under PQ policy err = %v, want ErrMissingRequiredLeg", err)
	}
}

// TestClassicalAggregateCannotSatisfyPQLeg: even if the cert LABELS classical
// evidence with a PQ leg kind, the classical-aggregate verifier refuses (I10).
// We make the policy ALLOW the (Pulsar, ClassicalAggregate, param) triple to get
// past the policy gate and reach the helper, proving the helper itself enforces
// kind == LegClassical.
func TestClassicalAggregateCannotSatisfyPQLeg(t *testing.T) {
	f := validFixture(t)
	p := f.policy
	// Allow the mislabeled triple so dispatch reaches VerifyClassicalAggregateLeg.
	p.allow[legModeParam{LegPulsarMLDSA, EvidenceClassicalAggregate, pqParam}] = true
	f.cert.Evidence = []LegEvidence{{
		Leg:  LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}, // PQ kind...
		Mode: EvidenceClassicalAggregate,                         // ...classical evidence
		Payload: encodeClassicalAggregatePayload(&ClassicalAggregatePayload{
			Scheme: ClassicalSchemeBLS12381, Payload: []byte{0x01},
		}),
	}}
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrClassicalCannotSatisfyPQLeg) {
		t.Fatalf("classical evidence on PQ leg err = %v, want ErrClassicalCannotSatisfyPQLeg", err)
	}
}

// TestClassicalAggregateAcceptedOnlyWithRequiredPQLegs proves the positive
// case: under a policy that requires BOTH a PQ leg (satisfied by weighted-sig-set
// PQ evidence) AND a classical leg, the classical aggregate is accepted — but
// only because the PQ leg was satisfied first. We use a real BLS aggregate.
func TestClassicalAggregateAcceptedOnlyWithRequiredPQLegs(t *testing.T) {
	f := validFixture(t)
	sc := scenarioFromFixture(t, &f)

	// Build a real BLS aggregate over the ENVELOPE message (what the classical
	// leg verifies against). The envelope message depends on the cert header,
	// which includes the required-legs root — so we compute it AFTER fixing the
	// two-leg policy.
	pqLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	classLeg := LegSpec{Kind: LegClassical, ParamSetID: uint8(ClassicalSchemeBLS12381)}
	f.policy.required = []LegSpec{pqLeg, classLeg}
	f.policy.allow[legModeParam{LegClassical, EvidenceClassicalAggregate, uint8(ClassicalSchemeBLS12381)}] = true
	f.policy.classical[ClassicalSchemeBLS12381] = true
	f.cert.RequiredLegsRoot = HashRequiredLegs(f.policy.required)

	// Now the envelope message is determined; sign it with a real BLS key.
	msg := consensusCertMessage(f.cert, HashRequiredLegs(f.policy.required))
	aggPub, aggSig := realBLSAggregate(t, msg)
	f.validators.classKeys[ClassicalSchemeBLS12381] = aggPub

	innerWire, err := sc.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}
	f.cert.Evidence = []LegEvidence{
		{Leg: pqLeg, Mode: EvidenceWeightedSigSet, Payload: innerWire},
		{Leg: classLeg, Mode: EvidenceClassicalAggregate, Payload: encodeClassicalAggregatePayload(&ClassicalAggregatePayload{
			Scheme: ClassicalSchemeBLS12381, Payload: aggSig,
		})},
	}

	if err := VerifyConsensusCert(f.store, f.validators, f.cert); err != nil {
		t.Fatalf("PQ + classical cert rejected: %v", err)
	}
}

// ----------------------------------------------------------------------------
// I6 — (kind, mode, param-set) must be policy-permitted.
// ----------------------------------------------------------------------------

// TestRejectsDisallowedParamSet: the cert's evidence names a param byte the
// policy does not permit for this (kind, mode). The verifier rejects before any
// signature math.
func TestRejectsDisallowedParamSet(t *testing.T) {
	f := validFixture(t)
	// The evidence claims ML-DSA-87 (0x43); the policy only permits 0x42.
	f.cert.Evidence[0].Leg.ParamSetID = uint8(QuorumSchemeMLDSA87)
	// The required-legs root is over the policy set (param 0x42), unchanged.
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrDisallowedEvidenceMode) {
		t.Fatalf("disallowed param set err = %v, want ErrDisallowedEvidenceMode", err)
	}
}

// ----------------------------------------------------------------------------
// I7 — SLH-DSA may not be threshold-signed.
// ----------------------------------------------------------------------------

// TestRejectsSLHDSAThresholdSigMode: a Magnetar (SLH-DSA) leg selecting
// EvidenceThresholdSig is a hard reject. We require a Magnetar leg, allow the
// (Magnetar, ThresholdSig, param) triple in policy, and confirm the leg verifier
// refuses regardless.
func TestRejectsSLHDSAThresholdSigMode(t *testing.T) {
	magLeg := LegSpec{Kind: LegMagnetarSLHDSA, ParamSetID: uint8(QuorumSchemeSLHDSA192s)}
	policy := &envPolicy{
		required: []LegSpec{magLeg},
		allow: map[legModeParam]bool{
			{LegMagnetarSLHDSA, EvidenceThresholdSig, uint8(QuorumSchemeSLHDSA192s)}: true,
		},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}
	var vsetRoot [48]byte
	for i := range vsetRoot {
		vsetRoot[i] = 0x11
	}
	validators := &envValidators{root: vsetRoot, epoch: 1, classKeys: map[ClassicalScheme][]byte{}}

	var bh [32]byte
	cert := newCertForBH(policy, 1, 1, 1, 1, bh, vsetRoot, []LegEvidence{{
		Leg:  magLeg,
		Mode: EvidenceThresholdSig,
		Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
			Signature:      []byte{0x01},
			Accountability: &ThresholdAccountability{AggregateWeight: 100},
		}),
	}})

	err := VerifyConsensusCert(store, validators, cert)
	if !errors.Is(err, ErrSLHDSAThresholdSigForbidden) {
		t.Fatalf("SLH-DSA threshold-sig err = %v, want ErrSLHDSAThresholdSigForbidden", err)
	}
}

// ----------------------------------------------------------------------------
// I8 — threshold-sig accountability.
// ----------------------------------------------------------------------------

// TestRejectsThresholdSigWithoutAccountability: a threshold-sig leg with no
// accountability binding is rejected — even before the signature is checked. A
// threshold sig that binds no signer set/weight is unacceptable.
func TestRejectsThresholdSigWithoutAccountability(t *testing.T) {
	pulsarLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	policy := &envPolicy{
		required:        []LegSpec{pulsarLeg},
		allow:           map[legModeParam]bool{{LegPulsarMLDSA, EvidenceThresholdSig, pqParam}: true},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}
	var vsetRoot [48]byte
	for i := range vsetRoot {
		vsetRoot[i] = 0x22
	}
	validators := &envValidators{
		root: vsetRoot, epoch: 1, classKeys: map[ClassicalScheme][]byte{},
		pulsarKey: []byte("a-group-key"),
	}
	var bh [32]byte
	cert := newCertForBH(policy, 1, 1, 1, 1, bh, vsetRoot, []LegEvidence{{
		Leg:  pulsarLeg,
		Mode: EvidenceThresholdSig,
		Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
			Signature:      []byte{0x01, 0x02},
			Accountability: nil, // <-- no accountability
		}),
	}})

	err := VerifyConsensusCert(store, validators, cert)
	if !errors.Is(err, ErrThresholdSigWithoutAccountability) {
		t.Fatalf("threshold-sig without accountability err = %v, want ErrThresholdSigWithoutAccountability", err)
	}
}

// TestRejectsThresholdSigSignerRootMismatch: accountability whose SignerRoot
// disagrees with the cert SignerRoot is rejected (the signer set the cert claims
// is not the one bound into the signed message).
func TestRejectsThresholdSigSignerRootMismatch(t *testing.T) {
	pulsarLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	policy := &envPolicy{
		required:        []LegSpec{pulsarLeg},
		allow:           map[legModeParam]bool{{LegPulsarMLDSA, EvidenceThresholdSig, pqParam}: true},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}
	var vsetRoot [48]byte
	for i := range vsetRoot {
		vsetRoot[i] = 0x22
	}
	validators := &envValidators{
		root: vsetRoot, epoch: 1, classKeys: map[ClassicalScheme][]byte{},
		pulsarKey: []byte("a-group-key"),
	}
	var bh [32]byte
	cert := newCertForBH(policy, 1, 1, 1, 1, bh, vsetRoot, nil)
	var wrongRoot [32]byte // all zero, != cert.SignerRoot (0x5A..)
	cert.Evidence = []LegEvidence{{
		Leg:  pulsarLeg,
		Mode: EvidenceThresholdSig,
		Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
			Signature:      []byte{0x01, 0x02},
			Accountability: &ThresholdAccountability{SignerRoot: wrongRoot, AggregateWeight: 100},
		}),
	}}
	err := VerifyConsensusCert(store, validators, cert)
	if !errors.Is(err, ErrSignerRootMismatch) {
		t.Fatalf("signer-root mismatch err = %v, want ErrSignerRootMismatch", err)
	}
}

// TestRejectsInsufficientWeight: accountability weight below the policy
// threshold weight is rejected.
func TestRejectsInsufficientWeight(t *testing.T) {
	pulsarLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	policy := &envPolicy{
		required:        []LegSpec{pulsarLeg},
		allow:           map[legModeParam]bool{{LegPulsarMLDSA, EvidenceThresholdSig, pqParam}: true},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}
	var vsetRoot [48]byte
	for i := range vsetRoot {
		vsetRoot[i] = 0x22
	}
	validators := &envValidators{
		root: vsetRoot, epoch: 1, classKeys: map[ClassicalScheme][]byte{},
		pulsarKey: []byte("a-group-key"),
	}
	var bh [32]byte
	cert := newCertForBH(policy, 1, 1, 1, 1, bh, vsetRoot, nil)
	cert.Evidence = []LegEvidence{{
		Leg:  pulsarLeg,
		Mode: EvidenceThresholdSig,
		Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
			Signature:      []byte{0x01, 0x02},
			Accountability: &ThresholdAccountability{SignerRoot: cert.SignerRoot, AggregateWeight: 99}, // < 100
		}),
	}}
	err := VerifyConsensusCert(store, validators, cert)
	if !errors.Is(err, ErrInsufficientWeight) {
		t.Fatalf("insufficient weight err = %v, want ErrInsufficientWeight", err)
	}
}

// TestThresholdSigLegAcceptsRealCoronaAggregate is the POSITIVE threshold-sig
// path: a REAL Corona (Ring-LWE) threshold aggregate signature over the envelope
// message, with valid accountability, is ACCEPTED. This proves the threshold-sig
// leg wires origin's verifyCoronaLeg against a genuine group key + signature —
// the verification path is live, not faked. Heavy (Corona keygen) → skipped
// under -short, like the foundation's Polaris fixture.
func TestThresholdSigLegAcceptsRealCoronaAggregate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Corona DKG-bound threshold-sig fixture under -short")
	}
	const threshold, n = 2, 3

	coronaLeg := LegSpec{Kind: LegCoronaLattice, ParamSetID: uint8(QuorumSchemeMLDSA65)}
	policy := &envPolicy{
		required:        []LegSpec{coronaLeg},
		allow:           map[legModeParam]bool{{LegCoronaLattice, EvidenceThresholdSig, uint8(QuorumSchemeMLDSA65)}: true},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}

	var vsetRoot [48]byte
	for i := range vsetRoot {
		vsetRoot[i] = 0x33
	}
	var bh [32]byte
	for i := range bh {
		bh[i] = 0xC0
	}
	// Build the cert FIRST (header fixed) so the envelope message is determined,
	// then sign THAT message with the real Corona committee.
	cert := newCertForBH(policy, 9, 9, 9, 9, bh, vsetRoot, nil)
	msg := consensusCertMessage(cert, HashRequiredLegs(policy.RequiredLegs()))

	// Real Corona threshold signing over the envelope message.
	shares, groupKey, err := coronaThreshold.GenerateKeys(threshold, n, nil)
	if err != nil {
		t.Fatalf("corona GenerateKeys: %v", err)
	}
	signers := make([]*coronaThreshold.Signer, threshold)
	ids := make([]int, threshold)
	for i := 0; i < threshold; i++ {
		signers[i] = coronaThreshold.NewSigner(shares[i])
		ids[i] = shares[i].Index
	}
	const sessionID = 27182
	prf := []byte("consensus-cert-corona-prf-32-byte")
	r1 := make(map[int]*coronaThreshold.Round1Data, threshold)
	for i := 0; i < threshold; i++ {
		r1[ids[i]] = signers[i].Round1(sessionID, prf, ids)
	}
	r2 := make(map[int]*coronaThreshold.Round2Data, threshold)
	for i := 0; i < threshold; i++ {
		rr2, rerr := signers[i].Round2(sessionID, string(msg), prf, ids, r1)
		if rerr != nil {
			t.Fatalf("corona Round2[%d]: %v", i, rerr)
		}
		r2[ids[i]] = rr2
	}
	coronaSig, err := signers[0].Finalize(r2)
	if err != nil {
		t.Fatalf("corona Finalize: %v", err)
	}
	sigBytes := EncodeCoronaSig(coronaSig)
	if len(sigBytes) == 0 {
		t.Fatal("corona signature encoded to empty bytes")
	}

	validators := &envValidators{
		root: vsetRoot, epoch: 9, classKeys: map[ClassicalScheme][]byte{},
		coronaKey: groupKey,
	}
	cert.Evidence = []LegEvidence{{
		Leg:  coronaLeg,
		Mode: EvidenceThresholdSig,
		Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
			Signature:      sigBytes,
			Accountability: &ThresholdAccountability{SignerRoot: cert.SignerRoot, AggregateWeight: 150},
		}),
	}}

	if err := VerifyConsensusCert(store, validators, cert); err != nil {
		t.Fatalf("real Corona threshold-sig leg rejected: %v", err)
	}

	// Negative cross-check: a one-byte flip of the threshold signature makes the
	// leg fail at the signature-verification clause (proves the verify is live).
	bad := append([]byte(nil), sigBytes...)
	bad[len(bad)/2] ^= 0xFF
	cert.Evidence[0].Payload = encodeThresholdSigPayload(&ThresholdSigPayload{
		Signature:      bad,
		Accountability: &ThresholdAccountability{SignerRoot: cert.SignerRoot, AggregateWeight: 150},
	})
	if err := VerifyConsensusCert(store, validators, cert); !errors.Is(err, ErrThresholdSigInvalid) {
		t.Fatalf("corrupted Corona sig err = %v, want ErrThresholdSigInvalid", err)
	}
}

// ----------------------------------------------------------------------------
// I3 — validator set is verifier-pinned.
// ----------------------------------------------------------------------------

// TestRejectsWrongValidatorSetRoot: a cert whose ValidatorSetRoot disagrees with
// the committed set's Root() is rejected.
func TestRejectsWrongValidatorSetRoot(t *testing.T) {
	f := validFixture(t)
	f.cert.ValidatorSetRoot[0] ^= 0xFF
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrValidatorSetRootMismatch) {
		t.Fatalf("wrong validator_set_root err = %v, want ErrValidatorSetRootMismatch", err)
	}
}

// ----------------------------------------------------------------------------
// I4 — domain message binds the full tuple (replay / mutation closed).
// ----------------------------------------------------------------------------

// TestRejectsCrossEpochReplay: the same cert presented under a different epoch
// fails — the store would return a policy for the claimed epoch and the message
// binds epoch, so the inner cert's position cross-check fails.
func TestRejectsCrossEpochReplay(t *testing.T) {
	f := validFixture(t)
	f.cert.Epoch++ // replay at a different epoch
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	// The inner weighted-sig-set cert was built at the original epoch; the
	// envelope's position cross-check (inner.Epoch != cert.Epoch) rejects.
	if !errors.Is(err, ErrEvidenceWireCorrupt) {
		t.Fatalf("cross-epoch replay err = %v, want ErrEvidenceWireCorrupt (position mismatch)", err)
	}
}

// TestRejectsCrossChainReplay: the same cert presented under a different chain
// id is rejected (position cross-check on the inner cert).
func TestRejectsCrossChainReplay(t *testing.T) {
	f := validFixture(t)
	f.cert.ChainID++
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrEvidenceWireCorrupt) {
		t.Fatalf("cross-chain replay err = %v, want ErrEvidenceWireCorrupt (position mismatch)", err)
	}
}

// TestRejectsBlockHashMutation: flipping the envelope block hash away from the
// inner cert's value hash is rejected (position cross-check).
func TestRejectsBlockHashMutation(t *testing.T) {
	f := validFixture(t)
	f.cert.BlockHash[0] ^= 0xFF
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrEvidenceWireCorrupt) {
		t.Fatalf("block-hash mutation err = %v, want ErrEvidenceWireCorrupt (position mismatch)", err)
	}
}

// TestRejectsSignerRootMutation: a threshold-sig cert whose SignerRoot is
// mutated (so the accountability echo no longer matches) is rejected.
func TestRejectsSignerRootMutation(t *testing.T) {
	pulsarLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	policy := &envPolicy{
		required:        []LegSpec{pulsarLeg},
		allow:           map[legModeParam]bool{{LegPulsarMLDSA, EvidenceThresholdSig, pqParam}: true},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
	store := &envStore{policy: policy}
	var vsetRoot [48]byte
	for i := range vsetRoot {
		vsetRoot[i] = 0x22
	}
	validators := &envValidators{
		root: vsetRoot, epoch: 1, classKeys: map[ClassicalScheme][]byte{},
		pulsarKey: []byte("a-group-key"),
	}
	var bh [32]byte
	cert := newCertForBH(policy, 1, 1, 1, 1, bh, vsetRoot, nil)
	cert.Evidence = []LegEvidence{{
		Leg:  pulsarLeg,
		Mode: EvidenceThresholdSig,
		Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
			Signature:      []byte{0x01, 0x02},
			Accountability: &ThresholdAccountability{SignerRoot: cert.SignerRoot, AggregateWeight: 100},
		}),
	}}
	// Mutate the cert's SignerRoot AFTER the accountability echo was fixed to the
	// original value → the SignerRoot != accountability.SignerRoot mismatch fires.
	cert.SignerRoot[0] ^= 0xFF
	err := VerifyConsensusCert(store, validators, cert)
	if !errors.Is(err, ErrSignerRootMismatch) {
		t.Fatalf("signer-root mutation err = %v, want ErrSignerRootMismatch", err)
	}
}

// TestRejectsPolicyIDMutation: changing the cert's PolicyID changes the domain
// message; for the weighted-sig-set leg the inner cert is unaffected by PolicyID
// (it binds its own round digest), so this test asserts the envelope message
// itself is a function of PolicyID — a separate cert whose PolicyID differs
// produces a different envelope message (cross-domain non-transferability).
func TestRejectsPolicyIDMutation(t *testing.T) {
	f := validFixture(t)
	msg1 := consensusCertMessage(f.cert, f.cert.RequiredLegsRoot)
	mutated := *f.cert
	mutated.PolicyID++
	msg2 := consensusCertMessage(&mutated, mutated.RequiredLegsRoot)
	if bytes.Equal(msg1, msg2) {
		t.Fatal("policy_id is not bound into the envelope message (msg unchanged after mutation)")
	}
}

// ----------------------------------------------------------------------------
// I12 + foundation reuse — signer-set integrity at the envelope level.
//
// The foundation's weighted-sig-set predicate already enforces duplicate /
// unsorted / out-of-set / weight-overflow rejection (quorum_cert_test.go,
// quorum_cto_attacks_test.go). At the ENVELOPE level a corrupted inner cert
// surfaces as the foundation's typed error verbatim. These tests assert each
// such invariant is reachable and rejected THROUGH the envelope.
// ----------------------------------------------------------------------------

// mutateInner re-decodes the fixture's inner cert, applies a mutation, re-encodes
// it as the evidence payload, and returns the fixture ready to verify.
func mutateInner(t *testing.T, f *envFixture, mut func(*WeightedQuorumCert)) {
	t.Helper()
	inner := scenarioFromFixture(t, f)
	mut(inner)
	wire, err := inner.MarshalBinary()
	if err != nil {
		t.Fatalf("re-marshal mutated inner: %v", err)
	}
	for i := range f.cert.Evidence {
		if f.cert.Evidence[i].Mode == EvidenceWeightedSigSet {
			f.cert.Evidence[i].Payload = wire
		}
	}
}

// TestRejectsDuplicateValidatorID: a duplicate signer in the inner weighted-sig-
// set cert is rejected through the envelope (foundation clause).
func TestRejectsDuplicateValidatorID(t *testing.T) {
	f := validFixture(t)
	mutateInner(t, &f, func(c *WeightedQuorumCert) {
		c.Signers[2] = c.Signers[1] // duplicate id adjacent after sort
		c.SignerCommitment = c.computeSignerCommitment()
	})
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrQCNotStrictlyIncreasing) {
		t.Fatalf("duplicate validator id err = %v, want ErrQCNotStrictlyIncreasing", err)
	}
}

// TestRejectsUnsortedValidatorIDs: unsorted signers in the inner cert are
// rejected through the envelope.
func TestRejectsUnsortedValidatorIDs(t *testing.T) {
	f := validFixture(t)
	mutateInner(t, &f, func(c *WeightedQuorumCert) {
		c.Signers[0], c.Signers[1] = c.Signers[1], c.Signers[0]
		c.SignerCommitment = c.computeSignerCommitment()
	})
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrQCNotStrictlyIncreasing) {
		t.Fatalf("unsorted validator ids err = %v, want ErrQCNotStrictlyIncreasing", err)
	}
}

// TestRejectsOutOfSetValidator: an out-of-set signer (bad Merkle path) in the
// inner cert is rejected through the envelope.
func TestRejectsOutOfSetValidator(t *testing.T) {
	f := validFixture(t)
	mutateInner(t, &f, func(c *WeightedQuorumCert) {
		c.Signers[1].VotingWeight = 999 // leaf no longer matches the committed set
		c.AggregateWeight = c.AggregateWeight - 25 + 999
		c.SignerCommitment = c.computeSignerCommitment()
	})
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrQCMerkleInclusion) {
		t.Fatalf("out-of-set validator err = %v, want ErrQCMerkleInclusion", err)
	}
}

// TestRejectsWeightOverflow: a weight-overflowing inner cert is rejected through
// the envelope. Built directly (not via the prover, which guards overflow at
// assembly) so the VERIFIER's guard is what trips.
func TestRejectsWeightOverflow(t *testing.T) {
	f := overflowFixture(t)
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrQCWeightOverflow) {
		t.Fatalf("weight overflow err = %v, want ErrQCWeightOverflow", err)
	}
}

// TestRejectsAggregateWeightMismatch: an inner cert whose claimed AggregateWeight
// disagrees with Σ signer weight is rejected through the envelope.
func TestRejectsAggregateWeightMismatch(t *testing.T) {
	f := validFixture(t)
	mutateInner(t, &f, func(c *WeightedQuorumCert) {
		c.AggregateWeight++ // claim one more than Σ weight
		c.SignerCommitment = c.computeSignerCommitment()
	})
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrQCAggregateWeight) {
		t.Fatalf("aggregate weight mismatch err = %v, want ErrQCAggregateWeight", err)
	}
}

// TestRejectsDuplicateEvidenceForSameKind: a cert carrying TWO evidence entries
// for the same leg kind is rejected up front — evidence is one-to-one with kind,
// so a forged second entry can never shadow the first.
func TestRejectsDuplicateEvidenceForSameKind(t *testing.T) {
	f := validFixture(t)
	// Append a second (bogus) evidence entry for the SAME Pulsar kind.
	f.cert.Evidence = append(f.cert.Evidence, LegEvidence{
		Leg:     LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam},
		Mode:    EvidenceWeightedSigSet,
		Payload: []byte{0x00, 0x00},
	})
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrDuplicateLegKind) {
		t.Fatalf("duplicate evidence err = %v, want ErrDuplicateLegKind", err)
	}
}

// ----------------------------------------------------------------------------
// I9 — STARK proves the SAME predicate; audit-gated.
// ----------------------------------------------------------------------------

// TestStarkCompressedAndWeightedSigSetSamePredicate pins the predicate identity:
// the stark-compressed mode's public statement equals the WeightedSigSet
// predicate's bound inputs for the SAME cert, AND the backend fails closed until
// audit-gated (it never silently accepts a different/"close-enough" verifier).
func TestStarkCompressedAndWeightedSigSetSamePredicate(t *testing.T) {
	f := validFixture(t)
	msg := consensusCertMessage(f.cert, f.cert.RequiredLegsRoot)

	// The canonical WeightedSigSet statement for this cert (single source of
	// truth shared by VerifyStarkCompressedSigSet's input check).
	want := starkPublicStatement(f.cert, msg)

	// A STARK payload that correctly commits to the SAME statement: the input
	// check PASSES, but the backend is not audit-gated → fail closed.
	pulsarLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	f.policy.allow[legModeParam{LegPulsarMLDSA, EvidenceStarkCompressedSigSet, pqParam}] = true
	f.cert.Evidence = []LegEvidence{{
		Leg:  pulsarLeg,
		Mode: EvidenceStarkCompressedSigSet,
		Payload: encodeStarkCompressedPayload(&StarkCompressedPayload{
			PublicInputs: want, // identical to the WeightedSigSet statement
			Proof:        []byte{0xAA, 0xBB},
		}),
	}}
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrStarkBackendNotAuditGated) {
		t.Fatalf("audit-gate: stark mode err = %v, want ErrStarkBackendNotAuditGated", err)
	}

	// A STARK payload that commits to a DIFFERENT statement is rejected at the
	// predicate-identity check (it would prove a different relation).
	bogus := append([]byte(nil), want...)
	bogus[0] ^= 0xFF
	f.cert.Evidence[0].Payload = encodeStarkCompressedPayload(&StarkCompressedPayload{
		PublicInputs: bogus,
		Proof:        []byte{0xAA, 0xBB},
	})
	err = VerifyConsensusCert(f.store, f.validators, f.cert)
	if !errors.Is(err, ErrStarkPublicInputsMismatch) {
		t.Fatalf("predicate identity: stark mode err = %v, want ErrStarkPublicInputsMismatch", err)
	}

	// Direct assertion of the identity: the statement function is deterministic
	// and equals itself for the same cert+message (the WeightedSigSet predicate
	// inputs ARE the STARK public statement — pinned here so a future divergence
	// trips CI).
	if !bytes.Equal(starkPublicStatement(f.cert, msg), want) {
		t.Fatal("stark public statement is not a deterministic function of the cert+message")
	}
}

// ----------------------------------------------------------------------------
// I13 — malformed evidence bytes: typed error, never a panic. Table + fuzz.
// ----------------------------------------------------------------------------

// TestConsensusCert_MalformedEvidenceNeverPanics drives hostile evidence bytes
// through every leg decoder and asserts a typed error and NO panic.
func TestConsensusCert_MalformedEvidenceNeverPanics(t *testing.T) {
	cases := map[string][]byte{
		"nil":           nil,
		"empty":         {},
		"single":        {0x00},
		"all_ff_short":  bytesRepeat(0xFF, 8),
		"all_ff_long":   bytesRepeat(0xFF, 256),
		"len_overrun":   {0xFF, 0xFF, 0xFF, 0xFF}, // a 4GB length prefix over 0 bytes
		"trailing_byte": {0x00, 0x00, 0x00, 0x00, 0x00, 0xDE},
	}
	for name, data := range cases {
		t.Run("threshold/"+name, func(t *testing.T) {
			if _, err := decodeThresholdSigPayload(data); err == nil && len(data) != 0 {
				// only the exact valid frame may decode; these are all malformed
				t.Fatalf("threshold decode accepted malformed %q", name)
			}
		})
		t.Run("stark/"+name, func(t *testing.T) {
			_, _ = decodeStarkCompressedPayload(data) // must not panic
		})
		t.Run("classical/"+name, func(t *testing.T) {
			_, _ = decodeClassicalAggregatePayload(data) // must not panic
		})
	}

	// Also drive a full envelope whose weighted-sig-set evidence is garbage: the
	// envelope must surface a typed error, not panic.
	f := validFixture(t)
	f.cert.Evidence[0].Payload = bytesRepeat(0xFF, 32)
	err := VerifyConsensusCert(f.store, f.validators, f.cert)
	if err == nil {
		t.Fatal("garbage weighted-sig-set evidence accepted")
	}
	if !errors.Is(err, ErrEvidenceWireCorrupt) {
		t.Fatalf("garbage evidence err = %v, want ErrEvidenceWireCorrupt", err)
	}
}

// FuzzVerifyConsensusCertEvidence asserts no leg decoder panics on arbitrary
// input and, when one decodes, the value re-encodes to bytes the decoder accepts
// again (idempotent on the accepted language).
func FuzzVerifyConsensusCertEvidence(f *testing.F) {
	f.Add(encodeThresholdSigPayload(&ThresholdSigPayload{Signature: []byte{1, 2}, Accountability: &ThresholdAccountability{AggregateWeight: 9}}))
	f.Add(encodeStarkCompressedPayload(&StarkCompressedPayload{PublicInputs: []byte{1}, Proof: []byte{2}}))
	f.Add(encodeClassicalAggregatePayload(&ClassicalAggregatePayload{Scheme: ClassicalSchemeBLS12381, Payload: []byte{3}}))
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Each decoder must never panic. Where it accepts, re-encode∘decode must
		// be stable.
		if p, err := decodeThresholdSigPayload(data); err == nil {
			if !bytes.Equal(encodeThresholdSigPayload(p), data) {
				t.Fatal("threshold payload not canonical on accept")
			}
		}
		if p, err := decodeStarkCompressedPayload(data); err == nil {
			if !bytes.Equal(encodeStarkCompressedPayload(p), data) {
				t.Fatal("stark payload not canonical on accept")
			}
		}
		if p, err := decodeClassicalAggregatePayload(data); err == nil {
			if !bytes.Equal(encodeClassicalAggregatePayload(p), data) {
				t.Fatal("classical payload not canonical on accept")
			}
		}
	})
}

// ----------------------------------------------------------------------------
// Helpers specific to the envelope tests.
// ----------------------------------------------------------------------------

// overflowFixture builds a fixture whose inner weighted-sig-set cert overflows
// Σweight on the SECOND accumulation (first signer weight = MaxUint64). Built by
// hand (not the prover) so the VERIFIER guard is exercised through the envelope.
func overflowFixture(t *testing.T) envFixture {
	t.Helper()
	// Runtime vars so the wrapping sum is computed at runtime (a const maxU64+100
	// is a compile-time overflow error).
	var maxU64 uint64 = ^uint64(0)
	var extra uint64 = 100
	s1 := newMLDSASigner(t, 0x01, maxU64)
	s2 := newMLDSASigner(t, 0x02, extra)

	env := testEnv()
	var vh [32]byte
	for i := range vh {
		vh[i] = 0xAB
	}
	env.ValueHash = vh
	env.QuorumThreshold = 50

	leaves := []WeightedValidatorLeaf{
		{ValidatorID: s1.id, PublicKey: s1.pubBytes, VotingWeight: maxU64, ParameterSetID: uint8(s1.scheme), KeyVersion: s1.keyVer},
		{ValidatorID: s2.id, PublicKey: s2.pubBytes, VotingWeight: extra, ParameterSetID: uint8(s2.scheme), KeyVersion: s2.keyVer},
	}
	set, err := BuildWeightedValidatorSet(env.Epoch, leaves)
	if err != nil {
		t.Fatalf("build set: %v", err)
	}
	env.ValidatorSetRoot = set.Root()
	msg, err := QuorumConsensusMessage(env)
	if err != nil {
		t.Fatalf("message: %v", err)
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
		proof, perr := set.InclusionProof(idxOf(s.id))
		if perr != nil {
			t.Fatalf("proof: %v", perr)
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
	inner := &WeightedQuorumCert{
		Version:          QuorumCertVersion,
		ChainID:          env.ChainID,
		Epoch:            env.Epoch,
		Height:           env.Height,
		Round:            env.Round,
		ValueHash:        env.ValueHash,
		QCType:           env.QCType,
		ValidatorSetRoot: set.Root(),
		QuorumThreshold:  env.QuorumThreshold,
		AggregateWeight:  maxU64 + extra, // wraps at runtime
		SignerCount:      2,
		Signers:          records,
	}
	inner.SignerCommitment = inner.computeSignerCommitment()
	innerWire, err := inner.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}

	requiredLeg := LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam}
	policy := &envPolicy{
		required:        []LegSpec{requiredLeg},
		allow:           map[legModeParam]bool{{LegPulsarMLDSA, EvidenceWeightedSigSet, pqParam}: true},
		thresholdWeight: 50,
		classical:       map[ClassicalScheme]bool{},
	}
	validators := &envValidators{
		root:      set.Root(),
		epoch:     env.Epoch,
		cfg:       QuorumVerifierConfig{Context: nil, MinThreshold: 50},
		env:       env,
		classKeys: map[ClassicalScheme][]byte{},
	}
	cert := newCertForBH(policy, env.ChainID, env.Epoch, env.Height, env.Round, env.ValueHash, set.Root(),
		[]LegEvidence{{Leg: requiredLeg, Mode: EvidenceWeightedSigSet, Payload: innerWire}})
	return envFixture{store: &envStore{policy: policy}, validators: validators, policy: policy, cert: cert}
}

// realBLSAggregate produces a real BLS-12-381 aggregate public key + signature
// over msg from a small committee, so the classical leg verifies a genuine
// aggregate (not a stub). Same-message aggregation: one Verify(aggPub, aggSig,
// msg) accepts iff every committee member signed msg — exactly what
// verifyBLSLeg checks.
func realBLSAggregate(t *testing.T, msg []byte) (aggPub, aggSig []byte) {
	t.Helper()
	const n = 3
	pubs := make([]*bls.PublicKey, 0, n)
	sigs := make([]*bls.Signature, 0, n)
	for i := 0; i < n; i++ {
		sk, err := bls.NewSecretKey()
		if err != nil {
			t.Fatalf("bls keygen: %v", err)
		}
		sig, err := sk.Sign(msg)
		if err != nil {
			t.Fatalf("bls sign: %v", err)
		}
		pubs = append(pubs, sk.PublicKey())
		sigs = append(sigs, sig)
	}
	apk, err := bls.AggregatePublicKeys(pubs)
	if err != nil {
		t.Fatalf("aggregate pubkeys: %v", err)
	}
	asig, err := bls.AggregateSignatures(sigs)
	if err != nil {
		t.Fatalf("aggregate sigs: %v", err)
	}
	return bls.PublicKeyToCompressedBytes(apk), bls.SignatureToBytes(asig)
}
