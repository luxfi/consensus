package chain

import (
    "context"

    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/core/wave"
    "github.com/luxfi/consensus/core/prism"
    "github.com/luxfi/consensus/protocol/nova"
)

type Transport[ID comparable] interface{ wave.Transport[ID] }

type Engine[ID comparable] struct {
    w  *wave.Impl[ID]
    nv *nova.Finalizer[ID]
}

func New[ID comparable](cfg config.Parameters, sel prism.Sampler[ID], tx Transport[ID]) *Engine[ID] {
    return &Engine[ID]{ w: wave.New[ID](cfg, sel, tx), nv: nova.New[ID]() }
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
