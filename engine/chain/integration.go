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
	// ChainID is the blockchain ID
	ChainID ids.ID
	// NetID is the subnet/network ID
	NetID ids.ID
	// Logger for consensus events
	Logger log.Logger
	// Gossiper broadcasts messages to validators
	Gossiper Gossiper
	// VM implements BlockBuilder for block creation
	VM BlockBuilder
}

// Gossiper abstracts the network layer for consensus message broadcasting.
// This minimal interface avoids importing the full node/network package.
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
//  2. Wires the network gossiper as the VoteEmitter
//  3. Registers the VM for block building
//  4. Returns a ready-to-start engine
//
// Usage in manager.go:
//
//	engine := chain.NewIntegratedEngine(chain.NetworkConfig{
//	    ChainID:  chainParams.ID,
//	    NetID:    chainParams.ChainID,
//	    Logger:   m.Log,
//	    Gossiper: &networkGossiper{net: m.Net, msgCreator: m.MsgCreator},
//	    VM:       vm.(chain.BlockBuilder),
//	})
//	if err := engine.Start(ctx, true); err != nil { return err }
//	go engine.ForwardVMNotifications(toEngine)
func NewIntegratedEngine(cfg NetworkConfig) *IntegratedEngine {
	engine := NewWithParams(config.DefaultParams())

	ie := &IntegratedEngine{
		Transitive: engine,
		config:     cfg,
	}

	// Wire the emitter (adapts Gossiper to VoteEmitter interface)
	engine.SetEmitter(&gossiperEmitter{
		gossiper: cfg.Gossiper,
		chainID:  cfg.ChainID,
		netID:    cfg.NetID,
		logger:   cfg.Logger,
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

// gossiperEmitter adapts a Gossiper to the VoteEmitter interface.
// This is an internal implementation detail - users don't see it.
type gossiperEmitter struct {
	gossiper Gossiper
	chainID  ids.ID
	netID    ids.ID
	logger   log.Logger
}

var _ VoteEmitter = (*gossiperEmitter)(nil)

func (e *gossiperEmitter) EmitBlockProposal(ctx context.Context, req VoteRequest) error {
	if e.gossiper == nil {
		if e.logger != nil {
			e.logger.Warn("cannot emit block proposal - gossiper is nil",
				log.Stringer("blockID", req.BlockID))
		}
		return nil // Not fatal - local acceptance still works
	}

	sentTo := e.gossiper.GossipPut(e.chainID, e.netID, req.BlockData)
	if e.logger != nil {
		e.logger.Info("emitted block proposal to validators (Photon phase)",
			log.Stringer("blockID", req.BlockID),
			log.Uint64("height", req.Height),
			log.Int("sentTo", sentTo))
	}
	return nil
}

func (e *gossiperEmitter) EmitVoteRequest(ctx context.Context, blockID ids.ID, validators []ids.NodeID) error {
	if e.gossiper == nil {
		if e.logger != nil {
			e.logger.Warn("cannot emit vote request - gossiper is nil",
				log.Stringer("blockID", blockID))
		}
		return nil
	}

	sentTo := e.gossiper.SendPullQuery(e.chainID, e.netID, blockID, validators)
	if e.logger != nil {
		e.logger.Debug("requested votes from validators (Wave phase)",
			log.Stringer("blockID", blockID),
			log.Int("requested", len(validators)),
			log.Int("sentTo", sentTo))
	}
	return nil
}

// SetVM sets the block builder VM on the engine.
func (t *Transitive) SetVM(vm BlockBuilder) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.vm = vm
}

// Notify handles VM notifications (PendingTxs triggers block building).
func (t *Transitive) Notify(ctx context.Context, msg Message) error {
	if msg.Type == PendingTxs {
		return t.buildAndProposeBlock(ctx)
	}
	return nil
}

// buildAndProposeBlock builds a block from the VM and adds it to consensus.
func (t *Transitive) buildAndProposeBlock(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.vm == nil {
		return nil // No VM set, skip
	}

	// Build the block
	blk, err := t.vm.BuildBlock(ctx)
	if err != nil {
		return err
	}

	t.blocksBuilt++
	blockID := blk.ID()

	// Create consensus block
	consensusBlk, err := t.consensus.Add(ctx, blockID, blk.Parent(), blk.Height())
	if err != nil {
		return err
	}

	// Track pending block
	t.pendingBlocks[blockID] = &PendingBlock{
		ConsensusBlock: consensusBlk,
		VMBlock:        blk,
		ProposedAt:     time.Now(),
	}

	// Emit block proposal (Photon phase)
	if t.emitter != nil {
		t.emitter.EmitBlockProposal(ctx, VoteRequest{
			BlockID:   blockID,
			BlockData: blk.Bytes(),
			Height:    blk.Height(),
			ParentID:  blk.Parent(),
		})
	}

	return nil
}
