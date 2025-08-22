package core

import (
	"context"
	"github.com/luxfi/ids"
)

// Decidable represents something that can be decided upon
type Decidable interface {
	// ID returns the ID of this decidable
	ID() ids.ID
	
	// Accept marks this as accepted
	Accept(context.Context) error
	
	// Reject marks this as rejected  
	Reject(context.Context) error
	
	// Status returns the current status
	Status() Status
}