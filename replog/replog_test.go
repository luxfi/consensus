// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package replog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/luxfi/consensus"
	"github.com/stretchr/testify/require"
)

// maxVolumeIdCommand mirrors the Hanzo S3 master's ENTIRE replicated FSM:
// a monotonic max volume id + a topology id. Replacing seaweedfs/raft's
// replicated log with this Log is exactly this command flowing through Lux
// consensus instead of Raft.
type maxVolumeIdCommand struct {
	MaxVolumeId uint32 `json:"max_volume_id"`
	TopologyId  string `json:"topology_id"`
}

// voteToFinality injects a finalizing quorum of validator votes for id —
// standing in for the peer votes that, in production, arrive over the
// zap-proto transport.
func voteToFinality(ctx context.Context, t *testing.T, chain *consensus.Chain, id consensus.ID) {
	t.Helper()
	validators := make([]consensus.NodeID, 20)
	for i := range validators {
		validators[i] = consensus.NodeID{byte(i + 1)}
	}
	for _, v := range validators {
		require.NoError(t, chain.RecordVote(ctx, consensus.NewVote(id, consensus.VotePreference, v)))
	}
}

// TestLog_ReplicatesMasterFSM proves the master's replicated-log pattern on
// Lux consensus: submit volume-id commands, finalize them via consensus
// votes, and apply them exactly once, in commit order, gap-free.
func TestLog_ReplicatesMasterFSM(t *testing.T) {
	chain := consensus.NewChain(consensus.DefaultConfig())
	ctx := context.Background()

	// The replicated state (the master's topology, in miniature).
	var maxVolumeId uint32
	var topologyId string
	log := New(chain, func(payload []byte) error {
		var cmd maxVolumeIdCommand
		if err := json.Unmarshal(payload, &cmd); err != nil {
			return err
		}
		if cmd.MaxVolumeId > maxVolumeId {
			maxVolumeId = cmd.MaxVolumeId
		}
		if cmd.TopologyId != "" {
			topologyId = cmd.TopologyId
		}
		return nil
	})
	require.NoError(t, log.Start(ctx))
	defer log.Stop()

	cmds := []maxVolumeIdCommand{
		{MaxVolumeId: 5, TopologyId: "cluster-A"},
		{MaxVolumeId: 9},
		{MaxVolumeId: 12},
	}
	ids := make([]consensus.ID, len(cmds))
	for i, c := range cmds {
		p, err := json.Marshal(c)
		require.NoError(t, err)
		id, err := log.Submit(ctx, p)
		require.NoError(t, err)
		ids[i] = id
	}

	// Proposed but not finalized -> nothing applied (commit, not proposal).
	n, err := log.Advance(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, uint32(0), maxVolumeId)
	require.Equal(t, 3, log.Pending())

	// Finalize the first two; leave the third open.
	voteToFinality(ctx, t, chain, ids[0])
	voteToFinality(ctx, t, chain, ids[1])

	n, err = log.Advance(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, uint32(9), maxVolumeId) // applied in order: 5 then 9
	require.Equal(t, "cluster-A", topologyId)
	require.Equal(t, 1, log.Pending()) // third still pending, gap-free

	// Finalize the third -> applied.
	voteToFinality(ctx, t, chain, ids[2])
	n, err = log.Advance(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, uint32(12), maxVolumeId)
	require.Equal(t, 0, log.Pending())
}
