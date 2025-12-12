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

// Sender is the warp Sender for cross-VM messaging.
type Sender = warp.Sender

// Handler handles warp messages.
type Handler = warp.Handler
