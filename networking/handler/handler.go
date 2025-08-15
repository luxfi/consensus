package handler

import "github.com/luxfi/consensus/engine/core"

// Handler handles consensus messages
type Handler interface {
	HandleMessage(msg core.Message) error
}
