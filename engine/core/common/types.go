package common

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
)

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// Sender sends warp messages.
type Sender = warp.Sender

// Handler handles warp messages.
type Handler = warp.Handler
