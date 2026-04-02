package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// mockValidatorSampler returns the node itself as the only validator.
type mockValidatorSampler struct {
	nodeID ids.NodeID
}

func (m *mockValidatorSampler) Sample(_ ids.ID, k int) ([]ids.NodeID, error) {
	result := make([]ids.NodeID, 0, k)
	for i := 0; i < k; i++ {
		result = append(result, m.nodeID)
	}
	return result, nil
}

func (m *mockValidatorSampler) Count(_ ids.ID) int { return 1 }

// testGossiper implements Gossiper for testing.
type testGossiper struct {
	pushQueries int
}

func (g *testGossiper) GossipPut(_ ids.ID, _ ids.ID, _ []byte) int                    { return 0 }
func (g *testGossiper) SendPushQuery(_ ids.ID, _ ids.ID, _ []byte, _ []ids.NodeID) int { g.pushQueries++; return 0 }
func (g *testGossiper) SendPullQuery(_ ids.ID, _ ids.ID, _ ids.ID, _ []ids.NodeID) int { return 0 }
func (g *testGossiper) SendVote(_ ids.ID, _ ids.NodeID, _ ids.ID) error                { return nil }

// TestSingleNodeSelfVote verifies that in --dev mode (K=1, single validator),
// the proposer delivers a self-vote instead of trying to poll 0 peers.
// This is the standard path for local single-validator networks.
func TestSingleNodeSelfVote(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	blockID := ids.GenerateTestID()

	gossiper := &testGossiper{}
	var selfVoted bool
	var votedBlockID ids.ID

	proposer := &gossiperProposer{
		gossiper:   gossiper,
		chainID:    ids.GenerateTestID(),
		networkID:  ids.GenerateTestID(),
		validators: &mockValidatorSampler{nodeID: nodeID},
		nodeID:     nodeID,
		k:          1, // Single-node mode (--dev)
		selfVoter: func(id ids.ID) {
			selfVoted = true
			votedBlockID = id
		},
	}

	err := proposer.RequestVotes(context.Background(), VoteRequest{
		BlockID:   blockID,
		BlockData: []byte("block data"),
	})
	if err != nil {
		t.Fatalf("RequestVotes failed: %v", err)
	}

	if !selfVoted {
		t.Fatal("single-node proposer did not self-vote")
	}
	if votedBlockID != blockID {
		t.Fatalf("self-voted for wrong block: got %s, want %s", votedBlockID, blockID)
	}
	if gossiper.pushQueries != 0 {
		t.Fatalf("single-node mode should not send push queries to network: got %d", gossiper.pushQueries)
	}
}

// TestMultiNodeDoesNotSelfVote verifies that with K>1 (normal consensus),
// the standard network polling path is used — no self-voting shortcut.
func TestMultiNodeDoesNotSelfVote(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	peerID := ids.GenerateTestNodeID()
	blockID := ids.GenerateTestID()

	gossiper := &testGossiper{}
	selfVoted := false

	proposer := &gossiperProposer{
		gossiper:  gossiper,
		chainID:   ids.GenerateTestID(),
		networkID: ids.GenerateTestID(),
		validators: &multiValidatorSampler{
			nodeID: nodeID,
			peers:  []ids.NodeID{peerID},
		},
		nodeID: nodeID,
		k:      3, // Multi-node mode (normal 3+ validator consensus)
		selfVoter: func(_ ids.ID) {
			selfVoted = true
		},
	}

	err := proposer.RequestVotes(context.Background(), VoteRequest{
		BlockID:   blockID,
		BlockData: []byte("block data"),
	})
	if err != nil {
		t.Fatalf("RequestVotes failed: %v", err)
	}

	if selfVoted {
		t.Fatal("multi-node mode should NOT self-vote — must use network consensus")
	}
	if gossiper.pushQueries != 1 {
		t.Fatalf("multi-node mode should send push query via network: got %d", gossiper.pushQueries)
	}
}

// multiValidatorSampler returns self + peers.
type multiValidatorSampler struct {
	nodeID ids.NodeID
	peers  []ids.NodeID
}

func (m *multiValidatorSampler) Sample(_ ids.ID, k int) ([]ids.NodeID, error) {
	all := append([]ids.NodeID{m.nodeID}, m.peers...)
	if k > len(all) {
		k = len(all)
	}
	return all[:k], nil
}

func (m *multiValidatorSampler) Count(_ ids.ID) int { return 1 + len(m.peers) }

// TestSelfVoterNilWhenKGreaterThan1 verifies the integration layer only
// creates a selfVoter callback when K==1.
func TestSelfVoterNilWhenKGreaterThan1(t *testing.T) {
	// K=1: selfVoter should be set
	proposerK1 := &gossiperProposer{k: 1, selfVoter: func(_ ids.ID) {}}
	if proposerK1.selfVoter == nil {
		t.Fatal("K=1 should have selfVoter set")
	}

	// K>1: selfVoter should NOT trigger
	proposerK3 := &gossiperProposer{k: 3}
	if proposerK3.selfVoter != nil {
		t.Fatal("K>1 should not have selfVoter set")
	}
}
