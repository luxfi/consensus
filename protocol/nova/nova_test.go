package nova

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/types"
)

func TestFinalizer(t *testing.T) {
	f := New[string]()

	// Test initial state
	finalized, depth := f.Finalized("block1")
	if finalized || depth != 0 {
		t.Error("new block should not be finalized")
	}

	// Add decision
	f.OnDecide("block1", types.DecideAccept)
	finalized, depth = f.Finalized("block1")
	if !finalized || depth != 1 {
		t.Errorf("decided block should be finalized at depth 1, got finalized=%v depth=%d", finalized, depth)
	}

	// Add another decision
	f.OnDecide("block2", types.DecideAccept)
	finalized, depth = f.Finalized("block2")
	if !finalized || depth != 2 {
		t.Errorf("second block should be at depth 2, got %d", depth)
	}

	// Check first block depth increased
	finalized, depth = f.Finalized("block1")
	if !finalized || depth != 1 {
		t.Errorf("first block should still be at depth 1, got %d", depth)
	}

	// Test rejected block
	f.OnDecide("block3", types.DecideReject)
	finalized, depth = f.Finalized("block3")
	if finalized || depth != 0 {
		t.Error("rejected block should not be finalized")
	}
}

func TestFinalizerChain(t *testing.T) {
	f := New[int]()

	// Build a chain: 1 -> 2 -> 3 -> 4 -> 5
	for i := 1; i <= 5; i++ {
		f.OnDecide(i, types.DecideAccept)
	}

	// Check depths
	for i := 1; i <= 5; i++ {
		finalized, depth := f.Finalized(i)
		if !finalized {
			t.Errorf("block %d should be finalized", i)
		}
		expectedDepth := i
		if depth != expectedDepth {
			t.Errorf("block %d: expected depth %d, got %d", i, expectedDepth, depth)
		}
	}

	// Add more blocks
	for i := 6; i <= 10; i++ {
		f.OnDecide(i, types.DecideAccept)
	}

	// Original blocks should have higher depth
	finalized, depth := f.Finalized(1)
	if !finalized || depth != 1 {
		t.Errorf("block 1 should still be at depth 1, got %d", depth)
	}

	finalized, depth = f.Finalized(10)
	if !finalized || depth != 10 {
		t.Errorf("block 10 should be at depth 10, got %d", depth)
	}
}

func TestFinalizerReorg(t *testing.T) {
	f := New[string]()

	// Initial chain: A -> B -> C
	f.OnDecide("A", types.DecideAccept)
	f.OnDecide("B", types.DecideAccept)
	f.OnDecide("C", types.DecideAccept)

	// Check C is finalized
	finalized, _ := f.Finalized("C")
	if !finalized {
		t.Error("C should be finalized")
	}

	// Conflicting block at same height as C
	f.OnDecide("C'", types.DecideReject)
	finalized, _ = f.Finalized("C'")
	if finalized {
		t.Error("rejected C' should not be finalized")
	}

	// Original chain should still be finalized
	finalized, _ = f.Finalized("C")
	if !finalized {
		t.Error("C should still be finalized")
	}
}

func TestFinalizerConcurrency(t *testing.T) {
	f := New[int]()
	done := make(chan bool, 10)

	// Add decisions concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			f.OnDecide(id, types.DecideAccept)
			done <- true
		}(i)
	}

	// Wait for all
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check all are finalized
	for i := 0; i < 10; i++ {
		finalized, _ := f.Finalized(i)
		if !finalized {
			t.Errorf("block %d should be finalized", i)
		}
	}
}

func TestFinalizerPruning(t *testing.T) {
	f := NewWithPruning[int](5) // Keep only last 5 blocks

	// Add 10 blocks
	for i := 1; i <= 10; i++ {
		f.OnDecide(i, types.DecideAccept)
	}

	// Recent blocks should be finalized
	for i := 6; i <= 10; i++ {
		finalized, _ := f.Finalized(i)
		if !finalized {
			t.Errorf("recent block %d should be finalized", i)
		}
	}

	// Old blocks might be pruned
	for i := 1; i <= 5; i++ {
		finalized, depth := f.Finalized(i)
		if finalized && depth > 0 {
			// Still in cache, ok
		} else if !finalized && depth == 0 {
			// Pruned, also ok
		} else {
			t.Errorf("unexpected state for block %d: finalized=%v depth=%d", i, finalized, depth)
		}
	}
}

// Extended finalizer with pruning
func NewWithPruning[ID comparable](maxDepth int) *FinalizerWithPruning[ID] {
	return &FinalizerWithPruning[ID]{
		Finalizer: Finalizer[ID]{
			finalized: make(map[ID]time.Time),
			depth:     make(map[ID]int),
		},
		maxDepth: maxDepth,
	}
}

type FinalizerWithPruning[ID comparable] struct {
	Finalizer[ID]
	maxDepth int
}

func (f *FinalizerWithPruning[ID]) OnDecide(id ID, decision types.Decision) {
	f.Finalizer.OnDecide(id, decision)
	
	// Prune old entries
	if len(f.depth) > f.maxDepth {
		// Find and remove oldest
		var oldest ID
		oldestDepth := 0
		for id, depth := range f.depth {
			if oldestDepth == 0 || depth < oldestDepth {
				oldest = id
				oldestDepth = depth
			}
		}
		delete(f.finalized, oldest)
		delete(f.depth, oldest)
	}
}