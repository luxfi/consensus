// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

/*
Package wire provides a unified sequencer stack that scales from K=1 to millions.

# Core Invariant

Everything is a Candidate with a canonical ID:

	candidate_id = H(domain || payload_bytes)

A network produces:
  - Candidate: what's being ordered
  - Attestations (Vote): who agrees
  - Certificate: proof of agreement per finality policy

# The 4-Layer Sequencer Stack

	┌─────────────────────────────────────────────────────────────────┐
	│                     SEQUENCER PIPELINE                          │
	├─────────────────────────────────────────────────────────────────┤
	│  1) Execution Payload                                           │
	│     What gets executed: tx list, state transition, AI decision  │
	├─────────────────────────────────────────────────────────────────┤
	│  2) Ordering (Proposer)                                         │
	│     Who proposes next candidate, conflict resolution            │
	├─────────────────────────────────────────────────────────────────┤
	│  3) Data Availability                                           │
	│     Where bytes live: local, IPFS, blob, P2P, MCP mesh          │
	├─────────────────────────────────────────────────────────────────┤
	│  4) Finality (FinalityPolicy)                                   │
	│     What proof required: none, quorum, sample, L1, hybrid       │
	└─────────────────────────────────────────────────────────────────┘

# Two-Phase Agreement

All modes use the same two-phase semantics:

  - Phase 1 (Soft): Fast, optimistic finality
  - Phase 2 (Hard): Slow, strong proof

Even OP Stack fits: "soft" is sequencer head, "hard" is L1 + dispute window.

# Supported Configurations

K=1 (Single Node):

	cfg := wire.SingleNodeConfig(domain)
	// Ordering: single proposer
	// DA: local
	// Finality: immediate (PolicyNone)

K=3/5 (Agent Mesh):

	cfg := wire.AgentMeshConfig(domain, 5)
	// Ordering: round-robin or leaderless
	// DA: MCP mesh / gossip
	// Finality: 3-of-5 quorum (PolicyQuorum)

K=large (Blockchain):

	cfg := wire.BlockchainConfig(domain)
	// Ordering: PoS/VRF leader election
	// DA: P2P + state sync
	// Finality: metastable sampling + quantum cert (PolicyQuantum)

K=external (OP Stack Rollup):

	cfg := wire.RollupConfig(domain)
	// Ordering: centralized sequencer
	// DA: blob DA
	// Finality: L1 inclusion (PolicyL1Inclusion)

# Wire Format

All messages use the same format regardless of K:

	type Candidate struct {
	    ID       CandidateID  // H(domain || payload)
	    ParentID CandidateID  // Previous candidate
	    Height   uint64       // Sequence number
	    Domain   []byte       // Context identifier
	    Payload  []byte       // Actual content
	    DARef    string       // Data availability reference
	}

	type Vote struct {
	    CandidateID CandidateID // What's being voted on
	    VoterID     VoterID     // Who's voting
	    Round       uint64      // Voting round
	    Preference  bool        // Accept or reject
	    Signature   []byte      // Scheme-tagged signature
	}

	type Certificate struct {
	    CandidateID CandidateID // What was finalized
	    Height      uint64      // At what height
	    PolicyID    PolicyID    // How finality was achieved
	    Proof       []byte      // Policy-specific proof
	    Signers     []byte      // Who attested
	}

# Usage Example

	// Create sequencer for 3-of-5 agent mesh
	cfg := wire.AgentMeshConfig([]byte("ai-mesh"), 5)
	policy := wire.NewQuorumPolicy(3, 5)

	// Create candidate
	candidate := wire.NewCandidate(
	    cfg.Domain,
	    []byte("What's the best approach?"),
	    wire.EmptyCandidateID,
	    1,
	)

	// Collect votes
	for _, agent := range agents {
	    vote := wire.NewVote(candidate.ID, agent.ID, 0, true)
	    policy.OnVote(ctx, vote)
	}

	// Check finality
	cert, _ := policy.MaybeFinalize(ctx, candidate.ID)
	if cert != nil {
	    // Candidate is finalized with quorum certificate
	}

# Interoperability

The wire format is language-agnostic (JSON serialization). Python clients
can participate in Go consensus and vice versa:

	# Python
	from wire import Candidate, Vote, Certificate, derive_voter_id

	voter_id = derive_voter_id("claude")
	candidate_id = derive_item_id(b"decision text")
	vote = Vote(candidate_id, voter_id, preference=True)

	# Serialize and send to Go sequencer
	data = vote.to_json()

See wire.py for the Python implementation.
*/
package wire
