package pq

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/protocols/quasar"
)

type Engine struct {
    inner   interface{}
    quasar  *quasar.Engine
    params  core.Params
    deps    core.Deps
}

func Wrap(inner interface{}, params core.Params, deps core.Deps) *Engine {
    return &Engine{
        inner:  inner,
        params: params,
        deps:   deps,
    }
}

func (e *Engine) Start(ctx context.Context) error {
    e.deps.Log.Info("Starting PQ wrapper", "enabled", e.params.PQEnabled)
    return nil
}
