package state

import "github.com/luxfi/ids"

// State manages graph consensus state
type State struct {
    vertices map[ids.ID]interface{}
}

// New creates a new state
func New() *State {
    return &State{
        vertices: make(map[ids.ID]interface{}),
    }
}