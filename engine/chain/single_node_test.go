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

func (g *testGossiper) GossipPut(_ ids.ID, _ ids.ID, _ []byte) int { return 0 }
func (g *testGossiper) SendPushQuery(_ ids.ID, _ ids.ID, _ []byte, _ []ids.NodeID) int {
	g.pushQueries++
	return 0
}
func (g *testGossiper) SendPullQuery(_ ids.ID, _ ids.ID, _ ids.ID, _ []ids.NodeID) int { return 0 }
func (g *testGossiper) SendVote(_ ids.ID, _ ids.NodeID, _ ids.ID) error                { return nil }

// TestSingleNode_PollsNobody verifies the DECOMPLECTED single-validator path: a K==1 proposer
// polls NOBODY (no peers) and does NOT self-vote — there is no self-voter shortcut. Finality
// comes solely from the inline build-path finalizer (proven in singlenode_decide_test.go).
// RequestVotes returns cleanly and sends zero network queries.
func TestSingleNode_PollsNobody(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	blockID := ids.GenerateTestID()

	gossiper := &testGossiper{}
	proposer := &gossiperProposer{
		gossiper:   gossiper,
		chainID:    ids.GenerateTestID(),
		networkID:  ids.GenerateTestID(),
		validators: &mockValidatorSampler{nodeID: nodeID},
		nodeID:     nodeID,
		k:          1, // single validator
	}

	if err := proposer.RequestVotes(context.Background(), VoteRequest{
		BlockID:   blockID,
		BlockData: []byte("block data"),
	}); err != nil {
		t.Fatalf("RequestVotes failed: %v", err)
	}

	if gossiper.pushQueries != 0 {
		t.Fatalf("K==1 must poll nobody (no peers, no self-vote shortcut): got %d push queries", gossiper.pushQueries)
	}
}

// TestMultiNode_PollsNetwork verifies that with K>1 the proposer polls the network (no
// self-voting shortcut ever existed for multi-validator, and none exists now).
func TestMultiNode_PollsNetwork(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	peerID := ids.GenerateTestNodeID()
	blockID := ids.GenerateTestID()

	gossiper := &testGossiper{}
	proposer := &gossiperProposer{
		gossiper:  gossiper,
		chainID:   ids.GenerateTestID(),
		networkID: ids.GenerateTestID(),
		validators: &multiValidatorSampler{
			nodeID: nodeID,
			peers:  []ids.NodeID{peerID},
		},
		nodeID: nodeID,
		k:      3, // multi-validator
	}

	if err := proposer.RequestVotes(context.Background(), VoteRequest{
		BlockID:   blockID,
		BlockData: []byte("block data"),
	}); err != nil {
		t.Fatalf("RequestVotes failed: %v", err)
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
