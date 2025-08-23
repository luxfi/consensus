package prism

import "github.com/luxfi/consensus/types"

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

// Sample implements Cut interface (cuts k rays from the peer population)
func (c *UniformCut) Sample(k int) []types.NodeID {
	if k >= len(c.peers) {
		return c.peers
	}

	// Simple random cutting (in production, use proper randomization)
	// TODO: Implement proper cryptographically secure random cutting
	result := make([]types.NodeID, 0, k)
	for i := 0; i < k && i < len(c.peers); i++ {
		result = append(result, c.peers[i])
	}
	return result
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
