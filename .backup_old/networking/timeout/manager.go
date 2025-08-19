// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timeout

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/timer"
)

// Manager manages request timeouts
type Manager interface {
	// RegisterRequest registers a new request
	RegisterRequest(nodeID ids.NodeID, chainID ids.ID, requestID uint32, timeout time.Duration)
	
	// RegisterResponse registers that we received a response
	RegisterResponse(nodeID ids.NodeID, chainID ids.ID, requestID uint32)
	
	// TimeoutRequest times out a request
	TimeoutRequest(nodeID ids.NodeID, chainID ids.ID, requestID uint32)
	
	// Stop stops the manager
	Stop()
	
	// Dispatch starts processing timeouts
	Dispatch()
}

type manager struct {
	lock         sync.Mutex
	requests     map[ids.ID]map[uint32]*request
	benchlist    interface{}
	timeoutConfig *timer.AdaptiveTimeoutConfig
	done         chan struct{}
	wg           sync.WaitGroup
}

type request struct {
	nodeID    ids.NodeID
	chainID   ids.ID
	requestID uint32
	deadline  time.Time
	timer     *time.Timer
}

// NewManager creates a new timeout manager
func NewManager(
	config *timer.AdaptiveTimeoutConfig,
	benchlist interface{},
	requestsReg prometheus.Registerer,
	responseReg prometheus.Registerer,
) (Manager, error) {
	return &manager{
		requests:      make(map[ids.ID]map[uint32]*request),
		benchlist:     benchlist,
		timeoutConfig: config,
		done:          make(chan struct{}),
	}, nil
}

func (m *manager) RegisterRequest(nodeID ids.NodeID, chainID ids.ID, requestID uint32, timeout time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()

	chainRequests, exists := m.requests[chainID]
	if !exists {
		chainRequests = make(map[uint32]*request)
		m.requests[chainID] = chainRequests
	}

	req := &request{
		nodeID:    nodeID,
		chainID:   chainID,
		requestID: requestID,
		deadline:  time.Now().Add(timeout),
	}
	
	req.timer = time.AfterFunc(timeout, func() {
		m.TimeoutRequest(nodeID, chainID, requestID)
	})
	
	chainRequests[requestID] = req
}

func (m *manager) RegisterResponse(nodeID ids.NodeID, chainID ids.ID, requestID uint32) {
	m.lock.Lock()
	defer m.lock.Unlock()

	chainRequests, exists := m.requests[chainID]
	if !exists {
		return
	}

	req, exists := chainRequests[requestID]
	if !exists {
		return
	}

	if req.timer != nil {
		req.timer.Stop()
	}
	delete(chainRequests, requestID)
	
	if len(chainRequests) == 0 {
		delete(m.requests, chainID)
	}
}

func (m *manager) TimeoutRequest(nodeID ids.NodeID, chainID ids.ID, requestID uint32) {
	m.lock.Lock()
	defer m.lock.Unlock()

	chainRequests, exists := m.requests[chainID]
	if !exists {
		return
	}

	delete(chainRequests, requestID)
	if len(chainRequests) == 0 {
		delete(m.requests, chainID)
	}
}

func (m *manager) Stop() {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Stop all timers
	for _, chainRequests := range m.requests {
		for _, req := range chainRequests {
			if req.timer != nil {
				req.timer.Stop()
			}
		}
	}
	
	close(m.done)
	m.wg.Wait()
}

func (m *manager) Dispatch() {
	m.wg.Add(1)
	defer m.wg.Done()
	
	<-m.done
}