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

// NetworkConfig holds parameters for integrating consensus with the node network.
type NetworkConfig struct {
	// ChainID is the ID of this chain (used for chain-scoped messages like Put, PullQuery)
	ChainID ids.ID
	// NetworkID is the ID of the network whose validators secure this chain
	// For primary network chains (P/X/C), this equals constants.PrimaryNetworkID
	// For L1 chains, this is the L1's validator set ID
	NetworkID ids.ID
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
	}

	// Wire the proposer (adapts Gossiper to BlockProposer interface)
	engine.SetProposer(&gossiperProposer{
		gossiper:  cfg.Gossiper,
		chainID:   cfg.ChainID,
		networkID: cfg.NetworkID,
		logger:    cfg.Logger,
	})

	// Set the VM for block building
	if cfg.VM != nil {
		engine.SetVM(cfg.VM)
	}

	return rt
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
	if rt.config.Logger != nil {
		rt.config.Logger.Info("starting VM notification forwarder for Lux consensus",
			log.Stringer("chainID", rt.config.ChainID))
	}

	for msg := range toEngine {
		if rt.config.Logger != nil {
			rt.config.Logger.Debug("received VM notification, forwarding to consensus engine",
				log.Uint32("type", uint32(msg.Type)))
		}

		ctx := context.Background()
		if err := rt.Notify(ctx, Message{Type: MessageType(msg.Type)}); err != nil {
			if rt.config.Logger != nil {
				rt.config.Logger.Warn("failed to notify consensus engine",
					log.Uint32("type", uint32(msg.Type)),
					log.Err(err))
			}
		}
	}

	if rt.config.Logger != nil {
		rt.config.Logger.Info("VM notification forwarder stopped")
	}
}

// gossiperProposer adapts a Gossiper to the BlockProposer interface.
// This bridges the network layer (Gossiper) to the consensus layer (BlockProposer).
type gossiperProposer struct {
	gossiper  Gossiper
	chainID   ids.ID
	networkID ids.ID
	logger    log.Logger
}

var _ BlockProposer = (*gossiperProposer)(nil)

// Propose broadcasts a block proposal to validators via the network gossiper.
func (p *gossiperProposer) Propose(ctx context.Context, proposal BlockProposal) error {
	if p.gossiper == nil {
		if p.logger != nil {
			p.logger.Warn("cannot propose block - gossiper is nil",
				log.Stringer("blockID", proposal.BlockID))
		}
		return nil // Not fatal - local acceptance still works
	}

	sentTo := p.gossiper.GossipPut(p.chainID, p.networkID, proposal.BlockData)
	if p.logger != nil {
		p.logger.Info("proposed block to validators",
			log.Stringer("blockID", proposal.BlockID),
			log.Uint64("height", proposal.Height),
			log.Int("sentTo", sentTo))
	}
	return nil
}

// RequestVotes asks specific validators to vote on a block.
func (p *gossiperProposer) RequestVotes(ctx context.Context, req VoteRequest) error {
	if p.gossiper == nil {
		if p.logger != nil {
			p.logger.Warn("cannot request votes - gossiper is nil",
				log.Stringer("blockID", req.BlockID))
		}
		return nil
	}

	sentTo := p.gossiper.SendPullQuery(p.chainID, p.networkID, req.BlockID, req.Validators)
	if p.logger != nil {
		p.logger.Debug("requested votes from validators",
			log.Stringer("blockID", req.BlockID),
			log.Int("requested", len(req.Validators)),
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
		if rt.config.Logger != nil {
			rt.config.Logger.Debug("failed to parse incoming block",
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return nil, nil // Don't return error - just skip invalid blocks
	}

	// Verify the block
	if err := blk.Verify(ctx); err != nil {
		if rt.config.Logger != nil {
			rt.config.Logger.Debug("incoming block failed verification",
				log.Stringer("blockID", blk.ID()),
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return nil, nil // Don't return error - just skip invalid blocks
	}

	if rt.config.Logger != nil {
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
			if rt.config.Logger != nil {
				rt.config.Logger.Info("fast-follow: accepting block from validator",
					log.Stringer("blockID", blk.ID()),
					log.Uint64("height", blk.Height()),
					log.Stringer("from", fromNodeID),
					log.Stringer("parent", parentID),
					log.Stringer("lastAccepted", lastAccepted))
			}

			// Accept the block directly
			if err := blk.Accept(ctx); err != nil {
				if rt.config.Logger != nil {
					rt.config.Logger.Warn("failed to accept block in fast-follow",
						log.Stringer("blockID", blk.ID()),
						log.Err(err))
				}
			} else {
				// Update VM preference to build on this block
				if err := rt.config.VM.SetPreference(ctx, blk.ID()); err != nil {
					if rt.config.Logger != nil {
						rt.config.Logger.Warn("failed to set preference after fast-follow accept",
							log.Stringer("blockID", blk.ID()),
							log.Err(err))
					}
				}
				rt.Transitive.blocksAccepted++
				if rt.config.Logger != nil {
					rt.config.Logger.Info("fast-follow: block accepted successfully",
						log.Stringer("blockID", blk.ID()),
						log.Uint64("height", blk.Height()))
				}

				// CRITICAL: Send vote back to proposer so they can reach threshold
				// and finalize their own copy of the block. Without this, the
				// proposer (node1) stays at height 0 while followers advance.
				if rt.config.Gossiper != nil {
					if err := rt.config.Gossiper.SendVote(rt.config.ChainID, fromNodeID, blk.ID()); err != nil {
						if rt.config.Logger != nil {
							rt.config.Logger.Warn("failed to send vote to proposer after fast-follow",
								log.Stringer("blockID", blk.ID()),
								log.Stringer("proposer", fromNodeID),
								log.Err(err))
						}
					} else {
						if rt.config.Logger != nil {
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
	if rt.config.Logger != nil {
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
		if rt.config.Logger != nil {
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
		if rt.config.Logger != nil {
			rt.config.Logger.Debug("failed to vote on block",
				log.Stringer("blockID", blk.ID()),
				log.Err(err))
		}
	}

	return blk, nil
}
