// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"crypto/rand"
	"time"

	"github.com/luxfi/ids"
)

// Photon represents the smallest quantum unit of consensus data.
// Like a particle of light carrying information across the cosmos,
// each Photon carries a single piece of consensus information that,
// when combined with others, forms the basis of our quantum-secure state.
type Photon struct {
	// Unique identifier
	ID ids.ID

	// Source node that emitted this photon
	Source ids.NodeID

	// Timestamp of creation
	Timestamp time.Time

	// Quantum state representation
	Quantum []byte

	// Payload - the actual data being transmitted
	Payload []byte

	// Energy level - priority/importance
	Energy uint64

	// Entangled photons - linked consensus units
	Entangled []ids.ID
}

// NewPhoton creates a new quantum consensus unit
func NewPhoton(source ids.NodeID, payload []byte) *Photon {
	quantum := make([]byte, 32)
	rand.Read(quantum)

	p := &Photon{
		ID:        ids.GenerateTestID(),
		Source:    source,
		Timestamp: time.Now(),
		Quantum:   quantum,
		Payload:   payload,
		Energy:    1,
		Entangled: make([]ids.ID, 0),
	}
	return p
}

// Entangle creates a quantum entanglement with another photon
func (p *Photon) Entangle(other *Photon) {
	p.Entangled = append(p.Entangled, other.ID)
	other.Entangled = append(other.Entangled, p.ID)
}

// Amplify increases the energy level of the photon
func (p *Photon) Amplify(factor uint64) {
	p.Energy *= factor
}

// Collapse finalizes the quantum state of the photon
func (p *Photon) Collapse() []byte {
	// Combine quantum state with payload for final state
	state := make([]byte, len(p.Quantum)+len(p.Payload))
	copy(state, p.Quantum)
	copy(state[len(p.Quantum):], p.Payload)
	return state
}

// IsEntangled checks if this photon is entangled with another
func (p *Photon) IsEntangled(id ids.ID) bool {
	for _, entangled := range p.Entangled {
		if entangled == id {
			return true
		}
	}
	return false
}