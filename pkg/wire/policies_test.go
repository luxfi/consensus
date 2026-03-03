// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"context"
	"fmt"
	"testing"
)

// --- NonePolicy edge cases ---

func TestNonePolicyVerifyWrongPolicy(t *testing.T) {
	policy := NewNonePolicy()
	cert := &Certificate{PolicyID: PolicyQuorum}
	ok, err := policy.Verify(context.Background(), cert)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("NonePolicy should reject certificate with wrong policy ID")
	}
}

func TestNonePolicyOnVote(t *testing.T) {
	policy := NewNonePolicy()
	vote := NewVote(EmptyCandidateID, EmptyVoterID, 0, true)
	err := policy.OnVote(context.Background(), vote)
	if err != nil {
		t.Errorf("NonePolicy.OnVote should always return nil, got %v", err)
	}
}

func TestNonePolicyMaybeFinalizeUnknownCandidate(t *testing.T) {
	policy := NewNonePolicy()
	cert, err := policy.MaybeFinalize(context.Background(), DeriveItemID([]byte("unknown")))
	if err != nil {
		t.Fatal(err)
	}
	if cert != nil {
		t.Error("should return nil for unknown candidate")
	}
}

func TestNonePolicyMaybeFinalizeCached(t *testing.T) {
	ctx := context.Background()
	policy := NewNonePolicy()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	cert1, _ := policy.MaybeFinalize(ctx, c.ID)
	cert2, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert1 != cert2 {
		t.Error("should return cached certificate")
	}
}

func TestNonePolicyCandidateLimit(t *testing.T) {
	ctx := context.Background()
	policy := NewNonePolicy()

	// Fill to capacity
	for i := 0; i < maxCandidates; i++ {
		c := NewCandidate([]byte("d"), []byte(fmt.Sprintf("p%d", i)), EmptyCandidateID, uint64(i))
		if err := policy.OnCandidate(ctx, c); err != nil {
			t.Fatalf("should accept candidate %d: %v", i, err)
		}
	}

	// One more should fail
	c := NewCandidate([]byte("d"), []byte("overflow"), EmptyCandidateID, maxCandidates)
	err := policy.OnCandidate(ctx, c)
	if err == nil {
		t.Error("should reject candidate beyond limit")
	}
}

// --- QuorumPolicy edge cases ---

func TestQuorumPolicyVerify(t *testing.T) {
	policy := NewQuorumPolicy(3, 5)
	ctx := context.Background()

	// Wrong policy ID
	ok, _ := policy.Verify(ctx, &Certificate{PolicyID: PolicyNone})
	if ok {
		t.Error("should reject wrong policy ID")
	}

	// Empty proof
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicyQuorum})
	if ok {
		t.Error("should reject empty proof")
	}

	// Proof present but signers too short
	ok, _ = policy.Verify(ctx, &Certificate{
		PolicyID: PolicyQuorum,
		Proof:    []byte("proof"),
		Signers:  []byte("short"),
	})
	if ok {
		t.Error("should reject short signers")
	}

	// Valid: proof and enough signers (3 * 32 bytes)
	signers := make([]byte, 3*32)
	ok, _ = policy.Verify(ctx, &Certificate{
		PolicyID: PolicyQuorum,
		Proof:    []byte("proof"),
		Signers:  signers,
	})
	if !ok {
		t.Error("should accept valid certificate")
	}
}

func TestQuorumPolicyVoteBeforeCandidate(t *testing.T) {
	ctx := context.Background()
	policy := NewQuorumPolicy(1, 1)

	// Vote for an unknown candidate -- should create vote map on the fly
	voter := DeriveVoterID("agent", []byte("a"))
	candidateID := DeriveItemID([]byte("unknown"))
	vote := NewVote(candidateID, voter, 0, true)
	err := policy.OnVote(ctx, vote)
	if err != nil {
		t.Errorf("should accept vote for unknown candidate: %v", err)
	}
}

func TestQuorumPolicyRejectVotesNotPreferred(t *testing.T) {
	ctx := context.Background()
	policy := NewQuorumPolicy(1, 1)
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	// Vote with preference=false (reject)
	voter := DeriveVoterID("agent", []byte("a"))
	vote := NewVote(c.ID, voter, 0, false)
	vote.Signature = []byte{SigBLS, 1}
	policy.OnVote(ctx, vote)

	cert, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert != nil {
		t.Error("should not finalize with only reject votes")
	}
}

func TestQuorumPolicyCandidateLimit(t *testing.T) {
	ctx := context.Background()
	policy := NewQuorumPolicy(1, 1)

	for i := 0; i < maxCandidates; i++ {
		c := NewCandidate([]byte("d"), []byte(fmt.Sprintf("p%d", i)), EmptyCandidateID, uint64(i))
		if err := policy.OnCandidate(ctx, c); err != nil {
			t.Fatalf("failed at %d: %v", i, err)
		}
	}

	c := NewCandidate([]byte("d"), []byte("overflow"), EmptyCandidateID, maxCandidates)
	if err := policy.OnCandidate(ctx, c); err == nil {
		t.Error("should reject beyond limit")
	}
}

// --- SamplePolicy edge cases ---

func TestSamplePolicyVerify(t *testing.T) {
	policy := NewSamplePolicy(3, 0.6, 2)
	ctx := context.Background()

	// Wrong policy
	ok, _ := policy.Verify(ctx, &Certificate{PolicyID: PolicyNone})
	if ok {
		t.Error("should reject wrong policy")
	}

	// Short proof
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicySampleConvergence, Proof: []byte{1}})
	if ok {
		t.Error("should reject short proof")
	}

	// Low confidence
	proof := make([]byte, 9)
	proof[0] = 1 // confidence=1, beta=2
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicySampleConvergence, Proof: proof})
	if ok {
		t.Error("should reject low confidence")
	}

	// Sufficient confidence
	proof[0] = 3
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicySampleConvergence, Proof: proof})
	if !ok {
		t.Error("should accept sufficient confidence")
	}
}

func TestSamplePolicyVoteUnknownCandidate(t *testing.T) {
	policy := NewSamplePolicy(3, 0.6, 2)
	vote := NewVote(DeriveItemID([]byte("x")), EmptyVoterID, 0, true)
	err := policy.OnVote(context.Background(), vote)
	if err != nil {
		t.Errorf("should silently ignore vote for unknown candidate: %v", err)
	}
}

func TestSamplePolicyConvergesToReject(t *testing.T) {
	ctx := context.Background()
	policy := NewSamplePolicy(3, 0.6, 2)

	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	// 2 rounds of reject votes (all false)
	for round := uint64(0); round < 2; round++ {
		for i := 0; i < 3; i++ {
			voter := DeriveVoterID("a", []byte{byte(i)})
			vote := NewVote(c.ID, voter, round, false)
			policy.OnVote(ctx, vote)
		}
	}

	cert, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert != nil {
		t.Error("should not produce cert when converged to reject")
	}
}

func TestSamplePolicyPreferenceFlip(t *testing.T) {
	ctx := context.Background()
	policy := NewSamplePolicy(3, 0.6, 3) // beta=3

	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	// Round 0: yes (confidence 1)
	for i := 0; i < 3; i++ {
		voter := DeriveVoterID("a", []byte{byte(i)})
		policy.OnVote(ctx, NewVote(c.ID, voter, 0, true))
	}

	// Round 1: no -- preference flips, confidence resets to 1
	for i := 0; i < 3; i++ {
		voter := DeriveVoterID("a", []byte{byte(i)})
		policy.OnVote(ctx, NewVote(c.ID, voter, 1, false))
	}

	// Round 2: yes -- preference flips again, confidence 1
	for i := 0; i < 3; i++ {
		voter := DeriveVoterID("a", []byte{byte(i)})
		policy.OnVote(ctx, NewVote(c.ID, voter, 2, true))
	}

	cert, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert != nil {
		t.Error("should not finalize -- confidence never reached beta=3")
	}
}

func TestSamplePolicyCandidateLimit(t *testing.T) {
	ctx := context.Background()
	policy := NewSamplePolicy(1, 1.0, 1)

	for i := 0; i < maxCandidates; i++ {
		c := NewCandidate([]byte("d"), []byte(fmt.Sprintf("p%d", i)), EmptyCandidateID, uint64(i))
		if err := policy.OnCandidate(ctx, c); err != nil {
			t.Fatalf("failed at %d: %v", i, err)
		}
	}

	c := NewCandidate([]byte("d"), []byte("overflow"), EmptyCandidateID, maxCandidates)
	if err := policy.OnCandidate(ctx, c); err == nil {
		t.Error("should reject beyond limit")
	}
}

// --- L1Policy ---

type mockL1Verifier struct {
	proofs     map[CandidateID][]byte
	verifyFunc func(ctx context.Context, id CandidateID, proof []byte) (bool, error)
}

func (m *mockL1Verifier) GetInclusionProof(ctx context.Context, id CandidateID) ([]byte, error) {
	p, ok := m.proofs[id]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (m *mockL1Verifier) VerifyInclusion(ctx context.Context, id CandidateID, proof []byte) (bool, error) {
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, id, proof)
	}
	return len(proof) > 0, nil
}

func TestL1PolicyFullLifecycle(t *testing.T) {
	ctx := context.Background()
	verifier := &mockL1Verifier{proofs: make(map[CandidateID][]byte)}
	policy := NewL1Policy(verifier)

	if policy.PolicyID() != PolicyL1Inclusion {
		t.Error("wrong policy ID")
	}

	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	if err := policy.OnCandidate(ctx, c); err != nil {
		t.Fatal(err)
	}

	// OnVote is a no-op
	if err := policy.OnVote(ctx, NewVote(c.ID, EmptyVoterID, 0, true)); err != nil {
		t.Errorf("OnVote should return nil: %v", err)
	}

	// Not included yet
	cert, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert != nil {
		t.Error("should not finalize without L1 inclusion")
	}

	// Simulate L1 inclusion
	verifier.proofs[c.ID] = []byte("merkle-proof")

	cert, _ = policy.MaybeFinalize(ctx, c.ID)
	if cert == nil {
		t.Fatal("should finalize with L1 inclusion proof")
	}
	if cert.PolicyID != PolicyL1Inclusion {
		t.Error("wrong cert policy ID")
	}

	// Cached return
	cert2, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert2 != cert {
		t.Error("should return cached cert")
	}
}

func TestL1PolicyVerify(t *testing.T) {
	ctx := context.Background()
	verifier := &mockL1Verifier{
		proofs: make(map[CandidateID][]byte),
		verifyFunc: func(_ context.Context, _ CandidateID, proof []byte) (bool, error) {
			return string(proof) == "valid", nil
		},
	}
	policy := NewL1Policy(verifier)

	// Wrong policy ID
	ok, _ := policy.Verify(ctx, &Certificate{PolicyID: PolicyNone})
	if ok {
		t.Error("should reject wrong policy")
	}

	// Valid proof
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicyL1Inclusion, Proof: []byte("valid")})
	if !ok {
		t.Error("should accept valid proof")
	}

	// Invalid proof
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicyL1Inclusion, Proof: []byte("invalid")})
	if ok {
		t.Error("should reject invalid proof")
	}
}

func TestL1PolicyUnknownCandidate(t *testing.T) {
	ctx := context.Background()
	verifier := &mockL1Verifier{proofs: make(map[CandidateID][]byte)}
	policy := NewL1Policy(verifier)

	cert, err := policy.MaybeFinalize(ctx, DeriveItemID([]byte("unknown")))
	if err != nil {
		t.Fatal(err)
	}
	if cert != nil {
		t.Error("should return nil for unknown candidate")
	}
}

func TestL1PolicyCandidateLimit(t *testing.T) {
	ctx := context.Background()
	verifier := &mockL1Verifier{proofs: make(map[CandidateID][]byte)}
	policy := NewL1Policy(verifier)

	for i := 0; i < maxCandidates; i++ {
		c := NewCandidate([]byte("d"), []byte(fmt.Sprintf("p%d", i)), EmptyCandidateID, uint64(i))
		if err := policy.OnCandidate(ctx, c); err != nil {
			t.Fatalf("failed at %d: %v", i, err)
		}
	}

	c := NewCandidate([]byte("d"), []byte("overflow"), EmptyCandidateID, maxCandidates)
	if err := policy.OnCandidate(ctx, c); err == nil {
		t.Error("should reject beyond limit")
	}
}

// --- QuantumPolicy additional edge cases ---

func TestQuantumPolicySetRequireRT(t *testing.T) {
	policy := NewQuantumPolicy(1)
	policy.SetRequireRT(false)

	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	// BLS-only should be accepted after disabling RT requirement
	voter := DeriveVoterID("a", []byte("v"))
	vote := NewVote(c.ID, voter, 0, true)
	vote.Signature = []byte{SigBLS, 1, 2, 3}
	err := policy.OnVote(ctx, vote)
	if err != nil {
		t.Errorf("BLS should be accepted with RT disabled: %v", err)
	}
}

func TestQuantumPolicyRejectVoteNoPreference(t *testing.T) {
	policy := NewQuantumPolicy(1)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	// Vote with preference=false -- silently ignored
	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, false)
	vote.Signature = []byte{SigQuasar, 0, 2, 1, 2, 3, 4}
	err := policy.OnVote(ctx, vote)
	if err != nil {
		t.Errorf("reject vote should be silently ignored: %v", err)
	}
}

func TestQuantumPolicyEmptySignatureRTRequired(t *testing.T) {
	policy := NewQuantumPolicy(1)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	// No signature
	err := policy.OnVote(ctx, vote)
	if err == nil {
		t.Error("should reject empty signature when RT required")
	}
	rtErr, ok := err.(*RTRequirementError)
	if !ok {
		t.Errorf("expected RTRequirementError, got %T", err)
	}
	if rtErr.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestQuantumPolicyEmptySignatureRTNotRequired(t *testing.T) {
	policy := NewQuantumPolicyWithOptions(1, false)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	err := policy.OnVote(ctx, vote)
	if err != nil {
		t.Errorf("empty sig should be ok when RT not required: %v", err)
	}
}

func TestQuantumPolicyQuasarTooShort(t *testing.T) {
	policy := NewQuantumPolicy(1)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	vote.Signature = []byte{SigQuasar, 0} // too short
	err := policy.OnVote(ctx, vote)
	if err == nil {
		t.Error("should reject too-short Quasar signature")
	}
}

func TestQuantumPolicyQuasarZeroBLS(t *testing.T) {
	policy := NewQuantumPolicy(1)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	vote.Signature = []byte{SigQuasar, 0, 0, 1} // bls_len=0
	err := policy.OnVote(ctx, vote)
	if err == nil {
		t.Error("should reject zero-length BLS in Quasar")
	}
}

func TestQuantumPolicyQuasarTruncatedBLS(t *testing.T) {
	policy := NewQuantumPolicy(1)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	vote.Signature = []byte{SigQuasar, 0, 10} // claims 10 bytes BLS but has 0
	err := policy.OnVote(ctx, vote)
	if err == nil {
		t.Error("should reject truncated BLS in Quasar")
	}
}

func TestQuantumPolicyFinalizationAndVerify(t *testing.T) {
	ctx := context.Background()
	policy := NewQuantumPolicy(2)

	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	// Add 2 Quasar votes (threshold=2)
	for i := 0; i < 2; i++ {
		voter := DeriveVoterID("a", []byte{byte(i)})
		blsSig := make([]byte, 48)
		blsSig[0] = byte(i)
		rtSig := make([]byte, 32)
		rtSig[0] = byte(i)
		sig := []byte{SigQuasar, 0, byte(len(blsSig))}
		sig = append(sig, blsSig...)
		sig = append(sig, rtSig...)

		vote := NewVote(c.ID, voter, 0, true)
		vote.Signature = sig
		if err := policy.OnVote(ctx, vote); err != nil {
			t.Fatalf("vote %d failed: %v", i, err)
		}
	}

	cert, err := policy.MaybeFinalize(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cert == nil {
		t.Fatal("should finalize with 2 votes")
	}
	if cert.PolicyID != PolicyQuantum {
		t.Error("wrong policy ID on cert")
	}

	// Verify
	ok, _ := policy.Verify(ctx, cert)
	if !ok {
		t.Error("should verify own certificate")
	}

	// Cached
	cert2, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert2 != cert {
		t.Error("should return cached cert")
	}
}

func TestQuantumPolicyVerifyEdgeCases(t *testing.T) {
	policy := NewQuantumPolicy(1)
	ctx := context.Background()

	// Wrong policy
	ok, _ := policy.Verify(ctx, &Certificate{PolicyID: PolicyNone})
	if ok {
		t.Error("should reject wrong policy")
	}

	// Short proof
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicyQuantum, Proof: []byte{SigQuasar}})
	if ok {
		t.Error("should reject short proof")
	}

	// Wrong scheme byte
	proof := make([]byte, 33)
	proof[0] = SigBLS // not SigQuasar
	ok, _ = policy.Verify(ctx, &Certificate{PolicyID: PolicyQuantum, Proof: proof})
	if ok {
		t.Error("should reject non-Quasar scheme byte")
	}
}

func TestQuantumPolicyMaybeFinalizeBelowThreshold(t *testing.T) {
	ctx := context.Background()
	policy := NewQuantumPolicy(3)

	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	// Only 1 vote (threshold=3)
	voter := DeriveVoterID("a", []byte("v"))
	sig := []byte{SigQuasar, 0, 2, 1, 2, 3, 4}
	vote := NewVote(c.ID, voter, 0, true)
	vote.Signature = sig
	policy.OnVote(ctx, vote)

	cert, _ := policy.MaybeFinalize(ctx, c.ID)
	if cert != nil {
		t.Error("should not finalize below threshold")
	}
}

func TestQuantumPolicyUnknownCandidate(t *testing.T) {
	policy := NewQuantumPolicy(1)
	cert, _ := policy.MaybeFinalize(context.Background(), DeriveItemID([]byte("nope")))
	if cert != nil {
		t.Error("should return nil for unknown")
	}
}

func TestQuantumPolicyCandidateLimit(t *testing.T) {
	ctx := context.Background()
	policy := NewQuantumPolicy(1)

	for i := 0; i < maxCandidates; i++ {
		c := NewCandidate([]byte("d"), []byte(fmt.Sprintf("p%d", i)), EmptyCandidateID, uint64(i))
		if err := policy.OnCandidate(ctx, c); err != nil {
			t.Fatalf("failed at %d: %v", i, err)
		}
	}

	c := NewCandidate([]byte("d"), []byte("overflow"), EmptyCandidateID, maxCandidates)
	if err := policy.OnCandidate(ctx, c); err == nil {
		t.Error("should reject beyond limit")
	}
}

func TestQuantumPolicyUnknownSchemeRTRequired(t *testing.T) {
	policy := NewQuantumPolicy(1)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	vote.Signature = []byte{0xFF, 1, 2, 3} // unknown scheme
	err := policy.OnVote(ctx, vote)
	if err == nil {
		t.Error("should reject unknown scheme when RT required")
	}
}

func TestQuantumPolicyUnknownSchemeRTNotRequired(t *testing.T) {
	policy := NewQuantumPolicyWithOptions(1, false)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	vote.Signature = []byte{0xFF, 1, 2, 3}
	err := policy.OnVote(ctx, vote)
	if err != nil {
		t.Errorf("should ignore unknown scheme when RT not required: %v", err)
	}
}

func TestQuantumPolicyRingtailOnlyRTNotRequired(t *testing.T) {
	policy := NewQuantumPolicyWithOptions(1, false)
	ctx := context.Background()
	c := NewCandidate([]byte("d"), []byte("p"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, c)

	vote := NewVote(c.ID, DeriveVoterID("a", []byte("v")), 0, true)
	vote.Signature = []byte{SigRingtail, 4, 5, 6}
	err := policy.OnVote(ctx, vote)
	if err != nil {
		t.Errorf("RT-only vote should be accepted when RT not required: %v", err)
	}
}

// --- sigSchemeToString ---

func TestSigSchemeToString(t *testing.T) {
	tests := []struct {
		scheme byte
		expect string
	}{
		{SigNone, "SigNone"},
		{SigEd25519, "SigEd25519"},
		{SigBLS, "SigBLS"},
		{SigRingtail, "SigRingtail"},
		{SigQuasar, "SigQuasar"},
		{0xFF, "Unknown"},
	}
	for _, tt := range tests {
		got := sigSchemeToString(tt.scheme)
		if got != tt.expect {
			t.Errorf("sigSchemeToString(%d) = %s, want %s", tt.scheme, got, tt.expect)
		}
	}
}

// --- HostnameValidationError.Error() ---

func TestHostnameValidationErrorMessage(t *testing.T) {
	e := &HostnameValidationError{Address: "1.2.3.4", Reason: "bad"}
	msg := e.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
}

// --- ValidatePeerAddress ---

func TestValidatePeerAddress(t *testing.T) {
	// Valid
	if err := ValidatePeerAddress("node1.lux.network:9651"); err != nil {
		t.Errorf("valid hostname should pass: %v", err)
	}
	// Invalid
	if err := ValidatePeerAddress("10.0.0.1:9651"); err == nil {
		t.Error("IP literal should be rejected")
	}
}

// --- RTRequirementError.Error() ---

func TestRTRequirementErrorMessage(t *testing.T) {
	e := &RTRequirementError{Reason: "test reason"}
	if e.Error() != "RT signature required: test reason" {
		t.Errorf("unexpected error: %s", e.Error())
	}
}

// --- CredentialValidationError.Error() ---

func TestCredentialValidationErrorMessage(t *testing.T) {
	e := &CredentialValidationError{Index: 0, Expected: 100, Got: 50, Reason: "mismatch"}
	if e.Error() != "mismatch" {
		t.Errorf("unexpected: %s", e.Error())
	}
}
