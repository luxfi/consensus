package vertex

import (
    "context"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/engine/graph"
)

// LinearizableVMWithEngine represents a linearizable vertex VM with engine
type LinearizableVMWithEngine interface {
    Initialize(ctx context.Context, chainCtx interface{}, dbManager interface{}, genesisBytes []byte, upgradeBytes []byte, configBytes []byte, toEngine chan<- interface{}, fxs []interface{}, appSender interface{}) error
    BuildVertex(ctx context.Context) (graph.Vertex, error)
    ParseVertex(ctx context.Context, vtxBytes []byte) (graph.Vertex, error)
    GetVertex(ctx context.Context, vtxID ids.ID) (graph.Vertex, error)
    ParseTx(ctx context.Context, txBytes []byte) (interface{}, error)
}