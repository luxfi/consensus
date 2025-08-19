package prism

import "github.com/luxfi/consensus/types"

type health struct{ m map[types.NodeID]int }

func newHealth() *health { return &health{m: make(map[types.NodeID]int)} }

func (h *health) bump(id types.NodeID, good bool) {
    if good { h.m[id]++ } else { h.m[id]-- }
}

func (h *health) score(id types.NodeID) float64 { return 1 + float64(h.m[id])*0.05 }
