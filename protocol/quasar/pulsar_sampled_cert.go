// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pulsar_sampled_cert.go — the Pulsar SAMPLED-CERTIFICATE types: the ML-DSA
// analogue of Avalanche's repeated-small-sample path.
//
// # The core principle (do NOT conflate one committee with finality)
//
// Avalanche's k=20/α=14/β=20 is REPEATED SAMPLING for preference convergence,
// not one committee signing finality; safety comes from repeated subsampling +
// metastability + β confidence accumulation. Pulsar mirrors this in the
// post-quantum signature domain: MANY small, unpredictably-sampled, DEALERLESS
// Mithril committees each emit ONE ordinary FIPS-204 ML-DSA group signature over
// the SAME Avalanche-finalized subject M. The post-quantum confidence is the
// committee-capture probability accumulated over the repetition count r
// (P_fail ≈ p^r for the all-of-r case), NOT a single committee's threshold.
//
// This is the BLS/Warp compact-certificate idea translated to STANDARD ML-DSA
// WITHOUT a dealer: a BLS compact cert aggregates many validator signatures into
// one; a Pulsar sampled cert SAMPLES committees and collects r ordinary ML-DSA
// signatures, each from a small dealerless Mithril committee. Compact: r ≈ 8-16
// signatures, never 1000+.
//
//	❌ NEVER  "one n=64, t=5 Mithril committee == Avalanche finality"
//	✅ ALWAYS "a family of r-of-m sampled Mithril committees gives PQ attestation
//	          confidence, analogous to Avalanche's repeated-sample β confidence"
//
// # Why a small committee threshold is SOUND here
//
// The consensus quorum is NOT the signing committee. Lux finality is
// Avalanche/Snow-family: the digest is decided over the full N>1000 validator
// set by repeated randomized subsampling. A Pulsar committee never decides
// anything — it emits compact post-quantum EVIDENCE of a digest the consensus
// has ALREADY finalized. The committee needs exactly one property: UNFORGEABILITY
// (an adversary controlling < t of its n members cannot produce the committee
// group signature). The dealerless Mithril RSS DKG (luxfi/dkg/rss, luxfi/pulsar
// mithril_rss.go) provides exactly that, and the threshold output is a STANDARD
// ML-DSA signature whose EUF-CMA reduces to Module-LWE / Module-SIS, verifiable
// under an unmodified FIPS-204 verifier (pulsarwire.VerifyBytes).
//
// Security then derives from REPETITION, not committee size: see
// pulsar_sampled_security.go for the exact binomial r-of-m failure bound.
//
// # Decomplected
//
// This file owns ONLY the certificate VALUE types. It decides nothing about:
//   - which committees are sampled            → pulsar_sortition.go
//   - the subject the committees sign (M)      → pulsar_sampled_subject.go
//   - whether a sampled cert verifies          → VerifyPulsarSampled (this file's
//                                                verify section, pulsar_sampled_verify.go)
//   - the per-committee group-signature check  → key_era.go / verifyPulsarLeg
//                                                (reused, NOT re-implemented)
//   - which finality posture requires it       → pulsar_sampled_policy.go
package quasar

import "github.com/luxfi/ids"

// PulsarCommitteeCert is one sampled dealerless-Mithril committee's contribution:
// a single ORDINARY FIPS-204 ML-DSA group signature over the canonical sampled
// finality subject M, plus the coordinates that resolve the committee's trusted
// group public key.
//
// It carries NO per-member signatures — its size is O(1) in committee size n,
// because the committee's t-of-n threshold signing already collapsed to one
// group signature offline (before the consensus boundary). The chain does not
// care HOW the committee produced it (dealerless RSS / TALUS MPC / ceremony);
// the committee's KeyEra (resolved by CommitteeID) records the keygen mode for
// audit, and VerifyPulsarSampled requires it to be a DEALERLESS mode.
type PulsarCommitteeCert struct {
	// CommitteeID is the sampled committee's deterministic identity. It is a pure
	// function of the unbiasable sortition seed, the committee's index in the
	// plan, and the committee's (stake-weighted) membership — see
	// CommitteeID in pulsar_sortition.go. The verifier RECOMPUTES the admissible
	// CommitteeIDs from the seed and rejects any cert whose committee is not in
	// the plan, so an adversary cannot substitute a committee it happens to control.
	CommitteeID ids.ID

	// KeyEraID + Generation identify the committee's key era. KeyEraID advances
	// on a committee membership rotation; Generation advances on a within-era key
	// refresh/reshare. Both resolve (with CommitteeID) the committee's trusted
	// group key from the CommitteeKeyResolver, and are re-checked against it.
	KeyEraID   uint64
	Generation uint64

	// PubKeyHash binds this cert to the EXACT committee group public key bytes the
	// resolver supplies: PubKeyHash == H("PULSAR_COMMITTEE_PK_V1" || groupPubKey).
	// The verifier recomputes it from the resolved era key and rejects a mismatch,
	// so a cert minted against a stale/forged key cannot be replayed against the
	// trusted one. It is a binding, not a trust root: the group key itself comes
	// from the resolver (registry), never from the cert.
	PubKeyHash []byte

	// Signature is the committee's ORDINARY FIPS-204 ML-DSA group signature over
	// the canonical sampled finality subject M. Verified by the stock, stateless
	// pulsarwire.VerifyBytes path (verifyPulsarLeg) under the committee's resolved
	// group public key — byte-for-byte the same verifier the envelope Pulsar leg
	// uses. No bespoke threshold verification; a dealerless Mithril RSS committee
	// signature is, by construction, a standard ML-DSA signature.
	Signature []byte
}

// PulsarSampledCert is the compact post-quantum finality certificate: the result
// of sampling m dealerless Mithril committees and requiring r of them to emit a
// valid ordinary ML-DSA signature over the SAME subject M.
//
// Compactness: it stores at most m committee signatures (r ≈ 8-16 for the
// production parameter sets), never the underlying 1000+ validator certificates.
// Soundness: the security budget is the repetition count r against the
// per-committee capture probability p (pulsar_sampled_security.go), NOT a single
// committee's threshold — exactly mirroring Avalanche's β confidence.
type PulsarSampledCert struct {
	// PlanHash is the committeePlanHash — H over (n, t, m, r, selectionAlgorithmID,
	// sortitionSeed, committeeKeyEraRoot). It commits the entire sampling plan and
	// is ALSO bound into the subject M the committees sign, so the committees can
	// not be adaptively reselected after the block is seen. The verifier
	// recomputes it from the unbiasable inputs and rejects a mismatch.
	PlanHash []byte

	// RequiredR is r: the minimum number of DISTINCT, INDEPENDENT committees that
	// must contribute a valid signature for the sampled cert to be accepted. It is
	// the post-quantum confidence parameter — security is p^r (all-of-r) up to the
	// exact r-of-m binomial tail.
	RequiredR uint16

	// TotalM is m: the total number of committees sampled into the plan. m ≥ r
	// gives liveness slack (the cert accepts if ANY r of the m committees sign,
	// tolerating up to m-r offline/slow committees) at a slight, exactly-quantified
	// security cost vs all-of-r (see pulsar_sampled_security.go).
	TotalM uint16

	// CommitteeCerts are the committee signatures collected, at most m. The
	// verifier counts DISTINCT, IN-PLAN, valid committees and requires ≥ r.
	CommitteeCerts []PulsarCommitteeCert
}
