// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

// cert_policy_verify.go wires the chain's canonical cert posture
// (config.CertPolicy) into QuasarCert verification.
//
// THE BUG THIS CLOSES (Red H3). The pre-policy verifier
// (VerifyWithRealKeys) made BLS *mandatory* and treated every PQ leg as
// *optional* — a leg was verified only if the cert happened to carry
// bytes for it and the caller happened to supply a key. Leg presence is
// attacker-controlled cert bytes; an adversary who has broken BLS-12-381
// (a CRQC) simply submits a cert with ONLY the (forged) BLS leg, omits
// every PQ leg, and the verifier returns true — the certificate
// degenerates to BLS-only. Symmetrically, a legitimate pure-PQ cert
// (Strict variant, no BLS) was wrongly REJECTED because BLS was
// hard-required.
//
// THE FIX. Which legs MUST be present is a property of the *chain's
// policy*, never of the cert bytes. config.CertPolicy.RequiredLegs()
// (config/cert_policy.go) is the single source of truth: it returns
// exactly {BLS?, Pulsar, Corona, Magnetar} for the chain's (Mode,
// Variant). verifyUnderPolicy enforces, for every required leg:
//
//	(1) the cert carries bytes for that leg, AND
//	(2) the caller supplied the verification key for that leg,
//
// rejecting if either is missing — BEFORE any signature math. Then it
// verifies every leg the policy requires. BLS is not special: it is
// required iff LegBLS ∈ RequiredLegs() (i.e. Variant=Hybrid). Under a
// Strict variant the required set is pure PQ and a no-BLS cert verifies.
//
// Decomplected: "which legs are mandatory" (policy) is separated from
// "does this leg's signature check out" (crypto). The former lives in
// config.CertPolicy; the latter in the verify<Leg>Leg helpers below.

import (
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// CertKeys bundles the per-leg verification keys a verifier needs. A nil
// entry means "no key for this leg" — which is fatal iff the policy
// requires that leg (see verifyUnderPolicy), and otherwise means the leg
// (if present in the cert) cannot be verified and the cert is rejected.
//
// One struct instead of six positional key arguments keeps the
// policy-driven entry points readable and makes "key absent" a single
// nil check per leg.
type CertKeys struct {
	BLS      *bls.PublicKey             // LegBLS
	Pulsar   []byte                     // LegPulsar (Module-LWE threshold group key)
	Corona   *coronaThreshold.GroupKey  // LegCorona (Ring-LWE threshold group key)
	MLDSA    []*mldsa.PublicKey         // identity rollup (per-validator ML-DSA-65)
	Magnetar map[magnetar.NodeID][]byte // LegMagnetar (known SLH-DSA validator keys)
}

// legPresent reports whether the cert carries bytes for the named leg.
func (c *QuasarCert) legPresent(leg config.LegName) bool {
	switch leg {
	case config.LegBLS:
		return len(c.BLS) > 0
	case config.LegPulsar:
		return len(c.Pulsar) > 0
	case config.LegCorona:
		return len(c.Corona) > 0
	case config.LegMagnetar:
		return len(c.Magnetar) > 0
	default:
		return false
	}
}

// keyPresent reports whether the caller supplied a verification key for
// the named leg.
func (k CertKeys) keyPresent(leg config.LegName) bool {
	switch leg {
	case config.LegBLS:
		return k.BLS != nil
	case config.LegPulsar:
		return len(k.Pulsar) > 0
	case config.LegCorona:
		return k.Corona != nil
	case config.LegMagnetar:
		return len(k.Magnetar) > 0
	default:
		return false
	}
}

// verifyLeg verifies a single leg's signature against its key. Caller has
// already established the leg is present and the key is supplied.
func (c *QuasarCert) verifyLeg(leg config.LegName, message []byte, k CertKeys) bool {
	switch leg {
	case config.LegBLS:
		return verifyBLSLeg(message, k.BLS, c.BLS)
	case config.LegPulsar:
		return verifyPulsarLeg(message, k.Pulsar, c.Pulsar)
	case config.LegCorona:
		return verifyCoronaLeg(message, k.Corona, c.Corona)
	case config.LegMagnetar:
		return verifyMagnetarLeg(message, k.Magnetar, c.Magnetar)
	default:
		return false
	}
}

// VerifyUnderPolicy is the policy-driven QuasarCert verifier and the ONLY
// verify path production code should use. It consults cp.RequiredLegs()
// to decide which legs are mandatory, and rejects unless EVERY required
// leg is (a) present in the cert and (b) backed by a supplied key — then
// it verifies every required leg.
//
// Leg presence is policy-driven, NOT cert-byte-driven: an adversary
// cannot weaken the cert by omitting legs, because an omitted required
// leg is an immediate rejection. BLS is required iff the policy includes
// it (Variant=Hybrid); a Strict-variant policy yields a pure-PQ required
// set and a no-BLS cert verifies.
//
// The identity rollup (MLDSARollup) is not a CertPolicy leg — it is
// carried by every profile per the QuasarCert doc — so it keeps
// "if present, it MUST verify" semantics: a cert that carries rollup
// bytes but supplies no ML-DSA keys (or fails the ML-DSA check) is
// rejected. It is never silently skipped.
func (c *QuasarCert) VerifyUnderPolicy(message []byte, cp config.CertPolicy, keys CertKeys) bool {
	if c == nil || len(message) == 0 {
		return false
	}

	// Gate 1 — every policy-required leg must be present AND keyed.
	// This runs before any signature math: a missing required leg or a
	// missing key is a hard rejection, independent of the cert bytes the
	// attacker chose to include.
	for _, leg := range cp.RequiredLegs() {
		if !c.legPresent(leg) {
			return false
		}
		if !keys.keyPresent(leg) {
			return false
		}
	}

	// Gate 2 — verify every policy-required leg's signature.
	for _, leg := range cp.RequiredLegs() {
		if !c.verifyLeg(leg, message, keys) {
			return false
		}
	}

	// Gate 3 — identity rollup ("ZK" leg). Per the QuasarCert doc every
	// PQ profile (Pulsar/Aurora/Polaris) carries this leg, so under any
	// PQ policy it is REQUIRED: present, keyed, and verifying. Dropping it
	// is a downgrade and is rejected here even though CertPolicy.
	// RequiredLegs() (which models only BLS/Pulsar/Corona/Magnetar) does
	// not enumerate it. Under a non-PQ (BLS-only) policy the rollup is
	// optional but present-implies-verify (never silently skipped).
	if cp.IsPostQuantum() {
		if len(c.MLDSARollup) == 0 || len(keys.MLDSA) == 0 {
			return false
		}
		if !verifyMLDSARollupLeg(message, keys.MLDSA, c.MLDSARollup) {
			return false
		}
	} else if len(c.MLDSARollup) > 0 {
		if !verifyMLDSARollupLeg(message, keys.MLDSA, c.MLDSARollup) {
			return false
		}
	}

	return true
}

// --- per-leg verification helpers (the crypto half; the policy half is
//     config.CertPolicy). Each returns true iff the leg verifies. ---

// verifyBLSLeg verifies the BLS-12-381 aggregate (classical fast path).
func verifyBLSLeg(message []byte, pub *bls.PublicKey, sigBytes []byte) bool {
	if pub == nil || len(sigBytes) == 0 {
		return false
	}
	sig, err := bls.SignatureFromBytes(sigBytes)
	if err != nil {
		return false
	}
	return bls.Verify(pub, sig, message)
}

// verifyCoronaLeg verifies the Corona (Ring-LWE threshold ML-DSA) leg.
func verifyCoronaLeg(message []byte, groupKey *coronaThreshold.GroupKey, sigBytes []byte) bool {
	if groupKey == nil || len(sigBytes) == 0 {
		return false
	}
	sig, err := decodeCoronaSig(sigBytes)
	if err != nil {
		return false
	}
	return coronaThreshold.Verify(groupKey, string(message), sig)
}

// verifyMagnetarLeg verifies the Magnetar (SLH-DSA / FIPS 205 hash-based)
// leg. The leg carries a magnetar.ValidatorAggregateCert wire blob; the
// Polaris quorum policy requires every claimed signer to verify.
func verifyMagnetarLeg(message []byte, known map[magnetar.NodeID][]byte, blob []byte) bool {
	if len(known) == 0 || len(blob) == 0 {
		return false
	}
	cert, err := DecodeMagnetarAggregate(blob)
	if err != nil {
		return false
	}
	validCount, err := magnetar.VerifyAggregateCert(cert, message, known)
	if err != nil {
		return false
	}
	return validCount == len(cert.Signers)
}

// verifyMLDSARollupLeg verifies the per-validator ML-DSA-65 identity
// rollup. (When MLDSARollup is a single succinct strict-PQ STARK/FRI
// rollup it is verified on-chain by precompile/starkfri; this Go path
// covers the per-validator concatenated-sigs encoding.)
func verifyMLDSARollupLeg(message []byte, pubKeys []*mldsa.PublicKey, rollup []byte) bool {
	if len(pubKeys) == 0 || len(rollup) == 0 {
		return false
	}
	sigs, err := decodeMLDSASigs(rollup)
	if err != nil {
		return false
	}
	if len(sigs) == 0 || len(sigs) > len(pubKeys) {
		return false
	}
	for i, sig := range sigs {
		if !pubKeys[i].Verify(message, sig, nil) {
			return false
		}
	}
	return true
}
