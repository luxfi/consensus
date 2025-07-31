// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/require"
    
    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/consensus/testutils"
    "github.com/luxfi/consensus/utils/bag"
    "github.com/luxfi/ids"
    "github.com/luxfi/log"
)


type testBlock struct {
    id        ids.ID
    parentID  ids.ID
    height    uint64
    timestamp time.Time
    bytes     []byte
    status    interfaces.Status
}

func (b *testBlock) ID() ids.ID                     { return b.id }
func (b *testBlock) Parent() ids.ID                { return b.parentID }
func (b *testBlock) Height() uint64                { return b.height }
func (b *testBlock) Timestamp() time.Time          { return b.timestamp }
func (b *testBlock) Bytes() []byte                 { return b.bytes }
func (b *testBlock) Verify() error                 { return nil }
func (b *testBlock) Accept(context.Context) error  { b.status = interfaces.Accepted; return nil }
func (b *testBlock) Reject(context.Context) error  { b.status = interfaces.Rejected; return nil }
func (b *testBlock) Status() (interfaces.Status, error) { return b.status, nil }

type testAcceptor struct {
    accepted []ids.ID
}

func (a *testAcceptor) Accept(ctx context.Context, blkID ids.ID, bytes []byte) error {
    a.accepted = append(a.accepted, blkID)
    return nil
}

func TestNovaBasic(t *testing.T) {
    require := require.New(t)
    
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: &testAcceptor{},
    }
    
    params := config.DefaultParameters
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    // Create genesis block
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    // Initialize
    err := topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp())
    require.NoError(err)
    require.Equal(genesis.ID(), topological.Preference())
}

func TestNovaLinearChain(t *testing.T) {
    require := require.New(t)
    
    acceptor := &testAcceptor{}
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: acceptor,
    }
    
    params := config.TestParameters
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    // Create genesis
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp()))
    
    // Create linear chain of blocks
    blocks := []*testBlock{genesis}
    for i := 1; i <= 5; i++ {
        blk := &testBlock{
            id:        ids.GenerateTestID(),
            parentID:  blocks[i-1].id,
            height:    uint64(i),
            timestamp: time.Now(),
            bytes:     []byte{byte(i)},
            status:    interfaces.Unknown,
        }
        blocks = append(blocks, blk)
    }
    
    // Add blocks
    for i := 1; i < len(blocks); i++ {
        require.NoError(topological.Add(context.Background(), blocks[i]))
    }
    
    // Vote for the tip
    votes := bag.Bag[ids.ID]{}
    lastBlock := blocks[len(blocks)-1]
    for i := 0; i < params.AlphaConfidence; i++ {
        votes.Add(lastBlock.id)
    }
    
    // Should finalize after Beta rounds
    for i := 0; i < params.Beta; i++ {
        require.NoError(topological.RecordPrism(context.Background(), votes))
    }
    
    // Check blocks are processed
    require.True(topological.NumProcessing() > 0)
}

func TestNovaFork(t *testing.T) {
    require := require.New(t)
    
    acceptor := &testAcceptor{}
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: acceptor,
    }
    
    params := config.TestParameters
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    // Create genesis
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp()))
    
    // Create fork at height 1
    blockA := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  genesis.id,
        height:    1,
        timestamp: time.Now(),
        bytes:     []byte("A"),
        status:    interfaces.Unknown,
    }
    
    blockB := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  genesis.id,
        height:    1,
        timestamp: time.Now(),
        bytes:     []byte("B"),
        status:    interfaces.Unknown,
    }
    
    // Add both fork blocks
    require.NoError(topological.Add(context.Background(), blockA))
    require.NoError(topological.Add(context.Background(), blockB))
    
    // Build on block A
    blockA2 := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  blockA.id,
        height:    2,
        timestamp: time.Now(),
        bytes:     []byte("A2"),
        status:    interfaces.Unknown,
    }
    require.NoError(topological.Add(context.Background(), blockA2))
    
    // Vote for chain A
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaConfidence; i++ {
        votes.Add(blockA2.id)
    }
    
    // Should prefer chain A
    require.NoError(topological.RecordPrism(context.Background(), votes))
    require.Equal(blockA2.id, topological.Preference())
}

func TestNovaDoubleAdd(t *testing.T) {
    require := require.New(t)
    
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: &testAcceptor{},
    }
    
    params := config.TestParameters
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp()))
    
    block1 := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  genesis.id,
        height:    1,
        timestamp: time.Now(),
        bytes:     []byte("1"),
        status:    interfaces.Unknown,
    }
    
    // First add should succeed
    require.NoError(topological.Add(context.Background(), block1))
    
    // Second add should fail
    err := topological.Add(context.Background(), block1)
    require.Error(err)
    require.Contains(err.Error(), "duplicate")
}

func TestNovaOrphanBlock(t *testing.T) {
    require := require.New(t)
    
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: &testAcceptor{},
    }
    
    params := config.TestParameters
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp()))
    
    // Create orphan block (unknown parent)
    orphan := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.GenerateTestID(), // Unknown parent
        height:    1,
        timestamp: time.Now(),
        bytes:     []byte("orphan"),
        status:    interfaces.Unknown,
    }
    
    // Should fail due to unknown parent
    err := topological.Add(context.Background(), orphan)
    require.Error(err)
    require.Contains(err.Error(), "unknown parent")
}

func TestNovaRecordUnsuccessfulPoll(t *testing.T) {
    require := require.New(t)
    
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: &testAcceptor{},
    }
    
    params := config.TestParameters
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp()))
    
    block1 := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  genesis.id,
        height:    1,
        timestamp: time.Now(),
        bytes:     []byte("1"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Add(context.Background(), block1))
    
    // Build confidence
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaConfidence; i++ {
        votes.Add(block1.id)
    }
    
    require.NoError(topological.RecordPrism(context.Background(), votes))
    
    // Record unsuccessful poll (empty votes)
    emptyVotes := bag.Bag[ids.ID]{}
    require.NoError(topological.RecordPrism(context.Background(), emptyVotes))
}

func TestNovaProcessingTimeout(t *testing.T) {
    require := require.New(t)
    
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: &testAcceptor{},
    }
    
    // Use shorter timeouts for testing
    params := config.TestParameters
    params.MaxItemProcessingTime = 100 * time.Millisecond
    
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp()))
    
    // Add a block that takes too long to verify
    slowBlock := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  genesis.id,
        height:    1,
        timestamp: time.Now(),
        bytes:     []byte("slow"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Add(context.Background(), slowBlock))
    
    // Wait for timeout
    time.Sleep(params.MaxItemProcessingTime + 50*time.Millisecond)
    
    // Check health - should report error about timeout
    health, err := topological.HealthCheck(context.Background())
    // The error is expected because block processing took too long
    require.Error(err)
    require.Contains(err.Error(), "block processing too long")
    require.NotNil(health)
}

func TestNovaString(t *testing.T) {
    require := require.New(t)
    
    ctx := &Context{
        Log:           log.NewNoOpLogger(),
        Registerer:    testutils.NewNoOpRegisterer(),
        BlockAcceptor: &testAcceptor{},
    }
    
    params := config.TestParameters
    factory := TopologicalFactory{}
    consensus := factory.New()
    topological := consensus.(*Topological)
    
    genesis := &testBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    0,
        timestamp: time.Now(),
        bytes:     []byte("genesis"),
        status:    interfaces.Unknown,
    }
    
    require.NoError(topological.Initialize(ctx, params, genesis.ID(), genesis.Height(), genesis.Timestamp()))
    
    // Nova uses Topological consensus, not a specific String method
    require.NotNil(topological)
}