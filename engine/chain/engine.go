package chain

import (
    "context"
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/consensus/protocols/nova"
)

type Engine struct {
    params core.Params
    deps   core.Deps
    nova   *nova.Topological
}

func New(params core.Params, deps core.Deps) *Engine {
    return &Engine{
        params: params,
        deps:   deps,
    }
}

func (e *Engine) Start(ctx context.Context) error {
    e.deps.Log.Info("Starting chain engine", "k", e.params.K)
    return nil
}

func (e *Engine) Stop() error {
    e.deps.Log.Info("Stopping chain engine")
    return nil
}
