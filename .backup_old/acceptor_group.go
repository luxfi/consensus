// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

type acceptorGroup struct {
	lock      sync.RWMutex
	log       log.Logger
	acceptors map[ids.ID]map[string]Acceptor
}

// NewAcceptorGroup creates a new acceptor group
func NewAcceptorGroup(log log.Logger) AcceptorGroup {
	return &acceptorGroup{
		log:       log,
		acceptors: make(map[ids.ID]map[string]Acceptor),
	}
}

func (a *acceptorGroup) RegisterAcceptor(chainID ids.ID, name string, acceptor Acceptor, dieOnError bool) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	chainAcceptors, exists := a.acceptors[chainID]
	if !exists {
		chainAcceptors = make(map[string]Acceptor)
		a.acceptors[chainID] = chainAcceptors
	}
	
	chainAcceptors[name] = acceptor
	return nil
}

func (a *acceptorGroup) DeregisterAcceptor(chainID ids.ID, name string) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	chainAcceptors, exists := a.acceptors[chainID]
	if !exists {
		return nil
	}
	
	delete(chainAcceptors, name)
	if len(chainAcceptors) == 0 {
		delete(a.acceptors, chainID)
	}
	return nil
}

func (a *acceptorGroup) Accept(chainID ids.ID, containerID ids.ID, container []byte) error {
	a.lock.RLock()
	chainAcceptors := a.acceptors[chainID]
	// Make a copy of acceptors to avoid holding lock while calling them
	acceptorsCopy := make(map[string]Acceptor, len(chainAcceptors))
	for name, acceptor := range chainAcceptors {
		acceptorsCopy[name] = acceptor
	}
	a.lock.RUnlock()

	// Call acceptors without holding lock
	for name, acceptor := range acceptorsCopy {
		if err := acceptor.Accept(context.Background(), containerID, container); err != nil {
			a.log.Error("acceptor failed",
				zap.String("acceptor", name),
				zap.Stringer("chainID", chainID),
				zap.Stringer("containerID", containerID),
				zap.Error(err),
			)
		}
	}
	return nil
}