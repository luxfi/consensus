package nova

import (
	"sync"
	"time"

	"github.com/luxfi/consensus/types"
)

// Finalizer provides classical finality for decided blocks
type Finalizer[ID comparable] struct {
	mu        sync.RWMutex
	finalized map[ID]time.Time
	depth     map[ID]int
}

func New[ID comparable]() *Finalizer[ID] {
	return &Finalizer[ID]{
		finalized: make(map[ID]time.Time),
		depth:     make(map[ID]int),
	}
}

func (f *Finalizer[ID]) OnDecide(id ID, decision types.Decision) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if decision == types.DecideAccept {
		f.finalized[id] = time.Now()
		
		// Calculate depth based on number of finalized blocks
		f.depth[id] = len(f.finalized)
	}
}

func (f *Finalizer[ID]) Finalized(id ID) (bool, int) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if _, ok := f.finalized[id]; ok {
		return true, f.depth[id]
	}
	return false, 0
}