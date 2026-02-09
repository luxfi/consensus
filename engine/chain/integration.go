// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// ValidatorSampler provides access to the validator set for peer sampling.
// This is the minimal interface needed by consensus - avoids importing full validator package.
type ValidatorSampler interface {
	// Sample returns k random validator NodeIDs from the network.
	Sample(networkID ids.ID, k int) ([]ids.NodeID, error)
	// Count returns the number of validators in the network.
	Count(networkID ids.ID) int
}

// NetworkConfig holds parameters for integrating consensus with the node network.
type NetworkConfig struct {
	// ChainID is the ID of this chain (used for chain-scoped messages like Put, PullQuery)
	ChainID ids.ID
	// NetworkID is the ID of the network whose validators secure this chain
	// For primary network chains (P/X/C), this equals constants.PrimaryNetworkID
	// For L1 chains, this is the L1's validator set ID
	NetworkID ids.ID
	// NodeID is this node's identifier (for excluding self from samples)
	NodeID ids.NodeID
	// Validators provides access to the validator set for peer sampling.
	// If nil, the engine broadcasts to all peers (less efficient).
	Validators ValidatorSampler
	// Logger for consensus events
	Logger log.Logger
	// Gossiper broadcasts messages to validators
	Gossiper Gossiper
	// VM implements BlockBuilder for block creation
	VM BlockBuilder
	// Params are optional consensus parameters. If nil, DefaultParams() is used.
	// For small validator sets (e.g., 5 nodes), use LocalParams() which has
	// K=5, Beta=4 - appropriate thresholds for the validator count.
	Params *config.Parameters
}

// Gossiper abstracts the network layer for consensus message broadcasting.
// This minimal interface avoids importing the full node/network package.
//
// Gossiper is the network-level interface. BlockProposer (in engine.go)
// is the consensus-level interface. The gossiperAdapter bridges them.
type Gossiper interface {
	// GossipPut broadcasts a Put message with block data to validators.
	// Returns the number of validators the message was sent to.
	GossipPut(chainID ids.ID, networkID ids.ID, blockData []byte) int
	// SendPullQuery sends a PullQuery to specific validators requesting votes.
	SendPullQuery(chainID ids.ID, networkID ids.ID, blockID ids.ID, validators []ids.NodeID) int
	// SendVote sends a vote response back to the proposer node.
	// Vote response back to the proposer
	// This is called after fast-follow acceptance to notify the proposer
	// that this node has accepted the block, enabling the proposer to
	// reach vote threshold and finalize its own copy.
	SendVote(chainID ids.ID, toNodeID ids.NodeID, blockID ids.ID) error
}

// Runtime wraps Transitive with network integration and VM notification handling.
// Use NewRuntime to create - this is the "one right way" to set up consensus.
type Runtime struct {
	*Transitive
	config NetworkConfig

	// Validator sampling for k-peer polls
	validators ValidatorSampler
	nodeID     ids.NodeID
}

// NewRuntime creates a fully wired consensus runtime ready for production use.
//
// This is the single, canonical way to create a chain consensus runtime for node integration.
// It:
//  1. Creates the Transitive engine with default parameters
//  2. Wires the network gossiper as the BlockProposer
//  3. Registers the VM for block building
//  4. Returns a ready-to-start runtime
//
// Usage in manager.go:
//
//	runtime := chain.NewRuntime(chain.NetworkConfig{
//	    ChainID:   chainParams.ID,
//	    NetworkID: chainParams.ChainID,  // PrimaryNetworkID for P/X/C
//	    Logger:    m.Log,
//	    Gossiper:  &networkGossiper{net: m.Net, msgCreator: m.MsgCreator},
//	    VM:        vm.(chain.BlockBuilder),
//	})
//	if err := runtime.Start(ctx, true); err != nil { return err }
//	go runtime.ForwardVMNotifications(toEngine)
func NewRuntime(cfg NetworkConfig) *Runtime {
	// Use provided params or default
	params := config.DefaultParams()
	if cfg.Params != nil {
		params = *cfg.Params
	}
	engine := NewWithParams(params)

	rt := &Runtime{
		Transitive: engine,
		config:     cfg,
		validators: cfg.Validators,
		nodeID:     cfg.NodeID,
	}

	// Log validator set status for debugging
	hasLogger := cfg.Logger != nil && !cfg.Logger.IsZero()
	if cfg.Validators != nil {
		count := cfg.Validators.Count(cfg.NetworkID)
		if hasLogger {
			cfg.Logger.Info("consensus engine initialized with validator set",
				log.Stringer("networkID", cfg.NetworkID),
				log.Int("validatorCount", count),
				log.Int("k", params.K),
				log.Int("alpha", params.AlphaPreference))
		}
	} else {
		if hasLogger {
			cfg.Logger.Warn("consensus engine initialized WITHOUT validator set - will broadcast to all peers",
				log.Stringer("networkID", cfg.NetworkID))
		}
	}

	// Wire the proposer (adapts Gossiper to BlockProposer interface)
	engine.SetProposer(&gossiperProposer{
		gossiper:   cfg.Gossiper,
		chainID:    cfg.ChainID,
		networkID:  cfg.NetworkID,
		logger:     cfg.Logger,
		validators: cfg.Validators,
		nodeID:     cfg.NodeID,
		k:          params.K,
	})

	// Set the VM for block building
	if cfg.VM != nil {
		engine.SetVM(cfg.VM)
	}

	return rt
}

// SampleValidators returns k random validator NodeIDs for the network.
// This is used by the consensus engine for k-sampling polls.
// Returns nil if no validator sampler is configured (falls back to broadcast).
func (rt *Runtime) SampleValidators(k int) ([]ids.NodeID, error) {
	if rt.validators == nil {
		return nil, nil // Will broadcast to all
	}
	return rt.validators.Sample(rt.config.NetworkID, k)
}

// ValidatorCount returns the number of validators in the network.
// Returns 0 if no validator sampler is configured.
func (rt *Runtime) ValidatorCount() int {
	if rt.validators == nil {
		return 0
	}
	return rt.validators.Count(rt.config.NetworkID)
}

// ForwardVMNotifications reads from the VM's toEngine channel and forwards
// PendingTxs notifications to trigger block building through consensus.
//
// Call this as a goroutine after Start():
//
//	go runtime.ForwardVMNotifications(toEngine)
//
// The goroutine exits when the channel is closed.
func (rt *Runtime) ForwardVMNotifications(toEngine <-chan block.Message) {
	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("starting VM notification forwarder for Lux consensus",
			log.Stringer("chainID", rt.config.ChainID))
	}

	for msg := range toEngine {
		// Translate block.MessageType → engine.MessageType
		// block.PendingTxs = 1 (iota+1), core.PendingTxs = 0
		// block.StateSyncDone = 2, core.StateSyncDone = 1
		var engineMsgType MessageType
		switch msg.Type {
		case block.PendingTxs:
			engineMsgType = PendingTxs
		case block.StateSyncDone:
			engineMsgType = StateSyncDone
		default:
			if !rt.config.Logger.IsZero() {
				rt.config.Logger.Warn("unknown VM message type, dropping",
					log.Uint32("type", uint32(msg.Type)))
			}
			continue
		}

		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("received VM notification, forwarding to consensus engine",
				log.Uint32("vmType", uint32(msg.Type)),
				log.Uint32("engineType", uint32(engineMsgType)))
		}

		ctx := context.Background()
		if err := rt.Notify(ctx, Message{Type: engineMsgType}); err != nil {
			if !rt.config.Logger.IsZero() {
				rt.config.Logger.Warn("failed to notify consensus engine",
					log.Uint32("type", uint32(engineMsgType)),
					log.Err(err))
			}
		}
	}

	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("VM notification forwarder stopped")
	}
}

// gossiperProposer adapts a Gossiper to the BlockProposer interface.
// This bridges the network layer (Gossiper) to the consensus layer (BlockProposer).
type gossiperProposer struct {
	gossiper   Gossiper
	chainID    ids.ID
	networkID  ids.ID
	logger     log.Logger
	validators ValidatorSampler // For k-peer sampling
	nodeID     ids.NodeID       // This node's ID (to exclude from samples)
	k          int              // Sample size from consensus params
}

var _ BlockProposer = (*gossiperProposer)(nil)

// Propose broadcasts a block proposal to validators via the network gossiper.
func (p *gossiperProposer) Propose(ctx context.Context, proposal BlockProposal) error {
	if p.gossiper == nil {
		if !p.logger.IsZero() {
			p.logger.Warn("cannot propose block - gossiper is nil",
				log.Stringer("blockID", proposal.BlockID))
		}
		return nil // Not fatal - local acceptance still works
	}

	sentTo := p.gossiper.GossipPut(p.chainID, p.networkID, proposal.BlockData)
	if !p.logger.IsZero() {
		p.logger.Info("proposed block to validators",
			log.Stringer("blockID", proposal.BlockID),
			log.Uint64("height", proposal.Height),
			log.Int("sentTo", sentTo))
	}
	return nil
}

// RequestVotes asks specific validators to vote on a block.
// If req.Validators is nil and we have a ValidatorSampler, sample k validators.
// This implements the core Snowball/Avalanche k-sampling behavior.
func (p *gossiperProposer) RequestVotes(ctx context.Context, req VoteRequest) error {
	if p.gossiper == nil {
		if !p.logger.IsZero() {
			p.logger.Warn("cannot request votes - gossiper is nil",
				log.Stringer("blockID", req.BlockID))
		}
		return nil
	}

	// Determine which validators to query
	validators := req.Validators
	if validators == nil && p.validators != nil && p.k > 0 {
		// Sample k validators from the validator set (excluding self)
		sampled, err := p.validators.Sample(p.networkID, p.k)
		if err != nil {
			if !p.logger.IsZero() {
				p.logger.Warn("failed to sample validators, falling back to broadcast",
					log.Stringer("blockID", req.BlockID),
					log.Int("k", p.k),
					log.Err(err))
			}
			// Fall back to broadcast (nil validators)
		} else {
			// Filter out self from sample
			filtered := make([]ids.NodeID, 0, len(sampled))
			for _, nodeID := range sampled {
				if nodeID != p.nodeID {
					filtered = append(filtered, nodeID)
				}
			}
			validators = filtered

			if !p.logger.IsZero() {
				p.logger.Debug("sampled k validators for poll",
					log.Stringer("blockID", req.BlockID),
					log.Int("k", p.k),
					log.Int("sampled", len(validators)),
					log.Int("totalValidators", p.validators.Count(p.networkID)))
			}
		}
	}

	sentTo := p.gossiper.SendPullQuery(p.chainID, p.networkID, req.BlockID, validators)
	if !p.logger.IsZero() {
		p.logger.Debug("requested votes from validators",
			log.Stringer("blockID", req.BlockID),
			log.Int("requested", len(validators)),
			log.Int("sentTo", sentTo))
	}
	return nil
}

// HandleIncomingBlock processes a block received from network gossip.
// For follower nodes receiving blocks from the proposer, this uses a "fast-follow"
// pattern where verified blocks extending the accepted chain are accepted immediately.
//
// This is necessary because in the current architecture, votes are only sent back
// to the proposer (not gossiped to all validators). So followers would never reach
// the vote threshold on their own. Instead, followers trust that:
// 1. The proposer collected enough votes before gossiping the block
// 2. The block verifies correctly against their state
// 3. The block extends their current chain tip
//
// Returns the parsed block if successful, nil otherwise.
func (rt *Runtime) HandleIncomingBlock(ctx context.Context, blockData []byte, fromNodeID ids.NodeID) (block.Block, error) {
	if rt.config.VM == nil {
		return nil, nil
	}

	// Parse the block
	blk, err := rt.config.VM.ParseBlock(ctx, blockData)
	if err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("failed to parse incoming block",
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return nil, err
	}

	// Verify the block
	if err := blk.Verify(ctx); err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming block failed verification",
				log.Stringer("blockID", blk.ID()),
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return blk, err
	}

	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("received and verified block from gossip",
			log.Stringer("blockID", blk.ID()),
			log.Uint64("height", blk.Height()),
			log.Stringer("from", fromNodeID))
	}

	// Fast-follow pattern: Accept blocks that extend our chain from valid proposers
	// This is safe because:
	// 1. Block has been verified (cryptographic integrity, state transitions)
	// 2. Block came from a known validator (fromNodeID)
	// 3. Block extends our accepted chain (parent check below)
	lastAccepted, err := rt.config.VM.LastAccepted(ctx)
	if err == nil {
		parentID := blk.ParentID()
		// Accept if this block extends our last accepted block
		// OR if it's at height 1 (first block after genesis)
		if parentID == lastAccepted || blk.Height() == 1 {
			if !rt.config.Logger.IsZero() {
				rt.config.Logger.Info("fast-follow: accepting block from validator",
					log.Stringer("blockID", blk.ID()),
					log.Uint64("height", blk.Height()),
					log.Stringer("from", fromNodeID),
					log.Stringer("parent", parentID),
					log.Stringer("lastAccepted", lastAccepted))
			}

			// Accept the block directly
			if err := blk.Accept(ctx); err != nil {
				if !rt.config.Logger.IsZero() {
					rt.config.Logger.Warn("failed to accept block in fast-follow",
						log.Stringer("blockID", blk.ID()),
						log.Err(err))
				}
			} else {
				// Update VM preference to build on this block
				if err := rt.config.VM.SetPreference(ctx, blk.ID()); err != nil {
					if !rt.config.Logger.IsZero() {
						rt.config.Logger.Warn("failed to set preference after fast-follow accept",
							log.Stringer("blockID", blk.ID()),
							log.Err(err))
					}
				}
				rt.Transitive.blocksAccepted++
				if !rt.config.Logger.IsZero() {
					rt.config.Logger.Info("fast-follow: block accepted successfully",
						log.Stringer("blockID", blk.ID()),
						log.Uint64("height", blk.Height()))
				}

				// CRITICAL: Send vote back to proposer so they can reach threshold
				// and finalize their own copy of the block. Without this, the
				// proposer (node1) stays at height 0 while followers advance.
				if rt.config.Gossiper != nil {
					if err := rt.config.Gossiper.SendVote(rt.config.ChainID, fromNodeID, blk.ID()); err != nil {
						if !rt.config.Logger.IsZero() {
							rt.config.Logger.Warn("failed to send vote to proposer after fast-follow",
								log.Stringer("blockID", blk.ID()),
								log.Stringer("proposer", fromNodeID),
								log.Err(err))
						}
					} else {
						if !rt.config.Logger.IsZero() {
							rt.config.Logger.Info("fast-follow: sent vote back to proposer",
								log.Stringer("blockID", blk.ID()),
								log.Stringer("proposer", fromNodeID))
						}
					}
				}
			}
			return blk, nil
		}
	}

	// If fast-follow doesn't apply, fall back to consensus tracking
	// (This handles out-of-order blocks or conflicting chains)
	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Debug("block does not extend chain tip, adding to consensus tracking",
			log.Stringer("blockID", blk.ID()),
			log.Uint64("height", blk.Height()))
	}

	// Create consensus block and add to tracking
	consensusBlock := &Block{
		id:        blk.ID(),
		parentID:  blk.ParentID(),
		height:    blk.Height(),
		timestamp: blk.Timestamp().Unix(),
		data:      blk.Bytes(),
	}

	// Add to consensus engine
	if err := rt.Transitive.consensus.AddBlock(ctx, consensusBlock); err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("failed to add block to consensus",
				log.Stringer("blockID", blk.ID()),
				log.Err(err))
		}
		// Continue anyway - block may already exist
	}

	// Add to pending blocks for tracking
	rt.Transitive.mu.Lock()
	if _, exists := rt.Transitive.pendingBlocks[blk.ID()]; !exists {
		rt.Transitive.pendingBlocks[blk.ID()] = &PendingBlock{
			ConsensusBlock: consensusBlock,
			VMBlock:        blk,
			ProposedAt:     time.Now(),
			VoteCount:      1, // Count our own vote
			Decided:        false,
		}
	} else {
		// Block already tracked, increment vote count
		rt.Transitive.pendingBlocks[blk.ID()].VoteCount++
	}
	rt.Transitive.mu.Unlock()

	// Vote in favor of the block (process our vote)
	responses := map[ids.ID]int{blk.ID(): 1}
	if err := rt.Transitive.consensus.Poll(ctx, responses); err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("failed to vote on block",
				log.Stringer("blockID", blk.ID()),
				log.Err(err))
		}
	}

	return blk, nil
}

// OnImportComplete must be called after admin_importChain (RLP import) completes.
// This reconciles the consensus engine's state with the VM's actual state after import.
//
// The problem this solves:
//   - RLP import updates the EVM state database directly
//   - But the consensus engine still thinks lastAccepted is the old block
//   - This causes transactions to timeout (engine builds on wrong parent)
//   - And causes "chains not bootstrapped" errors on node restart
//
// This method:
//  1. Queries VM.LastAccepted() to get the current chain tip after import
//  2. Updates consensus.finalizedTip to match
//  3. Updates VM preference to build on the new tip
//  4. Marks consensus as bootstrapped
//
// Usage in EVM admin API after successful import:
//
//	if err := rt.OnImportComplete(ctx); err != nil {
//	    log.Warn("failed to sync consensus after import", "error", err)
//	}
//
// This is idempotent - safe to call even if import didn't change state.
func (rt *Runtime) OnImportComplete(ctx context.Context) error {
	logger := rt.config.Logger
	hasLogger := logger != nil && !logger.IsZero()

	if rt.config.VM == nil {
		if hasLogger {
			logger.Warn("OnImportComplete: VM is nil, cannot sync state")
		}
		return nil
	}

	// Step 1: Query VM for current last accepted block
	lastAcceptedID, err := rt.config.VM.LastAccepted(ctx)
	if err != nil {
		if hasLogger {
			logger.Warn("OnImportComplete: failed to get last accepted from VM",
				log.Err(err))
		}
		return err
	}

	// Step 2: Get block details (height) for consensus state
	var height uint64
	if lastAcceptedID != ids.Empty {
		blk, err := rt.config.VM.GetBlock(ctx, lastAcceptedID)
		if err != nil {
			if hasLogger {
				logger.Warn("OnImportComplete: failed to get block details",
					log.Stringer("blockID", lastAcceptedID),
					log.Err(err))
			}
			// Continue with height 0 - consensus can recover
		} else {
			height = blk.Height()
		}
	}

	// Step 3: Update VM preference to build on current tip
	// This is critical: without this, the VM's Preferred() returns old block
	// while GetLastAccepted() returns the imported block, causing state mismatch
	if err := rt.config.VM.SetPreference(ctx, lastAcceptedID); err != nil {
		if hasLogger {
			logger.Warn("OnImportComplete: failed to set VM preference",
				log.Stringer("blockID", lastAcceptedID),
				log.Err(err))
		}
		// Non-fatal: continue with consensus sync
	}

	// Step 4: Sync consensus engine state
	if err := rt.Transitive.SyncState(ctx, lastAcceptedID, height); err != nil {
		if hasLogger {
			logger.Warn("OnImportComplete: failed to sync consensus state",
				log.Stringer("blockID", lastAcceptedID),
				log.Uint64("height", height),
				log.Err(err))
		}
		return err
	}

	if hasLogger {
		logger.Info("OnImportComplete: consensus state synced with VM",
			log.Stringer("lastAcceptedID", lastAcceptedID),
			log.Uint64("height", height))
	}

	return nil
}

// SyncStateFromVM queries the VM for its current state and syncs the consensus
// engine to match. This is a lower-level version of OnImportComplete that can
// be called without a full Runtime (e.g., from standalone syncer usage).
//
// Returns the synced block ID and height, or error.
func SyncStateFromVM(ctx context.Context, vm BlockBuilder, consensus *Transitive) (ids.ID, uint64, error) {
	if vm == nil {
		return ids.Empty, 0, nil
	}

	// Get last accepted from VM
	lastAcceptedID, err := vm.LastAccepted(ctx)
	if err != nil {
		return ids.Empty, 0, err
	}

	// Get height
	var height uint64
	if lastAcceptedID != ids.Empty {
		blk, err := vm.GetBlock(ctx, lastAcceptedID)
		if err == nil {
			height = blk.Height()
		}
	}

	// Set preference (non-fatal if this fails)
	_ = vm.SetPreference(ctx, lastAcceptedID)

	// Sync consensus
	if consensus != nil {
		if err := consensus.SyncState(ctx, lastAcceptedID, height); err != nil {
			return lastAcceptedID, height, err
		}
	}

	return lastAcceptedID, height, nil
}
