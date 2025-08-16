// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package enginetest

import (
	"context"
	"errors"
	"sync"

	"github.com/luxfi/consensus"
	"github.com/luxfi/ids"
)

var (
	ErrNotImplemented = errors.New("not implemented")
)

// Engine is a test implementation of consensus.Engine
type Engine struct {
	T interface {
		Fatalf(format string, args ...interface{})
		Helper()
	}

	CantStart,
	CantIsBootstrapped,
	CantTimeout,
	CantGossip,
	CantHalt,
	CantShutdown,

	CantContext,

	CantNotify,

	CantGetBlock,
	CantGetFailed,
	CantPutBlock,
	CantPushQuery,
	CantPullQuery,
	CantQueryFailed,
	CantChits,

	CantConnected,
	CantDisconnected,
	CantHealthCheck,
	CantGetVM,
	CantGetValidatorSet,
	CantGetLastAccepted bool

	StartF           func(ctx context.Context, startReqID uint32) error
	IsBootstrappedF  func() (bool, error)
	TimeoutF         func(ctx context.Context) error
	GossipF          func(ctx context.Context) error
	HaltF            func(ctx context.Context)
	ShutdownF        func(ctx context.Context) error
	ContextF         func() context.Context
	NotifyF          func(context.Context, consensus.Message) error
	GetBlockF        func(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID) error
	GetFailedF       func(ctx context.Context, nodeID ids.NodeID, requestID uint32) error
	PutBlockF        func(ctx context.Context, nodeID ids.NodeID, requestID uint32, block []byte) error
	PushQueryF       func(ctx context.Context, nodeID ids.NodeID, requestID uint32, block []byte, requestedHeight uint64) error
	PullQueryF       func(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID, requestedHeight uint64) error
	QueryFailedF     func(ctx context.Context, nodeID ids.NodeID, requestID uint32) error
	ChitsF           func(ctx context.Context, nodeID ids.NodeID, requestID uint32, preferredID ids.ID, preferredIDAtHeight ids.ID, acceptedID ids.ID) error
	ConnectedF       func(ctx context.Context, nodeID ids.NodeID, nodeVersion *version) error
	DisconnectedF    func(ctx context.Context, nodeID ids.NodeID) error
	HealthCheckF     func(ctx context.Context) (interface{}, error)
	GetVMF           func() consensus.VM
	GetValidatorSetF func() func() (map[ids.NodeID]uint64, error)
	LastAcceptedF    func(ctx context.Context) (ids.ID, error)
}

// version struct for testing
type version struct {
	Major int
	Minor int
	Patch int
}

// Default sets the default callable value to [cant]
func (e *Engine) Default(cant bool) {
	e.CantStart = cant
	e.CantIsBootstrapped = cant
	e.CantTimeout = cant
	e.CantGossip = cant
	e.CantHalt = cant
	e.CantShutdown = cant
	e.CantContext = cant
	e.CantNotify = cant
	e.CantGetBlock = cant
	e.CantGetFailed = cant
	e.CantPutBlock = cant
	e.CantPushQuery = cant
	e.CantPullQuery = cant
	e.CantQueryFailed = cant
	e.CantChits = cant
	e.CantConnected = cant
	e.CantDisconnected = cant
	e.CantHealthCheck = cant
	e.CantGetVM = cant
	e.CantGetValidatorSet = cant
	e.CantGetLastAccepted = cant
}

func (e *Engine) Start(ctx context.Context, startReqID uint32) error {
	if e.StartF != nil {
		return e.StartF(ctx, startReqID)
	}
	if e.CantStart && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Start")
	}
	return ErrNotImplemented
}

func (e *Engine) IsBootstrapped() (bool, error) {
	if e.IsBootstrappedF != nil {
		return e.IsBootstrappedF()
	}
	if e.CantIsBootstrapped && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called IsBootstrapped")
	}
	return false, ErrNotImplemented
}

func (e *Engine) Timeout(ctx context.Context) error {
	if e.TimeoutF != nil {
		return e.TimeoutF(ctx)
	}
	if e.CantTimeout && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Timeout")
	}
	return ErrNotImplemented
}

func (e *Engine) Gossip(ctx context.Context) error {
	if e.GossipF != nil {
		return e.GossipF(ctx)
	}
	if e.CantGossip && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Gossip")
	}
	return ErrNotImplemented
}

func (e *Engine) Halt(ctx context.Context) {
	if e.HaltF != nil {
		e.HaltF(ctx)
		return
	}
	if e.CantHalt && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Halt")
	}
}

func (e *Engine) Shutdown(ctx context.Context) error {
	if e.ShutdownF != nil {
		return e.ShutdownF(ctx)
	}
	if e.CantShutdown && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Shutdown")
	}
	return ErrNotImplemented
}

func (e *Engine) Context() context.Context {
	if e.ContextF != nil {
		return e.ContextF()
	}
	if e.CantContext && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Context")
	}
	return nil
}

func (e *Engine) Notify(ctx context.Context, msg consensus.Message) error {
	if e.NotifyF != nil {
		return e.NotifyF(ctx, msg)
	}
	if e.CantNotify && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Notify")
	}
	return ErrNotImplemented
}

func (e *Engine) GetBlock(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID) error {
	if e.GetBlockF != nil {
		return e.GetBlockF(ctx, nodeID, requestID, blockID)
	}
	if e.CantGetBlock && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called GetBlock")
	}
	return ErrNotImplemented
}

func (e *Engine) GetFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error {
	if e.GetFailedF != nil {
		return e.GetFailedF(ctx, nodeID, requestID)
	}
	if e.CantGetFailed && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called GetFailed")
	}
	return ErrNotImplemented
}

func (e *Engine) PutBlock(ctx context.Context, nodeID ids.NodeID, requestID uint32, block []byte) error {
	if e.PutBlockF != nil {
		return e.PutBlockF(ctx, nodeID, requestID, block)
	}
	if e.CantPutBlock && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called PutBlock")
	}
	return ErrNotImplemented
}

func (e *Engine) PushQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, block []byte, requestedHeight uint64) error {
	if e.PushQueryF != nil {
		return e.PushQueryF(ctx, nodeID, requestID, block, requestedHeight)
	}
	if e.CantPushQuery && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called PushQuery")
	}
	return ErrNotImplemented
}

func (e *Engine) PullQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID, requestedHeight uint64) error {
	if e.PullQueryF != nil {
		return e.PullQueryF(ctx, nodeID, requestID, blockID, requestedHeight)
	}
	if e.CantPullQuery && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called PullQuery")
	}
	return ErrNotImplemented
}

func (e *Engine) QueryFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error {
	if e.QueryFailedF != nil {
		return e.QueryFailedF(ctx, nodeID, requestID)
	}
	if e.CantQueryFailed && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called QueryFailed")
	}
	return ErrNotImplemented
}

func (e *Engine) Chits(ctx context.Context, nodeID ids.NodeID, requestID uint32, preferredID ids.ID, preferredIDAtHeight ids.ID, acceptedID ids.ID) error {
	if e.ChitsF != nil {
		return e.ChitsF(ctx, nodeID, requestID, preferredID, preferredIDAtHeight, acceptedID)
	}
	if e.CantChits && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Chits")
	}
	return ErrNotImplemented
}

func (e *Engine) Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion *version) error {
	if e.ConnectedF != nil {
		return e.ConnectedF(ctx, nodeID, nodeVersion)
	}
	if e.CantConnected && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Connected")
	}
	return ErrNotImplemented
}

func (e *Engine) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
	if e.DisconnectedF != nil {
		return e.DisconnectedF(ctx, nodeID)
	}
	if e.CantDisconnected && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called Disconnected")
	}
	return ErrNotImplemented
}

func (e *Engine) HealthCheck(ctx context.Context) (interface{}, error) {
	if e.HealthCheckF != nil {
		return e.HealthCheckF(ctx)
	}
	if e.CantHealthCheck && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called HealthCheck")
	}
	return nil, ErrNotImplemented
}

func (e *Engine) GetVM() consensus.VM {
	if e.GetVMF != nil {
		return e.GetVMF()
	}
	if e.CantGetVM && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called GetVM")
	}
	return nil
}

func (e *Engine) GetValidatorSet() func() (map[ids.NodeID]uint64, error) {
	if e.GetValidatorSetF != nil {
		return e.GetValidatorSetF()
	}
	if e.CantGetValidatorSet && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called GetValidatorSet")
	}
	return nil
}

func (e *Engine) LastAccepted(ctx context.Context) (ids.ID, error) {
	if e.LastAcceptedF != nil {
		return e.LastAcceptedF(ctx)
	}
	if e.CantGetLastAccepted && e.T != nil {
		e.T.Helper()
		e.T.Fatalf("unexpectedly called LastAccepted")
	}
	return ids.Empty, ErrNotImplemented
}

// Sender is a test implementation of consensus.Sender
type Sender struct {
	T interface {
		Fatalf(format string, args ...interface{})
		Helper()
	}

	CantSendGetAncestors,
	CantSendGet,
	CantSendPut,
	CantSendPushQuery,
	CantSendPullQuery,
	CantSendChits,
	CantSendGossip,
	CantSendGetAcceptedFrontier,
	CantSendAcceptedFrontier,
	CantSendGetAccepted,
	CantSendAccepted,
	CantSendGetAcceptedStateSummary,
	CantSendAcceptedStateSummary,
	CantSendGetStateSummaryFrontier,
	CantSendStateSummaryFrontier,
	CantSendAppRequest,
	CantSendAppResponse,
	CantSendAppGossip,
	CantSendCrossChainAppRequest,
	CantSendCrossChainAppResponse bool

	SendGetAncestorsF            func(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	SendGetF                     func(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	SendPutF                     func(ctx context.Context, nodeID ids.NodeID, requestID uint32, container []byte) error
	SendPushQueryF               func(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, container []byte, requestedHeight uint64) error
	SendPullQueryF               func(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, containerID ids.ID, requestedHeight uint64) error
	SendChitsF                   func(ctx context.Context, nodeID ids.NodeID, requestID uint32, preferredID ids.ID, preferredIDAtHeight ids.ID, acceptedID ids.ID) error
	SendGossipF                  func(ctx context.Context, container []byte) error
	SendGetAcceptedFrontierF     func(ctx context.Context, nodeID ids.NodeID, requestID uint32) error
	SendAcceptedFrontierF        func(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	SendGetAcceptedF             func(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID) error
	SendAcceptedF                func(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerIDs []ids.ID) error
	SendGetAcceptedStateSummaryF func(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, heights []uint64) error
	SendAcceptedStateSummaryF    func(ctx context.Context, nodeID ids.NodeID, requestID uint32, summaryIDs []ids.ID) error
	SendGetStateSummaryFrontierF func(ctx context.Context, nodeID ids.NodeID, requestID uint32) error
	SendStateSummaryFrontierF    func(ctx context.Context, nodeID ids.NodeID, requestID uint32, summary []byte) error
	SendAppRequestF              func(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, appRequestBytes []byte) error
	SendAppResponseF             func(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
	SendAppGossipF               func(ctx context.Context, nodeIDs []ids.NodeID, appGossipBytes []byte) error
	SendCrossChainAppRequestF    func(ctx context.Context, chainID ids.ID, requestID uint32, appRequestBytes []byte) error
	SendCrossChainAppResponseF   func(ctx context.Context, chainID ids.ID, requestID uint32, appResponseBytes []byte) error

	sentLock sync.Mutex
	sent     [][]byte
}

// Default sets the default callable value to [cant]
func (s *Sender) Default(cant bool) {
	s.CantSendGetAncestors = cant
	s.CantSendGet = cant
	s.CantSendPut = cant
	s.CantSendPushQuery = cant
	s.CantSendPullQuery = cant
	s.CantSendChits = cant
	s.CantSendGossip = cant
	s.CantSendGetAcceptedFrontier = cant
	s.CantSendAcceptedFrontier = cant
	s.CantSendGetAccepted = cant
	s.CantSendAccepted = cant
	s.CantSendGetAcceptedStateSummary = cant
	s.CantSendAcceptedStateSummary = cant
	s.CantSendGetStateSummaryFrontier = cant
	s.CantSendStateSummaryFrontier = cant
	s.CantSendAppRequest = cant
	s.CantSendAppResponse = cant
	s.CantSendAppGossip = cant
	s.CantSendCrossChainAppRequest = cant
	s.CantSendCrossChainAppResponse = cant
}

func (s *Sender) SendGetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error {
	if s.SendGetAncestorsF != nil {
		return s.SendGetAncestorsF(ctx, nodeID, requestID, containerID)
	}
	if s.CantSendGetAncestors && s.T != nil {
		s.T.Helper()
		s.T.Fatalf("unexpectedly called SendGetAncestors")
	}
	return nil
}

func (s *Sender) SendGet(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error {
	if s.SendGetF != nil {
		return s.SendGetF(ctx, nodeID, requestID, containerID)
	}
	if s.CantSendGet && s.T != nil {
		s.T.Helper()
		s.T.Fatalf("unexpectedly called SendGet")
	}
	return nil
}

func (s *Sender) SendPut(ctx context.Context, nodeID ids.NodeID, requestID uint32, container []byte) error {
	if s.SendPutF != nil {
		return s.SendPutF(ctx, nodeID, requestID, container)
	}
	if s.CantSendPut && s.T != nil {
		s.T.Helper()
		s.T.Fatalf("unexpectedly called SendPut")
	}
	s.sentLock.Lock()
	s.sent = append(s.sent, container)
	s.sentLock.Unlock()
	return nil
}

func (s *Sender) SendPushQuery(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, container []byte, requestedHeight uint64) error {
	if s.SendPushQueryF != nil {
		return s.SendPushQueryF(ctx, nodeIDs, requestID, container, requestedHeight)
	}
	if s.CantSendPushQuery && s.T != nil {
		s.T.Helper()
		s.T.Fatalf("unexpectedly called SendPushQuery")
	}
	s.sentLock.Lock()
	s.sent = append(s.sent, container)
	s.sentLock.Unlock()
	return nil
}

func (s *Sender) SendPullQuery(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, containerID ids.ID, requestedHeight uint64) error {
	if s.SendPullQueryF != nil {
		return s.SendPullQueryF(ctx, nodeIDs, requestID, containerID, requestedHeight)
	}
	if s.CantSendPullQuery && s.T != nil {
		s.T.Helper()
		s.T.Fatalf("unexpectedly called SendPullQuery")
	}
	return nil
}

func (s *Sender) SendChits(ctx context.Context, nodeID ids.NodeID, requestID uint32, preferredID ids.ID, preferredIDAtHeight ids.ID, acceptedID ids.ID) error {
	if s.SendChitsF != nil {
		return s.SendChitsF(ctx, nodeID, requestID, preferredID, preferredIDAtHeight, acceptedID)
	}
	if s.CantSendChits && s.T != nil {
		s.T.Helper()
		s.T.Fatalf("unexpectedly called SendChits")
	}
	return nil
}

func (s *Sender) SendGossip(ctx context.Context, container []byte) error {
	if s.SendGossipF != nil {
		return s.SendGossipF(ctx, container)
	}
	if s.CantSendGossip && s.T != nil {
		s.T.Helper()
		s.T.Fatalf("unexpectedly called SendGossip")
	}
	s.sentLock.Lock()
	s.sent = append(s.sent, container)
	s.sentLock.Unlock()
	return nil
}

// GetSent returns all containers that were sent
func (s *Sender) GetSent() [][]byte {
	s.sentLock.Lock()
	defer s.sentLock.Unlock()
	return s.sent
}

// ClearSent clears the sent containers
func (s *Sender) ClearSent() {
	s.sentLock.Lock()
	defer s.sentLock.Unlock()
	s.sent = nil
}
