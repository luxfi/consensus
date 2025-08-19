package chain

import (
    "context"

    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/core/wave"
    "github.com/luxfi/consensus/photon"
    "github.com/luxfi/consensus/protocol/nova"
    "github.com/luxfi/consensus/types"
)

type Transport[ID comparable] interface{ wave.Transport[ID] }

type Engine[ID comparable] struct {
    w  *wave.Impl[ID]
    nv *nova.Finalizer[ID]
}

func New[ID comparable](cfg config.Parameters, emitter photon.Emitter[types.NodeID], tx Transport[ID]) *Engine[ID] {
    return &Engine[ID]{ w: wave.New[ID](cfg, emitter, tx), nv: nova.New[ID]() }
}

func (e *Engine[ID]) Tick(ctx context.Context, id ID) {
    e.w.Tick(ctx, id)
    if st, ok := e.w.State(id); ok && st.Decided {
        e.nv.OnDecide(id, st.Result)
    }
}

func (e *Engine[ID]) State(id ID) (wave.State[ID], bool) {
    return e.w.State(id)
}
