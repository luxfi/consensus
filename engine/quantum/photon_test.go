// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestNewPhoton(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	payload := []byte("test transaction data")

	photon := NewPhoton(nodeID, payload)

	require.NotNil(photon)
	require.NotEqual(ids.Empty, photon.ID)
	require.Equal(nodeID, photon.Source)
	require.Equal(payload, photon.Payload)
	require.Equal(uint64(1), photon.Energy)
	require.Len(photon.Quantum, 32)
	require.Empty(photon.Entangled)
	require.WithinDuration(time.Now(), photon.Timestamp, time.Second)
}

func TestPhotonEntangle(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	photon1 := NewPhoton(nodeID, []byte("photon1"))
	photon2 := NewPhoton(nodeID, []byte("photon2"))

	// Test entanglement
	photon1.Entangle(photon2)

	require.True(photon1.IsEntangled(photon2.ID))
	require.True(photon2.IsEntangled(photon1.ID))
	require.Len(photon1.Entangled, 1)
	require.Len(photon2.Entangled, 1)
	require.Equal(photon2.ID, photon1.Entangled[0])
	require.Equal(photon1.ID, photon2.Entangled[0])
}

func TestPhotonAmplify(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	photon := NewPhoton(nodeID, []byte("test"))

	require.Equal(uint64(1), photon.Energy)

	// Test amplification
	photon.Amplify(10)
	require.Equal(uint64(10), photon.Energy)

	photon.Amplify(5)
	require.Equal(uint64(50), photon.Energy)
}

func TestPhotonCollapse(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	payload := []byte("quantum data")
	photon := NewPhoton(nodeID, payload)

	// Test collapse
	collapsed := photon.Collapse()

	require.NotNil(collapsed)
	require.Len(collapsed, len(photon.Quantum)+len(payload))
	
	// Verify quantum state is first
	require.Equal(photon.Quantum, collapsed[:32])
	// Verify payload follows
	require.Equal(payload, collapsed[32:])
}

func TestPhotonIsEntangled(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	photon1 := NewPhoton(nodeID, []byte("photon1"))
	photon2 := NewPhoton(nodeID, []byte("photon2"))
	photon3 := NewPhoton(nodeID, []byte("photon3"))

	// Initially not entangled
	require.False(photon1.IsEntangled(photon2.ID))
	require.False(photon1.IsEntangled(photon3.ID))

	// Entangle 1 and 2
	photon1.Entangle(photon2)

	require.True(photon1.IsEntangled(photon2.ID))
	require.False(photon1.IsEntangled(photon3.ID))
	require.True(photon2.IsEntangled(photon1.ID))
	require.False(photon2.IsEntangled(photon3.ID))
}

func TestPhotonMultipleEntanglements(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	photon1 := NewPhoton(nodeID, []byte("photon1"))
	photon2 := NewPhoton(nodeID, []byte("photon2"))
	photon3 := NewPhoton(nodeID, []byte("photon3"))

	// Create multiple entanglements
	photon1.Entangle(photon2)
	photon1.Entangle(photon3)

	require.Len(photon1.Entangled, 2)
	require.True(photon1.IsEntangled(photon2.ID))
	require.True(photon1.IsEntangled(photon3.ID))
	require.True(photon2.IsEntangled(photon1.ID))
	require.True(photon3.IsEntangled(photon1.ID))
}

func TestPhotonQuantumStateUniqueness(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	
	// Create multiple photons with same data
	photons := make([]*Photon, 10)
	for i := 0; i < 10; i++ {
		photons[i] = NewPhoton(nodeID, []byte("same data"))
	}

	// Verify each has unique quantum state
	quantumStates := make(map[string]bool)
	for _, p := range photons {
		stateStr := string(p.Quantum)
		require.False(quantumStates[stateStr], "Duplicate quantum state found")
		quantumStates[stateStr] = true
	}
}

func BenchmarkNewPhoton(b *testing.B) {
	nodeID := ids.GenerateTestNodeID()
	payload := []byte("benchmark payload data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewPhoton(nodeID, payload)
	}
}

func BenchmarkPhotonEntangle(b *testing.B) {
	nodeID := ids.GenerateTestNodeID()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		photon1 := NewPhoton(nodeID, []byte("photon1"))
		photon2 := NewPhoton(nodeID, []byte("photon2"))
		photon1.Entangle(photon2)
	}
}

func BenchmarkPhotonCollapse(b *testing.B) {
	nodeID := ids.GenerateTestNodeID()
	photon := NewPhoton(nodeID, []byte("benchmark data"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = photon.Collapse()
	}
}