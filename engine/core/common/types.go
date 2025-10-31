package common

import (
	consensus_core "github.com/luxfi/consensus/core"
	"github.com/luxfi/ids"
)

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// AppSender sends application-level messages
type AppSender = consensus_core.AppSender

// AppHandler handles application messages
type AppHandler = consensus_core.AppHandler
