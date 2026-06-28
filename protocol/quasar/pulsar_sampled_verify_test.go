// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"errors"
	"fmt"
	"testing"

	"github.com/luxfi/ids"
	pulsarwire "github.com/luxfi/pulsar/ref/go/pkg/pulsar"
)

// mapResolver is a test CommitteeKeyResolver backed by a map. A committee with
// no entry resolves to an error (excluded, like an offline committee).
type mapResolver struct{ eras map[ids.ID]CommitteeKeyEra }

func (r *mapResolver) ResolveCommitteeKey(id ids.ID, _, _ uint64) (CommitteeKeyEra, error) {
	e, ok := r.eras[id]
	if !ok {
		return CommitteeKeyEra{}, errors.New("no key era for committee")
	}
	return e, nil
}

// signCommittee generates a fresh, distinct ML-DSA-65 keypair (committee group
// key stand-in — the verify layer treats a dealerless Mithril group signature as
// an ordinary FIPS-204 signature, which is exactly the point) and signs M. The
// signature verifies under the UNMODIFIED FIPS-204 verifier (pulsarwire.VerifyBytes).
func signCommittee(t *testing.T, M []byte, idx int) (sigWire, gkWire []byte) {
	t.Helper()
	params := pulsarwire.MustParamsFor(pulsarwire.ModeP65)
	var seed [pulsarwire.SeedSize]byte
	copy(seed[:], fmt.Sprintf("pulsar-sampled-committee-key-%03d", idx))
	sk, err := pulsarwire.KeyFromSeed(params, seed)
	if err != nil {
		t.Fatalf("KeyFromSeed: %v", err)
	}
	sig, err := pulsarwire.Sign(params, sk, M, nil, false, nil)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if gkWire, err = sk.Pub.MarshalBinary(); err != nil {
		t.Fatalf("Pub.MarshalBinary: %v", err)
	}
	if sigWire, err = sig.MarshalBinary(); err != nil {
		t.Fatalf("sig.MarshalBinary: %v", err)
	}
	return sigWire, gkWire
}

// sampledFixture is a ready-to-verify sampled-cert scenario.
type sampledFixture struct {
	req      PulsarSampledVerifyRequest
	cert     *PulsarSampledCert
	resolver *mapResolver
	plan     *CommitteePlan
	M        []byte
}

// newSampledFixture builds a request + a cert in which the FIRST numValid
// committees of the re-derived plan each contribute a valid stock-ML-DSA
// signature over the canonical M, with their dealerless key-eras registered in
// the resolver. params drives the plan; numValid committees sign.
func newSampledFixture(t *testing.T, params SortitionParams, numValid int) *sampledFixture {
	t.Helper()
	var prev ids.ID
	copy(prev[:], "prev-finalized-block-id-32bytes!")
	var ss [48]byte
	copy(ss[:], "signer-set-id-48-bytes-padding-aaaaaaaaaaaaaaaa!")

	req := PulsarSampledVerifyRequest{
		Params:               params,
		Validators:           mkValidators(300, 1_000_000),
		PrevFinalizedBlockID: prev,
		Epoch:                9,
		ChainID:              96369,
		Height:               1_000_000,
		Round:                7,
		SignerSetID:          ss,
		PChainHeight:         4242,
		PolicyID:             0x0C0DE002,
	}
	for i := range req.BlockID {
		req.BlockID[i] = byte(i + 1)
	}
	for i := range req.StateRoot {
		req.StateRoot[i] = byte(0x40 + i)
	}
	for i := range req.BeamQCHash {
		req.BeamQCHash[i] = byte(0x80 + i)
	}

	// Re-derive plan + M exactly as the verifier will.
	seed := SortitionSeed(prev, ss, req.PChainHeight, req.Epoch, req.PolicyID)
	plan, err := DeriveCommitteePlan(params, seed, req.Validators)
	if err != nil {
		t.Fatalf("DeriveCommitteePlan: %v", err)
	}
	M := PulsarSampledSubject(PulsarSampledSubjectParams{
		ChainID: req.ChainID, Height: req.Height, Round: req.Round,
		BlockID: req.BlockID, StateRoot: req.StateRoot, BeamQCHash: req.BeamQCHash,
		SignerSetID: req.SignerSetID, PChainHeight: req.PChainHeight, PolicyID: req.PolicyID,
		CommitteePlanHash: plan.PlanHash,
	})

	resolver := &mapResolver{eras: map[ids.ID]CommitteeKeyEra{}}
	certs := make([]PulsarCommitteeCert, 0, numValid)
	for i := 0; i < numValid; i++ {
		c := plan.Committees[i]
		sigWire, gkWire := signCommittee(t, M, i)
		resolver.eras[c.ID] = CommitteeKeyEra{
			GroupPubKey: gkWire,
			KeygenMode:  KeygenModeMithrilRSS,
			T:           int(params.T),
			N:           int(params.N),
		}
		certs = append(certs, PulsarCommitteeCert{
			CommitteeID: c.ID,
			KeyEraID:    1,
			Generation:  1,
			PubKeyHash:  CommitteePubKeyHash(gkWire),
			Signature:   sigWire,
		})
	}
	cert := &PulsarSampledCert{
		PlanHash:       plan.PlanHash,
		RequiredR:      params.R,
		TotalM:         params.M,
		CommitteeCerts: certs,
	}
	req.Cert = cert
	req.Resolver = resolver
	return &sampledFixture{req: req, cert: cert, resolver: resolver, plan: plan, M: M}
}

// TestVerifySampled_Positive: the production default (n=8,t=7,m=12,r=8) with
// exactly r valid committees verifies, and the verifier reports the canonical M
// + the distinct-valid count.
func TestVerifySampled_Positive(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	res, err := VerifyPulsarSampled(f.req)
	if err != nil {
		t.Fatalf("VerifyPulsarSampled: %v", err)
	}
	if res.ValidCount != int(PulsarHybridPQv1.R) {
		t.Fatalf("ValidCount = %d, want %d", res.ValidCount, PulsarHybridPQv1.R)
	}
	if string(res.M) != string(f.M) {
		t.Fatal("returned M differs from the canonical subject")
	}
}

// TestVerifySampled_FullM: all m committees signing also verifies (liveness slack
// unused), with ValidCount == m.
func TestVerifySampled_FullM(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.M))
	res, err := VerifyPulsarSampled(f.req)
	if err != nil {
		t.Fatalf("VerifyPulsarSampled: %v", err)
	}
	if res.ValidCount != int(PulsarHybridPQv1.M) {
		t.Fatalf("ValidCount = %d, want %d", res.ValidCount, PulsarHybridPQv1.M)
	}
}

// TestVerifySampled_InsufficientCommittees: r-1 valid committees is rejected —
// the r-of-m bar is enforced.
func TestVerifySampled_InsufficientCommittees(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R)-1)
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("want ErrInsufficientCommittees, got %v", err)
	}
}

// TestVerifySampled_OneBadSigStillRejectsAtBoundary: exactly r committees present
// but one carries a signature over a DIFFERENT message — it is excluded, leaving
// r-1 valid, so the cert is rejected (the r-of-m bar holds against a tampered sig).
func TestVerifySampled_OneBadSigAtBoundary(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	// Re-sign committee 0 over a different message — verifies under its key but
	// NOT over M.
	badSig, _ := signCommittee(t, []byte("a totally different message"), 0)
	f.cert.CommitteeCerts[0].Signature = badSig
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("want ErrInsufficientCommittees (bad sig excluded), got %v", err)
	}
}

// TestVerifySampled_LivenessSlackToleratesBadSig: with r+1 committees present,
// one bad signature is tolerated — r valid remain.
func TestVerifySampled_LivenessSlackToleratesBadSig(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R)+1)
	badSig, _ := signCommittee(t, []byte("different message"), 0)
	f.cert.CommitteeCerts[0].Signature = badSig
	res, err := VerifyPulsarSampled(f.req)
	if err != nil {
		t.Fatalf("expected acceptance with r valid after one exclusion, got %v", err)
	}
	if res.ValidCount != int(PulsarHybridPQv1.R) {
		t.Fatalf("ValidCount = %d, want %d", res.ValidCount, PulsarHybridPQv1.R)
	}
}

// TestVerifySampled_TamperedPlanHash: a cert whose PlanHash does not match the
// re-derived plan is a hard reject.
func TestVerifySampled_TamperedPlanHash(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.M))
	f.cert.PlanHash[0] ^= 1
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrSampledPlanMismatch) {
		t.Fatalf("want ErrSampledPlanMismatch, got %v", err)
	}
}

// TestVerifySampled_ParamsMismatch: a cert claiming a weaker r (or m) than the
// policy is a hard reject.
func TestVerifySampled_ParamsMismatch(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.M))
	f.cert.RequiredR = 1 // claim a far weaker threshold
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrSampledParamsMismatch) {
		t.Fatalf("want ErrSampledParamsMismatch, got %v", err)
	}
}

// TestVerifySampled_OutOfPlanCommittee: a committee not sampled by the seed is
// excluded — an adversary cannot inject a committee it controls.
func TestVerifySampled_OutOfPlanCommittee(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	// Replace committee 0's identity with one that is NOT in the plan, but keep a
	// signature valid under a registered key — it must still be excluded because
	// the committee id is not InPlan.
	var fakeID ids.ID
	copy(fakeID[:], "this-committee-id-was-never-sampled!!")
	sigWire, gkWire := signCommittee(t, f.M, 999)
	f.resolver.eras[fakeID] = CommitteeKeyEra{GroupPubKey: gkWire, KeygenMode: KeygenModeMithrilRSS, T: int(PulsarHybridPQv1.T), N: int(PulsarHybridPQv1.N)}
	f.cert.CommitteeCerts[0] = PulsarCommitteeCert{CommitteeID: fakeID, KeyEraID: 1, Generation: 1, PubKeyHash: CommitteePubKeyHash(gkWire), Signature: sigWire}
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("out-of-plan committee must be excluded; want ErrInsufficientCommittees, got %v", err)
	}
}

// TestVerifySampled_NonDealerlessExcluded: a committee whose key-era is NOT a
// dealerless mode is excluded — the sampled cert requires dealerless committees.
func TestVerifySampled_NonDealerlessExcluded(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	c0 := f.plan.Committees[0].ID
	era := f.resolver.eras[c0]
	era.KeygenMode = "ceremony" // dealer ceremony — not admissible for a sampled cert
	f.resolver.eras[c0] = era
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("non-dealerless committee must be excluded; got %v", err)
	}
}

// TestVerifySampled_PubKeyHashMismatch: a cert whose PubKeyHash does not bind the
// resolver's key bytes is excluded — a cert minted against a different key cannot
// be replayed against the trusted one.
func TestVerifySampled_PubKeyHashMismatch(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	f.cert.CommitteeCerts[0].PubKeyHash[0] ^= 1
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("pubkey-hash mismatch must exclude; got %v", err)
	}
}

// TestVerifySampled_WrongTNExcluded: a key-era whose (t,n) does not match the
// plan is excluded — the resolver cannot substitute a weaker-threshold key.
func TestVerifySampled_WrongTNExcluded(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	c0 := f.plan.Committees[0].ID
	era := f.resolver.eras[c0]
	era.T = 2 // weaker than the plan's t=7
	f.resolver.eras[c0] = era
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("wrong (t,n) era must exclude; got %v", err)
	}
}

// TestVerifySampled_DuplicatesDontInflate: the same valid committee listed twice
// counts once — an adversary cannot pad the count with duplicates.
func TestVerifySampled_DuplicatesDontInflate(t *testing.T) {
	// r valid committees, but duplicate one of them so the list has r+1 entries
	// from only r distinct committees. Must still count r (the duplicate is free)
	// — to prove duplicates don't INFLATE, drop to r-1 distinct and duplicate.
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R)-1)
	// Append a duplicate of committee 0 — now r entries but only r-1 distinct.
	f.cert.CommitteeCerts = append(f.cert.CommitteeCerts, f.cert.CommitteeCerts[0])
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("duplicates must not inflate the distinct count; got %v", err)
	}
}

// TestVerifySampled_TooManyCommittees: a cert carrying more than m committees is
// structurally malformed.
func TestVerifySampled_TooManyCommittees(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.M))
	// Append one more (a duplicate) to exceed m.
	f.cert.CommitteeCerts = append(f.cert.CommitteeCerts, f.cert.CommitteeCerts[0])
	if _, err := VerifyPulsarSampled(f.req); !errors.Is(err, ErrSampledTooManyCommittees) {
		t.Fatalf("want ErrSampledTooManyCommittees, got %v", err)
	}
}

// TestVerifySampled_NilGuards: nil cert / nil resolver fail closed.
func TestVerifySampled_NilGuards(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.R))
	noCert := f.req
	noCert.Cert = nil
	if _, err := VerifyPulsarSampled(noCert); !errors.Is(err, ErrSampledNilCert) {
		t.Fatalf("want ErrSampledNilCert, got %v", err)
	}
	noRes := f.req
	noRes.Resolver = nil
	if _, err := VerifyPulsarSampled(noRes); !errors.Is(err, ErrSampledNilResolver) {
		t.Fatalf("want ErrSampledNilResolver, got %v", err)
	}
}

// TestVerifySampled_WrongFinalityPositionFailsAll: changing any finality-position
// field at verify time re-derives a DIFFERENT M, so none of the committee
// signatures (made over the original M) verify — the cert is rejected. This is
// cross-position non-transferability end to end.
func TestVerifySampled_WrongFinalityPositionFailsAll(t *testing.T) {
	f := newSampledFixture(t, PulsarHybridPQv1, int(PulsarHybridPQv1.M))
	bad := f.req
	bad.BlockID[0] ^= 1 // a different finalized block ⇒ a different M
	if _, err := VerifyPulsarSampled(bad); !errors.Is(err, ErrInsufficientCommittees) {
		t.Fatalf("changing the finalized block must invalidate every committee sig; got %v", err)
	}
}
