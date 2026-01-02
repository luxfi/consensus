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
}

// Gossiper abstracts the network layer for consensus message broadcasting.
// This minimal interface avoids importing the full node/network package.
//
// Gossiper is the network-level interface. BlockProposer (in engine.go)
// is the consensus-level interface. The gossiperAdapter bridges them.
type Gossiper interface {
	// GossipPut broadcasts a Put message with block data to validators.
	// Returns the number of validators the message was sent to.
	GossipPut(chainID ids.ID, netID ids.ID, blockData []byte) int
	// SendPullQuery sends a PullQuery to specific validators requesting votes.
	SendPullQuery(chainID ids.ID, netID ids.ID, blockID ids.ID, validators []ids.NodeID) int
}

// IntegratedEngine wraps Transitive with network integration and VM notification handling.
// Use NewIntegratedEngine to create - this is the "one right way" to set up consensus.
type IntegratedEngine struct {
	*Transitive
	config NetworkConfig
}

// NewIntegratedEngine creates a fully wired consensus engine ready for production use.
//
// This is the single, canonical way to create a chain consensus engine for node integration.
// It:
//  1. Creates the Transitive engine with default parameters
//  2. Wires the network gossiper as the BlockProposer
//  3. Registers the VM for block building
//  4. Returns a ready-to-start engine
//
// Usage in manager.go:
//
//	engine := chain.NewIntegratedEngine(chain.NetworkConfig{
//	    ChainID:   chainParams.ID,
//	    NetworkID: chainParams.ChainID,  // PrimaryNetworkID for P/X/C
//	    Logger:    m.Log,
//	    Gossiper:  &networkGossiper{net: m.Net, msgCreator: m.MsgCreator},
//	    VM:        vm.(chain.BlockBuilder),
//	})
//	if err := engine.Start(ctx, true); err != nil { return err }
//	go engine.ForwardVMNotifications(toEngine)
func NewIntegratedEngine(cfg NetworkConfig) *IntegratedEngine {
	engine := NewWithParams(config.DefaultParams())

	ie := &IntegratedEngine{
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

	return ie
}

// ForwardVMNotifications reads from the VM's toEngine channel and forwards
// PendingTxs notifications to trigger block building through consensus.
//
// Call this as a goroutine after Start():
//
//	go engine.ForwardVMNotifications(toEngine)
//
// The goroutine exits when the channel is closed.
func (ie *IntegratedEngine) ForwardVMNotifications(toEngine <-chan block.Message) {
	if ie.config.Logger != nil {
		ie.config.Logger.Info("starting VM notification forwarder for Lux consensus",
			log.Stringer("chainID", ie.config.ChainID))
	}

	for msg := range toEngine {
		if ie.config.Logger != nil {
			ie.config.Logger.Debug("received VM notification, forwarding to consensus engine",
				log.Uint32("type", uint32(msg.Type)))
		}

		ctx := context.Background()
		if err := ie.Notify(ctx, Message{Type: MessageType(msg.Type)}); err != nil {
			if ie.config.Logger != nil {
				ie.config.Logger.Warn("failed to notify consensus engine",
					log.Uint32("type", uint32(msg.Type)),
					log.Err(err))
			}
		}
	}

	if ie.config.Logger != nil {
		ie.config.Logger.Info("VM notification forwarder stopped")
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
// Instead of auto-accepting, it:
// 1. Parses and verifies the block
// 2. Adds it to pending blocks for consensus tracking
// 3. Votes in favor of the block
// Returns the parsed block if successful, nil otherwise.
func (ie *IntegratedEngine) HandleIncomingBlock(ctx context.Context, blockData []byte, fromNodeID ids.NodeID) (block.Block, error) {
	if ie.config.VM == nil {
		return nil, nil
	}

	// Parse the block
	blk, err := ie.config.VM.ParseBlock(ctx, blockData)
	if err != nil {
		if ie.config.Logger != nil {
			ie.config.Logger.Debug("failed to parse incoming block",
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return nil, nil // Don't return error - just skip invalid blocks
	}

	// Verify the block
	if err := blk.Verify(ctx); err != nil {
		if ie.config.Logger != nil {
			ie.config.Logger.Debug("incoming block failed verification",
				log.Stringer("blockID", blk.ID()),
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return nil, nil // Don't return error - just skip invalid blocks
	}

	if ie.config.Logger != nil {
		ie.config.Logger.Info("received and verified block from gossip",
			log.Stringer("blockID", blk.ID()),
			log.Uint64("height", blk.Height()),
			log.Stringer("from", fromNodeID))
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
	if err := ie.Transitive.consensus.AddBlock(ctx, consensusBlock); err != nil {
		if ie.config.Logger != nil {
			ie.config.Logger.Debug("failed to add block to consensus",
				log.Stringer("blockID", blk.ID()),
				log.Err(err))
		}
		// Continue anyway - block may already exist
	}

	// Add to pending blocks for tracking
	ie.Transitive.mu.Lock()
	if _, exists := ie.Transitive.pendingBlocks[blk.ID()]; !exists {
		ie.Transitive.pendingBlocks[blk.ID()] = &PendingBlock{
			ConsensusBlock: consensusBlock,
			VMBlock:        blk,
			ProposedAt:     time.Now(),
			VoteCount:      1, // Count our own vote
			Decided:        false,
		}
	} else {
		// Block already tracked, increment vote count
		ie.Transitive.pendingBlocks[blk.ID()].VoteCount++
	}
	ie.Transitive.mu.Unlock()

	// Vote in favor of the block (process our vote)
	responses := map[ids.ID]int{blk.ID(): 1}
	if err := ie.Transitive.consensus.Poll(ctx, responses); err != nil {
		if ie.config.Logger != nil {
			ie.config.Logger.Debug("failed to vote on block",
				log.Stringer("blockID", blk.ID()),
				log.Err(err))
		}
	}

	return blk, nil
}

