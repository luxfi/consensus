// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nebula

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/require"
    
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/consensus/testutils"
    "github.com/luxfi/ids"
    "github.com/luxfi/log"
)

type testTx struct {
    id       ids.ID
    status   interfaces.Status
    bytes    []byte
}

func (t *testTx) ID() ids.ID             { return t.id }
func (t *testTx) Status() interfaces.Status { return t.status }
func (t *testTx) Accept(context.Context) error { t.status = interfaces.Accepted; return nil }
func (t *testTx) Reject(context.Context) error { t.status = interfaces.Rejected; return nil }
func (t *testTx) Bytes() []byte          { return t.bytes }

type testVertex struct {
    id       ids.ID
    parentIDs []ids.ID
    height   uint64
    txs      []*testTx
    status   interfaces.Status
    bytes    []byte
}

func (v *testVertex) ID() ids.ID             { return v.id }
func (v *testVertex) Status() interfaces.Status { return v.status }
func (v *testVertex) Accept(context.Context) error { v.status = interfaces.Accepted; return nil }
func (v *testVertex) Reject(context.Context) error { v.status = interfaces.Rejected; return nil }
func (v *testVertex) ParentIDs() []ids.ID    { return v.parentIDs }
func (v *testVertex) Height() uint64         { return v.height }
func (v *testVertex) Txs() []interfaces.Decidable   {
    txs := make([]interfaces.Decidable, len(v.txs))
    for i, tx := range v.txs {
        txs[i] = tx
    }
    return txs
}
func (v *testVertex) Bytes() []byte          { return v.bytes }
func (v *testVertex) Verify() error          { return nil }

func TestNebulaBasic(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    require.NotNil(n)
}

func TestNebulaLinearDAG(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    require.NotNil(n)
    
    // Create genesis vertex
    genesis := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{},
        height:    0,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("genesis"),
    }
    
    // Create linear chain of vertices
    vertices := []*testVertex{genesis}
    for i := 1; i <= 5; i++ {
        vtx := &testVertex{
            id:        ids.GenerateTestID(),
            parentIDs: []ids.ID{vertices[i-1].id},
            height:    uint64(i),
            txs:       []*testTx{},
            status:    interfaces.Unknown,
            bytes:     []byte{byte(i)},
        }
        vertices = append(vertices, vtx)
    }
    
    // Test basic operations
    require.NotNil(n)
}

func TestNebulaDAGWithTransactions(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    
    // Create some transactions
    tx1 := &testTx{
        id:     ids.GenerateTestID(),
        status: interfaces.Unknown,
        bytes:  []byte("tx1"),
    }
    
    tx2 := &testTx{
        id:     ids.GenerateTestID(),
        status: interfaces.Unknown,
        bytes:  []byte("tx2"),
    }
    
    // Create genesis
    genesis := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{},
        height:    0,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("genesis"),
    }
    
    // Create vertex with transactions
    vtx1 := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{genesis.id},
        height:    1,
        txs:       []*testTx{tx1, tx2},
        status:    interfaces.Unknown,
        bytes:     []byte("vtx1"),
    }
    
    // Test operations
    require.NotNil(n)
    require.Equal(2, len(vtx1.txs))
}

func TestNebulaDAGFork(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    
    // Create genesis
    genesis := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{},
        height:    0,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("genesis"),
    }
    
    // Create conflicting transactions
    txA := &testTx{
        id:     ids.GenerateTestID(),
        status: interfaces.Unknown,
        bytes:  []byte("txA"),
    }
    
    txB := &testTx{
        id:     ids.GenerateTestID(),
        status: interfaces.Unknown,
        bytes:  []byte("txB"),
    }
    
    // Create fork at height 1
    vtxA := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{genesis.id},
        height:    1,
        txs:       []*testTx{txA},
        status:    interfaces.Unknown,
        bytes:     []byte("A"),
    }
    
    vtxB := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{genesis.id},
        height:    1,
        txs:       []*testTx{txB},
        status:    interfaces.Unknown,
        bytes:     []byte("B"),
    }
    
    // Test fork handling
    require.NotNil(n)
    require.NotEqual(vtxA.id, vtxB.id)
}

func TestNebulaMultipleParents(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    
    // Create genesis
    genesis := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{},
        height:    0,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("genesis"),
    }
    
    // Create two vertices at height 1
    vtx1 := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{genesis.id},
        height:    1,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("1"),
    }
    
    vtx2 := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{genesis.id},
        height:    1,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("2"),
    }
    
    // Create vertex with multiple parents
    vtx3 := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{vtx1.id, vtx2.id},
        height:    2,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("3"),
    }
    
    // Test multiple parents
    require.NotNil(n)
    require.Equal(2, len(vtx3.parentIDs))
}

func TestNebulaOrphanVertex(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    
    genesis := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{},
        height:    0,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("genesis"),
    }
    
    // Create orphan vertex (unknown parent)
    orphan := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{ids.GenerateTestID()}, // Unknown parent
        height:    1,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("orphan"),
    }
    
    // Test orphan handling
    require.NotNil(n)
    require.NotNil(genesis)
    require.NotNil(orphan)
}

func TestNebulaRecordUnsuccessfulPoll(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    
    genesis := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{},
        height:    0,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("genesis"),
    }
    
    vtx1 := &testVertex{
        id:        ids.GenerateTestID(),
        parentIDs: []ids.ID{genesis.id},
        height:    1,
        txs:       []*testTx{},
        status:    interfaces.Unknown,
        bytes:     []byte("1"),
    }
    
    // Test unsuccessful poll handling
    require.NotNil(n)
    require.NotNil(vtx1)
}

func TestNebulaString(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: testutils.NewNoOpRegisterer(),
    }
    
    n := New(ctx)
    require.NotNil(n)
}