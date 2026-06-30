// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// consensus_cert_dualpq_test.go — the multi-node integration proof for
// PARALLEL post-quantum finality: every cert carries a Pulsar (Module-LWE /
// FIPS-204 ML-DSA) leg AND a Corona (Ring-LWE) leg, both required (AND-mode),
// both verified live against real group keys and real signatures.
//
// What these tests prove, end to end, with REAL crypto (no fakes, no stubs):
//
//  1. PARALLEL SIGNING. The Corona leg is produced by a genuine multi-node
//     two-round threshold ceremony — N separate *coronaThreshold.Signer
//     objects, each holding EXACTLY ONE key share, exchanging Round1/Round2
//     messages over an explicit map "bus", aggregated by one designated node.
//     The Pulsar leg is a real FIPS-204 ML-DSA-65 signature its on-chain
//     verifier (verifyPulsarLeg → pulsarwire.VerifyBytes) accepts byte-for-byte.
//
//  2. AND-MODE. The dual-PQ ConsensusCert verifies ONLY when BOTH legs are
//     present and BOTH verify. Dropping or corrupting EITHER leg is a hard
//     reject — the policy's RequiredLegs() = {Pulsar, Corona} is conjunctive.
//
//  3. SUB-QUORUM FAILS, EITHER LEG. A Corona sub-quorum (t-1 signers) yields a
//     signature that does not verify under the group key ⇒ finality fails.
//     A missing/corrupt Pulsar leg ⇒ finality fails.
//
//  4. SINGLE-SHARE INVARIANT. No signing-path constructor takes a quorum of
//     shares. The Corona signer constructor takes EXACTLY ONE *KeyShare
//     (compile-time witness below); the aggregator's Finalize takes Round2
//     DATA, never shares. The single-share property is structural, not
//     incidental.
//
// HONEST SCOPE (see luxfi/pulsar BLOCKERS.md PULSAR-V12-PARALLEL-PQ): the Corona
// leg here is a true no-reconstruct t-of-n threshold (Ring-LWE shares are
// linear, so a dealerless Pedersen DKG + per-node signing never reconstructs
// the secret). The Pulsar leg's SIGNATURE is a real FIPS-204 ML-DSA group-key
// signature — its no-reconstruct t-of-n THRESHOLD production (gate-C's
// per-node algebraic signer) is blocked on a Pulsar release carrying a
// poly-vector secret-share type (ML-DSA's s1 is non-linear in the GF(257)
// seed-share, so the current KeyShare admits only the reconstruct path). The
// permissionless guarantee therefore comes from the Corona leg in AND-mode,
// exactly as dualpq.go documents. This file does NOT claim a single-share
// Pulsar threshold ceremony it cannot run.
package quasar

import (
	"errors"
	"testing"

	pulsarwire "github.com/luxfi/pulsar/pkg/pulsar"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// Compile-time single-share witness. The ONLY Corona signing constructor on
// the chain path takes EXACTLY ONE *KeyShare. If anyone ever adds a
// constructor that takes a slice of shares (a quorum in one process), this
// line keeps compiling but the structural tests below — and a source grep —
// catch it. The aggregator (Signer.Finalize) takes a map of Round2 DATA, not
// shares, so no process is ever one Lagrange combine from the key.
var _ func(*coronaThreshold.KeyShare) *coronaThreshold.Signer = coronaThreshold.NewSigner

const (
	dualPQThreshold = 2 // t — the quorum that must sign
	dualPQN         = 3 // n — committee size
	dualPQSession   = 31415
)

// dualPQPrf is the per-session PRF key the Corona ceremony binds (32 bytes).
var dualPQPrf = []byte("consensus-dualpq-corona-prf-32by")

// dualPQPolicy returns the AND-mode dual-PQ policy: BOTH the Pulsar and Corona
// threshold-sig legs are required, both at ML-DSA-65. This mirrors a config
// CertPolicy{Mode: CertModeStrict, Variant: CertVariantStrict}, whose
// RequiredLegs() is {Pulsar, Corona}.
func dualPQPolicy() *envPolicy {
	return &envPolicy{
		required: DualPQRequiredLegs(pqParam),
		allow: map[legModeParam]bool{
			{LegPulsarMLDSA, EvidenceThresholdSig, pqParam}:   true,
			{LegCoronaLattice, EvidenceThresholdSig, pqParam}: true,
		},
		thresholdWeight: 100,
		classical:       map[ClassicalScheme]bool{},
	}
}

// dualPQCertAndMessage builds a fixed dual-PQ cert header and the envelope
// message both legs sign. Header first, message determined, THEN sign — so the
// two threshold ceremonies sign exactly the bytes the verifier will recompute.
func dualPQCertAndMessage(policy *envPolicy) (*ConsensusCert, []byte, [48]byte) {
	var vsetRoot [48]byte
	for i := range vsetRoot {
		vsetRoot[i] = 0x33
	}
	var bh [32]byte
	for i := range bh {
		bh[i] = 0xC0
	}
	cert := newCertForBH(policy, 9, 9, 9, 9, bh, vsetRoot, nil)
	msg := consensusCertMessage(cert, HashRequiredLegs(policy.RequiredLegs()))
	return cert, msg, vsetRoot
}

// coronaNode is one validator's LOCAL state for the Corona threshold ceremony:
// exactly one *Signer (built from exactly one share) and its Shamir index. The
// node never sees another node's share — only Round1/Round2 messages on the
// bus. This is the structural single-share boundary made explicit.
type coronaNode struct {
	id     int
	signer *coronaThreshold.Signer
}

// signCoronaLegMultiNode runs a genuine multi-node Corona (Ring-LWE) threshold
// signature over msg with `participants` signers out of n, threshold t.
//
// Each node is a SEPARATE coronaNode holding ONE share; Round1 and Round2
// outputs are exchanged via explicit maps (the message bus); the designated
// aggregator (node 0) calls Finalize over the collected Round2 DATA. No node,
// and not the aggregator, ever holds a second node's share.
//
// With participants == t this yields a valid signature; with participants < t
// the produced signature does not verify under the group key (the sub-quorum
// proof). Returns (sig, groupKey, nodes, err) — nodes is returned so a caller
// can assert the per-node single-share structure.
func signCoronaLegMultiNode(t testing.TB, participants, thr, n int, msg []byte) (*coronaThreshold.Signature, *coronaThreshold.GroupKey, []coronaNode) {
	t.Helper()

	// Genesis keygen. GenerateKeys is the trusted-dealer fixture for the test
	// vector; Corona's PRODUCTION genesis is the dealerless Pedersen DKG
	// (corona/keyera), which never forms the master secret. Keygen is a
	// genesis concern; the per-block CHAIN path below is what must be — and is
	// — single-share.
	shares, groupKey, err := coronaThreshold.GenerateKeys(thr, n, nil)
	if err != nil {
		t.Fatalf("corona GenerateKeys(t=%d,n=%d): %v", thr, n, err)
	}

	nodes := make([]coronaNode, participants)
	ids := make([]int, participants)
	for i := 0; i < participants; i++ {
		// ONE share per node. coronaThreshold.NewSigner's signature
		// (*KeyShare)→*Signer is the compile-time witness that this is a
		// single-share constructor.
		nodes[i] = coronaNode{id: shares[i].Index, signer: coronaThreshold.NewSigner(shares[i])}
		ids[i] = shares[i].Index
	}

	// Round 1 — each node emits its Round1 message onto the bus.
	bus1 := make(map[int]*coronaThreshold.Round1Data, participants)
	for _, nd := range nodes {
		r1, rerr := nd.signer.Round1(dualPQSession, dualPQPrf, ids)
		if rerr != nil {
			t.Fatalf("corona Round1[node %d]: %v", nd.id, rerr)
		}
		bus1[nd.id] = r1
	}

	// Round 2 — each node consumes the Round1 bus and emits its Round2 message.
	bus2 := make(map[int]*coronaThreshold.Round2Data, participants)
	for _, nd := range nodes {
		r2, rerr := nd.signer.Round2(dualPQSession, string(msg), dualPQPrf, ids, bus1)
		if rerr != nil {
			t.Fatalf("corona Round2[node %d]: %v", nd.id, rerr)
		}
		bus2[nd.id] = r2
	}

	// Aggregate — the designated node combines Round2 DATA (not shares).
	sig, err := nodes[0].signer.Finalize(bus2)
	if err != nil {
		t.Fatalf("corona Finalize: %v", err)
	}
	return sig, groupKey, nodes
}

// signPulsarLegFIPS204 produces a real FIPS-204 ML-DSA-65 signature over msg
// and the matching wire-framed group public key. verifyPulsarLeg →
// pulsarwire.VerifyBytes accepts this byte-for-byte (empty FIPS-204 context,
// matching the verifier). See the HONEST SCOPE note in the file header for why
// this is a group-key signature rather than a no-reconstruct t-of-n ceremony.
func signPulsarLegFIPS204(t testing.TB, msg []byte) (sigWire, gkWire []byte) {
	t.Helper()
	params := pulsarwire.MustParamsFor(pulsarwire.ModeP65)
	var seed [pulsarwire.SeedSize]byte
	copy(seed[:], "lux-dualpq-pulsar-leg-genkey!!!!")
	sk, err := pulsarwire.KeyFromSeed(params, seed)
	if err != nil {
		t.Fatalf("pulsar KeyFromSeed: %v", err)
	}
	// ctx=nil, randomized=false ⇒ deterministic, matching VerifyBytes' empty
	// context. The signature verifies under an unmodified FIPS-204 verifier.
	sig, err := pulsarwire.Sign(params, sk, msg, nil, false, nil)
	if err != nil {
		t.Fatalf("pulsar Sign: %v", err)
	}
	gkWire, err = sk.Pub.MarshalBinary()
	if err != nil {
		t.Fatalf("pulsar Pub.MarshalBinary: %v", err)
	}
	sigWire, err = sig.MarshalBinary()
	if err != nil {
		t.Fatalf("pulsar sig.MarshalBinary: %v", err)
	}
	return sigWire, gkWire
}

// TestDualPQ_AndMode_ParallelThresholdFinality is the POSITIVE end-to-end
// proof: a cert carrying BOTH a real Corona (Ring-LWE, multi-node t-of-n) leg
// AND a real Pulsar (FIPS-204 ML-DSA) leg, over the same message, with both
// group keys supplied, verifies.
func TestDualPQ_AndMode_ParallelThresholdFinality(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dual-PQ Corona-DKG integration fixture under -short")
	}
	policy := dualPQPolicy()
	store := &envStore{policy: policy}
	cert, msg, vsetRoot := dualPQCertAndMessage(policy)

	// Both legs sign the SAME envelope message, in parallel.
	coronaSig, coronaGK, nodes := signCoronaLegMultiNode(t, dualPQThreshold, dualPQThreshold, dualPQN, msg)
	pulsarSig, pulsarGK := signPulsarLegFIPS204(t, msg)

	// Structural single-share check: exactly t separate per-node signers, each
	// a distinct object (no shared aggregate-of-shares state).
	if len(nodes) != dualPQThreshold {
		t.Fatalf("expected %d per-node signers, got %d", dualPQThreshold, len(nodes))
	}
	seen := map[*coronaThreshold.Signer]bool{}
	for _, nd := range nodes {
		if nd.signer == nil || seen[nd.signer] {
			t.Fatal("corona nodes must be distinct single-share signers")
		}
		seen[nd.signer] = true
	}

	// Compose the dual-PQ evidence via the production helper.
	evidence, err := ComposeDualPQEvidence(pqParam, cert.SignerRoot, 150, pulsarSig, coronaSig)
	if err != nil {
		t.Fatalf("ComposeDualPQEvidence: %v", err)
	}
	cert.Evidence = evidence

	validators := &envValidators{
		root: vsetRoot, epoch: 9, classKeys: map[ClassicalScheme][]byte{},
		coronaKey: coronaGK,
		pulsarKey: pulsarGK,
	}

	// AND-mode verify: BOTH legs present, BOTH verify, both bound to the
	// envelope. This is the dual-PQ finality predicate.
	if err := VerifyConsensusCert(store, validators, cert); err != nil {
		t.Fatalf("dual-PQ (Pulsar ‖ Corona) cert rejected: %v", err)
	}
}

// TestDualPQ_AndMode_SubQuorumCoronaFails proves the Corona leg is a real
// t-of-n threshold: a sub-quorum (t-1 signers) produces a signature that does
// not verify under the group key, so the dual-PQ cert is rejected even though
// the Pulsar leg is perfect.
func TestDualPQ_AndMode_SubQuorumCoronaFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dual-PQ Corona-DKG integration fixture under -short")
	}
	policy := dualPQPolicy()
	store := &envStore{policy: policy}
	cert, msg, vsetRoot := dualPQCertAndMessage(policy)

	// t-1 participants: a sub-quorum.
	coronaSig, coronaGK, _ := signCoronaLegMultiNode(t, dualPQThreshold-1, dualPQThreshold, dualPQN, msg)
	pulsarSig, pulsarGK := signPulsarLegFIPS204(t, msg)

	evidence, err := ComposeDualPQEvidence(pqParam, cert.SignerRoot, 150, pulsarSig, coronaSig)
	if err != nil {
		t.Fatalf("ComposeDualPQEvidence: %v", err)
	}
	cert.Evidence = evidence

	validators := &envValidators{
		root: vsetRoot, epoch: 9, classKeys: map[ClassicalScheme][]byte{},
		coronaKey: coronaGK, pulsarKey: pulsarGK,
	}

	// Sub-quorum Corona signature must fail the live Corona verify ⇒ cert
	// rejected at the threshold-sig clause.
	if err := VerifyConsensusCert(store, validators, cert); !errors.Is(err, ErrThresholdSigInvalid) {
		t.Fatalf("sub-quorum Corona leg: err = %v, want ErrThresholdSigInvalid", err)
	}
}

// TestDualPQ_AndMode_MissingPulsarLegFails proves AND-mode: a cert with a
// perfect Corona leg but NO Pulsar leg is rejected — the policy requires both.
func TestDualPQ_AndMode_MissingPulsarLegFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dual-PQ Corona-DKG integration fixture under -short")
	}
	policy := dualPQPolicy()
	store := &envStore{policy: policy}
	cert, msg, vsetRoot := dualPQCertAndMessage(policy)

	coronaSig, coronaGK, _ := signCoronaLegMultiNode(t, dualPQThreshold, dualPQThreshold, dualPQN, msg)
	_, pulsarGK := signPulsarLegFIPS204(t, msg)

	// Corona leg ONLY — the Pulsar required leg is absent.
	cert.Evidence = []LegEvidence{{
		Leg:  LegSpec{Kind: LegCoronaLattice, ParamSetID: pqParam},
		Mode: EvidenceThresholdSig,
		Payload: encodeThresholdSigPayload(&ThresholdSigPayload{
			Signature:      EncodeCoronaSig(coronaSig),
			Accountability: &ThresholdAccountability{SignerRoot: cert.SignerRoot, AggregateWeight: 150},
		}),
	}}

	validators := &envValidators{
		root: vsetRoot, epoch: 9, classKeys: map[ClassicalScheme][]byte{},
		coronaKey: coronaGK, pulsarKey: pulsarGK,
	}

	if err := VerifyConsensusCert(store, validators, cert); err == nil {
		t.Fatal("AND-mode violated: cert with a missing required Pulsar leg was accepted")
	}
}

// TestDualPQ_AndMode_CorruptPulsarLegFails proves the Pulsar leg verify is
// live: a one-byte flip of the FIPS-204 signature is rejected even though the
// Corona leg is a valid t-of-n threshold signature.
func TestDualPQ_AndMode_CorruptPulsarLegFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dual-PQ Corona-DKG integration fixture under -short")
	}
	policy := dualPQPolicy()
	store := &envStore{policy: policy}
	cert, msg, vsetRoot := dualPQCertAndMessage(policy)

	coronaSig, coronaGK, _ := signCoronaLegMultiNode(t, dualPQThreshold, dualPQThreshold, dualPQN, msg)
	pulsarSig, pulsarGK := signPulsarLegFIPS204(t, msg)

	// Flip one byte of the Pulsar signature.
	badPulsar := append([]byte(nil), pulsarSig...)
	badPulsar[len(badPulsar)/2] ^= 0xFF

	evidence, err := ComposeDualPQEvidence(pqParam, cert.SignerRoot, 150, badPulsar, coronaSig)
	if err != nil {
		t.Fatalf("ComposeDualPQEvidence: %v", err)
	}
	cert.Evidence = evidence

	validators := &envValidators{
		root: vsetRoot, epoch: 9, classKeys: map[ClassicalScheme][]byte{},
		coronaKey: coronaGK, pulsarKey: pulsarGK,
	}

	if err := VerifyConsensusCert(store, validators, cert); !errors.Is(err, ErrThresholdSigInvalid) {
		t.Fatalf("corrupt Pulsar leg: err = %v, want ErrThresholdSigInvalid", err)
	}
}

// TestDualPQ_AndMode_CorruptCoronaLegFails proves the Corona leg verify is
// live: a one-byte flip of the Ring-LWE threshold signature is rejected even
// though the Pulsar leg is a valid FIPS-204 signature.
func TestDualPQ_AndMode_CorruptCoronaLegFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dual-PQ Corona-DKG integration fixture under -short")
	}
	policy := dualPQPolicy()
	store := &envStore{policy: policy}
	cert, msg, vsetRoot := dualPQCertAndMessage(policy)

	coronaSig, coronaGK, _ := signCoronaLegMultiNode(t, dualPQThreshold, dualPQThreshold, dualPQN, msg)
	pulsarSig, pulsarGK := signPulsarLegFIPS204(t, msg)

	coronaBytes := EncodeCoronaSig(coronaSig)
	coronaBytes[len(coronaBytes)/2] ^= 0xFF // flip one byte

	acct := &ThresholdAccountability{SignerRoot: cert.SignerRoot, AggregateWeight: 150}
	cert.Evidence = []LegEvidence{
		{
			Leg:     LegSpec{Kind: LegPulsarMLDSA, ParamSetID: pqParam},
			Mode:    EvidenceThresholdSig,
			Payload: encodeThresholdSigPayload(&ThresholdSigPayload{Signature: pulsarSig, Accountability: acct}),
		},
		{
			Leg:     LegSpec{Kind: LegCoronaLattice, ParamSetID: pqParam},
			Mode:    EvidenceThresholdSig,
			Payload: encodeThresholdSigPayload(&ThresholdSigPayload{Signature: coronaBytes, Accountability: acct}),
		},
	}

	validators := &envValidators{
		root: vsetRoot, epoch: 9, classKeys: map[ClassicalScheme][]byte{},
		coronaKey: coronaGK, pulsarKey: pulsarGK,
	}

	if err := VerifyConsensusCert(store, validators, cert); !errors.Is(err, ErrThresholdSigInvalid) {
		t.Fatalf("corrupt Corona leg: err = %v, want ErrThresholdSigInvalid", err)
	}
}

// TestDualPQ_ComposeRejectsMissingLeg pins the composer's AND-mode contract: it
// refuses to build a dual-PQ evidence set with a missing leg, so a half cert is
// never even constructed.
func TestDualPQ_ComposeRejectsMissingLeg(t *testing.T) {
	var sr [32]byte
	if _, err := ComposeDualPQEvidence(pqParam, sr, 100, nil, nil); !errors.Is(err, ErrDualPQMissingPulsar) {
		t.Fatalf("missing Pulsar: err = %v, want ErrDualPQMissingPulsar", err)
	}
	if _, err := ComposeDualPQEvidence(pqParam, sr, 100, []byte{1, 2, 3}, nil); !errors.Is(err, ErrDualPQMissingCorona) {
		t.Fatalf("missing Corona: err = %v, want ErrDualPQMissingCorona", err)
	}
}
