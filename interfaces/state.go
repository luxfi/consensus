package interfaces

import (
	"context"

	"github.com/luxfi/consensus/choices"
)

// State describes the consensus state of the system
type State byte

const (
	// Bootstrapping is the state of the consensus engine when it is in the
	// process of bootstrapping
	Bootstrapping State = iota
	// NormalOp is the state of the consensus engine when it is operating
	// normally (not bootstrapping)
	NormalOp
)

func (s State) String() string {
	switch s {
	case Bootstrapping:
		return "Bootstrapping"
	case NormalOp:
		return "NormalOp"
	default:
		return "Unknown"
	}
}

// Block defines the interface for a consensus block
type Block interface {
	ID() [32]byte
	Parent() [32]byte
	Height() uint64
	Timestamp() uint64
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
	Status() choices.Status
	Bytes() []byte
}