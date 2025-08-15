package state

import (
	"github.com/luxfi/database"
	"github.com/luxfi/log"
	"github.com/luxfi/metric"
)

// Serializer manages state serialization
type Serializer interface {
	// Initialize the serializer
	Initialize() error
	// Close the serializer
	Close() error
}

// NewSerializer creates a new state serializer
func NewSerializer(log log.Logger, db database.Database, metrics metric.Metrics) Serializer {
	return &noOpSerializer{}
}

type noOpSerializer struct{}

func (n *noOpSerializer) Initialize() error {
	return nil
}

func (n *noOpSerializer) Close() error {
	return nil
}