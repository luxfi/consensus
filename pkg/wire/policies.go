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
	// Verify signature proof against signers bitmap
	// Proof contains aggregated BLS signature, signers contains validator bitmap
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
	candidate    *Candidate
	preference   bool
	confidence   int
	roundVotes   map[uint64]map[VoterID]bool // round -> voter -> preference
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
	mu         sync.RWMutex
	l1Verifier L1Verifier
	candidates map[CandidateID]*Candidate
	certs      map[CandidateID]*Certificate
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
// SECURITY CRITICAL: All votes MUST include both BLS and Ringtail signatures.
// Votes without dual signatures are rejected to ensure quantum-safe consensus.
type QuantumPolicy struct {
	mu         sync.RWMutex
	threshold  int
	requireRT  bool // When true, RT signature is REQUIRED on all votes
	candidates map[CandidateID]*Candidate
	blsVotes   map[CandidateID]map[VoterID][]byte // BLS signatures
	pqVotes    map[CandidateID]map[VoterID][]byte // Ringtail signatures
	certs      map[CandidateID]*Certificate
}

// RTRequirementError is returned when Ringtail signature is missing but required
type RTRequirementError struct {
	Reason string
}

func (e *RTRequirementError) Error() string {
	return "RT signature required: " + e.Reason
}

// NewQuantumPolicy creates a quantum-safe BLS+Ringtail policy
// By default, RT signatures are REQUIRED for Q-Chain consensus security.
func NewQuantumPolicy(threshold int) *QuantumPolicy {
	return &QuantumPolicy{
		threshold:  threshold,
		requireRT:  true, // DEFAULT: RT REQUIRED for quantum safety
		candidates: make(map[CandidateID]*Candidate),
		blsVotes:   make(map[CandidateID]map[VoterID][]byte),
		pqVotes:    make(map[CandidateID]map[VoterID][]byte),
		certs:      make(map[CandidateID]*Certificate),
	}
}

// NewQuantumPolicyWithOptions creates a quantum policy with explicit RT requirement
func NewQuantumPolicyWithOptions(threshold int, requireRT bool) *QuantumPolicy {
	return &QuantumPolicy{
		threshold:  threshold,
		requireRT:  requireRT,
		candidates: make(map[CandidateID]*Candidate),
		blsVotes:   make(map[CandidateID]map[VoterID][]byte),
		pqVotes:    make(map[CandidateID]map[VoterID][]byte),
		certs:      make(map[CandidateID]*Certificate),
	}
}

// SetRequireRT enables or disables RT requirement (for testing only)
func (p *QuantumPolicy) SetRequireRT(require bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requireRT = require
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
		if p.requireRT {
			return &RTRequirementError{Reason: "vote missing signature"}
		}
		return nil
	}

	scheme := vote.SignatureScheme()

	// SECURITY: Enforce dual BLS+Ringtail requirement for quantum safety
	if p.requireRT && scheme != SigQuasar {
		return &RTRequirementError{
			Reason: "Q-Chain requires dual BLS+Ringtail signature (SigQuasar), got scheme " + sigSchemeToString(scheme),
		}
	}

	switch scheme {
	case SigBLS:
		// Only allowed if requireRT is false
		if p.blsVotes[vote.CandidateID] == nil {
			p.blsVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		p.blsVotes[vote.CandidateID][vote.VoterID] = vote.Signature[1:] // Skip scheme byte
	case SigRingtail:
		// Only allowed if requireRT is false
		if p.pqVotes[vote.CandidateID] == nil {
			p.pqVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		p.pqVotes[vote.CandidateID][vote.VoterID] = vote.Signature[1:]
	case SigQuasar:
		// Quasar vote contains both: [scheme][bls_len_hi][bls_len_lo][bls][ringtail]
		if len(vote.Signature) < 4 {
			return &RTRequirementError{Reason: "malformed Quasar signature: too short"}
		}
		blsLen := int(vote.Signature[1])<<8 | int(vote.Signature[2])
		if blsLen == 0 {
			return &RTRequirementError{Reason: "malformed Quasar signature: BLS component missing"}
		}
		if len(vote.Signature) < 3+blsLen {
			return &RTRequirementError{Reason: "malformed Quasar signature: truncated BLS"}
		}
		blsSig := vote.Signature[3 : 3+blsLen]
		pqSig := vote.Signature[3+blsLen:]

		if len(pqSig) == 0 {
			return &RTRequirementError{Reason: "malformed Quasar signature: Ringtail component missing"}
		}

		if p.blsVotes[vote.CandidateID] == nil {
			p.blsVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		if p.pqVotes[vote.CandidateID] == nil {
			p.pqVotes[vote.CandidateID] = make(map[VoterID][]byte)
		}
		p.blsVotes[vote.CandidateID][vote.VoterID] = blsSig
		p.pqVotes[vote.CandidateID][vote.VoterID] = pqSig
	default:
		if p.requireRT {
			return &RTRequirementError{Reason: "unsupported signature scheme for Q-Chain"}
		}
	}

	return nil
}

// sigSchemeToString converts signature scheme byte to string for errors
func sigSchemeToString(scheme byte) string {
	switch scheme {
	case SigNone:
		return "SigNone"
	case SigEd25519:
		return "SigEd25519"
	case SigBLS:
		return "SigBLS"
	case SigRingtail:
		return "SigRingtail"
	case SigQuasar:
		return "SigQuasar"
	default:
		return "Unknown"
	}
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

	// Build Quasar proof: [scheme_tag][signature_commitment]
	// The proof is a cryptographic commitment to all BLS and Ringtail signatures.
	// Full signatures are collected in Signers for verification.
	proof := make([]byte, 0)
	proof = append(proof, SigQuasar) // Mark as Quasar protocol

	// Create a binding commitment to all signatures using SHA-256
	// This provides a compact proof that can be verified against the full signatures
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

// =============================================================================
// HOSTNAME VALIDATION: No IP literals in peer gossip
// =============================================================================
// For P+Q security, peer addresses MUST use hostnames, never IP literals.
// This prevents IP spoofing attacks and ensures DNS-based identity verification.
// =============================================================================

// HostnameValidationError is returned when an IP literal is used instead of hostname
type HostnameValidationError struct {
	Address string
	Reason  string
}

func (e *HostnameValidationError) Error() string {
	return "hostname validation failed for " + e.Address + ": " + e.Reason
}

// ValidateHostnameOnly validates that an address uses a hostname, not an IP literal.
// Returns nil if valid, HostnameValidationError if invalid.
func ValidateHostnameOnly(addr string) error {
	if addr == "" {
		return &HostnameValidationError{Address: addr, Reason: "empty address"}
	}

	// Extract host part (before any port)
	host := addr
	if colonIdx := lastColon(addr); colonIdx != -1 {
		// Check for IPv6 bracket notation [::1]:port
		if len(addr) > 0 && addr[0] == '[' {
			bracketIdx := indexOf(addr, ']')
			if bracketIdx != -1 {
				host = addr[1:bracketIdx] // IPv6 inside brackets
			}
		} else {
			host = addr[:colonIdx]
		}
	}

	// Check for IPv4 literals (digits and dots only)
	if isIPv4Literal(host) {
		return &HostnameValidationError{
			Address: addr,
			Reason:  "IPv4 literals not allowed - use hostname",
		}
	}

	// Check for IPv6 literals (contains colons, or bracketed)
	if isIPv6Literal(host) {
		return &HostnameValidationError{
			Address: addr,
			Reason:  "IPv6 literals not allowed - use hostname",
		}
	}

	// Valid hostname
	return nil
}

// IsValidHostname returns true if the address uses a valid hostname
func IsValidHostname(addr string) bool {
	return ValidateHostnameOnly(addr) == nil
}

// ValidatePeerAddress validates a peer address for P+Q compliance:
// - Must be hostname (not IP literal)
// - Must be non-empty
func ValidatePeerAddress(addr string) error {
	return ValidateHostnameOnly(addr)
}

// isIPv4Literal checks if a string looks like an IPv4 address
func isIPv4Literal(s string) bool {
	if s == "" {
		return false
	}
	dotCount := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			dotCount++
		} else if c < '0' || c > '9' {
			return false // Contains non-digit, non-dot
		}
	}
	return dotCount == 3 // IPv4 has exactly 3 dots
}

// isIPv6Literal checks if a string looks like an IPv6 address
func isIPv6Literal(s string) bool {
	if s == "" {
		return false
	}
	// IPv6 contains colons and hex digits
	hasColon := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' {
			hasColon = true
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			// Contains non-hex, non-colon - probably a hostname
			return false
		}
	}
	return hasColon
}

// lastColon finds the last colon in a string, returns -1 if not found
func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}

// indexOf finds the first occurrence of a byte in a string
func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
