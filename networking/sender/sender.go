package sender

import (
	"context"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// Sender sends messages
type Sender interface {
	// Send sends a message
	Send(context.Context, Message) error

	// SendAppRequest sends an app request
	SendAppRequest(context.Context, set.Set[ids.NodeID], uint32, []byte) error

	// SendAppResponse sends an app response
	SendAppResponse(context.Context, ids.NodeID, uint32, []byte) error

	// SendAppGossip sends app gossip
	SendAppGossip(context.Context, set.Set[ids.NodeID], []byte) error
}

// Message represents a message to send
type Message struct {
	NodeIDs   set.Set[ids.NodeID]
	RequestID uint32
	Op        Op
	Bytes     []byte
}

// Op represents an operation
type Op byte

const (
	// GetAcceptedFrontier gets accepted frontier
	GetAcceptedFrontier Op = iota
	// AcceptedFrontier is accepted frontier response
	AcceptedFrontier
	// GetAccepted gets accepted
	GetAccepted
	// Accepted is accepted response
	Accepted
	// Get gets an item
	Get
	// Put puts an item
	Put
	// PushQuery pushes a query
	PushQuery
	// PullQuery pulls a query
	PullQuery
	// Chits is chits response
	Chits
)

// ExternalSender sends messages to external nodes
type ExternalSender interface {
	Sender
	
	// Send a message to a specific set of nodes
	SendGetStateSummaryFrontier(context.Context, set.Set[ids.NodeID], uint32) error
	SendStateSummaryFrontier(context.Context, ids.NodeID, uint32, []byte) error
	SendGetAcceptedStateSummary(context.Context, set.Set[ids.NodeID], uint32, []uint64) error
	SendAcceptedStateSummary(context.Context, ids.NodeID, uint32, []ids.ID) error
	
	// Consensus messages
	SendGetAcceptedFrontier(context.Context, set.Set[ids.NodeID], uint32) error
	SendAcceptedFrontier(context.Context, ids.NodeID, uint32, []ids.ID) error
	SendGetAccepted(context.Context, set.Set[ids.NodeID], uint32, []ids.ID) error
	SendAccepted(context.Context, ids.NodeID, uint32, []ids.ID) error
	SendGetAncestors(context.Context, ids.NodeID, uint32, ids.ID) error
	SendAncestors(context.Context, ids.NodeID, uint32, [][]byte) error
	SendGet(context.Context, ids.NodeID, uint32, ids.ID) error
	SendPut(context.Context, ids.NodeID, uint32, []byte) error
	SendPushQuery(context.Context, set.Set[ids.NodeID], uint32, []byte, uint64) error
	SendPullQuery(context.Context, set.Set[ids.NodeID], uint32, ids.ID, uint64) error
	SendChits(context.Context, ids.NodeID, uint32, ids.ID, ids.ID, ids.ID) error
	
	// CrossChain messages
	SendCrossChainAppRequest(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppResponse(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppError(context.Context, ids.ID, uint32, int32, string) error
}
