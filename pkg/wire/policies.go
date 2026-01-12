// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"context"
	"crypto/sha256"
	"sync"
)

// =============================================================================
// POLICY IMPLEMENTATIONS
// =============================================================================
//
// Each policy implements FinalityPolicy for different K scenarios:
// - NonePolicy: K=1 self-sequencing (immediate finality)
// - QuorumPolicy: K=small threshold signature (3/5, 2/3)
// - SamplePolicy: K=large metastable sampling
// - L1Policy: K=external chain inclusion (OP Stack)
// - QuantumPolicy: BLS + Ringtail post-quantum
// =============================================================================

// =============================================================================
// NONE POLICY: K=1 Self-Sequencing
// =============================================================================

// NonePolicy provides immediate finality for single-node operation
type NonePolicy struct {
	mu         sync.RWMutex
	candidates map[CandidateID]*Candidate
	certs      map[CandidateID]*Certificate
}

// NewNonePolicy creates a none policy
func NewNonePolicy() *NonePolicy {
	return &NonePolicy{
		candidates: make(map[CandidateID]*Candidate),
		certs:      make(map[CandidateID]*Certificate),
	}
}

func (p *NonePolicy) PolicyID() PolicyID {
	return PolicyNone
}

func (p *NonePolicy) OnCandidate(ctx context.Context, candidate *Candidate) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.candidates[candidate.ID] = candidate
	return nil
}

func (p *NonePolicy) OnVote(ctx context.Context, vote *Vote) error {
	// No votes needed for K=1
	return nil
}

func (p *NonePolicy) MaybeFinalize(ctx context.Context, candidateID CandidateID) (*Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cert, ok := p.certs[candidateID]; ok {
		return cert, nil
	}

	candidate, ok := p.candidates[candidateID]
	if !ok {
		return nil, nil // Not observed yet
	}

	// Immediate finality - create self-signed certificate
	cert := &Certificate{
		CandidateID: candidateID,
		Height:      candidate.Height,
		PolicyID:    PolicyNone,
		Proof:       []byte("self"), // Minimal proof
	}
	p.certs[candidateID] = cert
	return cert, nil
}

func (p *NonePolicy) Verify(ctx context.Context, cert *Certificate) (bool, error) {
	return cert.PolicyID == PolicyNone, nil
}

// =============================================================================
// QUORUM POLICY: K=small Threshold Signature
// =============================================================================

// QuorumPolicy provides threshold-based finality
type QuorumPolicy struct {
	mu         sync.RWMutex
	threshold  int // Number of votes needed (e.g., 3 of 5)
	total      int // Total validators
	candidates map[CandidateID]*Candidate
	votes      map[CandidateID]map[VoterID]*Vote
	certs      map[CandidateID]*Certificate
}

// NewQuorumPolicy creates a quorum policy
func NewQuorumPolicy(threshold, total int) *QuorumPolicy {
	return &QuorumPolicy{
		threshold:  threshold,
		total:      total,
		candidates: make(map[CandidateID]*Candidate),
		votes:      make(map[CandidateID]map[VoterID]*Vote),
		certs:      make(map[CandidateID]*Certificate),
	}
}

func (p *QuorumPolicy) PolicyID() PolicyID {
	return PolicyQuorum
}

func (p *QuorumPolicy) OnCandidate(ctx context.Context, candidate *Candidate) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.candidates[candidate.ID] = candidate
	if _, ok := p.votes[candidate.ID]; !ok {
		p.votes[candidate.ID] = make(map[VoterID]*Vote)
	}
	return nil
}

func (p *QuorumPolicy) OnVote(ctx context.Context, vote *Vote) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.votes[vote.CandidateID]; !ok {
		p.votes[vote.CandidateID] = make(map[VoterID]*Vote)
	}
	p.votes[vote.CandidateID][vote.VoterID] = vote
	return nil
}

func (p *QuorumPolicy) MaybeFinalize(ctx context.Context, candidateID CandidateID) (*Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cert, ok := p.certs[candidateID]; ok {
		return cert, nil
	}

	candidate, ok := p.candidates[candidateID]
	if !ok {
		return nil, nil
	}

	votes := p.votes[candidateID]
	acceptVotes := 0
	for _, v := range votes {
		if v.Preference {
			acceptVotes++
		}
	}

	if acceptVotes < p.threshold {
		return nil, nil // Not enough votes
	}

	// Build certificate proof: aggregated signatures
	var proof []byte
	var signers []byte
	for voterID, vote := range votes {
		if vote.Preference && len(vote.Signature) > 0 {
			proof = append(proof, vote.Signature...)
			signers = append(signers, voterID[:]...)
		}
	}

	cert := &Certificate{
		CandidateID: candidateID,
		Height:      candidate.Height,
		PolicyID:    PolicyQuorum,
		Proof:       proof,
		Signers:     signers,
	}
	p.certs[candidateID] = cert
	return cert, nil
}

func (p *QuorumPolicy) Verify(ctx context.Context, cert *Certificate) (bool, error) {
	if cert.PolicyID != PolicyQuorum {
		return false, nil
	}
	// In production: verify each signature in proof against signers
	return len(cert.Proof) > 0 && len(cert.Signers) >= p.threshold*32, nil
}

// =============================================================================
// SAMPLE POLICY: K=large Metastable Sampling
// =============================================================================

// SamplePolicy provides metastable consensus for large validator sets
type SamplePolicy struct {
	mu         sync.RWMutex
	k          int     // Sample size per round
	alpha      float64 // Agreement threshold
	beta       int     // Consecutive rounds needed
	candidates map[CandidateID]*sampleState
	certs      map[CandidateID]*Certificate
}

type sampleState struct {
	candidate   *Candidate
	preference  bool
	confidence  int
	roundVotes  map[uint64]map[VoterID]bool // round -> voter -> preference
	currentRound uint64
}

// NewSamplePolicy creates a sample convergence policy
func NewSamplePolicy(k int, alpha float64, beta int) *SamplePolicy {
	return &SamplePolicy{
		k:          k,
		alpha:      alpha,
		beta:       beta,
		candidates: make(map[CandidateID]*sampleState),
		certs:      make(map[CandidateID]*Certificate),
	}
}

func (p *SamplePolicy) PolicyID() PolicyID {
	return PolicySampleConvergence
}

func (p *SamplePolicy) OnCandidate(ctx context.Context, candidate *Candidate) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.candidates[candidate.ID] = &sampleState{
		candidate:  candidate,
		preference: true, // Initial preference
		roundVotes: make(map[uint64]map[VoterID]bool),
	}
	return nil
}

func (p *SamplePolicy) OnVote(ctx context.Context, vote *Vote) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.candidates[vote.CandidateID]
	if !ok {
		return nil
	}

	if _, ok := state.roundVotes[vote.Round]; !ok {
		state.roundVotes[vote.Round] = make(map[VoterID]bool)
	}
	state.roundVotes[vote.Round][vote.VoterID] = vote.Preference

	// Check if round is complete
	roundVotes := state.roundVotes[vote.Round]
	if len(roundVotes) >= p.k {
		// Count votes
		yes := 0
		for _, pref := range roundVotes {
			if pref {
				yes++
			}
		}

		threshold := int(float64(p.k) * p.alpha)
		newPref := yes >= threshold

		if newPref == state.preference {
			state.confidence++
		} else {
			state.preference = newPref
			state.confidence = 1
		}
		state.currentRound = vote.Round
	}

	return nil
}

func (p *SamplePolicy) MaybeFinalize(ctx context.Context, candidateID CandidateID) (*Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cert, ok := p.certs[candidateID]; ok {
		return cert, nil
	}

	state, ok := p.candidates[candidateID]
	if !ok {
		return nil, nil
	}

	if state.confidence < p.beta {
		return nil, nil // Not converged yet
	}

	if !state.preference {
		return nil, nil // Converged to reject
	}

	// Build proof: confidence score + final round votes
	proof := make([]byte, 0, 100)
	proof = append(proof, byte(state.confidence))
	proof = append(proof, Uint64ToBytes(state.currentRound)...)

	cert := &Certificate{
		CandidateID: candidateID,
		Height:      state.candidate.Height,
		PolicyID:    PolicySampleConvergence,
		Proof:       proof,
	}
	p.certs[candidateID] = cert
	return cert, nil
}

func (p *SamplePolicy) Verify(ctx context.Context, cert *Certificate) (bool, error) {
	if cert.PolicyID != PolicySampleConvergence {
		return false, nil
	}
	if len(cert.Proof) < 9 {
		return false, nil
	}
	confidence := int(cert.Proof[0])
	return confidence >= p.beta, nil
}

// =============================================================================
// L1 INCLUSION POLICY: External Chain Finality (OP Stack)
// =============================================================================

// L1Policy provides finality via L1 chain inclusion
type L1Policy struct {
	mu          sync.RWMutex
	l1Verifier  L1Verifier
	candidates  map[CandidateID]*Candidate
	certs       map[CandidateID]*Certificate
}

// L1Verifier verifies L1 inclusion proofs
type L1Verifier interface {
	// VerifyInclusion checks if candidate is included in L1
	VerifyInclusion(ctx context.Context, candidateID CandidateID, proof []byte) (bool, error)

	// GetInclusionProof retrieves inclusion proof for a candidate
	GetInclusionProof(ctx context.Context, candidateID CandidateID) ([]byte, error)
}

// NewL1Policy creates an L1 inclusion policy
func NewL1Policy(verifier L1Verifier) *L1Policy {
	return &L1Policy{
		l1Verifier: verifier,
		candidates: make(map[CandidateID]*Candidate),
		certs:      make(map[CandidateID]*Certificate),
	}
}

func (p *L1Policy) PolicyID() PolicyID {
	return PolicyL1Inclusion
}

func (p *L1Policy) OnCandidate(ctx context.Context, candidate *Candidate) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.candidates[candidate.ID] = candidate
	return nil
}

func (p *L1Policy) OnVote(ctx context.Context, vote *Vote) error {
	// L1 policy doesn't use votes
	return nil
}

func (p *L1Policy) MaybeFinalize(ctx context.Context, candidateID CandidateID) (*Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cert, ok := p.certs[candidateID]; ok {
		return cert, nil
	}

	candidate, ok := p.candidates[candidateID]
	if !ok {
		return nil, nil
	}

	// Get L1 inclusion proof
	proof, err := p.l1Verifier.GetInclusionProof(ctx, candidateID)
	if err != nil || proof == nil {
		return nil, nil // Not included yet
	}

	cert := &Certificate{
		CandidateID: candidateID,
		Height:      candidate.Height,
		PolicyID:    PolicyL1Inclusion,
		Proof:       proof,
	}
	p.certs[candidateID] = cert
	return cert, nil
}

func (p *L1Policy) Verify(ctx context.Context, cert *Certificate) (bool, error) {
	if cert.PolicyID != PolicyL1Inclusion {
		return false, nil
	}
	return p.l1Verifier.VerifyInclusion(ctx, cert.CandidateID, cert.Proof)
}

// =============================================================================
// QUANTUM POLICY: BLS + Ringtail Post-Quantum
// =============================================================================

// QuantumPolicy combines BLS and Ringtail post-quantum signatures
type QuantumPolicy struct {
	mu         sync.RWMutex
	threshold  int
	candidates map[CandidateID]*Candidate
	blsVotes   map[CandidateID]map[VoterID][]byte // BLS signatures
	pqVotes    map[CandidateID]map[VoterID][]byte // Ringtail signatures
	certs      map[CandidateID]*Certificate
}

// NewQuantumPolicy creates a quantum-safe BLS+Ringtail policy
func NewQuantumPolicy(threshold int) *QuantumPolicy {
	return &QuantumPolicy{
		threshold:  threshold,
		candidates: make(map[CandidateID]*Candidate),
		blsVotes:   make(map[CandidateID]map[VoterID][]byte),
		pqVotes:    make(map[CandidateID]map[VoterID][]byte),
		certs:      make(map[CandidateID]*Certificate),
	}
}

func (p *QuantumPolicy) PolicyID() PolicyID {
	return PolicyQuantum
}

func (p *QuantumPolicy) OnCandidate(ctx context.Context, candidate *Candidate) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.candidates[candidate.ID] = candidate
	p.blsVotes[candidate.ID] = make(map[VoterID][]byte)
	p.pqVotes[candidate.ID] = make(map[VoterID][]byte)
	return nil
}

func (p *QuantumPolicy) OnVote(ctx context.Context, vote *Vote) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !vote.Preference {
		return nil // Only track accept votes for cert
	}

	if len(vote.Signature) == 0 {
		return nil
	}

	scheme := vote.SignatureScheme()
	switch scheme {
	case SigBLS:
		if p.blsVotes[vote.CandidateID] == nil {
			p.blsVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		p.blsVotes[vote.CandidateID][vote.VoterID] = vote.Signature[1:] // Skip scheme byte
	case SigRingtail:
		if p.pqVotes[vote.CandidateID] == nil {
			p.pqVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		p.pqVotes[vote.CandidateID][vote.VoterID] = vote.Signature[1:]
	case SigQuasar:
		// Quasar vote contains both: [scheme][bls_len][bls][ringtail]
		if len(vote.Signature) < 4 {
			return nil
		}
		blsLen := int(vote.Signature[1])<<8 | int(vote.Signature[2])
		if len(vote.Signature) < 3+blsLen {
			return nil
		}
		blsSig := vote.Signature[3 : 3+blsLen]
		pqSig := vote.Signature[3+blsLen:]

		if p.blsVotes[vote.CandidateID] == nil {
			p.blsVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		if p.pqVotes[vote.CandidateID] == nil {
			p.pqVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		p.blsVotes[vote.CandidateID][vote.VoterID] = blsSig
		p.pqVotes[vote.CandidateID][vote.VoterID] = pqSig
	}

	return nil
}

func (p *QuantumPolicy) MaybeFinalize(ctx context.Context, candidateID CandidateID) (*Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cert, ok := p.certs[candidateID]; ok {
		return cert, nil
	}

	candidate, ok := p.candidates[candidateID]
	if !ok {
		return nil, nil
	}

	// Need threshold of BOTH BLS and PQ signatures
	blsCount := len(p.blsVotes[candidateID])
	pqCount := len(p.pqVotes[candidateID])

	if blsCount < p.threshold || pqCount < p.threshold {
		return nil, nil
	}

	// Build Quasar proof: [bls_aggregate][ringtail_signatures...]
	// In production: aggregate BLS signatures, concat Ringtail signatures
	proof := make([]byte, 0)
	proof = append(proof, SigQuasar) // Mark as Quasar

	// Placeholder: just hash the signatures for demo
	h := sha256.New()
	for _, sig := range p.blsVotes[candidateID] {
		h.Write(sig)
	}
	for _, sig := range p.pqVotes[candidateID] {
		h.Write(sig)
	}
	proof = append(proof, h.Sum(nil)...)

	var signers []byte
	for voter := range p.blsVotes[candidateID] {
		signers = append(signers, voter[:]...)
	}

	cert := &Certificate{
		CandidateID: candidateID,
		Height:      candidate.Height,
		PolicyID:    PolicyQuantum,
		Proof:       proof,
		Signers:     signers,
	}
	p.certs[candidateID] = cert
	return cert, nil
}

func (p *QuantumPolicy) Verify(ctx context.Context, cert *Certificate) (bool, error) {
	if cert.PolicyID != PolicyQuantum {
		return false, nil
	}
	if len(cert.Proof) < 33 { // scheme + hash
		return false, nil
	}
	return cert.Proof[0] == SigQuasar, nil
}
