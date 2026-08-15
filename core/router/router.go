package router

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// Message represents a network message
type Message interface {
	NodeID() ids.NodeID
	Op() Op
	Get(Field) interface{}
	Bytes() []byte
	BytesSavedCompression() int
	AddRef()
	DecRef()
	IsProposal() bool
}

// Op represents a message operation type
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
	// Vote is the authenticated preference signal returned by polling
	Vote
	// GetContext requests the verification context (parent chain) for a block
	GetContext
	// Context is the response containing prerequisite blocks needed to verify/attach a tip
	Context
	// Gossip is an unsolicited app-gossip payload (e.g. the α-of-K quorum
	// vote/cert envelope broadcast to all validators). It carries application
	// bytes, not a consensus container, and is demuxed by the chain handler's
	// Gossip method. Its wire value is fixed; append any new op ABOVE NumOps.
	Gossip

	// State-sync op-space. A node that pruned the ancestor state it needs re-fetches
	// it from peers instead of stranding (the ErrPrunedAncestor / "side chain
	// insertion" restart wedge). The four summary ops carry the frontier round — a
	// behind node discovers a committed state summary and its α-weight; the three App
	// ops are the EVM's own request/response transport for trie-leaf and code fetches,
	// which the state-sync client rides to download the state under that summary.
	// Appended ABOVE the pre-existing ops, so every wire value below is unchanged.
	GetStateSummaryFrontier
	StateSummaryFrontier
	GetAcceptedStateSummary
	AcceptedStateSummary
	AppRequest
	AppResponse
	AppError

	// NumOps is the count of router ops and the single source of truth for the
	// op-space size. It is not a wire op — it is one past the last op. Node-side
	// routing (node message.ToConsensusOp) is asserted to be a bijection onto
	// [0, NumOps), so an op added here without a matching node mapping fails the
	// alignment test instead of being silently dropped by the chain router — the
	// gap that wedged α-of-K finality when Gossip had no node mapping.
	NumOps
)

// Field represents a message field
type Field byte

// InboundHandler handles inbound messages
type InboundHandler interface {
	HandleInbound(context.Context, Message) error
}

// ExternalHandler handles messages from external chains
type ExternalHandler interface {
	Connected(nodeID ids.NodeID, nodeVersion interface{}, chainID ids.ID)
	Disconnected(nodeID ids.NodeID)
	HandleInbound(context.Context, Message) error
}

// HealthConfig configures health checks
type HealthConfig struct {
	Enabled              bool
	Interval             time.Duration
	Timeout              time.Duration
	MaxOutstandingChecks int
}
