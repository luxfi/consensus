// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"context"
	"sync/atomic"
	
	"github.com/luxfi/ids"
)

// Integration provides hooks to wire WaveFPC into your consensus engine
type Integration struct {
	fpc         WaveFPC
	enabled     atomic.Bool
	epochClosing atomic.Bool
}

// NewIntegration creates a new FPC integration
func NewIntegration(cfg *Config, committee Committee, myIndex ValidatorIndex, cls Classifier, dag DAGTap, pq PQEngine, src CandidateSource) *Integration {
	if cfg == nil || !cfg.IsEnabled() {
		return &Integration{}
	}
	
	fpc := New(*cfg, committee, myIndex, cls, dag, pq, src)
	i := &Integration{fpc: fpc}
	i.enabled.Store(true)
	return i
}

// IsEnabled returns true if FPC is enabled
func (i *Integration) IsEnabled() bool {
	return i.enabled.Load()
}

// OnBlockObserved should be called when a block is gossiped/validated (pre-accept)
func (i *Integration) OnBlockObserved(blk BlockWithFPC) {
	if !i.IsEnabled() || blk == nil {
		return
	}
	
	blkID := blk.ID()
	i.fpc.OnBlockObserved(&ObservedBlock{
		ID:       blkID[:],
		Author:   blk.Author(),
		FPCVotes: blk.FPCVotes(),
		EpochBit: blk.EpochBit(),
	})
}

// OnBlockAccepted should be called when a block becomes accepted/committed
func (i *Integration) OnBlockAccepted(blk BlockWithFPC) {
	if !i.IsEnabled() || blk == nil {
		return
	}
	
	blkID := blk.ID()
	i.fpc.OnBlockAccepted(&ObservedBlock{
		ID:       blkID[:],
		Author:   blk.Author(),
		FPCVotes: blk.FPCVotes(),
		EpochBit: blk.EpochBit(),
	})
}

// NextVotes returns transactions to vote for in the next block
func (i *Integration) NextVotes(budget int) []TxRef {
	if !i.IsEnabled() {
		return nil
	}
	return i.fpc.NextVotes(budget)
}

// Status returns the status of a transaction
func (i *Integration) Status(tx TxRef) (Status, Proof) {
	if !i.IsEnabled() {
		return Pending, Proof{}
	}
	return i.fpc.Status(tx)
}

// StartEpochClose signals the start of epoch closing
func (i *Integration) StartEpochClose() {
	i.epochClosing.Store(true)
	if i.IsEnabled() {
		i.fpc.OnEpochCloseStart()
	}
}

// EndEpochClose signals the end of epoch closing
func (i *Integration) EndEpochClose() {
	i.epochClosing.Store(false)
	if i.IsEnabled() {
		i.fpc.OnEpochClosed()
	}
}

// IsEpochClosing returns true if epoch is closing
func (i *Integration) IsEpochClosing() bool {
	return i.epochClosing.Load()
}

// MarkMixed marks a transaction as requiring Final status (owned+shared)
func (i *Integration) MarkMixed(tx TxRef) {
	if i.IsEnabled() {
		i.fpc.MarkMixed(tx)
	}
}

// Config extension helpers
func (c *Config) IsEnabled() bool {
	return c != nil && c.VoteLimitPerBlock > 0
}

// ValidatorCommitteeAdapter adapts validator set to Committee interface
type ValidatorCommitteeAdapter struct {
	size      int
	nodeToIdx map[ids.NodeID]ValidatorIndex
}

// NewValidatorCommitteeAdapter creates a new adapter from node IDs
func NewValidatorCommitteeAdapter(nodeIDs []ids.NodeID) *ValidatorCommitteeAdapter {
	adapter := &ValidatorCommitteeAdapter{
		size:      len(nodeIDs),
		nodeToIdx: make(map[ids.NodeID]ValidatorIndex),
	}
	
	// Build index mapping
	for i, nodeID := range nodeIDs {
		adapter.nodeToIdx[nodeID] = ValidatorIndex(i)
	}
	
	return adapter
}

// Size returns the committee size
func (a *ValidatorCommitteeAdapter) Size() int {
	return a.size
}

// IndexOf maps author bytes to validator index
func (a *ValidatorCommitteeAdapter) IndexOf(author []byte) (ValidatorIndex, bool) {
	if len(author) != 32 {
		return 0, false
	}
	var nodeID ids.NodeID
	copy(nodeID[:], author)
	idx, ok := a.nodeToIdx[nodeID]
	return idx, ok
}

// SimpleDAGTap provides a basic DAG ancestry check
type SimpleDAGTap struct {
	getBlock func(context.Context, ids.ID) (BlockWithFPC, error)
}

// NewSimpleDAGTap creates a simple DAG tap
func NewSimpleDAGTap(getBlock func(context.Context, ids.ID) (BlockWithFPC, error)) *SimpleDAGTap {
	return &SimpleDAGTap{getBlock: getBlock}
}

// InAncestry checks if needleTx is in the ancestry of blockID
func (d *SimpleDAGTap) InAncestry(blockID []byte, needleTx TxRef) bool {
	if d.getBlock == nil {
		return false
	}
	
	var blkID ids.ID
	copy(blkID[:], blockID)
	
	ctx := context.Background()
	visited := make(map[ids.ID]bool)
	
	// Simple DFS up to 100 ancestors
	var check func(ids.ID, int) bool
	check = func(id ids.ID, depth int) bool {
		if depth > 100 || visited[id] {
			return false
		}
		visited[id] = true
		
		blk, err := d.getBlock(ctx, id)
		if err != nil || blk == nil {
			return false
		}
		
		// Check if this block votes for the tx
		for _, vote := range blk.FPCVotes() {
			if vote == needleTx {
				return true
			}
		}
		
		// Check parent
		return check(blk.Parent(), depth+1)
	}
	
	return check(blkID, 0)
}

// MockClassifier provides a basic transaction classifier for testing
type MockClassifier struct {
	owned map[TxRef][]ObjectID
}

// NewMockClassifier creates a mock classifier
func NewMockClassifier() *MockClassifier {
	return &MockClassifier{
		owned: make(map[TxRef][]ObjectID),
	}
}

// AddTx adds a transaction with its owned objects
func (m *MockClassifier) AddTx(tx TxRef, objects ...ObjectID) {
	m.owned[tx] = objects
}

// OwnedInputs returns owned objects for a transaction
func (m *MockClassifier) OwnedInputs(tx TxRef) []ObjectID {
	return m.owned[tx]
}

// Conflicts checks if two transactions conflict (simplified: shared objects)
func (m *MockClassifier) Conflicts(a, b TxRef) bool {
	aObjs := m.owned[a]
	bObjs := m.owned[b]
	
	objSet := make(map[ObjectID]bool)
	for _, o := range aObjs {
		objSet[o] = true
	}
	for _, o := range bObjs {
		if objSet[o] {
			return true
		}
	}
	return false
}