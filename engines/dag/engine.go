package dag

import (
    "context"
    "github.com/luxfi/consensus/core/dag"
)

type Engine struct {
    flare *dag.Flare
}

func New(n, f int) *Engine { return &Engine{flare: dag.NewFlare(dag.Params{N: n, F: f})} }

func (e *Engine) Tick(_ context.Context, v dag.View, proposers []dag.Meta) {
    for _, p := range proposers {
        _ = e.flare.Classify(v, p)
    }
}
