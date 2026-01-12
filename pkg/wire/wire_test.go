// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDeriveVoterID(t *testing.T) {
	// Test that deriving from same string gives same ID
	id1 := DeriveVoterID("claude")
	id2 := DeriveVoterID("claude")
	if id1 != id2 {
		t.Error("DeriveVoterID should be deterministic")
	}

	// Test that different strings give different IDs
	id3 := DeriveVoterID("gpt4")
	if id1 == id3 {
		t.Error("Different strings should give different IDs")
	}

	// Test ID is 32 bytes
	if len(id1) != 32 {
		t.Errorf("VoterID should be 32 bytes, got %d", len(id1))
	}
}

func TestDeriveItemID(t *testing.T) {
	data := []byte("What's the best approach?")
	id1 := DeriveItemID(data)
	id2 := DeriveItemID(data)

	if id1 != id2 {
		t.Error("DeriveItemID should be deterministic")
	}

	if len(id1) != 32 {
		t.Errorf("ItemID should be 32 bytes, got %d", len(id1))
	}
}

func TestCandidateSerialization(t *testing.T) {
	domain := []byte("test-domain")
	payload := []byte("test payload content")

	candidate := NewCandidate(domain, payload, EmptyCandidateID, 1)

	// Verify ID is computed
	if candidate.ID == EmptyCandidateID {
		t.Error("Candidate ID should be computed")
	}

	// Verify content-addressed
	if !candidate.Verify() {
		t.Error("Candidate should verify")
	}

	// Serialize
	data, err := MarshalCandidate(candidate)
	if err != nil {
		t.Fatalf("Failed to marshal candidate: %v", err)
	}

	// Deserialize
	candidate2, err := UnmarshalCandidate(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal candidate: %v", err)
	}

	// Verify roundtrip
	if candidate2.ID != candidate.ID {
		t.Error("ID mismatch after roundtrip")
	}
	if string(candidate2.Domain) != string(candidate.Domain) {
		t.Error("Domain mismatch after roundtrip")
	}
	if string(candidate2.Payload) != string(candidate.Payload) {
		t.Error("Payload mismatch after roundtrip")
	}
}

func TestVoteSerialization(t *testing.T) {
	voterID := DeriveVoterID("claude")
	candidateID := DeriveItemID([]byte("test prompt"))

	vote := NewVote(candidateID, voterID, 0, true)
	vote.Signature = []byte{SigBLS, 1, 2, 3, 4}

	// Serialize
	data, err := MarshalVote(vote)
	if err != nil {
		t.Fatalf("Failed to marshal vote: %v", err)
	}

	// Deserialize
	vote2, err := UnmarshalVote(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal vote: %v", err)
	}

	// Verify
	if vote2.CandidateID != vote.CandidateID {
		t.Error("CandidateID mismatch after roundtrip")
	}
	if vote2.VoterID != vote.VoterID {
		t.Error("VoterID mismatch after roundtrip")
	}
	if vote2.Preference != vote.Preference {
		t.Error("Preference mismatch after roundtrip")
	}
	if vote2.Round != vote.Round {
		t.Error("Round mismatch after roundtrip")
	}
	if vote2.SignatureScheme() != SigBLS {
		t.Error("SignatureScheme mismatch after roundtrip")
	}
}

func TestCertificateSerialization(t *testing.T) {
	candidateID := DeriveItemID([]byte("test"))

	cert := NewCertificate(candidateID, 100, PolicyQuorum, []byte("proof-data"))
	cert.Signers = []byte("signers-bitmap")

	data, err := MarshalCertificate(cert)
	if err != nil {
		t.Fatalf("Failed to marshal certificate: %v", err)
	}

	cert2, err := UnmarshalCertificate(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal certificate: %v", err)
	}

	if cert2.CandidateID != cert.CandidateID {
		t.Error("CandidateID mismatch")
	}
	if cert2.Height != cert.Height {
		t.Error("Height mismatch")
	}
	if cert2.PolicyID != cert.PolicyID {
		t.Error("PolicyID mismatch")
	}
	if string(cert2.Proof) != string(cert.Proof) {
		t.Error("Proof mismatch")
	}
	if string(cert2.Signers) != string(cert.Signers) {
		t.Error("Signers mismatch")
	}
}

func TestConsensusParams(t *testing.T) {
	// Test defaults
	params := DefaultParams()
	if params.K != 3 {
		t.Errorf("Expected K=3, got %d", params.K)
	}
	if params.Alpha != 0.6 {
		t.Errorf("Expected Alpha=0.6, got %f", params.Alpha)
	}
	if params.Beta1 != 0.5 {
		t.Errorf("Expected Beta1=0.5, got %f", params.Beta1)
	}
	if params.Beta2 != 0.8 {
		t.Errorf("Expected Beta2=0.8, got %f", params.Beta2)
	}

	// Test blockchain params
	blockchain := BlockchainParams()
	if blockchain.K != 20 {
		t.Errorf("Expected K=20, got %d", blockchain.K)
	}

	// Test AI agent params
	ai := AIAgentParams()
	if ai.K != 3 {
		t.Errorf("Expected K=3, got %d", ai.K)
	}
}

func TestCrossLanguageCompatibility(t *testing.T) {
	// This tests that our JSON format matches what Python expects
	voterID := DeriveVoterID("claude")
	candidateID := DeriveItemID([]byte("test"))

	vote := NewVote(candidateID, voterID, 0, true)

	data, err := json.Marshal(vote)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Check JSON structure has expected fields
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	expected := []string{"candidate_id", "voter_id", "preference", "round", "timestamp_ms"}
	for _, field := range expected {
		if _, ok := m[field]; !ok {
			t.Errorf("Missing expected field: %s", field)
		}
	}
}

func TestSequencerConfigs(t *testing.T) {
	domain := []byte("test")

	// Single node
	single := SingleNodeConfig(domain)
	if single.K != 1 {
		t.Errorf("Single node should have K=1, got %d", single.K)
	}
	if single.SoftPolicy != PolicyNone {
		t.Error("Single node should use PolicyNone")
	}

	// Agent mesh
	mesh := AgentMeshConfig(domain, 5)
	if mesh.K != 5 {
		t.Errorf("Agent mesh should have K=5, got %d", mesh.K)
	}
	if mesh.SoftPolicy != PolicyQuorum {
		t.Error("Agent mesh should use PolicyQuorum")
	}

	// Blockchain
	bc := BlockchainConfig(domain)
	if bc.K != 20 {
		t.Errorf("Blockchain should have K=20, got %d", bc.K)
	}
	if bc.HardPolicy != PolicyQuantum {
		t.Error("Blockchain should use PolicyQuantum for hard finality")
	}

	// Rollup
	rollup := RollupConfig(domain)
	if rollup.K != 1 {
		t.Errorf("Rollup should have K=1, got %d", rollup.K)
	}
	if rollup.HardPolicy != PolicyL1Inclusion {
		t.Error("Rollup should use PolicyL1Inclusion for hard finality")
	}
}

func TestNonePolicy(t *testing.T) {
	ctx := context.Background()
	policy := NewNonePolicy()

	if policy.PolicyID() != PolicyNone {
		t.Error("Wrong policy ID")
	}

	candidate := NewCandidate([]byte("test"), []byte("payload"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, candidate)

	// Should finalize immediately
	cert, err := policy.MaybeFinalize(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("MaybeFinalize error: %v", err)
	}
	if cert == nil {
		t.Error("Should produce certificate for K=1")
	}
	if cert.PolicyID != PolicyNone {
		t.Error("Certificate should have PolicyNone")
	}

	// Verify
	ok, _ := policy.Verify(ctx, cert)
	if !ok {
		t.Error("Certificate should verify")
	}
}

func TestQuorumPolicy(t *testing.T) {
	ctx := context.Background()
	policy := NewQuorumPolicy(3, 5) // 3 of 5

	if policy.PolicyID() != PolicyQuorum {
		t.Error("Wrong policy ID")
	}

	candidate := NewCandidate([]byte("test"), []byte("payload"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, candidate)

	// Not enough votes yet
	cert, _ := policy.MaybeFinalize(ctx, candidate.ID)
	if cert != nil {
		t.Error("Should not finalize with 0 votes")
	}

	// Add 2 votes (not enough)
	for i := 0; i < 2; i++ {
		voter := DeriveVoterID(string(rune('A' + i)))
		vote := NewVote(candidate.ID, voter, 0, true)
		vote.Signature = []byte{SigBLS, byte(i)}
		policy.OnVote(ctx, vote)
	}

	cert, _ = policy.MaybeFinalize(ctx, candidate.ID)
	if cert != nil {
		t.Error("Should not finalize with 2 votes")
	}

	// Add 1 more vote (now 3)
	voter := DeriveVoterID("C")
	vote := NewVote(candidate.ID, voter, 0, true)
	vote.Signature = []byte{SigBLS, 2}
	policy.OnVote(ctx, vote)

	cert, _ = policy.MaybeFinalize(ctx, candidate.ID)
	if cert == nil {
		t.Error("Should finalize with 3 votes")
	}
	if cert.PolicyID != PolicyQuorum {
		t.Error("Certificate should have PolicyQuorum")
	}
}

func TestSamplePolicy(t *testing.T) {
	ctx := context.Background()
	policy := NewSamplePolicy(3, 0.6, 2) // k=3, alpha=0.6, beta=2

	if policy.PolicyID() != PolicySampleConvergence {
		t.Error("Wrong policy ID")
	}

	candidate := NewCandidate([]byte("test"), []byte("payload"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, candidate)

	// Round 0: 3 yes votes
	for i := 0; i < 3; i++ {
		voter := DeriveVoterID(string(rune('A' + i)))
		vote := NewVote(candidate.ID, voter, 0, true)
		policy.OnVote(ctx, vote)
	}

	// Not finalized yet (need 2 consecutive rounds)
	cert, _ := policy.MaybeFinalize(ctx, candidate.ID)
	if cert != nil {
		t.Error("Should not finalize after 1 round")
	}

	// Round 1: 3 yes votes
	for i := 0; i < 3; i++ {
		voter := DeriveVoterID(string(rune('A' + i)))
		vote := NewVote(candidate.ID, voter, 1, true)
		policy.OnVote(ctx, vote)
	}

	// Should be finalized now
	cert, _ = policy.MaybeFinalize(ctx, candidate.ID)
	if cert == nil {
		t.Error("Should finalize after 2 consecutive rounds")
	}
	if cert.PolicyID != PolicySampleConvergence {
		t.Error("Certificate should have PolicySampleConvergence")
	}
}

func TestTwoPhaseAgreement(t *testing.T) {
	candidateID := DeriveItemID([]byte("test"))

	state := AgreementState{
		CandidateID: candidateID,
	}

	// Initially not finalized
	if state.SoftFinalized || state.HardFinalized {
		t.Error("Should start unfinalized")
	}

	// Soft finalize
	state.SoftFinalized = true
	state.SoftCert = NewCertificate(candidateID, 1, PolicyQuorum, []byte("soft"))

	if !state.SoftFinalized {
		t.Error("Should be soft finalized")
	}
	if state.HardFinalized {
		t.Error("Should not be hard finalized yet")
	}

	// Hard finalize
	state.HardFinalized = true
	state.HardCert = NewCertificate(candidateID, 1, PolicyQuantum, []byte("hard"))

	if !state.HardFinalized {
		t.Error("Should be hard finalized")
	}
}
