package getter

import (
	"time"

	"github.com/luxfi/consensus/engine/dag/state"
	"github.com/luxfi/consensus/networking/sender"
	"github.com/luxfi/log"
)

// Handler handles get requests
type Handler interface {
	// GetAncestors retrieves ancestors
	GetAncestors() error

	// Start starts the handler
	Start() error

	// Stop stops the handler
	Stop() error
}

// NewHandler creates a new getter handler
func NewHandler(
	vtxManager state.Serializer,
	sender sender.Sender,
	log log.Logger,
	maxTimeGetAncestors time.Duration,
	maxContainersSent int,
) (Handler, error) {
	return &noOpHandler{}, nil
}

type noOpHandler struct{}

func (n *noOpHandler) GetAncestors() error {
	return nil
}

func (n *noOpHandler) Start() error {
	return nil
}

func (n *noOpHandler) Stop() error {
	return nil
}
