package validators

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/version"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// Test ValidatorImpl
func TestValidatorImpl(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	light := uint64(1000)

	validator := &ValidatorImpl{
		NodeID:   nodeID,
		LightVal: light,
	}

	require.Equal(t, nodeID, validator.ID())
	require.Equal(t, light, validator.Light())
}

// Mock State implementation
type mockState struct {
	validators     map[ids.NodeID]*GetValidatorOutput
	currentHeight  uint64
	getValidatorErr error
	getHeightErr   error
}

func (m *mockState) GetValidatorSet(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error) {
	if m.getValidatorErr != nil {
		return nil, m.getValidatorErr
	}
	return m.validators, nil
}

func (m *mockState) GetCurrentValidators(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error) {
	if m.getValidatorErr != nil {
		return nil, m.getValidatorErr
	}
	return m.validators, nil
}

func (m *mockState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	if m.getHeightErr != nil {
		return 0, m.getHeightErr
	}
	return m.currentHeight, nil
}

// Mock Set implementation
type mockSet struct {
	validators map[ids.NodeID]Validator
	lightVal   uint64
	sampleErr  error
}

func (m *mockSet) Has(nodeID ids.NodeID) bool {
	_, exists := m.validators[nodeID]
	return exists
}

func (m *mockSet) Len() int {
	return len(m.validators)
}

func (m *mockSet) List() []Validator {
	list := make([]Validator, 0, len(m.validators))
	for _, v := range m.validators {
		list = append(list, v)
	}
	return list
}

func (m *mockSet) Light() uint64 {
	if m.lightVal > 0 {
		return m.lightVal
	}
	var total uint64
	for _, v := range m.validators {
		total += v.Light()
	}
	return total
}

func (m *mockSet) Sample(size int) ([]ids.NodeID, error) {
	if m.sampleErr != nil {
		return nil, m.sampleErr
	}
	if size > len(m.validators) {
		return nil, errors.New("sample size too large")
	}
	if size < 0 {
		return nil, errors.New("negative sample size")
	}

	result := make([]ids.NodeID, 0, size)
	for nodeID := range m.validators {
		if len(result) >= size {
			break
		}
		result = append(result, nodeID)
	}
	return result, nil
}

// Mock Manager implementation
type mockManager struct {
	sets       map[ids.ID]Set
	validators map[ids.ID]map[ids.NodeID]*GetValidatorOutput
	err        error
}

func (m *mockManager) GetValidators(netID ids.ID) (Set, error) {
	if m.err != nil {
		return nil, m.err
	}
	if set, ok := m.sets[netID]; ok {
		return set, nil
	}
	return nil, errors.New("set not found")
}

func (m *mockManager) GetValidator(netID ids.ID, nodeID ids.NodeID) (*GetValidatorOutput, bool) {
	if vals, ok := m.validators[netID]; ok {
		if val, ok := vals[nodeID]; ok {
			return val, true
		}
	}
	return nil, false
}

func (m *mockManager) GetLight(netID ids.ID, nodeID ids.NodeID) uint64 {
	if val, ok := m.GetValidator(netID, nodeID); ok {
		return val.Light
	}
	return 0
}

func (m *mockManager) GetWeight(netID ids.ID, nodeID ids.NodeID) uint64 {
	return m.GetLight(netID, nodeID)
}

func (m *mockManager) TotalLight(netID ids.ID) (uint64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if vals, ok := m.validators[netID]; ok {
		var total uint64
		for _, v := range vals {
			total += v.Light
		}
		return total, nil
	}
	return 0, errors.New("net not found")
}

func (m *mockManager) TotalWeight(netID ids.ID) (uint64, error) {
	return m.TotalLight(netID)
}

// Mock Connector implementation
type mockConnector struct {
	connectedNodes    map[ids.NodeID]*version.Application
	disconnectedNodes map[ids.NodeID]bool
	connectErr        error
	disconnectErr     error
}

func (m *mockConnector) Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion *version.Application) error {
	if m.connectErr != nil {
		return m.connectErr
	}
	if m.connectedNodes == nil {
		m.connectedNodes = make(map[ids.NodeID]*version.Application)
	}
	m.connectedNodes[nodeID] = nodeVersion
	return nil
}

func (m *mockConnector) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
	if m.disconnectErr != nil {
		return m.disconnectErr
	}
	if m.disconnectedNodes == nil {
		m.disconnectedNodes = make(map[ids.NodeID]bool)
	}
	m.disconnectedNodes[nodeID] = true
	delete(m.connectedNodes, nodeID)
	return nil
}

// Tests

func TestState(t *testing.T) {
	ctx := context.Background()

	t.Run("GetValidatorSet", func(t *testing.T) {
		state := &mockState{
			validators: map[ids.NodeID]*GetValidatorOutput{
				ids.GenerateTestNodeID(): {
					NodeID:    ids.GenerateTestNodeID(),
					PublicKey: []byte("key1"),
					Light:     100,
					Weight:    100,
					TxID:      ids.GenerateTestID(),
				},
			},
			currentHeight: 1000,
		}

		vals, err := state.GetValidatorSet(ctx, 500, ids.GenerateTestID())
		require.NoError(t, err)
		require.Len(t, vals, 1)

		// Test error case
		state.getValidatorErr = errors.New("get error")
		_, err = state.GetValidatorSet(ctx, 500, ids.GenerateTestID())
		require.Error(t, err)
	})

	t.Run("GetCurrentHeight", func(t *testing.T) {
		state := &mockState{
			currentHeight: 5000,
		}

		height, err := state.GetCurrentHeight(ctx)
		require.NoError(t, err)
		require.Equal(t, uint64(5000), height)

		// Test error case
		state.getHeightErr = errors.New("height error")
		_, err = state.GetCurrentHeight(ctx)
		require.Error(t, err)
	})
}

func TestSet(t *testing.T) {
	t.Run("Basic operations", func(t *testing.T) {
		set := &mockSet{
			validators: map[ids.NodeID]Validator{
				ids.GenerateTestNodeID(): &ValidatorImpl{
					NodeID:   ids.GenerateTestNodeID(),
					LightVal: 100,
				},
				ids.GenerateTestNodeID(): &ValidatorImpl{
					NodeID:   ids.GenerateTestNodeID(),
					LightVal: 200,
				},
			},
		}

		// Test Has
		for nodeID := range set.validators {
			require.True(t, set.Has(nodeID))
		}
		require.False(t, set.Has(ids.GenerateTestNodeID()))

		// Test Len
		require.Equal(t, 2, set.Len())

		// Test List
		list := set.List()
		require.Len(t, list, 2)

		// Test Light
		require.Equal(t, uint64(300), set.Light())
	})

	t.Run("Sample", func(t *testing.T) {
		set := &mockSet{
			validators: make(map[ids.NodeID]Validator),
		}

		// Add validators
		for i := 0; i < 5; i++ {
			nodeID := ids.GenerateTestNodeID()
			set.validators[nodeID] = &ValidatorImpl{
				NodeID:   nodeID,
				LightVal: uint64((i + 1) * 100),
			}
		}

		// Sample subset
		sample, err := set.Sample(3)
		require.NoError(t, err)
		require.Len(t, sample, 3)

		// Sample all
		sample, err = set.Sample(5)
		require.NoError(t, err)
		require.Len(t, sample, 5)

		// Sample too many
		_, err = set.Sample(10)
		require.Error(t, err)

		// Sample negative
		_, err = set.Sample(-1)
		require.Error(t, err)

		// Test error case
		set.sampleErr = errors.New("sample error")
		_, err = set.Sample(1)
		require.Error(t, err)
	})
}

func TestManager(t *testing.T) {
	t.Run("GetValidators", func(t *testing.T) {
		netID := ids.GenerateTestID()
		mockSetInstance := &mockSet{
			validators: make(map[ids.NodeID]Validator),
		}

		manager := &mockManager{
			sets: map[ids.ID]Set{
				netID: mockSetInstance,
			},
		}

		set, err := manager.GetValidators(netID)
		require.NoError(t, err)
		require.NotNil(t, set)

		// Test not found
		_, err = manager.GetValidators(ids.GenerateTestID())
		require.Error(t, err)

		// Test error case
		manager.err = errors.New("manager error")
		_, err = manager.GetValidators(netID)
		require.Error(t, err)
	})

	t.Run("GetValidator and Light", func(t *testing.T) {
		netID := ids.GenerateTestID()
		nodeID := ids.GenerateTestNodeID()

		manager := &mockManager{
			validators: map[ids.ID]map[ids.NodeID]*GetValidatorOutput{
				netID: {
					nodeID: {
						NodeID:    nodeID,
						PublicKey: []byte("key"),
						Light:     500,
						Weight:    500,
					},
				},
			},
		}

		// Get existing validator
		val, ok := manager.GetValidator(netID, nodeID)
		require.True(t, ok)
		require.NotNil(t, val)
		require.Equal(t, uint64(500), val.Light)

		// Test GetLight
		light := manager.GetLight(netID, nodeID)
		require.Equal(t, uint64(500), light)

		// Test GetWeight (deprecated)
		weight := manager.GetWeight(netID, nodeID)
		require.Equal(t, uint64(500), weight)

		// Get non-existent validator
		_, ok = manager.GetValidator(netID, ids.GenerateTestNodeID())
		require.False(t, ok)
	})

	t.Run("TotalLight", func(t *testing.T) {
		netID := ids.GenerateTestID()

		manager := &mockManager{
			validators: map[ids.ID]map[ids.NodeID]*GetValidatorOutput{
				netID: {
					ids.GenerateTestNodeID(): {Light: 100},
					ids.GenerateTestNodeID(): {Light: 200},
					ids.GenerateTestNodeID(): {Light: 300},
				},
			},
		}

		total, err := manager.TotalLight(netID)
		require.NoError(t, err)
		require.Equal(t, uint64(600), total)

		// Test TotalWeight (deprecated)
		totalWeight, err := manager.TotalWeight(netID)
		require.NoError(t, err)
		require.Equal(t, uint64(600), totalWeight)

		// Test not found
		_, err = manager.TotalLight(ids.GenerateTestID())
		require.Error(t, err)

		// Test error case
		manager.err = errors.New("total error")
		_, err = manager.TotalLight(netID)
		require.Error(t, err)
	})
}

func TestConnector(t *testing.T) {
	ctx := context.Background()

	t.Run("Connected", func(t *testing.T) {
		connector := &mockConnector{}

		nodeID := ids.GenerateTestNodeID()
		nodeVersion := &version.Application{
			Major: 1,
			Minor: 0,
			Patch: 0,
		}

		err := connector.Connected(ctx, nodeID, nodeVersion)
		require.NoError(t, err)
		require.NotNil(t, connector.connectedNodes[nodeID])
		require.Equal(t, nodeVersion, connector.connectedNodes[nodeID])

		// Test error case
		connector.connectErr = errors.New("connect error")
		err = connector.Connected(ctx, ids.GenerateTestNodeID(), nodeVersion)
		require.Error(t, err)
	})

	t.Run("Disconnected", func(t *testing.T) {
		connector := &mockConnector{}

		nodeID := ids.GenerateTestNodeID()
		nodeVersion := &version.Application{
			Major: 1,
			Minor: 0,
			Patch: 0,
		}

		// Connect first
		_ = connector.Connected(ctx, nodeID, nodeVersion)
		require.NotNil(t, connector.connectedNodes[nodeID])

		// Disconnect
		err := connector.Disconnected(ctx, nodeID)
		require.NoError(t, err)
		require.True(t, connector.disconnectedNodes[nodeID])
		require.Nil(t, connector.connectedNodes[nodeID])

		// Test error case
		connector.disconnectErr = errors.New("disconnect error")
		err = connector.Disconnected(ctx, ids.GenerateTestNodeID())
		require.Error(t, err)
	})
}

func TestGetValidatorOutput(t *testing.T) {
	output := &GetValidatorOutput{
		NodeID:    ids.GenerateTestNodeID(),
		PublicKey: []byte("test-key"),
		Light:     1000,
		Weight:    1000,
		TxID:      ids.GenerateTestID(),
	}

	require.NotEqual(t, ids.EmptyNodeID, output.NodeID)
	require.NotNil(t, output.PublicKey)
	require.Equal(t, uint64(1000), output.Light)
	require.Equal(t, uint64(1000), output.Weight)
	require.NotEqual(t, ids.Empty, output.TxID)
}

// Mock callback listeners for testing
type mockSetCallbackListener struct {
	addedNodes   map[ids.NodeID]uint64
	removedNodes map[ids.NodeID]uint64
	changedNodes map[ids.NodeID]struct {
		oldLight uint64
		newLight uint64
	}
}

func (m *mockSetCallbackListener) OnValidatorAdded(nodeID ids.NodeID, light uint64) {
	if m.addedNodes == nil {
		m.addedNodes = make(map[ids.NodeID]uint64)
	}
	m.addedNodes[nodeID] = light
}

func (m *mockSetCallbackListener) OnValidatorRemoved(nodeID ids.NodeID, light uint64) {
	if m.removedNodes == nil {
		m.removedNodes = make(map[ids.NodeID]uint64)
	}
	m.removedNodes[nodeID] = light
}

func (m *mockSetCallbackListener) OnValidatorLightChanged(nodeID ids.NodeID, oldLight, newLight uint64) {
	if m.changedNodes == nil {
		m.changedNodes = make(map[ids.NodeID]struct {
			oldLight uint64
			newLight uint64
		})
	}
	m.changedNodes[nodeID] = struct {
		oldLight uint64
		newLight uint64
	}{oldLight, newLight}
}

func TestSetCallbackListener(t *testing.T) {
	listener := &mockSetCallbackListener{}

	nodeID := ids.GenerateTestNodeID()

	// Test OnValidatorAdded
	listener.OnValidatorAdded(nodeID, 100)
	require.Equal(t, uint64(100), listener.addedNodes[nodeID])

	// Test OnValidatorRemoved
	listener.OnValidatorRemoved(nodeID, 100)
	require.Equal(t, uint64(100), listener.removedNodes[nodeID])

	// Test OnValidatorLightChanged
	listener.OnValidatorLightChanged(nodeID, 100, 200)
	require.Equal(t, uint64(100), listener.changedNodes[nodeID].oldLight)
	require.Equal(t, uint64(200), listener.changedNodes[nodeID].newLight)
}

// Verify interfaces are satisfied
func TestInterfaceCompliance(t *testing.T) {
	var _ State = (*mockState)(nil)
	var _ Set = (*mockSet)(nil)
	var _ Manager = (*mockManager)(nil)
	var _ Connector = (*mockConnector)(nil)
	var _ Validator = (*ValidatorImpl)(nil)
	var _ SetCallbackListener = (*mockSetCallbackListener)(nil)
}