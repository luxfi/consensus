package prism

import (
    "context"
    "sync"

    "github.com/luxfi/consensus/types"
)

type DefaultSampler struct {
    mu     sync.RWMutex
    peers  []types.NodeID
    opts   Options
    health *health
}

func New(peers []types.NodeID, opts Options) *DefaultSampler {
    if opts.MinPeers == 0 { opts.MinPeers = 8 }
    if opts.MaxPeers == 0 { opts.MaxPeers = 64 }
    return &DefaultSampler{peers: peers, opts: opts, health: newHealth()}
}

func (s *DefaultSampler) Sample(ctx context.Context, k int, _ types.Topic) []types.NodeID {
    _ = ctx
    s.mu.RLock(); defer s.mu.RUnlock()
    if k <= 0 { k = s.opts.MinPeers }
    if k > s.opts.MaxPeers { k = s.opts.MaxPeers }

    type scored struct{ id types.NodeID; w float64 }
    all := make([]scored, 0, len(s.peers))
    for _, id := range s.peers {
        w := 1.0
        if s.opts.Stake != nil { w *= s.opts.Stake(id) }
        if s.opts.Latency != nil {
            if lat := s.opts.Latency(id); lat > 0 {
                w *= 1.0 / (1.0 + float64(lat.Milliseconds()))
            }
        }
        w *= s.health.score(id)
        all = append(all, scored{id, w})
    }
    out := make([]types.NodeID, 0, k)
    for i := 0; i < k && len(all) > 0; i++ {
        best := 0
        for j := 1; j < len(all); j++ {
            if all[j].w > all[best].w { best = j }
        }
        out = append(out, all[best].id)
        all[best].w *= 0.5
    }
    return out
}

func (s *DefaultSampler) Report(node types.NodeID, probe types.Probe) {
    s.mu.Lock(); defer s.mu.Unlock()
    s.health.bump(node, probe == types.ProbeGood)
}
func (s *DefaultSampler) Allow(types.Topic) bool { return true }
