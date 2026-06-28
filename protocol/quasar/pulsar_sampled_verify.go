// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pulsar_sampled_verify.go — VerifyPulsarSampled: the r-of-m sampled-certificate
// verifier. It decides whether a PulsarSampledCert is valid post-quantum
// evidence that the finalized block was attested by enough independent
// dealerless Mithril committees.
//
// # The six checks (owner spec)
//
// VerifyPulsarSampled enforces, in order:
//
//	(a) the committeePlanHash is bound into the subject M       — M is re-derived
//	    from the re-derived plan, never taken on faith;
//	(b) the committees are the ones the UNBIASABLE seed sampled — every counted
//	    committee must be InPlan for the plan re-derived here;
//	(c) each committee is a valid DEALERLESS Mithril key-era    — the trusted
//	    resolver supplies the group key, its KeygenMode is dealerless, and its
//	    (t,n) matches the plan; the cert's PubKeyHash binds the exact key bytes;
//	(d) each signature stock-verifies under its committee key   — verifyPulsarLeg
//	    (pulsarwire.VerifyBytes, unmodified FIPS-204) over M;
//	(e) at least r distinct INDEPENDENT committees are valid    — distinct in-plan
//	    committees counted once each, ≥ RequiredR;
//	(f) every counted committee signs the SAME M               — one M is derived
//	    and every signature is checked against it, so agreement is by construction.
//
// # Trust-minimised by re-derivation
//
// The verifier trusts NOTHING the cert claims about WHICH committees exist or
// WHAT they signed. It re-derives the committee plan from the unbiasable
// sortition inputs (so a cert naming a committee the adversary controls but the
// seed never sampled is rejected at (b)), and it re-derives the subject M from
// that plan's committeePlanHash plus the finality position (so a cert whose
// committees signed some other M is rejected at (d)). The ONLY trusted input is
// the CommitteeKeyResolver, which supplies each sampled committee's group key —
// the registry of keys activated by dealerless Mithril DKG transcripts. The cert
// binds itself to the resolver's exact key bytes via PubKeyHash, so a cert minted
// against a stale or forged key cannot be replayed against the trusted one.
//
// # r-of-m counting semantics (liveness slack is built in)
//
// The cert carries only the committees that signed (≤ m); up to m−r may be
// offline and simply absent. A committee that fails ANY per-committee check is
// EXCLUDED (not counted), exactly like an offline one — the r-of-m model
// tolerates up to m−r such exclusions. A committee is counted at most once
// (independence), and counts iff it has at least one valid cert in the list. The
// cert is accepted iff ≥ r DISTINCT in-plan committees are valid. This is the
// post-quantum analogue of Avalanche's β confidence: security is the repetition
// count r against the per-committee capture probability (pulsar_sampled_security.go),
// never a single committee's threshold.
//
// # Decomplected
//
// This file owns ONLY r-of-m verification. The plan derivation is
// pulsar_sortition.go; the subject is pulsar_sampled_subject.go; the per-committee
// signature check is verifyPulsarLeg (polaris.go, reused — NOT re-implemented);
// the security budget is pulsar_sampled_security.go; the finality posture is
// pulsar_sampled_policy.go.
package quasar

import (
	"bytes"
	"errors"

	"github.com/luxfi/ids"
)

// KeygenModeMithrilRSS is the dealerless Replicated-Secret-Sharing (Mithril,
// ePrint 2026/013) keygen mode. A sampled committee's group key MUST be produced
// dealerlessly: the whole sampled-cert security argument assumes no single
// trusted dealer per committee. The resolver reports the committee key-era's
// KeygenMode and VerifyPulsarSampled counts a committee only if it is dealerless.
const KeygenModeMithrilRSS = "mithril-rss"

// isDealerlessKeygenMode reports whether a committee key-era's keygen mode is a
// dealerless mode admissible for a sampled certificate. Only Mithril RSS
// qualifies today; a dealer ceremony ("ceremony") never does.
func isDealerlessKeygenMode(mode string) bool { return mode == KeygenModeMithrilRSS }

// Domain separation for the committee group-public-key binding hash.
const (
	pulsarCommitteePKDomain = "PULSAR_COMMITTEE_PK_V1"
	pulsarCommitteePKCustom = "Lux/PulsarCommitteePK/v1"
)

// committeePubKeyHash = H("PULSAR_COMMITTEE_PK_V1" || groupPubKey). It binds a
// PulsarCommitteeCert to the EXACT committee group public key bytes the resolver
// supplies, so a cert minted against a stale/forged key cannot verify against
// the trusted one.
func committeePubKeyHash(groupPubKey []byte) []byte {
	return tupleHash256RoundDigest(
		[][]byte{[]byte(pulsarCommitteePKDomain), groupPubKey}, 32, pulsarCommitteePKCustom)
}

// CommitteePubKeyHash is the exported pure-function form, for producers minting a
// PulsarCommitteeCert.PubKeyHash from a committee's resolved group key.
func CommitteePubKeyHash(groupPubKey []byte) []byte { return committeePubKeyHash(groupPubKey) }

// CommitteeKeyEra is the trusted key-era record the resolver supplies for one
// sampled committee: its compact group public key, the keygen mode that produced
// it (must be dealerless), and the (t,n) it was generated for.
type CommitteeKeyEra struct {
	// GroupPubKey is the committee's ML-DSA-65 group public key, wire-framed as
	// pulsarwire.VerifyBytes accepts it. ONE key verifies the committee's whole
	// t-of-n threshold signature.
	GroupPubKey []byte

	// KeygenMode records how the group key was produced. For a sampled cert it
	// MUST be a dealerless mode (KeygenModeMithrilRSS).
	KeygenMode string

	// T, N are the committee threshold and size the key-era was generated for.
	// VerifyPulsarSampled requires them to match the plan's (t,n), so the resolver
	// cannot substitute a weaker-threshold key for a sampled committee.
	T int
	N int
}

// CommitteeKeyResolver resolves the trusted dealerless group key-era for a
// sampled committee. It is the registry of committee keys activated by Mithril
// DKG transcripts; the verifier loads keys from here, never from the cert. A
// committee with no activated dealerless key-era resolves to an error and is
// excluded (not counted), exactly like an offline committee.
type CommitteeKeyResolver interface {
	ResolveCommitteeKey(committeeID ids.ID, keyEraID, generation uint64) (CommitteeKeyEra, error)
}

// Typed errors. Per-committee failures are EXCLUSIONS (no error — the committee
// just does not count); these name only whole-cert structural rejections.
var (
	ErrSampledNilCert     = errors.New("quasar: nil sampled certificate")
	ErrSampledNilResolver = errors.New("quasar: nil committee-key resolver")

	// ErrSampledParamsMismatch — the cert's claimed (r, m) do not match the
	// sortition parameters the verifier enforces. A cert claiming a weaker r/m
	// than the policy is a hard reject.
	ErrSampledParamsMismatch = errors.New("quasar: sampled cert r/m do not match the sortition parameters")

	// ErrSampledPlanMismatch — the cert's PlanHash does not match the plan the
	// verifier re-derived from the unbiasable seed.
	ErrSampledPlanMismatch = errors.New("quasar: sampled cert plan hash does not match the re-derived committee plan")

	// ErrSampledTooManyCommittees — the cert lists more committees than the plan
	// sampled (m); a structurally malformed cert.
	ErrSampledTooManyCommittees = errors.New("quasar: sampled cert lists more committees than the plan sampled (m)")

	// ErrInsufficientCommittees — fewer than r distinct valid in-plan committees
	// signed M. The cert does not meet the r-of-m bar.
	ErrInsufficientCommittees = errors.New("quasar: fewer than r distinct valid in-plan committees signed M")
)

// PulsarSampledVerifyRequest carries everything VerifyPulsarSampled needs to
// check a sampled cert from first principles. The sortition inputs re-derive the
// committee plan; the finality-position inputs re-derive the subject M; the
// resolver supplies the trusted committee keys. Nothing the cert claims about
// committees or M is trusted.
type PulsarSampledVerifyRequest struct {
	Cert *PulsarSampledCert

	// --- sortition inputs: re-derive the committee plan ---
	Params               SortitionParams
	Validators           []SortitionValidator
	PrevFinalizedBlockID ids.ID
	Epoch                uint64

	// --- finality position: re-derive the subject M ---
	ChainID    uint32
	Height     uint64
	Round      uint32
	BlockID    [32]byte
	StateRoot  [32]byte
	BeamQCHash [32]byte

	// --- shared by the sortition seed AND the subject ---
	SignerSetID  [48]byte
	PChainHeight uint64
	PolicyID     uint32

	// --- the one trusted input ---
	Resolver CommitteeKeyResolver
}

// PulsarSampledResult is the outcome of a successful VerifyPulsarSampled: the
// canonical subject M every counted committee signed, the re-derived plan, and
// the number of distinct valid in-plan committees (≥ r).
type PulsarSampledResult struct {
	M          []byte
	Plan       *CommitteePlan
	ValidCount int
}

// VerifyPulsarSampled verifies a sampled certificate against the six checks. On
// success it returns the canonical subject M (so the caller can bind other lanes
// to the same finalized block), the re-derived plan, and the distinct-valid
// committee count. On failure it returns a typed structural error, or
// ErrInsufficientCommittees if fewer than r committees were valid.
func VerifyPulsarSampled(req PulsarSampledVerifyRequest) (*PulsarSampledResult, error) {
	if req.Cert == nil {
		return nil, ErrSampledNilCert
	}
	if req.Resolver == nil {
		return nil, ErrSampledNilResolver
	}
	cert := req.Cert

	// (b) re-derive the committee plan from the unbiasable seed. A bad parameter
	// set, an empty validator set, or a set too small to form a committee fails
	// here (the sortition's fail-closed errors).
	seed := SortitionSeed(req.PrevFinalizedBlockID, req.SignerSetID, req.PChainHeight, req.Epoch, req.PolicyID)
	plan, err := DeriveCommitteePlan(req.Params, seed, req.Validators)
	if err != nil {
		return nil, err
	}

	// Whole-cert structural checks. The cert may not claim a weaker (r, m) than
	// the policy, may not name a different plan, and may not carry more than m
	// committees.
	if cert.RequiredR != req.Params.R || cert.TotalM != req.Params.M {
		return nil, ErrSampledParamsMismatch
	}
	if !bytes.Equal(cert.PlanHash, plan.PlanHash) {
		return nil, ErrSampledPlanMismatch
	}
	if len(cert.CommitteeCerts) > int(req.Params.M) {
		return nil, ErrSampledTooManyCommittees
	}

	// (a, f) re-derive the ONE subject M from the re-derived plan's committeePlanHash
	// plus the finality position. Binding plan.PlanHash here is exactly check (a):
	// the committeePlanHash is bound into M. Every committee signature is checked
	// against this single M, which is check (f).
	M := PulsarSampledSubject(PulsarSampledSubjectParams{
		ChainID:           req.ChainID,
		Height:            req.Height,
		Round:             req.Round,
		BlockID:           req.BlockID,
		StateRoot:         req.StateRoot,
		BeamQCHash:        req.BeamQCHash,
		SignerSetID:       req.SignerSetID,
		PChainHeight:      req.PChainHeight,
		PolicyID:          req.PolicyID,
		CommitteePlanHash: plan.PlanHash,
	})

	// (c, d, e) count distinct valid in-plan committees. A committee is counted
	// at most once (independence) and counts iff it has a valid cert.
	counted := make(map[ids.ID]bool, len(cert.CommitteeCerts))
	validCount := 0
	for i := range cert.CommitteeCerts {
		cc := cert.CommitteeCerts[i]
		if counted[cc.CommitteeID] {
			continue
		}
		if req.committeeCounts(cc, plan, M) {
			counted[cc.CommitteeID] = true
			validCount++
		}
	}

	// (e) the r-of-m bar.
	if validCount < int(req.Params.R) {
		return nil, ErrInsufficientCommittees
	}

	return &PulsarSampledResult{M: M, Plan: plan, ValidCount: validCount}, nil
}

// committeeCounts runs the per-committee checks (b)(c)(d) for one committee cert
// and reports whether it is a valid, in-plan, dealerless committee whose ordinary
// FIPS-204 ML-DSA group signature verifies under its trusted group key over M.
// Any failure is an EXCLUSION (returns false), never a whole-cert error.
func (req *PulsarSampledVerifyRequest) committeeCounts(cc PulsarCommitteeCert, plan *CommitteePlan, M []byte) bool {
	// (b) the committee must be one the unbiasable seed actually sampled.
	if _, ok := plan.InPlan(cc.CommitteeID); !ok {
		return false
	}
	// (c) resolve the committee's trusted key-era and require it to be a
	// dealerless Mithril key for the plan's (t,n).
	era, err := req.Resolver.ResolveCommitteeKey(cc.CommitteeID, cc.KeyEraID, cc.Generation)
	if err != nil {
		return false
	}
	if !isDealerlessKeygenMode(era.KeygenMode) {
		return false
	}
	if era.N != int(req.Params.N) || era.T != int(req.Params.T) {
		return false
	}
	if len(era.GroupPubKey) == 0 {
		return false
	}
	// (c) bind the cert to the EXACT resolved key bytes — a cert minted against a
	// different key cannot be replayed against the trusted one.
	if !bytes.Equal(cc.PubKeyHash, committeePubKeyHash(era.GroupPubKey)) {
		return false
	}
	// (d) the committee's ordinary FIPS-204 ML-DSA group signature must verify
	// under its group key over M — the SAME stateless path the envelope Pulsar
	// leg uses (verifyPulsarLeg → pulsarwire.VerifyBytes), never a bespoke check.
	return verifyPulsarLeg(M, era.GroupPubKey, cc.Signature)
}
