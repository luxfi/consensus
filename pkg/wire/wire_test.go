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
	id1 := DeriveVoterID("agent", []byte("claude"))
	id2 := DeriveVoterID("agent", []byte("claude"))
	if id1 != id2 {
		t.Error("DeriveVoterID should be deterministic")
	}

	// Test that different strings give different IDs
	id3 := DeriveVoterID("agent", []byte("gpt4"))
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
	voterID := DeriveVoterID("agent", []byte("claude"))
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
	voterID := DeriveVoterID("agent", []byte("claude"))
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
		voter := DeriveVoterID("agent", []byte{byte('A' + i)})
		vote := NewVote(candidate.ID, voter, 0, true)
		vote.Signature = []byte{SigBLS, byte(i)}
		policy.OnVote(ctx, vote)
	}

	cert, _ = policy.MaybeFinalize(ctx, candidate.ID)
	if cert != nil {
		t.Error("Should not finalize with 2 votes")
	}

	// Add 1 more vote (now 3)
	voter := DeriveVoterID("agent", []byte("C"))
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
		voter := DeriveVoterID("agent", []byte{byte('A' + i)})
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
		voter := DeriveVoterID("agent", []byte{byte('A' + i)})
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

// =============================================================================
// RT ENFORCEMENT TESTS
// =============================================================================

func TestQuantumPolicyRTEnforcement(t *testing.T) {
	ctx := context.Background()
	policy := NewQuantumPolicy(2) // threshold 2, RT required by default

	if policy.PolicyID() != PolicyQuantum {
		t.Error("Wrong policy ID")
	}

	candidate := NewCandidate([]byte("test"), []byte("payload"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, candidate)

	// Test 1: BLS-only vote should be rejected when RT is required
	voter1 := DeriveVoterID("agent", []byte("voter1"))
	blsVote := NewVote(candidate.ID, voter1, 0, true)
	blsVote.Signature = []byte{SigBLS, 1, 2, 3} // BLS signature only

	err := policy.OnVote(ctx, blsVote)
	if err == nil {
		t.Error("BLS-only vote should be rejected when RT is required")
	}
	if _, ok := err.(*RTRequirementError); !ok {
		t.Errorf("Expected RTRequirementError, got %T", err)
	}

	// Test 2: Ringtail-only vote should also be rejected
	rtVote := NewVote(candidate.ID, voter1, 0, true)
	rtVote.Signature = []byte{SigRingtail, 4, 5, 6} // Ringtail only

	err = policy.OnVote(ctx, rtVote)
	if err == nil {
		t.Error("Ringtail-only vote should be rejected when RT is required")
	}

	// Test 3: Quasar (dual BLS+RT) vote should be accepted
	// Format: [scheme][bls_len_hi][bls_len_lo][bls...][rt...]
	blsSig := make([]byte, 96)  // 96 byte BLS signature
	rtSig := make([]byte, 100)  // Ringtail signature
	quasarSig := make([]byte, 0)
	quasarSig = append(quasarSig, SigQuasar)
	quasarSig = append(quasarSig, byte(len(blsSig)>>8), byte(len(blsSig))) // BLS length
	quasarSig = append(quasarSig, blsSig...)
	quasarSig = append(quasarSig, rtSig...)

	quasarVote := NewVote(candidate.ID, voter1, 0, true)
	quasarVote.Signature = quasarSig

	err = policy.OnVote(ctx, quasarVote)
	if err != nil {
		t.Errorf("Quasar vote should be accepted: %v", err)
	}

	// Test 4: Malformed Quasar vote (missing RT component) should be rejected
	malformedSig := make([]byte, 0)
	malformedSig = append(malformedSig, SigQuasar)
	malformedSig = append(malformedSig, byte(len(blsSig)>>8), byte(len(blsSig)))
	malformedSig = append(malformedSig, blsSig...)
	// No RT component

	malformedVote := NewVote(candidate.ID, DeriveVoterID("agent", []byte("voter2")), 0, true)
	malformedVote.Signature = malformedSig

	err = policy.OnVote(ctx, malformedVote)
	if err == nil {
		t.Error("Malformed Quasar vote (missing RT) should be rejected")
	}
}

func TestQuantumPolicyRTDisabled(t *testing.T) {
	ctx := context.Background()
	policy := NewQuantumPolicyWithOptions(2, false) // RT NOT required

	candidate := NewCandidate([]byte("test"), []byte("payload"), EmptyCandidateID, 1)
	policy.OnCandidate(ctx, candidate)

	// BLS-only vote should be accepted when RT is not required
	voter1 := DeriveVoterID("agent", []byte("voter1"))
	blsVote := NewVote(candidate.ID, voter1, 0, true)
	blsVote.Signature = []byte{SigBLS, 1, 2, 3}

	err := policy.OnVote(ctx, blsVote)
	if err != nil {
		t.Errorf("BLS-only vote should be accepted when RT is not required: %v", err)
	}
}

// =============================================================================
// HOSTNAME VALIDATION TESTS
// =============================================================================

func TestHostnameValidation(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		wantValid bool
	}{
		// Valid hostnames
		{"simple hostname", "example.com", true},
		{"hostname with port", "example.com:9651", true},
		{"subdomain", "node1.lux.network:9651", true},
		{"localhost", "localhost:9651", true},
		{"single word", "mynode", true},

		// Invalid - IPv4 literals
		{"IPv4", "192.168.1.1", false},
		{"IPv4 with port", "192.168.1.1:9651", false},
		{"IPv4 localhost", "127.0.0.1", false},
		{"IPv4 localhost with port", "127.0.0.1:9651", false},

		// Invalid - IPv6 literals
		{"IPv6 localhost", "::1", false},
		{"IPv6 with port", "[::1]:9651", false},
		{"IPv6 full", "2001:db8::1", false},
		{"IPv6 full with port", "[2001:db8::1]:9651", false},

		// Edge cases
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostnameOnly(tt.addr)
			gotValid := err == nil

			if gotValid != tt.wantValid {
				t.Errorf("ValidateHostnameOnly(%q) = %v, want valid=%v", tt.addr, err, tt.wantValid)
			}

			// Test IsValidHostname too
			if IsValidHostname(tt.addr) != tt.wantValid {
				t.Errorf("IsValidHostname(%q) mismatch", tt.addr)
			}
		})
	}
}

func TestHostnameValidationError(t *testing.T) {
	err := ValidateHostnameOnly("192.168.1.1")
	if err == nil {
		t.Fatal("Expected error for IPv4 literal")
	}

	hvErr, ok := err.(*HostnameValidationError)
	if !ok {
		t.Fatalf("Expected HostnameValidationError, got %T", err)
	}

	if hvErr.Address != "192.168.1.1" {
		t.Errorf("Wrong address in error: %s", hvErr.Address)
	}
	if hvErr.Reason == "" {
		t.Error("Error reason should not be empty")
	}
}

// =============================================================================
// ML-DSA CREDENTIAL TESTS
// =============================================================================

func TestMLDSACredentialTypes(t *testing.T) {
	// Test credential creation
	cred := NewCredential(CredentialTypeMLDSA65)
	if cred.Type != CredentialTypeMLDSA65 {
		t.Errorf("Expected MLDSA65, got %d", cred.Type)
	}
	if !cred.IsPostQuantum() {
		t.Error("MLDSA65 should be post-quantum")
	}

	// Test non-PQ credential
	ecCred := NewCredential(CredentialTypeSecp256k1)
	if ecCred.IsPostQuantum() {
		t.Error("Secp256k1 should not be post-quantum")
	}
}

func TestMLDSACredentialSerialization(t *testing.T) {
	cred := NewCredential(CredentialTypeMLDSA65)

	// Add some signatures (using fake data for testing)
	cred.AddSignature(make([]byte, MLDSA65SignatureSize))
	cred.AddSignature(make([]byte, MLDSA65SignatureSize))

	// Serialize
	data := cred.Serialize()
	if len(data) == 0 {
		t.Error("Serialized credential should not be empty")
	}

	// Deserialize
	cred2, err := DeserializeCredential(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if cred2.Type != cred.Type {
		t.Error("Type mismatch after roundtrip")
	}
	if len(cred2.Signatures) != len(cred.Signatures) {
		t.Error("Signature count mismatch after roundtrip")
	}
}

func TestMLDSASignatureValidation(t *testing.T) {
	cred := NewCredential(CredentialTypeMLDSA65)

	// Add correct size signature
	cred.AddSignature(make([]byte, MLDSA65SignatureSize))

	err := cred.ValidateSignatureSizes()
	if err != nil {
		t.Errorf("Valid signature size should pass: %v", err)
	}

	// Add wrong size signature
	cred.AddSignature(make([]byte, 100)) // Wrong size

	err = cred.ValidateSignatureSizes()
	if err == nil {
		t.Error("Invalid signature size should fail")
	}
}

func TestMLDSAOutputOwners(t *testing.T) {
	// Create some fake public keys
	pk1 := make([]byte, MLDSA65PublicKeySize)
	pk2 := make([]byte, MLDSA65PublicKeySize)
	pk1[0] = 1
	pk2[0] = 2

	owners := NewMLDSAOutputOwners(2, [][]byte{pk1, pk2}, CredentialTypeMLDSA65)

	if owners.Threshold != 2 {
		t.Errorf("Expected threshold 2, got %d", owners.Threshold)
	}
	if owners.AddressType != CredentialTypeMLDSA65 {
		t.Error("Wrong address type")
	}
	if len(owners.Addresses) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(owners.Addresses))
	}
	if !owners.IsPostQuantum() {
		t.Error("Should be post-quantum")
	}

	// Addresses should be hashes (32 bytes)
	for i, addr := range owners.Addresses {
		if len(addr) != 32 {
			t.Errorf("Address %d should be 32 bytes, got %d", i, len(addr))
		}
	}
}

func TestCredentialTypeName(t *testing.T) {
	tests := []struct {
		credType byte
		expected string
	}{
		{CredentialTypeSecp256k1, "secp256k1"},
		{CredentialTypeEd25519, "ed25519"},
		{CredentialTypeBLS, "bls12-381"},
		{CredentialTypeMLDSA44, "ML-DSA-44"},
		{CredentialTypeMLDSA65, "ML-DSA-65"},
		{CredentialTypeMLDSA87, "ML-DSA-87"},
		{0xFF, "unknown"},
	}

	for _, tt := range tests {
		got := CredentialTypeName(tt.credType)
		if got != tt.expected {
			t.Errorf("CredentialTypeName(%d) = %s, want %s", tt.credType, got, tt.expected)
		}
	}
}

func TestMLDSASizes(t *testing.T) {
	// Verify size constants match FIPS 204 spec
	if SignatureSizeForType(CredentialTypeMLDSA44) != 2420 {
		t.Error("MLDSA44 signature size wrong")
	}
	if SignatureSizeForType(CredentialTypeMLDSA65) != 3293 {
		t.Error("MLDSA65 signature size wrong")
	}
	if SignatureSizeForType(CredentialTypeMLDSA87) != 4595 {
		t.Error("MLDSA87 signature size wrong")
	}

	if PublicKeySizeForType(CredentialTypeMLDSA44) != 1312 {
		t.Error("MLDSA44 public key size wrong")
	}
	if PublicKeySizeForType(CredentialTypeMLDSA65) != 1952 {
		t.Error("MLDSA65 public key size wrong")
	}
	if PublicKeySizeForType(CredentialTypeMLDSA87) != 2592 {
		t.Error("MLDSA87 public key size wrong")
	}
}

func TestRecommendedMLDSAType(t *testing.T) {
	recommended := RecommendedMLDSAType()
	if recommended != CredentialTypeMLDSA65 {
		t.Errorf("Expected MLDSA65 as recommended, got %d", recommended)
	}
}
