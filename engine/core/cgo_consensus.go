// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo
// +build cgo

package core

/*
#cgo CFLAGS: -I${SRCDIR}/../../c/include
#cgo LDFLAGS: -L${SRCDIR}/../../c/lib -lluxconsensus -pthread
#include "lux_consensus.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/luxfi/consensus/engine/core/common"
	"github.com/luxfi/ids"
)

// CGOConsensus wraps the C consensus implementation
type CGOConsensus struct {
	engine *C.lux_consensus_engine_t
	mu     sync.RWMutex
}

// NewCGOConsensus creates a new consensus engine using the C implementation
func NewCGOConsensus(params ConsensusParams) (Consensus, error) {
	// Initialize the C library
	if err := C.lux_consensus_init(); err != C.LUX_SUCCESS {
		return nil, fmt.Errorf("failed to initialize C consensus library: %s", C.GoString(C.lux_error_string(err)))
	}

	// Create config
	config := C.lux_consensus_config_t{
		k:                          C.uint32_t(params.K),
		alpha_preference:          C.uint32_t(params.AlphaPreference),
		alpha_confidence:          C.uint32_t(params.AlphaConfidence),
		beta:                      C.uint32_t(params.Beta),
		concurrent_polls:          C.uint32_t(params.ConcurrentPolls),
		optimal_processing:        C.uint32_t(params.OptimalProcessing),
		max_outstanding_items:     C.uint32_t(params.MaxOutstandingItems),
		max_item_processing_time_ns: C.uint64_t(params.MaxItemProcessingTime),
		engine_type:               C.LUX_ENGINE_DAG,
	}

	// Create engine
	var engine *C.lux_consensus_engine_t
	if err := C.lux_consensus_engine_create(&engine, &config); err != C.LUX_SUCCESS {
		return nil, fmt.Errorf("failed to create C consensus engine: %s", C.GoString(C.lux_error_string(err)))
	}

	return &CGOConsensus{
		engine: engine,
	}, nil
}

// Add implements Consensus interface
func (c *CGOConsensus) Add(block Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Convert Go block to C block
	cBlock := C.lux_block_t{
		height:    C.uint64_t(block.Height()),
		timestamp: C.uint64_t(block.Timestamp().Unix()),
	}

	// Copy block ID
	blockID := block.ID()
	C.memcpy(unsafe.Pointer(&cBlock.id[0]), unsafe.Pointer(&blockID[0]), 32)

	// Copy parent ID
	parentID := block.Parent()
	C.memcpy(unsafe.Pointer(&cBlock.parent_id[0]), unsafe.Pointer(&parentID[0]), 32)

	// Add block data if available
	if blockBytes := block.Bytes(); len(blockBytes) > 0 {
		cBlock.data = C.malloc(C.size_t(len(blockBytes)))
		defer C.free(cBlock.data)
		C.memcpy(cBlock.data, unsafe.Pointer(&blockBytes[0]), C.size_t(len(blockBytes)))
		cBlock.data_size = C.size_t(len(blockBytes))
	}

	// Add block to consensus engine
	if err := C.lux_consensus_add_block(c.engine, &cBlock); err != C.LUX_SUCCESS {
		return fmt.Errorf("failed to add block to C consensus engine: %s", C.GoString(C.lux_error_string(err)))
	}

	return nil
}

// IsAccepted implements Consensus interface
func (c *CGOConsensus) IsAccepted(blockID ids.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var isAccepted C.bool
	var cBlockID [32]C.uint8_t
	C.memcpy(unsafe.Pointer(&cBlockID[0]), unsafe.Pointer(&blockID[0]), 32)

	if err := C.lux_consensus_is_accepted(c.engine, &cBlockID[0], &isAccepted); err != C.LUX_SUCCESS {
		return false
	}

	return bool(isAccepted)
}

// GetPreference implements Consensus interface
func (c *CGOConsensus) GetPreference() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var cBlockID [32]C.uint8_t
	if err := C.lux_consensus_get_preference(c.engine, &cBlockID[0]); err != C.LUX_SUCCESS {
		return ids.Empty
	}

	var blockID ids.ID
	copy(blockID[:], cBlockID[:])
	return blockID
}

// ProcessVote implements Consensus interface
func (c *CGOConsensus) ProcessVote(voterID ids.NodeID, blockID ids.ID, isPreference bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	vote := C.lux_vote_t{
		is_preference: C.bool(isPreference),
	}

	// Copy voter ID
	C.memcpy(unsafe.Pointer(&vote.voter_id[0]), unsafe.Pointer(&voterID[0]), 32)

	// Copy block ID
	C.memcpy(unsafe.Pointer(&vote.block_id[0]), unsafe.Pointer(&blockID[0]), 32)

	// Process vote
	if err := C.lux_consensus_process_vote(c.engine, &vote); err != C.LUX_SUCCESS {
		return fmt.Errorf("failed to process vote in C consensus engine: %s", C.GoString(C.lux_error_string(err)))
	}

	return nil
}

// Poll implements Consensus interface
func (c *CGOConsensus) Poll(validatorIDs []ids.NodeID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Convert validator IDs to C format
	numValidators := len(validatorIDs)
	if numValidators == 0 {
		return nil
	}

	// Allocate array of pointers to validator IDs
	validatorPtrs := make([]*C.uint8_t, numValidators)
	validatorData := make([][32]C.uint8_t, numValidators)

	for i, validatorID := range validatorIDs {
		C.memcpy(unsafe.Pointer(&validatorData[i][0]), unsafe.Pointer(&validatorID[0]), 32)
		validatorPtrs[i] = &validatorData[i][0]
	}

	// Call C poll function
	if err := C.lux_consensus_poll(
		c.engine,
		C.uint32_t(numValidators),
		(**C.uint8_t)(unsafe.Pointer(&validatorPtrs[0])),
	); err != C.LUX_SUCCESS {
		return fmt.Errorf("failed to poll in C consensus engine: %s", C.GoString(C.lux_error_string(err)))
	}

	return nil
}

// GetStats implements Consensus interface
func (c *CGOConsensus) GetStats() (Stats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var cStats C.lux_consensus_stats_t
	if err := C.lux_consensus_get_stats(c.engine, &cStats); err != C.LUX_SUCCESS {
		return Stats{}, fmt.Errorf("failed to get stats from C consensus engine: %s", C.GoString(C.lux_error_string(err)))
	}

	return Stats{
		BlocksAccepted:      uint64(cStats.blocks_accepted),
		BlocksRejected:      uint64(cStats.blocks_rejected),
		PollsCompleted:      uint64(cStats.polls_completed),
		VotesProcessed:      uint64(cStats.votes_processed),
		AverageDecisionTime: float64(cStats.average_decision_time_ms),
	}, nil
}

// Destroy cleans up the C consensus engine
func (c *CGOConsensus) Destroy() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.engine != nil {
		if err := C.lux_consensus_engine_destroy(c.engine); err != C.LUX_SUCCESS {
			return fmt.Errorf("failed to destroy C consensus engine: %s", C.GoString(C.lux_error_string(err)))
		}
		c.engine = nil
	}

	if err := C.lux_consensus_cleanup(); err != C.LUX_SUCCESS {
		return fmt.Errorf("failed to cleanup C consensus library: %s", C.GoString(C.lux_error_string(err)))
	}

	return nil
}

// RegisterDecisionCallback registers a callback for consensus decisions
func (c *CGOConsensus) RegisterDecisionCallback(callback func(blockID ids.ID)) error {
	// This would require creating a C callback wrapper
	// For now, we'll leave this unimplemented as it requires more complex CGO callback handling
	return fmt.Errorf("callback registration not yet implemented in CGO version")
}

// Ensure CGOConsensus implements the Consensus interface
var _ Consensus = (*CGOConsensus)(nil)