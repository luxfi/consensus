package getter

import (
	"time"

	"github.com/luxfi/consensus/networking/sender"
	"github.com/luxfi/log"
)

// Handler handles get requests
type Handler interface {
	// GetAncestors retrieves ancestors
	GetAncestors() error
}

// New creates a new getter handler
func New(
	vm interface{},
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
