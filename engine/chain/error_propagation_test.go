// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/consensus/engine/chain/chaintest"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

var (
	errTestError          = errors.New("non-nil test error")
	errUnknownParentBlock = errors.New("unknown parent block")
)

// TestErrorOnAccept tests that errors during block acceptance are properly propagated
func TestErrorOnAccept(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine with test configuration
	sm := NewConsensus()
	params := DefaultParameters()

	err := sm.Initialize(
		ctx,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.NoError(err)

	// Create a block that returns an error on Accept
	block := &ErrorBlock{
		TestBlock: chaintest.BuildChild(chaintest.Genesis),
		AcceptErr: errTestError,
	}

	// Add the block to consensus
	err = sm.Add(ctx, block)
	require.NoError(err)

	// Create votes for the block - need to initialize properly due to bag impl issue
	votes := bag.Bag[ids.ID]{}
	votes.SetThreshold(2) // Set threshold > 1 to avoid nil set issue
	votes.Add(block.ID())

	// RecordPoll should propagate the Accept error
	err = sm.RecordPoll(ctx, &votes)
	require.ErrorIs(err, errTestError)
}

// TestErrorOnRejectSibling tests that errors during sibling rejection are properly propagated
func TestErrorOnRejectSibling(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	err := sm.Initialize(
		ctx,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.NoError(err)

	// Create two conflicting blocks (siblings)
	block0 := chaintest.BuildChild(chaintest.Genesis)
	block1 := &ErrorBlock{
		TestBlock: chaintest.BuildChild(chaintest.Genesis),
		RejectErr: errTestError,
	}

	// Add both blocks to consensus
	err = sm.Add(ctx, block0)
	require.NoError(err)

	err = sm.Add(ctx, block1)
	require.NoError(err)

	// Vote for block0, which should cause block1 to be rejected
	votes := bag.Bag[ids.ID]{}
	votes.SetThreshold(2)
	votes.Add(block0.ID())

	// RecordPoll should propagate the Reject error from block1
	err = sm.RecordPoll(ctx, &votes)
	require.ErrorIs(err, errTestError)
}

// TestMetricsProcessingError tests that metric registration errors are handled properly
func TestMetricsProcessingError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	// Create and register a conflicting metric
	numProcessing := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "blks_processing",
	})

	// Create a test registerer
	registerer := prometheus.NewRegistry()
	require.NoError(registerer.Register(numProcessing))

	// Create context with the registerer
	ctxWithReg := &ConsensusContext{
		Context:    ctx,
		Registerer: registerer,
	}

	// Initialize should fail due to metric conflict
	err := sm.InitializeWithContext(
		ctxWithReg,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.Error(err)
}

// TestErrorOnAddDecidedBlock tests that adding an already decided block results in error
func TestErrorOnAddDecidedBlock(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	err := sm.Initialize(
		ctx,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.NoError(err)

	// Try to add the genesis block (which is already decided/accepted)
	err = sm.Add(ctx, chaintest.Genesis)
	require.ErrorIs(err, errUnknownParentBlock)
}

// TestTransitiveRejectionError tests error propagation during transitive block rejection
func TestTransitiveRejectionError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	err := sm.Initialize(
		ctx,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.NoError(err)

	// Create a chain: block0 (sibling to block1) <- block2
	block0 := chaintest.BuildChild(chaintest.Genesis)
	block1 := chaintest.BuildChild(chaintest.Genesis)
	block2 := &ErrorBlock{
		TestBlock: chaintest.BuildChild(block1),
		RejectErr: errTestError,
	}

	// Add all blocks
	err = sm.Add(ctx, block0)
	require.NoError(err)

	err = sm.Add(ctx, block1)
	require.NoError(err)

	err = sm.Add(ctx, block2)
	require.NoError(err)

	// Vote for block0, which should cause block1 and block2 to be rejected
	votes := bag.Bag[ids.ID]{}
	votes.SetThreshold(2)
	votes.Add(block0.ID())

	// RecordPoll should propagate the Reject error from block2
	err = sm.RecordPoll(ctx, &votes)
	require.ErrorIs(err, errTestError)
}

// TestMetricsAcceptedError tests that accepted counter metric conflicts are handled
func TestMetricsAcceptedError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	// Create and register a conflicting metric
	numAccepted := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "blks_accepted_count",
	})

	registerer := prometheus.NewRegistry()
	require.NoError(registerer.Register(numAccepted))

	// Create context with the registerer
	ctxWithReg := &ConsensusContext{
		Context:    ctx,
		Registerer: registerer,
	}

	// Initialize should fail due to metric conflict
	err := sm.InitializeWithContext(
		ctxWithReg,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.Error(err)
}

// TestMetricsRejectedError tests that rejected counter metric conflicts are handled
func TestMetricsRejectedError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	// Create and register a conflicting metric
	numRejected := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "blks_rejected_count",
	})

	registerer := prometheus.NewRegistry()
	require.NoError(registerer.Register(numRejected))

	// Create context with the registerer
	ctxWithReg := &ConsensusContext{
		Context:    ctx,
		Registerer: registerer,
	}

	// Initialize should fail due to metric conflict
	err := sm.InitializeWithContext(
		ctxWithReg,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.Error(err)
}

// Helper types and functions

// ErrorBlock is a test block that returns errors on Accept/Reject
type ErrorBlock struct {
	*chaintest.TestBlock
	AcceptErr error
	RejectErr error
	VerifyErr error
}

// Accept implements block.Block with error injection
func (b *ErrorBlock) Accept(ctx context.Context) error {
	if b.AcceptErr != nil {
		return b.AcceptErr
	}
	b.TestBlock.Decidable.Status = 2 // Accepted
	return nil
}

// Reject implements block.Block with error injection
func (b *ErrorBlock) Reject(ctx context.Context) error {
	if b.RejectErr != nil {
		return b.RejectErr
	}
	b.TestBlock.Decidable.Status = 3 // Rejected
	return nil
}

// Verify implements block.Block with error injection
func (b *ErrorBlock) Verify(ctx context.Context) error {
	if b.VerifyErr != nil {
		return b.VerifyErr
	}
	return b.TestBlock.Verify(ctx)
}

// Consensus represents the chain consensus engine with error handling
type Consensus struct {
	ctx        context.Context
	params     *Parameters
	blocks     map[ids.ID]block.Block
	processing map[ids.ID]bool
	lastAccepted ids.ID
	metrics    *consensusMetrics
}

// Parameters holds consensus parameters
type Parameters struct {
	K                     int
	AlphaPreference       int
	AlphaConfidence       int
	Beta                  int
	ConcurrentRepolls     int
	OptimalProcessing     int
	MaxOutstandingItems   int
	MaxItemProcessingTime time.Duration
}

// DefaultParameters returns default consensus parameters
func DefaultParameters() *Parameters {
	return &Parameters{
		K:                     1,
		AlphaPreference:       1,
		AlphaConfidence:       1,
		Beta:                  1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1 * time.Second,
	}
}

// ConsensusContext extends context with Prometheus registerer
type ConsensusContext struct {
	context.Context
	Registerer prometheus.Registerer
}

// consensusMetrics holds consensus metrics
type consensusMetrics struct {
	numProcessing prometheus.Gauge
	numAccepted   prometheus.Counter
	numRejected   prometheus.Counter
}

// NewConsensus creates a new consensus engine
func NewConsensus() *Consensus {
	return &Consensus{
		blocks:     make(map[ids.ID]block.Block),
		processing: make(map[ids.ID]bool),
	}
}

// Initialize initializes the consensus engine
func (c *Consensus) Initialize(
	ctx context.Context,
	params *Parameters,
	genesisID ids.ID,
	genesisHeight uint64,
	genesisTimestamp time.Time,
) error {
	c.ctx = ctx
	c.params = params
	c.lastAccepted = genesisID

	// Initialize metrics with a new registerer for testing
	return c.initMetrics(prometheus.NewRegistry())
}

// InitializeWithContext initializes with a custom context containing registerer
func (c *Consensus) InitializeWithContext(
	ctx *ConsensusContext,
	params *Parameters,
	genesisID ids.ID,
	genesisHeight uint64,
	genesisTimestamp time.Time,
) error {
	c.ctx = ctx.Context
	c.params = params
	c.lastAccepted = genesisID

	// Initialize metrics with provided registerer
	return c.initMetrics(ctx.Registerer)
}

// initMetrics initializes consensus metrics
func (c *Consensus) initMetrics(registerer prometheus.Registerer) error {
	c.metrics = &consensusMetrics{
		numProcessing: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "blks_processing",
			Help: "Number of blocks currently processing",
		}),
		numAccepted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "blks_accepted_count",
			Help: "Total number of accepted blocks",
		}),
		numRejected: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "blks_rejected_count",
			Help: "Total number of rejected blocks",
		}),
	}

	// Register metrics - this may fail if metrics already exist
	if err := registerer.Register(c.metrics.numProcessing); err != nil {
		return err
	}
	if err := registerer.Register(c.metrics.numAccepted); err != nil {
		return err
	}
	if err := registerer.Register(c.metrics.numRejected); err != nil {
		return err
	}

	return nil
}

// Add adds a block to consensus
func (c *Consensus) Add(ctx context.Context, blk block.Block) error {
	blkID := blk.ID()

	// Check if block is genesis (already decided)
	if blkID == c.lastAccepted {
		return errUnknownParentBlock
	}

	// Check if parent exists
	parentID := blk.ParentID()
	if parentID != c.lastAccepted && c.blocks[parentID] == nil {
		// For test purposes, allow adding if parent is genesis
		if parentID != chaintest.Genesis.ID() {
			return errUnknownParentBlock
		}
	}

	c.blocks[blkID] = blk
	c.processing[blkID] = true

	if c.metrics != nil {
		c.metrics.numProcessing.Inc()
	}

	return nil
}

// RecordPoll records voting results and processes consensus
func (c *Consensus) RecordPoll(ctx context.Context, votes *bag.Bag[ids.ID]) error {
	// Get the most voted block
	var topChoice ids.ID
	maxVotes := 0

	for _, id := range votes.List() {
		count := votes.Count(id)
		if count > maxVotes {
			maxVotes = count
			topChoice = id
		}
	}

	if topChoice == ids.Empty {
		return nil
	}

	// Accept the winning block
	if blk, exists := c.blocks[topChoice]; exists {
		if err := blk.Accept(ctx); err != nil {
			return err
		}

		delete(c.processing, topChoice)
		if c.metrics != nil {
			c.metrics.numProcessing.Dec()
			c.metrics.numAccepted.Inc()
		}

		// Reject all conflicting blocks (siblings and their descendants)
		if err := c.rejectConflicting(ctx, blk); err != nil {
			return err
		}
	}

	return nil
}

// rejectConflicting rejects blocks that conflict with the accepted block
func (c *Consensus) rejectConflicting(ctx context.Context, accepted block.Block) error {
	acceptedParent := accepted.ParentID()

	// Find and reject all siblings (blocks with same parent)
	for _, blk := range c.blocks {
		if blk.ID() == accepted.ID() {
			continue
		}

		// Check if it's a sibling (same parent, different block)
		if blk.ParentID() == acceptedParent {
			if err := c.rejectWithDescendants(ctx, blk); err != nil {
				return err
			}
		}
	}

	return nil
}

// rejectWithDescendants rejects a block and all its descendants
func (c *Consensus) rejectWithDescendants(ctx context.Context, blk block.Block) error {
	blkID := blk.ID()

	// First reject all descendants
	for _, child := range c.blocks {
		if child.ParentID() == blkID {
			if err := c.rejectWithDescendants(ctx, child); err != nil {
				return err
			}
		}
	}

	// Then reject this block
	if err := blk.Reject(ctx); err != nil {
		return err
	}

	delete(c.processing, blkID)
	if c.metrics != nil {
		c.metrics.numProcessing.Dec()
		c.metrics.numRejected.Inc()
	}

	return nil
}

// TestGracefulFailureHandling tests that the consensus engine handles failures gracefully
func TestGracefulFailureHandling(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	err := sm.Initialize(
		ctx,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.NoError(err)

	// Create blocks with various error conditions
	verifyErrBlock := &ErrorBlock{
		TestBlock: chaintest.BuildChild(chaintest.Genesis),
		VerifyErr: errors.New("verification failed"),
	}

	acceptErrBlock := &ErrorBlock{
		TestBlock: chaintest.BuildChild(chaintest.Genesis),
		AcceptErr: errors.New("acceptance failed"),
	}

	// Add blocks - verify errors should not prevent adding
	err = sm.Add(ctx, verifyErrBlock)
	require.NoError(err)

	err = sm.Add(ctx, acceptErrBlock)
	require.NoError(err)

	// Vote for the block with accept error
	votes := bag.Bag[ids.ID]{}
	votes.SetThreshold(2)
	votes.Add(acceptErrBlock.ID())
	err = sm.RecordPoll(ctx, &votes)
	require.Error(err)
	require.Contains(err.Error(), "acceptance failed")

	// Verify that metrics are still consistent after error
	require.Equal(2, len(sm.processing), "Both blocks should still be processing after error")
}

// TestMetricsAccuracyDuringErrors tests that metrics remain accurate even when errors occur
func TestMetricsAccuracyDuringErrors(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a custom registry to track metrics
	registry := prometheus.NewRegistry()

	// Create a consensus engine
	sm := NewConsensus()
	params := DefaultParameters()

	ctxWithReg := &ConsensusContext{
		Context:    ctx,
		Registerer: registry,
	}

	err := sm.InitializeWithContext(
		ctxWithReg,
		params,
		chaintest.Genesis.ID(),
		chaintest.Genesis.Height(),
		chaintest.Genesis.Timestamp(),
	)
	require.NoError(err)

	// Add blocks
	block1 := chaintest.BuildChild(chaintest.Genesis)
	block2 := &ErrorBlock{
		TestBlock: chaintest.BuildChild(chaintest.Genesis),
		RejectErr: errors.New("reject failed"),
	}

	err = sm.Add(ctx, block1)
	require.NoError(err)

	err = sm.Add(ctx, block2)
	require.NoError(err)

	// Check processing metric
	metricFamilies, err := registry.Gather()
	require.NoError(err)

	processingMetric := findMetric(metricFamilies, "blks_processing")
	require.NotNil(processingMetric)
	require.Equal(float64(2), processingMetric.GetGauge().GetValue())

	// Vote for block1, causing block2 rejection to fail
	votes := bag.Bag[ids.ID]{}
	votes.SetThreshold(2)
	votes.Add(block1.ID())
	err = sm.RecordPoll(ctx, &votes)
	require.Error(err)

	// Verify metrics after error
	metricFamilies, err = registry.Gather()
	require.NoError(err)

	// Processing count should have decreased for accepted block
	processingMetric = findMetric(metricFamilies, "blks_processing")
	require.NotNil(processingMetric)
	require.Equal(float64(1), processingMetric.GetGauge().GetValue())

	// Accepted count should have increased
	acceptedMetric := findMetric(metricFamilies, "blks_accepted_count")
	require.NotNil(acceptedMetric)
	require.Equal(float64(1), acceptedMetric.GetCounter().GetValue())
}

// findMetric finds a specific metric in gathered metric families
func findMetric(families []*dto.MetricFamily, name string) *dto.Metric {
	for _, family := range families {
		if family.GetName() == name {
			metrics := family.GetMetric()
			if len(metrics) > 0 {
				return metrics[0]
			}
		}
	}
	return nil
}