// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"encoding/json"
)

// =============================================================================
// BACKWARD COMPATIBILITY: Legacy wire types
// =============================================================================
// These types are preserved for backward compatibility with existing code.
// New code should use Candidate, Vote (from candidate.go), and Certificate.
// =============================================================================

// EmptyID is the zero item ID (deprecated: use EmptyCandidateID)
var EmptyID = EmptyCandidateID

// Result represents the outcome of a consensus round.
// This is a legacy type; new code should use Certificate with policy-based proofs.
type Result struct {
	// ItemID is the 32-byte ID of the item that reached consensus
	ItemID ItemID `json:"item_id"`

	// Finalized indicates if the consensus reached finality
	Finalized bool `json:"finalized"`

	// Accepted indicates the final decision (true=accepted, false=rejected)
	Accepted bool `json:"accepted"`

	// Confidence is a 0-100 score indicating consensus strength
	Confidence int `json:"confidence"`

	// Signatures is the list of voter signatures forming the certificate
	Signatures [][]byte `json:"signatures,omitempty"`

	// Synthesis is the final output text (for AI consensus)
	Synthesis string `json:"synthesis,omitempty"`
}

// ConsensusParams holds the metastable consensus parameters.
// Both blockchain and AI consensus use the same core parameters.
type ConsensusParams struct {
	// K is the sample size per round
	K int `json:"k"`

	// Alpha is the agreement threshold (0.0-1.0)
	Alpha float64 `json:"alpha"`

	// Beta1 is the preference threshold (0.0-1.0) - Phase I
	Beta1 float64 `json:"beta_1"`

	// Beta2 is the decision threshold (0.0-1.0) - Phase II
	Beta2 float64 `json:"beta_2"`

	// Rounds is the maximum number of sampling rounds
	Rounds int `json:"rounds"`
}

// DefaultParams returns default consensus parameters
func DefaultParams() ConsensusParams {
	return ConsensusParams{
		K:      3,
		Alpha:  0.6,
		Beta1:  0.5,
		Beta2:  0.8,
		Rounds: 3,
	}
}

// BlockchainParams returns parameters tuned for blockchain consensus
func BlockchainParams() ConsensusParams {
	return ConsensusParams{
		K:      20,
		Alpha:  0.65,
		Beta1:  0.5,
		Beta2:  0.8,
		Rounds: 10,
	}
}

// AIAgentParams returns parameters tuned for AI agent consensus
func AIAgentParams() ConsensusParams {
	return ConsensusParams{
		K:      3,
		Alpha:  0.6,
		Beta1:  0.5,
		Beta2:  0.8,
		Rounds: 3,
	}
}

// =============================================================================
// JSON SERIALIZATION
// =============================================================================

// MarshalCandidate serializes a candidate to JSON
func MarshalCandidate(c *Candidate) ([]byte, error) {
	return json.Marshal(c)
}

// UnmarshalCandidate deserializes a candidate from JSON
func UnmarshalCandidate(data []byte) (*Candidate, error) {
	var c Candidate
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// MarshalVote serializes a vote to JSON
func MarshalVote(v *Vote) ([]byte, error) {
	return json.Marshal(v)
}

// UnmarshalVote deserializes a vote from JSON
func UnmarshalVote(data []byte) (*Vote, error) {
	var v Vote
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// MarshalCertificate serializes a certificate to JSON
func MarshalCertificate(c *Certificate) ([]byte, error) {
	return json.Marshal(c)
}

// UnmarshalCertificate deserializes a certificate from JSON
func UnmarshalCertificate(data []byte) (*Certificate, error) {
	var c Certificate
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// MarshalResult serializes a result to JSON
func MarshalResult(r *Result) ([]byte, error) {
	return json.Marshal(r)
}

// UnmarshalResult deserializes a result from JSON
func UnmarshalResult(data []byte) (*Result, error) {
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// MarshalValidatorSet serializes a validator set to JSON
func MarshalValidatorSet(vs *ValidatorSet) ([]byte, error) {
	return json.Marshal(vs)
}

// UnmarshalValidatorSet deserializes a validator set from JSON
func UnmarshalValidatorSet(data []byte) (*ValidatorSet, error) {
	var vs ValidatorSet
	if err := json.Unmarshal(data, &vs); err != nil {
		return nil, err
	}
	return &vs, nil
}
