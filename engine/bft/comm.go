//go:build ignore

// Copyright (C) 2019-2025, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package simplex

import (
	"errors"
	"fmt"

	simplex "github.com/luxfi/bft"
	"go.uber.org/zap"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/message"
	"github.com/luxfi/node/network"
	"github.com/luxfi/node/proto/pb/p2p"
	"github.com/luxfi/consensus/utils/set"
)

var (
	_               simplex.Communication = (*Comm)(nil)
	errNodeNotFound                       = errors.New("node not found in the validator list")
)

type Comm struct {
	logger   simplex.Logger
	subnetID ids.ID
	chainID  ids.ID
	// broadcastNodes are the nodes that should receive broadcast messages
	broadcastNodes set.Set[ids.NodeID]
	// allNodes are the IDs of all the nodes in the subnet
	allNodes []simplex.NodeID

	// sender is used to send messages to other nodes
	sender     network.ExternalSender
	msgBuilder message.OutboundMsgBuilder
}

func NewComm(config *Config) (*Comm, error) {
	if _, ok := config.Validators[config.Ctx.NodeID]; !ok {
		config.Log.Warn("Node is not a validator for the subnet",
			zap.Stringer("nodeID", config.Ctx.NodeID),
			zap.Stringer("chainID", config.Ctx.ChainID),
			zap.Stringer("subnetID", config.Ctx.SubnetID),
		)
		return nil, fmt.Errorf("our %w: %s", errNodeNotFound, config.Ctx.NodeID)
	}

	broadcastNodes := set.NewSet[ids.NodeID](len(config.Validators) - 1)
	allNodes := make([]simplex.NodeID, 0, len(config.Validators))
	// grab all the nodes that are validators for the subnet
	for _, vd := range config.Validators {
		allNodes = append(allNodes, vd.NodeID[:])
		if vd.NodeID == config.Ctx.NodeID {
			continue // skip our own node ID
		}

		broadcastNodes.Add(vd.NodeID)
	}

	return &Comm{
		subnetID:       config.Ctx.SubnetID,
		broadcastNodes: broadcastNodes,
		allNodes:       allNodes,
		logger:         NewLoggerWrapper(config.Log),
		sender:         config.Sender,
		msgBuilder:     config.OutboundMsgBuilder,
		chainID:        config.Ctx.ChainID,
	}, nil
}

func (c *Comm) Nodes() []simplex.NodeID {
	return c.allNodes
}

func (c *Comm) Send(msg *simplex.Message, destination simplex.NodeID) {
	outboundMsg, err := c.simplexMessageToOutboundMessage(msg)
	if err != nil {
		c.logger.Error("Failed creating message", zap.Error(err))
		return
	}

	dest, err := ids.ToNodeID(destination)
	if err != nil {
		c.logger.Error("Failed to convert destination NodeID", zap.Error(err))
		return
	}

	c.sender.Send(outboundMsg, set.Of(dest), c.subnetID, 0)
}

func (c *Comm) Broadcast(msg *simplex.Message) {
	outboundMsg, err := c.simplexMessageToOutboundMessage(msg)
	if err != nil {
		c.logger.Error("Failed creating message", zap.Error(err))
		return
	}

	c.sender.Send(outboundMsg, c.broadcastNodes, c.subnetID, 0)
}

func (c *Comm) simplexMessageToOutboundMessage(msg *simplex.Message) (message.OutboundMessage, error) {
	var bftMsg *p2p.BFT
	switch {
	case msg.VerifiedBlockMessage != nil:
		bytes, err := msg.VerifiedBlockMessage.VerifiedBlock.Bytes()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize block: %w", err)
		}
		bftMsg = newBlockProposal(c.chainID, bytes, msg.VerifiedBlockMessage.Vote)
	case msg.VoteMessage != nil:
		bftMsg = newVote(c.chainID, msg.VoteMessage)
	case msg.EmptyVoteMessage != nil:
		bftMsg = newEmptyVote(c.chainID, msg.EmptyVoteMessage)
	case msg.FinalizeVote != nil:
		bftMsg = newFinalizeVote(c.chainID, msg.FinalizeVote)
	case msg.Notarization != nil:
		bftMsg = newNotarization(c.chainID, msg.Notarization)
	case msg.EmptyNotarization != nil:
		bftMsg = newEmptyNotarization(c.chainID, msg.EmptyNotarization)
	case msg.Finalization != nil:
		bftMsg = newFinalization(c.chainID, msg.Finalization)
	case msg.ReplicationRequest != nil:
		bftMsg = newReplicationRequest(c.chainID, msg.ReplicationRequest)
	case msg.VerifiedReplicationResponse != nil:
		msg, err := newReplicationResponse(c.chainID, msg.VerifiedReplicationResponse)
		if err != nil {
			return nil, fmt.Errorf("failed to create replication response: %w", err)
		}
		bftMsg = msg
	default:
		return nil, fmt.Errorf("unknown message type: %+v", msg)
	}

	return c.msgBuilder.BFTMessage(bftMsg)
}
