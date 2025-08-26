// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// Manager defines validator manager operations
type Manager interface {
	GetWeight(netID ids.ID, nodeID ids.NodeID) uint64
	GetLight(netID ids.ID, nodeID ids.NodeID) uint64
	TotalWeight(netID ids.ID) (uint64, error)
	TotalLight(netID ids.ID) (uint64, error)
	GetValidator(netID ids.ID, nodeID ids.NodeID) (*Validator, bool)
	SubsetWeight(netID ids.ID, nodeIDs set.Set[ids.NodeID]) (uint64, error)
	AddStaker(netID ids.ID, nodeID ids.NodeID, publicKey []byte, txID ids.ID, weight uint64) error
	RegisterSetCallbackListener(netID ids.ID, listener SetCallbackListener)
}

// SetCallbackListener is notified when validators change
type SetCallbackListener interface {
	OnValidatorAdded(netID ids.ID, nodeID ids.NodeID, pk []byte, txID ids.ID, weight uint64)
	OnValidatorRemoved(netID ids.ID, nodeID ids.NodeID, weight uint64)
	OnValidatorWeightChanged(netID ids.ID, nodeID ids.NodeID, oldWeight, newWeight uint64)
	OnValidatorLightChanged(netID ids.ID, nodeID ids.NodeID, oldLight, newLight uint64)
}

// NewManager creates a new validator manager
func NewManager() Manager {
	return &manager{
		validators: make(map[ids.ID]map[ids.NodeID]*ValidatorImpl),
	}
}

// manager implements Manager interface
type manager struct {
	validators map[ids.ID]map[ids.NodeID]*ValidatorImpl
	listeners  map[ids.ID][]SetCallbackListener
}

func (m *manager) GetWeight(netID ids.ID, nodeID ids.NodeID) uint64 {
	if netValidators, ok := m.validators[netID]; ok {
		if v, ok := netValidators[nodeID]; ok {
			return v.Light()
		}
	}
	return 0
}

func (m *manager) GetLight(netID ids.ID, nodeID ids.NodeID) uint64 {
	return m.GetWeight(netID, nodeID)
}

func (m *manager) TotalWeight(netID ids.ID) (uint64, error) {
	var total uint64
	if netValidators, ok := m.validators[netID]; ok {
		for _, v := range netValidators {
			total += v.Light()
		}
	}
	return total, nil
}

func (m *manager) TotalLight(netID ids.ID) (uint64, error) {
	return m.TotalWeight(netID)
}

func (m *manager) GetValidator(netID ids.ID, nodeID ids.NodeID) (*Validator, bool) {
	if netValidators, ok := m.validators[netID]; ok {
		if v, ok := netValidators[nodeID]; ok {
			val := Validator(v)
			return &val, true
		}
	}
	return nil, false
}

func (m *manager) SubsetWeight(netID ids.ID, nodeIDs set.Set[ids.NodeID]) (uint64, error) {
	var total uint64
	if netValidators, ok := m.validators[netID]; ok {
		for nodeID := range nodeIDs {
			if v, ok := netValidators[nodeID]; ok {
				total += v.Light()
			}
		}
	}
	return total, nil
}

func (m *manager) AddStaker(netID ids.ID, nodeID ids.NodeID, publicKey []byte, txID ids.ID, weight uint64) error {
	if m.validators[netID] == nil {
		m.validators[netID] = make(map[ids.NodeID]*ValidatorImpl)
	}
	m.validators[netID][nodeID] = &ValidatorImpl{
		NodeID:   nodeID,
		LightVal: weight,
	}
	
	// Notify listeners
	if listeners, ok := m.listeners[netID]; ok {
		for _, listener := range listeners {
			listener.OnValidatorAdded(netID, nodeID, publicKey, txID, weight)
		}
	}
	return nil
}

func (m *manager) RegisterSetCallbackListener(netID ids.ID, listener SetCallbackListener) {
	if m.listeners == nil {
		m.listeners = make(map[ids.ID][]SetCallbackListener)
	}
	m.listeners[netID] = append(m.listeners[netID], listener)
}