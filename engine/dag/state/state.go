package state

import (
	"github.com/luxfi/consensus/engine/dag"
)

// Serializer handles serialization of DAG state
type Serializer interface {
	// ParseVertex parses a vertex from bytes
	ParseVertex([]byte) (dag.Vertex, error)
	
	// ParseTx parses a transaction from bytes
	ParseTx([]byte) (dag.Tx, error)
}

// NewSerializer creates a new state serializer
func NewSerializer(vtxGetter interface{}, txGetter interface{}) Serializer {
	return &noOpSerializer{}
}

type noOpSerializer struct{}

func (n *noOpSerializer) ParseVertex([]byte) (dag.Vertex, error) {
	return nil, nil
}

func (n *noOpSerializer) ParseTx([]byte) (dag.Tx, error) {
	return nil, nil
}