// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package replog

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/luxfi/consensus"
	"github.com/stretchr/testify/require"
)

// voteAll records a finalizing quorum for id without using the test handle,
// so it is safe to call from a helper goroutine.
func voteAll(ctx context.Context, chain *consensus.Chain, id consensus.ID) {
	for i := 0; i < 20; i++ {
		_ = chain.RecordVote(ctx, consensus.NewVote(id, consensus.VotePreference, consensus.NodeID{byte(i + 1)}))
	}
}

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

// TestLog_RunAutoApplies proves the background driver applies a command once
// consensus finalizes it, with no manual Advance.
func TestLog_RunAutoApplies(t *testing.T) {
	chain := consensus.NewChain(consensus.DefaultConfig())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	applied := make(chan string, 4)
	log := New(chain, func(p []byte) error { applied <- string(p); return nil })
	require.NoError(t, log.Start(ctx))
	defer log.Stop()
	go log.Run(ctx, 5*time.Millisecond)

	id, err := log.Submit(ctx, []byte("auto-1"))
	require.NoError(t, err)

	// Not finalized -> Run must not apply it.
	select {
	case <-applied:
		t.Fatal("applied before finalization")
	case <-time.After(60 * time.Millisecond):
	}

	voteToFinality(ctx, t, chain, id)
	select {
	case p := <-applied:
		require.Equal(t, "auto-1", p)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not apply after finalization")
	}
}

// TestLog_CommitBlocksUntilApplied proves Commit (the synchronous raft.Do
// drop-in) blocks until consensus finalizes AND applies the command.
func TestLog_CommitBlocksUntilApplied(t *testing.T) {
	chain := consensus.NewChain(consensus.DefaultConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var got string
	log := New(chain, func(p []byte) error { got = string(p); return nil })
	require.NoError(t, log.Start(ctx))
	defer log.Stop()
	go log.Run(ctx, 5*time.Millisecond)

	payload := []byte("commit-me")
	expID := blockID(1, consensus.GenesisID, payload)

	// Finalize once Commit's Submit has added the block (peers, simulated).
	go func() {
		for chain.GetStatus(expID) == consensus.StatusUnknown {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
		voteAll(ctx, chain, expID)
	}()

	require.NoError(t, log.Commit(ctx, payload))
	require.Equal(t, "commit-me", got)
}
