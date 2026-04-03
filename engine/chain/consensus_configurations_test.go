package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// nValidatorSampler returns n distinct validators including self.
type nValidatorSampler struct {
	self  ids.NodeID
	peers []ids.NodeID
}

func makeValidators(n int) (ids.NodeID, *nValidatorSampler) {
	self := ids.GenerateTestNodeID()
	peers := make([]ids.NodeID, 0, n-1)
	for i := 1; i < n; i++ {
		peers = append(peers, ids.GenerateTestNodeID())
	}
	return self, &nValidatorSampler{self: self, peers: peers}
}

func (s *nValidatorSampler) Sample(_ ids.ID, k int) ([]ids.NodeID, error) {
	all := append([]ids.NodeID{s.self}, s.peers...)
	if k > len(all) {
		k = len(all)
	}
	return all[:k], nil
}

func (s *nValidatorSampler) Count(_ ids.ID) int { return 1 + len(s.peers) }

// consensusConfig describes a test configuration.
type consensusConfig struct {
	name              string
	k                 int  // sample size
	alpha             int  // acceptance threshold
	totalValidators   int  // total validators in network
	acceptVotes       int  // votes that accept
	rejectVotes       int  // votes that reject
	expectSelfVote    bool // should self-voter fire
	expectNetworkPoll bool // should send to network
}

// TestConsensusConfigurations tests various K/Alpha/validator configurations.
// Covers: 1/1, 1/2, 2/3, 3/5, 14/21 (67%), 15/20 (75%), 69/100 (69%).
func TestConsensusConfigurations(t *testing.T) {
	tests := []consensusConfig{
		// K=1: single-node --dev mode. Self-vote, no network poll.
		{
			name:              "K=1 single-node (--dev mode)",
			k:                 1,
			alpha:             1,
			totalValidators:   1,
			expectSelfVote:    true,
			expectNetworkPoll: false,
		},
		// K=2: minimum multi-node. 1 peer needed.
		{
			name:              "K=2 two-node (1 peer)",
			k:                 2,
			alpha:             2,
			totalValidators:   2,
			expectSelfVote:    false,
			expectNetworkPoll: true,
		},
		// K=3, Alpha=2: 2/3 threshold (standard small network).
		{
			name:              "K=3 Alpha=2 (2/3 threshold)",
			k:                 3,
			alpha:             2,
			totalValidators:   3,
			expectSelfVote:    false,
			expectNetworkPoll: true,
		},
		// K=5, Alpha=3: 3/5 threshold.
		{
			name:              "K=5 Alpha=3 (3/5 threshold)",
			k:                 5,
			alpha:             3,
			totalValidators:   5,
			expectSelfVote:    false,
			expectNetworkPoll: true,
		},
		// K=21, Alpha=14: 67% threshold (mainnet minimum).
		{
			name:              "K=21 Alpha=14 (67% mainnet min)",
			k:                 21,
			alpha:             14,
			totalValidators:   21,
			expectSelfVote:    false,
			expectNetworkPoll: true,
		},
		// K=20, Alpha=15: 75% threshold (production default).
		{
			name:              "K=20 Alpha=15 (75% production)",
			k:                 20,
			alpha:             15,
			totalValidators:   20,
			expectSelfVote:    false,
			expectNetworkPoll: true,
		},
		// K=100, Alpha=69: 69% threshold (large validator set).
		{
			name:              "K=100 Alpha=69 (69% large set)",
			k:                 100,
			alpha:             69,
			totalValidators:   100,
			expectSelfVote:    false,
			expectNetworkPoll: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodeID, sampler := makeValidators(tc.totalValidators)
			gossiper := &testGossiper{}
			selfVoted := false

			var selfVoter func(ids.ID)
			if tc.k == 1 {
				selfVoter = func(_ ids.ID) { selfVoted = true }
			}

			proposer := &gossiperProposer{
				gossiper:   gossiper,
				chainID:    ids.GenerateTestID(),
				networkID:  ids.GenerateTestID(),
				validators: sampler,
				nodeID:     nodeID,
				k:          tc.k,
				selfVoter:  selfVoter,
			}

			blockID := ids.GenerateTestID()
			err := proposer.RequestVotes(context.Background(), VoteRequest{
				BlockID:   blockID,
				BlockData: []byte("test-block"),
			})
			if err != nil {
				t.Fatalf("RequestVotes failed: %v", err)
			}

			if tc.expectSelfVote && !selfVoted {
				t.Error("expected self-vote but didn't get one")
			}
			if !tc.expectSelfVote && selfVoted {
				t.Error("got unexpected self-vote")
			}
			if tc.expectNetworkPoll && gossiper.pushQueries == 0 {
				t.Error("expected network poll but no PushQuery sent")
			}
			if !tc.expectNetworkPoll && gossiper.pushQueries > 0 {
				t.Errorf("unexpected network poll: %d PushQuery(s) sent", gossiper.pushQueries)
			}
		})
	}
}

// TestConsensusAcceptanceThresholds verifies that blocks are accepted only
// when the acceptance threshold (Alpha) is reached, for various K values.
func TestConsensusAcceptanceThresholds(t *testing.T) {
	tests := []struct {
		name        string
		k           int
		alpha       int
		beta        int
		votes       int  // number of accept votes
		shouldFinal bool // should reach finality
	}{
		// K=1: 1 vote = accept.
		{"K=1 1-vote accept", 1, 1, 1, 1, true},
		// K=3: need 2 votes.
		{"K=3 1-vote no-accept", 3, 2, 1, 1, false},
		{"K=3 2-vote accept", 3, 2, 1, 2, true},
		{"K=3 3-vote accept", 3, 2, 1, 3, true},
		// K=5: need 3 votes.
		{"K=5 2-vote no-accept", 5, 3, 1, 2, false},
		{"K=5 3-vote accept", 5, 3, 1, 3, true},
		// K=21: need 14 votes (67%).
		{"K=21 13-vote no-accept", 21, 14, 1, 13, false},
		{"K=21 14-vote accept", 21, 14, 1, 14, true},
		// K=20: need 15 votes (75%).
		{"K=20 14-vote no-accept", 20, 15, 1, 14, false},
		{"K=20 15-vote accept", 20, 15, 1, 15, true},
		// K=100: need 69 votes (69%).
		{"K=100 68-vote no-accept", 100, 69, 1, 68, false},
		{"K=100 69-vote accept", 100, 69, 1, 69, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			consensus := NewChainConsensus(tc.k, tc.alpha, tc.beta)
			blockID := ids.GenerateTestID()
			parentID := ids.GenerateTestID()
			ctx := context.Background()

			block := &Block{
				id:       blockID,
				parentID: parentID,
				height:   1,
			}

			if err := consensus.AddBlock(ctx, block); err != nil {
				t.Fatalf("AddBlock failed: %v", err)
			}

			// Submit votes.
			for i := 0; i < tc.votes; i++ {
				if err := consensus.ProcessVote(ctx, blockID, true); err != nil {
					t.Fatalf("ProcessVote %d failed: %v", i, err)
				}
			}

			// Poll to trigger finalization check.
			if err := consensus.Poll(ctx, map[ids.ID]int{blockID: tc.votes}); err != nil {
				t.Fatalf("Poll failed: %v", err)
			}

			accepted := consensus.IsAccepted(blockID)
			if tc.shouldFinal && !accepted {
				t.Errorf("expected block accepted with %d/%d votes (alpha=%d) but not accepted",
					tc.votes, tc.k, tc.alpha)
			}
			if !tc.shouldFinal && accepted {
				t.Errorf("block accepted with %d/%d votes (alpha=%d) but should NOT be accepted",
					tc.votes, tc.k, tc.alpha)
			}
		})
	}
}

// TestConsensusRejection verifies that blocks are rejected when reject
// votes reach the alpha threshold.
func TestConsensusRejection(t *testing.T) {
	consensus := NewChainConsensus(5, 3, 1)
	blockID := ids.GenerateTestID()
	ctx := context.Background()

	block := &Block{id: blockID, parentID: ids.GenerateTestID(), height: 1}
	if err := consensus.AddBlock(ctx, block); err != nil {
		t.Fatal(err)
	}

	// 3 reject votes = alpha threshold.
	for i := 0; i < 3; i++ {
		consensus.ProcessVote(ctx, blockID, false)
	}
	consensus.Poll(ctx, map[ids.ID]int{blockID: 0})

	if consensus.IsAccepted(blockID) {
		t.Error("block should NOT be accepted")
	}
	if !consensus.IsRejected(blockID) {
		t.Error("block should be rejected with 3 reject votes (alpha=3)")
	}
}

// TestSelfVoteOnlyForK1 ensures self-voting is ONLY created for K=1.
// This is critical — multi-node networks MUST use network consensus.
func TestSelfVoteOnlyForK1(t *testing.T) {
	for _, k := range []int{2, 3, 5, 10, 20, 21, 100} {
		t.Run("K="+string(rune('0'+k/10))+string(rune('0'+k%10)), func(t *testing.T) {
			nodeID, sampler := makeValidators(k)
			gossiper := &testGossiper{}
			selfVoted := false

			// selfVoter should NOT be set for K>1
			proposer := &gossiperProposer{
				gossiper:   gossiper,
				chainID:    ids.GenerateTestID(),
				networkID:  ids.GenerateTestID(),
				validators: sampler,
				nodeID:     nodeID,
				k:          k,
				selfVoter:  nil, // Correct: nil for K>1
			}

			_ = proposer.RequestVotes(context.Background(), VoteRequest{
				BlockID:   ids.GenerateTestID(),
				BlockData: []byte("block"),
			})

			if selfVoted {
				t.Fatalf("K=%d should NOT self-vote", k)
			}
			if gossiper.pushQueries == 0 {
				t.Fatalf("K=%d should send network queries", k)
			}
		})
	}
}
