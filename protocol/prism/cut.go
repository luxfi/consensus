package prism

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/luxfi/consensus/core/types"
)

// Cut provides random cutting of peers for consensus voting (like a prism cuts light)
type Cut[T comparable] interface {
	// Sample returns k random peers for voting (cuts k rays from the population)
	Sample(k int) []types.NodeID

	// Luminance returns light intensity metrics for the cut
	Luminance() Luminance
}

// Luminance measures the intensity of light across the peer network
// Following SI units: lux (lx) = lumens per square meter
type Luminance struct {
	ActivePeers int
	TotalPeers  int
	Lx          float64 // Illuminance in lux (lx) - minimum 1 lx per active peer/photon
}

// UniformCut implements uniform random cutting
type UniformCut struct {
	peers []types.NodeID
}

// NewUniformCut creates a new uniform cut
func NewUniformCut(peers []types.NodeID) *UniformCut {
	return &UniformCut{peers: peers}
}

// Sample implements Cut interface using Fisher-Yates shuffle with crypto/rand.
// This provides uniform random sampling of k peers from the population.
func (c *UniformCut) Sample(k int) []types.NodeID {
	n := len(c.peers)
	if k >= n {
		return c.peers
	}

	// Create a copy to shuffle (don't modify original)
	shuffled := make([]types.NodeID, n)
	copy(shuffled, c.peers)

	// Fisher-Yates shuffle with cryptographic randomness
	// Only need to shuffle first k elements
	for i := 0; i < k; i++ {
		// Generate random index j where i <= j < n
		j := i + cryptoRandInt(n-i)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:k]
}

// cryptoRandInt returns a cryptographically secure random integer in [0, max).
func cryptoRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return int(binary.LittleEndian.Uint64(buf[:]) % uint64(max))
}

// Luminance implements Cut interface
func (c *UniformCut) Luminance() Luminance {
	activePeers := len(c.peers)
	// Minimum 1 lx per active peer/photon, scaling with network health
	lx := float64(activePeers) // Base: 1 lx per peer
	if activePeers >= 100 {
		lx = 500.0 // Office lighting level for healthy large networks
	} else if activePeers >= 20 {
		lx = 300.0 // Classroom level for medium networks
	}

	return Luminance{
		ActivePeers: activePeers,
		TotalPeers:  activePeers,
		Lx:          lx,
	}
}
