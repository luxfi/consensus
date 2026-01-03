// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	"github.com/stretchr/testify/require"
)

// TestNewManager tests the NewManager constructor
func TestNewManager(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	require.NotNil(m)
	require.NotNil(m.validators)
	require.NotNil(m.mu)
	require.NotNil(m.listeners)
	require.Equal(0, m.NumNets())
}

// TestManagerAddStaker tests adding validators
func TestManagerAddStaker(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()
	pubKey := []byte("test-public-key")
	txID := ids.GenerateTestID()
	light := uint64(1000)

	// Add a staker
	err := m.AddStaker(netID, nodeID, pubKey, txID, light)
	require.NoError(err)

	// Verify staker was added
	val, ok := m.GetValidator(netID, nodeID)
	require.True(ok)
	require.Equal(nodeID, val.NodeID)
	require.Equal(pubKey, val.PublicKey)
	require.Equal(light, val.Light)
	require.Equal(light, val.Weight)
	require.Equal(txID, val.TxID)

	// Add second staker to same network
	nodeID2 := ids.GenerateTestNodeID()
	err = m.AddStaker(netID, nodeID2, []byte("key2"), ids.GenerateTestID(), 500)
	require.NoError(err)
	require.Equal(2, m.Count(netID))

	// Add staker to different network
	netID2 := ids.GenerateTestID()
	err = m.AddStaker(netID2, ids.GenerateTestNodeID(), nil, ids.GenerateTestID(), 200)
	require.NoError(err)
	require.Equal(2, m.NumNets())
}

// TestManagerAddStakerWithListener tests that listeners are notified
func TestManagerAddStakerWithListener(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	listener := &testListener{}
	m.RegisterCallbackListener(listener)

	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()
	light := uint64(1000)

	err := m.AddStaker(netID, nodeID, nil, ids.Empty, light)
	require.NoError(err)

	require.Len(listener.added, 1)
	require.Equal(netID, listener.added[0].netID)
	require.Equal(nodeID, listener.added[0].nodeID)
	require.Equal(light, listener.added[0].light)
}

// TestManagerAddWeight tests adding weight to existing validators
func TestManagerAddWeight(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	// Add staker first
	err := m.AddStaker(netID, nodeID, nil, ids.Empty, 1000)
	require.NoError(err)

	// Add weight
	err = m.AddWeight(netID, nodeID, 500)
	require.NoError(err)

	val, ok := m.GetValidator(netID, nodeID)
	require.True(ok)
	require.Equal(uint64(1500), val.Light)
	require.Equal(uint64(1500), val.Weight)
}

// TestManagerAddWeightNonExistent tests adding weight to non-existent validator
func TestManagerAddWeightNonExistent(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	// Adding weight to non-existent validator should not error
	err := m.AddWeight(netID, nodeID, 500)
	require.NoError(err)

	// Also test when netID exists but nodeID doesn't
	err = m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 1000)
	require.NoError(err)

	err = m.AddWeight(netID, nodeID, 500)
	require.NoError(err)
}

// TestManagerRemoveWeight tests removing weight from validators
func TestManagerRemoveWeight(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	// Add staker first
	err := m.AddStaker(netID, nodeID, nil, ids.Empty, 1000)
	require.NoError(err)

	// Remove partial weight
	err = m.RemoveWeight(netID, nodeID, 300)
	require.NoError(err)

	val, ok := m.GetValidator(netID, nodeID)
	require.True(ok)
	require.Equal(uint64(700), val.Light)
}

// TestManagerRemoveWeightUnderflow tests removing more weight than available
func TestManagerRemoveWeightUnderflow(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	err := m.AddStaker(netID, nodeID, nil, ids.Empty, 100)
	require.NoError(err)

	// Remove more than available - should set to 0 and remove validator
	err = m.RemoveWeight(netID, nodeID, 500)
	require.NoError(err)

	_, ok := m.GetValidator(netID, nodeID)
	require.False(ok)

	// Network should also be removed since no validators
	require.Equal(0, m.NumNets())
}

// TestManagerRemoveWeightToZero tests removing exact weight removes validator
func TestManagerRemoveWeightToZero(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	err := m.AddStaker(netID, nodeID, nil, ids.Empty, 100)
	require.NoError(err)

	// Remove exact amount
	err = m.RemoveWeight(netID, nodeID, 100)
	require.NoError(err)

	_, ok := m.GetValidator(netID, nodeID)
	require.False(ok)
}

// TestManagerRemoveWeightNonExistent tests removing weight from non-existent
func TestManagerRemoveWeightNonExistent(t *testing.T) {
	require := require.New(t)

	m := NewManager()

	// Non-existent netID
	err := m.RemoveWeight(ids.GenerateTestID(), ids.GenerateTestNodeID(), 100)
	require.NoError(err)

	// Existing netID, non-existent nodeID
	netID := ids.GenerateTestID()
	err = m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 100)
	require.NoError(err)

	err = m.RemoveWeight(netID, ids.GenerateTestNodeID(), 100)
	require.NoError(err)
}

// TestManagerRemoveWeightKeepsOtherValidators tests other validators remain
func TestManagerRemoveWeightKeepsOtherValidators(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()

	err := m.AddStaker(netID, nodeID1, nil, ids.Empty, 100)
	require.NoError(err)
	err = m.AddStaker(netID, nodeID2, nil, ids.Empty, 200)
	require.NoError(err)

	// Remove first validator entirely
	err = m.RemoveWeight(netID, nodeID1, 100)
	require.NoError(err)

	// Second should still exist
	_, ok := m.GetValidator(netID, nodeID2)
	require.True(ok)
	require.Equal(1, m.Count(netID))
	require.Equal(1, m.NumNets())
}

// TestManagerNumNets tests network counting
func TestManagerNumNets(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	require.Equal(0, m.NumNets())

	// Add to first network
	err := m.AddStaker(ids.GenerateTestID(), ids.GenerateTestNodeID(), nil, ids.Empty, 100)
	require.NoError(err)
	require.Equal(1, m.NumNets())

	// Add to second network
	err = m.AddStaker(ids.GenerateTestID(), ids.GenerateTestNodeID(), nil, ids.Empty, 100)
	require.NoError(err)
	require.Equal(2, m.NumNets())
}

// TestManagerGetValidators tests getting validator set
func TestManagerGetValidators(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	// Empty network returns empty set
	set, err := m.GetValidators(netID)
	require.NoError(err)
	require.NotNil(set)
	require.Equal(0, set.Len())

	// Add validators
	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()
	err = m.AddStaker(netID, nodeID1, nil, ids.Empty, 100)
	require.NoError(err)
	err = m.AddStaker(netID, nodeID2, nil, ids.Empty, 200)
	require.NoError(err)

	set, err = m.GetValidators(netID)
	require.NoError(err)
	require.Equal(2, set.Len())
	require.True(set.Has(nodeID1))
	require.True(set.Has(nodeID2))
}

// TestManagerGetValidator tests getting single validator
func TestManagerGetValidator(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	// Non-existent
	val, ok := m.GetValidator(netID, nodeID)
	require.False(ok)
	require.Nil(val)

	// Add and retrieve
	err := m.AddStaker(netID, nodeID, []byte("key"), ids.GenerateTestID(), 100)
	require.NoError(err)

	val, ok = m.GetValidator(netID, nodeID)
	require.True(ok)
	require.NotNil(val)
	require.Equal(nodeID, val.NodeID)
}

// TestManagerGetLight tests light retrieval
func TestManagerGetLight(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	// Non-existent returns 0
	light := m.GetLight(netID, nodeID)
	require.Equal(uint64(0), light)

	// Add and retrieve
	err := m.AddStaker(netID, nodeID, nil, ids.Empty, 500)
	require.NoError(err)

	light = m.GetLight(netID, nodeID)
	require.Equal(uint64(500), light)
}

// TestManagerGetWeight tests deprecated weight retrieval
func TestManagerGetWeight(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	err := m.AddStaker(netID, nodeID, nil, ids.Empty, 500)
	require.NoError(err)

	// GetWeight should equal GetLight
	require.Equal(m.GetLight(netID, nodeID), m.GetWeight(netID, nodeID))
}

// TestManagerTotalLight tests total light calculation
func TestManagerTotalLight(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	// Empty returns 0
	total, err := m.TotalLight(netID)
	require.NoError(err)
	require.Equal(uint64(0), total)

	// Add validators
	err = m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 100)
	require.NoError(err)
	err = m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 200)
	require.NoError(err)
	err = m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 300)
	require.NoError(err)

	total, err = m.TotalLight(netID)
	require.NoError(err)
	require.Equal(uint64(600), total)
}

// TestManagerTotalWeight tests deprecated total weight
func TestManagerTotalWeight(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	err := m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 100)
	require.NoError(err)

	totalLight, err := m.TotalLight(netID)
	require.NoError(err)
	totalWeight, err := m.TotalWeight(netID)
	require.NoError(err)
	require.Equal(totalLight, totalWeight)
}

// TestManagerCount tests validator counting
func TestManagerCount(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	// Empty
	require.Equal(0, m.Count(netID))

	// Add validators
	for i := 0; i < 5; i++ {
		err := m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 100)
		require.NoError(err)
	}

	require.Equal(5, m.Count(netID))
}

// TestManagerNumValidators tests alias for Count
func TestManagerNumValidators(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	err := m.AddStaker(netID, ids.GenerateTestNodeID(), nil, ids.Empty, 100)
	require.NoError(err)

	require.Equal(m.Count(netID), m.NumValidators(netID))
}

// TestManagerSample tests sampling validators
func TestManagerSample(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	// Empty sample
	sample, err := m.Sample(netID, 5)
	require.NoError(err)
	require.Empty(sample)

	// Add validators
	nodeIDs := make([]ids.NodeID, 10)
	for i := range nodeIDs {
		nodeIDs[i] = ids.GenerateTestNodeID()
		err := m.AddStaker(netID, nodeIDs[i], nil, ids.Empty, 100)
		require.NoError(err)
	}

	// Sample subset
	sample, err = m.Sample(netID, 5)
	require.NoError(err)
	require.Len(sample, 5)

	// Sample all
	sample, err = m.Sample(netID, 10)
	require.NoError(err)
	require.Len(sample, 10)

	// Sample more than available
	sample, err = m.Sample(netID, 20)
	require.NoError(err)
	require.Len(sample, 10)
}

// TestManagerGetValidatorIDs tests getting all validator IDs
func TestManagerGetValidatorIDs(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	// Empty
	nodeIDs := m.GetValidatorIDs(netID)
	require.Nil(nodeIDs)

	// Add validators
	expected := make(map[ids.NodeID]bool)
	for i := 0; i < 5; i++ {
		nodeID := ids.GenerateTestNodeID()
		expected[nodeID] = true
		err := m.AddStaker(netID, nodeID, nil, ids.Empty, 100)
		require.NoError(err)
	}

	nodeIDs = m.GetValidatorIDs(netID)
	require.Len(nodeIDs, 5)
	for _, nodeID := range nodeIDs {
		require.True(expected[nodeID])
	}
}

// TestManagerSubsetWeight tests calculating subset weight
func TestManagerSubsetWeight(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	// Add validators with known weights
	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()
	nodeID3 := ids.GenerateTestNodeID()

	err := m.AddStaker(netID, nodeID1, nil, ids.Empty, 100)
	require.NoError(err)
	err = m.AddStaker(netID, nodeID2, nil, ids.Empty, 200)
	require.NoError(err)
	err = m.AddStaker(netID, nodeID3, nil, ids.Empty, 300)
	require.NoError(err)

	// Subset with nodes 1 and 3
	subset := set.Set[ids.NodeID]{}
	subset.Add(nodeID1)
	subset.Add(nodeID3)

	weight, err := m.SubsetWeight(netID, subset)
	require.NoError(err)
	require.Equal(uint64(400), weight)

	// Empty subset
	emptySubset := set.Set[ids.NodeID]{}
	weight, err = m.SubsetWeight(netID, emptySubset)
	require.NoError(err)
	require.Equal(uint64(0), weight)

	// Non-existent network
	weight, err = m.SubsetWeight(ids.GenerateTestID(), subset)
	require.NoError(err)
	require.Equal(uint64(0), weight)
}

// TestManagerGetMap tests getting validator map copy
func TestManagerGetMap(t *testing.T) {
	require := require.New(t)

	m := NewManager()
	netID := ids.GenerateTestID()

	// Empty
	vmap := m.GetMap(netID)
	require.NotNil(vmap)
	require.Empty(vmap)

	// Add validators
	nodeID := ids.GenerateTestNodeID()
	err := m.AddStaker(netID, nodeID, []byte("key"), ids.GenerateTestID(), 100)
	require.NoError(err)

	vmap = m.GetMap(netID)
	require.Len(vmap, 1)
	require.NotNil(vmap[nodeID])

	// Verify it's a copy by modifying
	delete(vmap, nodeID)
	vmap2 := m.GetMap(netID)
	require.Len(vmap2, 1)
}

// TestManagerRegisterCallbackListener tests callback registration
func TestManagerRegisterCallbackListener(t *testing.T) {
	require := require.New(t)

	m := NewManager()

	// Add validators before listener
	netID := ids.GenerateTestID()
	nodeID1 := ids.GenerateTestNodeID()
	err := m.AddStaker(netID, nodeID1, nil, ids.Empty, 100)
	require.NoError(err)

	// Register listener - should be notified of existing validators
	listener := &testListener{}
	m.RegisterCallbackListener(listener)

	require.Len(listener.added, 1)
	require.Equal(netID, listener.added[0].netID)
	require.Equal(nodeID1, listener.added[0].nodeID)

	// Add new validator - listener should be notified
	nodeID2 := ids.GenerateTestNodeID()
	err = m.AddStaker(netID, nodeID2, nil, ids.Empty, 200)
	require.NoError(err)

	require.Len(listener.added, 2)
}

// TestManagerRegisterSetCallbackListener tests set callback (no-op)
func TestManagerRegisterSetCallbackListener(t *testing.T) {
	m := NewManager()
	netID := ids.GenerateTestID()

	// This is a no-op but should not panic
	listener := &testSetListener{}
	m.RegisterSetCallbackListener(netID, listener)
}

// TestValidatorSetHas tests validatorSet.Has
func TestValidatorSetHas(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	set := &validatorSet{
		validators: map[ids.NodeID]*GetValidatorOutput{
			nodeID: {NodeID: nodeID, Light: 100},
		},
	}

	require.True(set.Has(nodeID))
	require.False(set.Has(ids.GenerateTestNodeID()))
}

// TestValidatorSetLen tests validatorSet.Len
func TestValidatorSetLen(t *testing.T) {
	require := require.New(t)

	set := &validatorSet{
		validators: map[ids.NodeID]*GetValidatorOutput{
			ids.GenerateTestNodeID(): {Light: 100},
			ids.GenerateTestNodeID(): {Light: 200},
		},
	}

	require.Equal(2, set.Len())
}

// TestValidatorSetList tests validatorSet.List
func TestValidatorSetList(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	set := &validatorSet{
		validators: map[ids.NodeID]*GetValidatorOutput{
			nodeID: {NodeID: nodeID, Light: 100},
		},
	}

	list := set.List()
	require.Len(list, 1)
	require.Equal(nodeID, list[0].ID())
	require.Equal(uint64(100), list[0].Light())
}

// TestValidatorSetLight tests validatorSet.Light
func TestValidatorSetLight(t *testing.T) {
	require := require.New(t)

	set := &validatorSet{
		validators: map[ids.NodeID]*GetValidatorOutput{
			ids.GenerateTestNodeID(): {Light: 100},
			ids.GenerateTestNodeID(): {Light: 200},
			ids.GenerateTestNodeID(): {Light: 300},
		},
	}

	require.Equal(uint64(600), set.Light())
}

// TestValidatorSetSample tests validatorSet.Sample
func TestValidatorSetSample(t *testing.T) {
	require := require.New(t)

	nodeIDs := make([]ids.NodeID, 5)
	validators := make(map[ids.NodeID]*GetValidatorOutput)
	for i := range nodeIDs {
		nodeIDs[i] = ids.GenerateTestNodeID()
		validators[nodeIDs[i]] = &GetValidatorOutput{NodeID: nodeIDs[i], Light: 100}
	}

	set := &validatorSet{validators: validators}

	// Sample subset
	sample, err := set.Sample(3)
	require.NoError(err)
	require.Len(sample, 3)

	// Sample all
	sample, err = set.Sample(5)
	require.NoError(err)
	require.Len(sample, 5)

	// Sample more than available
	sample, err = set.Sample(10)
	require.NoError(err)
	require.Len(sample, 5)
}

// TestEmptySetHas tests emptySet.Has
func TestEmptySetHas(t *testing.T) {
	require := require.New(t)

	set := &emptySet{}
	require.False(set.Has(ids.GenerateTestNodeID()))
}

// TestEmptySetLen tests emptySet.Len
func TestEmptySetLen(t *testing.T) {
	require := require.New(t)

	set := &emptySet{}
	require.Equal(0, set.Len())
}

// TestEmptySetList tests emptySet.List
func TestEmptySetList(t *testing.T) {
	require := require.New(t)

	set := &emptySet{}
	require.Nil(set.List())
}

// TestEmptySetLight tests emptySet.Light
func TestEmptySetLight(t *testing.T) {
	require := require.New(t)

	set := &emptySet{}
	require.Equal(uint64(0), set.Light())
}

// TestEmptySetSample tests emptySet.Sample
func TestEmptySetSample(t *testing.T) {
	require := require.New(t)

	set := &emptySet{}
	sample, err := set.Sample(10)
	require.NoError(err)
	require.Nil(sample)
}

// Test helpers

type validatorEvent struct {
	netID  ids.ID
	nodeID ids.NodeID
	light  uint64
}

type testListener struct {
	added   []validatorEvent
	removed []validatorEvent
}

func (l *testListener) OnValidatorAdded(netID ids.ID, nodeID ids.NodeID, light uint64) {
	l.added = append(l.added, validatorEvent{netID, nodeID, light})
}

func (l *testListener) OnValidatorRemoved(netID ids.ID, nodeID ids.NodeID, light uint64) {
	l.removed = append(l.removed, validatorEvent{netID, nodeID, light})
}

func (l *testListener) OnValidatorLightChanged(netID ids.ID, nodeID ids.NodeID, oldLight, newLight uint64) {
	// Not implemented in manager yet
}

type testSetListener struct{}

func (l *testSetListener) OnValidatorAdded(nodeID ids.NodeID, light uint64)                     {}
func (l *testSetListener) OnValidatorRemoved(nodeID ids.NodeID, light uint64)                   {}
func (l *testSetListener) OnValidatorLightChanged(nodeID ids.NodeID, oldLight, newLight uint64) {}
