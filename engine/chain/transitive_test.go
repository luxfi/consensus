// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/ids"
)

var (
	errTest = errors.New("non-nil test error")
)

// TestBlock implements a simple test block
type TestBlock struct {
	IDV        ids.ID
	HeightV    uint64
	ParentV    ids.ID
	TimestampV time.Time
	StatusV    uint8
	VerifyV    error
	AcceptV    error
	RejectV    error
}

// Block status constants
const (
	StatusUnknown    uint8 = 0
	StatusProcessing uint8 = 1
	StatusAccepted   uint8 = 2
	StatusRejected   uint8 = 3
)

func (b *TestBlock) ID() ids.ID               { return b.IDV }
func (b *TestBlock) Height() uint64           { return b.HeightV }
func (b *TestBlock) Parent() ids.ID           { return b.ParentV }
func (b *TestBlock) ParentID() ids.ID         { return b.ParentV }
func (b *TestBlock) Timestamp() time.Time     { return b.TimestampV }
func (b *TestBlock) Status() uint8            { return b.StatusV }
func (b *TestBlock) Bytes() []byte            { return nil }
func (b *TestBlock) Verify(context.Context) error {
	return b.VerifyV
}
func (b *TestBlock) Accept(context.Context) error {
	if b.AcceptV != nil {
		return b.AcceptV
	}
	b.StatusV = StatusAccepted
	return nil
}
func (b *TestBlock) Reject(context.Context) error {
	if b.RejectV != nil {
		return b.RejectV
	}
	b.StatusV = StatusRejected
	return nil
}

// Bag is a multiset for tracking votes
type Bag[T comparable] struct {
	counts map[T]int
}

func NewBag[T comparable]() *Bag[T] {
	return &Bag[T]{counts: make(map[T]int)}
}

func (b *Bag[T]) Add(item T) {
	b.counts[item]++
}

func (b *Bag[T]) AddCount(item T, count int) {
	b.counts[item] += count
}

func (b *Bag[T]) List() map[T]int {
	return b.counts
}

// Set is a set data structure
type Set[T comparable] struct {
	items map[T]struct{}
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{items: make(map[T]struct{})}
}

func (s *Set[T]) Add(item T) {
	s.items[item] = struct{}{}
}

func (s *Set[T]) Remove(item T) {
	delete(s.items, item)
}

func (s *Set[T]) Contains(item T) bool {
	_, exists := s.items[item]
	return exists
}

func (s *Set[T]) Len() int {
	return len(s.items)
}

// TransitiveParams defines consensus parameters for transitive voting
type TransitiveParams struct {
	K               int
	AlphaPreference int
	AlphaConfidence int
	Beta            int
}

// TransitiveEngine implements a transitive voting consensus engine
type TransitiveEngine struct {
	params       TransitiveParams
	blocks       map[ids.ID]*TestBlock
	processing   *Set[ids.ID]
	preference   ids.ID
	lastAccepted ids.ID
	bootstrapped bool

	// Voting tracking
	votes      map[ids.ID]int
	confidence map[ids.ID]int
	finalized  *Set[ids.ID]
}

// NewTransitiveEngine creates a new transitive voting engine
func NewTransitiveEngine() *TransitiveEngine {
	return &TransitiveEngine{
		blocks:     make(map[ids.ID]*TestBlock),
		processing: NewSet[ids.ID](),
		votes:      make(map[ids.ID]int),
		confidence: make(map[ids.ID]int),
		finalized:  NewSet[ids.ID](),
	}
}

// Initialize initializes the engine with parameters
func (e *TransitiveEngine) Initialize(
	ctx context.Context,
	params TransitiveParams,
	lastAcceptedID ids.ID,
	lastAcceptedHeight uint64,
	lastAcceptedTime time.Time,
) error {
	e.params = params
	e.lastAccepted = lastAcceptedID
	e.preference = lastAcceptedID

	// Add genesis block
	genesis := &TestBlock{
		IDV:        lastAcceptedID,
		HeightV:    lastAcceptedHeight,
		ParentV:    ids.Empty,
		TimestampV: lastAcceptedTime,
		StatusV:    StatusAccepted,
	}
	e.blocks[lastAcceptedID] = genesis
	e.finalized.Add(lastAcceptedID)
	e.bootstrapped = true

	return nil
}

// Add adds a new block to the engine
func (e *TransitiveEngine) Add(block *TestBlock) error {
	blockID := block.ID()

	// Check if block already exists
	if _, exists := e.blocks[blockID]; exists {
		return nil
	}

	// Check parent exists
	parentID := block.ParentID()
	parent, parentExists := e.blocks[parentID]
	if !parentExists {
		return errors.New("unknown parent block")
	}

	// Check parent status
	if parent.StatusV == StatusRejected {
		// Parent rejected, so reject this block transitively
		block.StatusV = StatusRejected
	} else {
		// Add to processing set
		e.processing.Add(blockID)
		block.StatusV = StatusProcessing
	}

	e.blocks[blockID] = block

	// Update preference if needed
	if e.shouldUpdatePreference(block) {
		e.preference = blockID
	}

	return nil
}

// RecordPoll records votes and processes transitive voting
func (e *TransitiveEngine) RecordPoll(ctx context.Context, votes *Bag[ids.ID]) error {
	// Count votes for each block
	for blockID, count := range votes.List() {
		if _, exists := e.blocks[blockID]; !exists {
			continue // Skip unknown blocks
		}

		e.votes[blockID] += count

		// Check for transitive votes (votes for child blocks count for parents)
		e.applyTransitiveVotes(blockID, count)
	}

	// Check for blocks that meet confidence threshold
	for blockID := range e.votes {
		if e.votes[blockID] >= e.params.AlphaConfidence {
			e.confidence[blockID]++

			// Check if block should be accepted
			if e.confidence[blockID] >= e.params.Beta {
				if err := e.acceptBlock(ctx, blockID); err != nil {
					return err
				}
			}
		} else {
			// Reset confidence if threshold not met
			e.confidence[blockID] = 0
		}
	}

	// Update preference based on votes
	e.updatePreferenceFromVotes()

	return nil
}

// applyTransitiveVotes applies votes transitively to parent blocks
func (e *TransitiveEngine) applyTransitiveVotes(blockID ids.ID, count int) {
	block, exists := e.blocks[blockID]
	if !exists {
		return
	}

	// Apply votes to parent transitively
	parentID := block.ParentID()
	if parentID != ids.Empty && parentID != e.lastAccepted {
		e.votes[parentID] += count
		e.applyTransitiveVotes(parentID, count)
	}
}

// acceptBlock accepts a block and rejects conflicting blocks
func (e *TransitiveEngine) acceptBlock(ctx context.Context, blockID ids.ID) error {
	block, exists := e.blocks[blockID]
	if !exists {
		return errors.New("block not found")
	}

	// Accept all ancestors first
	if err := e.acceptAncestors(ctx, block.ParentID()); err != nil {
		return err
	}

	// Accept the block
	if err := block.Accept(ctx); err != nil {
		return err
	}

	e.processing.Remove(blockID)
	e.finalized.Add(blockID)
	e.lastAccepted = blockID
	e.preference = blockID

	// Reject all conflicting blocks transitively
	if err := e.rejectConflicting(ctx, block); err != nil {
		return err
	}

	return nil
}

// acceptAncestors accepts all ancestors of a block
func (e *TransitiveEngine) acceptAncestors(ctx context.Context, blockID ids.ID) error {
	if blockID == ids.Empty || e.finalized.Contains(blockID) {
		return nil
	}

	block, exists := e.blocks[blockID]
	if !exists {
		return nil
	}

	// Recursively accept parent first
	if err := e.acceptAncestors(ctx, block.ParentID()); err != nil {
		return err
	}

	// Accept this block
	if err := block.Accept(ctx); err != nil {
		return err
	}

	e.processing.Remove(blockID)
	e.finalized.Add(blockID)

	return nil
}

// rejectConflicting rejects all blocks that conflict with the accepted block
func (e *TransitiveEngine) rejectConflicting(ctx context.Context, accepted *TestBlock) error {
	acceptedHeight := accepted.HeightV
	acceptedParent := accepted.ParentV

	// Find and reject all blocks at the same height with different parent
	for blockID, block := range e.blocks {
		if block.StatusV == StatusProcessing {
			// Check if this block conflicts
			if block.HeightV == acceptedHeight && block.ParentV == acceptedParent && blockID != accepted.ID() {
				// Direct conflict - reject this block and all descendants
				if err := e.rejectBlockAndDescendants(ctx, blockID); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// rejectBlockAndDescendants rejects a block and all its descendants
func (e *TransitiveEngine) rejectBlockAndDescendants(ctx context.Context, blockID ids.ID) error {
	block, exists := e.blocks[blockID]
	if !exists || block.StatusV != StatusProcessing {
		return nil
	}

	// Reject the block
	if err := block.Reject(ctx); err != nil {
		return err
	}

	e.processing.Remove(blockID)

	// Find and reject all descendants
	for childID, child := range e.blocks {
		if child.ParentV == blockID {
			if err := e.rejectBlockAndDescendants(ctx, childID); err != nil {
				return err
			}
		}
	}

	return nil
}

// shouldUpdatePreference checks if preference should be updated
func (e *TransitiveEngine) shouldUpdatePreference(block *TestBlock) bool {
	if e.preference == e.lastAccepted {
		return true // Update from genesis
	}

	prefBlock, exists := e.blocks[e.preference]
	if !exists {
		return true
	}

	// Prefer higher blocks
	return block.HeightV > prefBlock.HeightV
}

// updatePreferenceFromVotes updates preference based on vote counts
func (e *TransitiveEngine) updatePreferenceFromVotes() {
	maxVotes := 0
	newPref := e.preference

	for blockID, votes := range e.votes {
		if e.processing.Contains(blockID) && votes > maxVotes {
			maxVotes = votes
			newPref = blockID
		}
	}

	e.preference = newPref
}

// NumProcessing returns the number of processing blocks
func (e *TransitiveEngine) NumProcessing() int {
	return e.processing.Len()
}

// Preference returns the current preferred block
func (e *TransitiveEngine) Preference() ids.ID {
	return e.preference
}

// TestRecordPollTransitiveVotingTest tests transitive voting
func TestRecordPollTransitiveVotingTest(t *testing.T) {
	require := require.New(t)

	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               3,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            1,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Build block tree
	block0 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block0.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    3,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(3 * time.Second),
	}
	block3 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block0.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	block4 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    3,
		ParentV:    block3.ID(),
		TimestampV: genesisTime.Add(3 * time.Second),
	}

	require.NoError(engine.Add(block0))
	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))
	require.NoError(engine.Add(block3))
	require.NoError(engine.Add(block4))

	// Current graph structure:
	//   G
	//   |
	//   0
	//  / \
	// 1   3
	// |   |
	// 2   4

	// Vote for blocks 0, 2, and 4
	votes0_2_4 := NewBag[ids.ID]()
	votes0_2_4.Add(block0.ID())
	votes0_2_4.Add(block2.ID())
	votes0_2_4.Add(block4.ID())
	require.NoError(engine.RecordPoll(ctx, votes0_2_4))

	// Block 0 should be accepted due to transitive votes
	require.Equal(StatusAccepted, block0.StatusV)
	require.Equal(4, engine.NumProcessing())
	require.Equal(block2.ID(), engine.Preference())

	// Vote decisively for block 2
	votes2 := NewBag[ids.ID]()
	votes2.AddCount(block2.ID(), 3)
	require.NoError(engine.RecordPoll(ctx, votes2))

	// Block 2 and ancestors should be accepted, 3 and 4 rejected
	require.Equal(StatusAccepted, block0.StatusV)
	require.Equal(StatusAccepted, block1.StatusV)
	require.Equal(StatusAccepted, block2.StatusV)
	require.Equal(StatusRejected, block3.StatusV)
	require.Equal(StatusRejected, block4.StatusV)
	require.Zero(engine.NumProcessing())
}

// TestRecordPollTransitivelyResetConfidenceTest tests confidence reset
func TestRecordPollTransitivelyResetConfidenceTest(t *testing.T) {
	require := require.New(t)

	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            2, // Requires 2 rounds
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Build block tree
	block0 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	block3 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}

	require.NoError(engine.Add(block0))
	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))
	require.NoError(engine.Add(block3))

	// Current graph structure:
	//   G
	//  / \
	// 0   1
	//    / \
	//   2   3

	// Vote for block 2
	votesFor2 := NewBag[ids.ID]()
	votesFor2.Add(block2.ID())
	require.NoError(engine.RecordPoll(ctx, votesFor2))
	require.Equal(4, engine.NumProcessing())
	require.Equal(block2.ID(), engine.Preference())

	// Empty votes (no progress)
	emptyVotes := NewBag[ids.ID]()
	require.NoError(engine.RecordPoll(ctx, emptyVotes))
	require.Equal(4, engine.NumProcessing())
	require.Equal(block2.ID(), engine.Preference())

	// Vote for block 2 again (should build confidence)
	require.NoError(engine.RecordPoll(ctx, votesFor2))
	require.Equal(4, engine.NumProcessing())

	// Switch vote to block 3 (should reset confidence)
	votesFor3 := NewBag[ids.ID]()
	votesFor3.Add(block3.ID())
	require.NoError(engine.RecordPoll(ctx, votesFor3))
	require.Equal(2, engine.NumProcessing()) // Block 0 rejected
	require.Equal(block3.ID(), engine.Preference())

	// Vote for block 3 again to finalize
	require.NoError(engine.RecordPoll(ctx, votesFor3))
	require.Zero(engine.NumProcessing())
	require.Equal(block3.ID(), engine.Preference())
	require.Equal(StatusRejected, block0.StatusV)
	require.Equal(StatusAccepted, block1.StatusV)
	require.Equal(StatusRejected, block2.StatusV)
	require.Equal(StatusAccepted, block3.StatusV)
}

// TestErrorOnTransitiveRejectionTest tests error propagation on transitive rejection
func TestErrorOnTransitiveRejectionTest(t *testing.T) {
	require := require.New(t)

	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            1,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Build blocks with error on rejection
	block0 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
		RejectV:    errTest, // Will error on rejection
	}

	require.NoError(engine.Add(block0))
	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))

	// Vote for block 0, which should reject blocks 1 and 2
	votes := NewBag[ids.ID]()
	votes.Add(block0.ID())
	err := engine.RecordPoll(ctx, votes)

	// Should get error from block2's rejection
	require.ErrorIs(err, errTest)
}

// TestParentChildRejectionPropagation tests parent/child rejection propagation
func TestParentChildRejectionPropagation(t *testing.T) {
	require := require.New(t)

	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            1,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Build a chain of blocks
	block0 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	block3 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    3,
		ParentV:    block2.ID(),
		TimestampV: genesisTime.Add(3 * time.Second),
	}

	require.NoError(engine.Add(block0))
	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))
	require.NoError(engine.Add(block3))

	// Current graph structure:
	//   G
	//  / \
	// 0   1
	//     |
	//     2
	//     |
	//     3

	require.Equal(4, engine.NumProcessing())

	// Vote for block 0, which should reject 1, 2, and 3
	votes := NewBag[ids.ID]()
	votes.Add(block0.ID())
	require.NoError(engine.RecordPoll(ctx, votes))

	// All blocks should be decided
	require.Zero(engine.NumProcessing())
	require.Equal(StatusAccepted, block0.StatusV)
	require.Equal(StatusRejected, block1.StatusV)
	require.Equal(StatusRejected, block2.StatusV)
	require.Equal(StatusRejected, block3.StatusV)
}

// TestConfidenceResetScenarios tests various confidence reset scenarios
func TestConfidenceResetScenarios(t *testing.T) {
	require := require.New(t)

	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               2,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            3, // Requires 3 rounds
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Build competing chains
	block0 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}

	require.NoError(engine.Add(block0))
	require.NoError(engine.Add(block1))

	// Build confidence for block 0
	votes0 := NewBag[ids.ID]()
	votes0.AddCount(block0.ID(), 2)
	require.NoError(engine.RecordPoll(ctx, votes0))
	require.NoError(engine.RecordPoll(ctx, votes0))

	// Confidence should be 2 for block 0
	require.Equal(2, engine.NumProcessing())

	// Vote for block 1 (insufficient to meet threshold)
	votes1 := NewBag[ids.ID]()
	votes1.Add(block1.ID())
	require.NoError(engine.RecordPoll(ctx, votes1))

	// Confidence for block 0 should be reset
	require.Equal(2, engine.NumProcessing())

	// Continue voting for block 0 to finalize
	require.NoError(engine.RecordPoll(ctx, votes0))
	require.NoError(engine.RecordPoll(ctx, votes0))
	require.NoError(engine.RecordPoll(ctx, votes0))

	// Block 0 accepted, block 1 rejected
	require.Zero(engine.NumProcessing())
	require.Equal(StatusAccepted, block0.StatusV)
	require.Equal(StatusRejected, block1.StatusV)
}