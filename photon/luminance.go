package photon

import (
    "github.com/luxfi/consensus/types"
)

// Luminance tracks node brightness based on consensus participation.
// More successful votes = higher lux = brighter emission probability.
type Luminance struct {
    lux map[types.NodeID]float64 // Brightness level per node (in lux units)
}

// newLuminance creates a new brightness tracker
func newLuminance() *Luminance {
    return &Luminance{
        lux: make(map[types.NodeID]float64),
    }
}

// illuminate increases or decreases node brightness based on performance
func (l *Luminance) illuminate(id types.NodeID, success bool) {
    if _, exists := l.lux[id]; !exists {
        l.lux[id] = 100.0 // Base illumination: 100 lux (office lighting)
    }
    
    if success {
        // Successful vote increases brightness (max: 1000 lux = bright daylight)
        l.lux[id] *= 1.1
        if l.lux[id] > 1000.0 {
            l.lux[id] = 1000.0
        }
    } else {
        // Failed vote dims the node (min: 10 lux = twilight)
        l.lux[id] *= 0.9
        if l.lux[id] < 10.0 {
            l.lux[id] = 10.0
        }
    }
}

// brightness returns emission weight based on lux level (0.1 to 10.0)
func (l *Luminance) brightness(id types.NodeID) float64 {
    if lux, exists := l.lux[id]; exists {
        return lux / 100.0 // Normalize to base level
    }
    return 1.0 // Default brightness
}