package nova

import "github.com/luxfi/consensus/types"

type Finalizer[ID comparable] struct{}
func New[ID comparable]() *Finalizer[ID] { return &Finalizer[ID]{} }

func (f *Finalizer[ID]) OnDecide(id ID, res types.Decision) { _ = id; _ = res }
