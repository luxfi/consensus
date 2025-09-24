#!/usr/bin/env bash
set -euo pipefail

mkdir -p config core/prism core/wave core/dag protocol/nova protocol/nebula protocol/quasar engine/chain engine/dag types witness examples/fastchain core/focus

cat > go.mod <<'EOF'
module consensus
go 1.22
EOF

cat > config/parameters.go <<'EOF'
package config

import "time"

type Parameters struct {
    K         int
    Alpha     float64
    Beta      uint32
    RoundTO   time.Duration
    BlockTime time.Duration
}

func DefaultParams() Parameters {
    return Parameters{
        K:         20,
        Alpha:     0.8,
        Beta:      15,
        RoundTO:   250 * time.Millisecond,
        BlockTime: 100 * time.Millisecond,
    }
}
EOF

cat > config/wave.go <<'EOF'
package config

type WaveConfig struct {
    Enable              bool
    VoteLimitPerBlock   int
    ExecuteOwned        bool
    ExecuteMixedOnFinal bool
    EpochFence          bool
    VotePrefix          []byte
}

func DefaultWave() WaveConfig {
    return WaveConfig{
        Enable:              true,
        VoteLimitPerBlock:   256,
        ExecuteOwned:        true,
        ExecuteMixedOnFinal: true,
        EpochFence:          true,
        VotePrefix:          []byte("WAVE/V1"),
    }
}
EOF

cat > types/types.go <<'EOF'
package types

import "time"

type NodeID string
type Topic string

type Probe int
const (
    ProbeGood Probe = iota
    ProbeTimeout
    ProbeBadSig
)

type Decision int
const (
    DecideAccept Decision = iota
    DecideReject
)

type Digest [32]byte

type Round struct {
    Height uint64
    Time   time.Time
}
EOF

cat > witness/verkle.go <<'EOF'
package witness

// VerkleHints lets consensus request compact witnesses for a tx batch.
type VerkleHints interface {
    PrepareHints(keys [][]byte) (witnessBlob []byte, estSize int, err error)
}
EOF

cat > core/prism/prism.go <<'EOF'
package prism

import (
    "context"
    "time"

    "consensus/types"
)

type Options struct {
    MinPeers int
    MaxPeers int
    Stake    func(types.NodeID) float64
    Latency  func(types.NodeID) time.Duration
}

type Sampler[T comparable] interface {
    Sample(ctx context.Context, k int, topic types.Topic) []types.NodeID
    Report(node types.NodeID, probe types.Probe)
    Allow(topic types.Topic) bool
}

func DefaultOptions() Options { return Options{MinPeers: 8, MaxPeers: 64} }
EOF

cat > core/prism/cut.go <<'EOF'
package prism

import "consensus/types"

type health struct{ m map[types.NodeID]int }

func newHealth() *health { return &health{m: make(map[types.NodeID]int)} }

func (h *health) bump(id types.NodeID, good bool) {
    if good { h.m[id]++ } else { h.m[id]-- }
}

func (h *health) score(id types.NodeID) float64 { return 1 + float64(h.m[id])*0.05 }
EOF

cat > core/prism/refract.go <<'EOF'
package prism

import (
    "context"
    "sync"

    "consensus/types"
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
EOF

cat > core/wave/wave.go <<'EOF'
package wave

import (
    "context"
    "time"

    "consensus/config"
    "consensus/prism"
    "consensus/types"
)

type VoteMsg[ID comparable] struct {
    Item   ID
    Prefer bool
    From   types.NodeID
}

type Transport[ID comparable] interface {
    RequestVotes(ctx context.Context, peers []types.NodeID, item ID) (<-chan VoteMsg[ID], error)
}

type Step[ID comparable] struct {
    Prefer bool
    Conf   uint32
}

type State[ID comparable] struct {
    Step    Step[ID]
    Decided bool
    Result  types.Decision
    Last    time.Time
}

type Impl[ID comparable] struct {
    cfg   config.Parameters
    sel   prism.Sampler[ID]
    tx    Transport[ID]
    state map[ID]State[ID]
}

func New[ID comparable](cfg config.Parameters, sel prism.Sampler[ID], tx Transport[ID]) *Impl[ID] {
    return &Impl[ID]{cfg: cfg, sel: sel, tx: tx, state: make(map[ID]State[ID])}
}

func (w *Impl[ID]) ensure(id ID) State[ID] {
    if st, ok := w.state[id]; ok { return st }
    st := State[ID]{}
    w.state[id] = st
    return st
}

func (w *Impl[ID]) Tick(ctx context.Context, id ID) {
    st := w.ensure(id)
    peers := w.sel.Sample(ctx, w.cfg.K, types.Topic("votes"))
    ch, _ := w.tx.RequestVotes(ctx, peers, id)

    yes, n := 0, 0
    timer := time.NewTimer(w.cfg.RoundTO); defer timer.Stop()

loop:
    for {
        select {
        case <-ctx.Done():
            break loop
        case <-timer.C:
            break loop
        case v, ok := <-ch:
            if !ok { break loop }
            n++
            if v.Prefer { yes++ }
        }
    }

    if n == 0 { return }

    ratio := float64(yes) / float64(n)
    next := st
    if ratio >= w.cfg.Alpha {
        if st.Step.Prefer { next.Step.Conf++ } else { next.Step.Prefer, next.Step.Conf = true, 1 }
    } else if ratio <= 1.0-w.cfg.Alpha {
        if !st.Step.Prefer { next.Step.Conf++ } else { next.Step.Prefer, next.Step.Conf = false, 1 }
    }
    if next.Step.Conf >= w.cfg.Beta {
        next.Decided = true
        if next.Step.Prefer { next.Result = types.DecideAccept } else { next.Result = types.DecideReject }
    }
    next.Last = time.Now()
    w.state[id] = next
}

func (w *Impl[ID]) State(id ID) (State[ID], bool) { st, ok := w.state[id]; return st, ok }
EOF

cat > core/dag/horizon.go <<'EOF'
package dag

type VertexID [32]byte

type Meta interface {
    ID() VertexID
    Author() string
    Round() uint64
    Parents() []VertexID
}

type View interface {
    Get(VertexID) (Meta, bool)
    ByRound(round uint64) []Meta
    Supports(from VertexID, author string, round uint64) bool
}

type Params struct{ N, F int }
EOF

cat > core/dag/flare.go <<'EOF'
package dag

type Decision int
const (
    DecideUndecided Decision = iota
    DecideCommit
    DecideSkip
)

// Cert: >=2f+1 in r+1 support proposer(author,round). Skip: >=2f+1 in r+1 not supporting.
func HasCertificate(v View, proposer Meta, p Params) bool {
    r1 := proposer.Round() + 1
    next := v.ByRound(r1)
    support := 0
    for _, m := range next {
        if v.Supports(m.ID(), proposer.Author(), proposer.Round()) {
            support++
            if support >= 2*p.F+1 { return true }
        }
    }
    return false
}

func HasSkip(v View, proposer Meta, p Params) bool {
    r1 := proposer.Round() + 1
    next := v.ByRound(r1)
    nos := 0
    for _, m := range next {
        if !v.Supports(m.ID(), proposer.Author(), proposer.Round()) {
            nos++
            if nos >= 2*p.F+1 { return true }
        }
    }
    return false
}

type Flare struct{ p Params }
func NewFlare(p Params) *Flare { return &Flare{p: p} }

func (f *Flare) Classify(v View, proposer Meta) Decision {
    switch {
    case HasCertificate(v, proposer, f.p):
        return DecideCommit
    case HasSkip(v, proposer, f.p):
        return DecideSkip
    default:
        return DecideUndecided
    }
}
EOF

cat > core/focus/focus.go <<'EOF'
package focus

// optional separate β accounting if you keep it outside wave; stub for now.
type Focus[ID comparable] struct{}
func New[ID comparable]() *Focus[ID] { return &Focus[ID]{} }
EOF

cat > protocol/nova/nova.go <<'EOF'
package nova

import "consensus/types"

type Finalizer[ID comparable] struct{}
func New[ID comparable]() *Finalizer[ID] { return &Finalizer[ID]{} }

func (f *Finalizer[ID]) OnDecide(id ID, res types.Decision) { _ = id; _ = res }
EOF

cat > protocol/nebula/nebula.go <<'EOF'
package nebula

// placeholder for epoch/cross-chain features (e.g., checkpoint bundling).
type Service struct{}
func New() *Service { return &Service{} }
EOF

cat > protocol/quasar/quasar.go <<'EOF'
package quasar

type Bundle struct {
    Epoch   uint64
    Root    []byte
    BLSAgg  []byte
    PQBatch []byte
    Binding []byte
}

type Client interface {
    SubmitCheckpoint(epoch uint64, root []byte, attest []byte) error
    FetchBundle(epoch uint64) (Bundle, error)
    Verify(Bundle) bool
}
EOF

cat > engine/chain/engine.go <<'EOF'
package chain

import (
    "context"

    "consensus/config"
    "consensus/core/wave"
    "consensus/prism"
    "consensus/protocol/nova"
    "consensus/types"
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

type voteTx[ID comparable] struct{}
func (voteTx[ID]) RequestVotes(ctx context.Context, peers []types.NodeID, item ID) (<-chan wave.VoteMsg[ID], error) {
    _ = ctx; _ = peers
    out := make(chan wave.VoteMsg[ID], 1)
    out <- wave.VoteMsg[ID]{Item: item, Prefer: true}; close(out)
    return out, nil
}
EOF

cat > engine/dag/engine.go <<'EOF'
package dag

import (
    "context"
    "consensus/core/dag"
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
EOF

cat > examples/fastchain/main.go <<'EOF'
package main

import (
    "context"
    "fmt"
    "time"

    "consensus/config"
    "consensus/engine/chain"
    "consensus/prism"
    "consensus/types"
    "consensus/core/wave"
)

type ItemID string

type txStub struct{}
func (txStub) RequestVotes(ctx context.Context, peers []types.NodeID, item ItemID) (<-chan wave.VoteMsg[ItemID], error) {
    _ = ctx; _ = peers
    out := make(chan wave.VoteMsg[ItemID], 3)
    out <- wave.VoteMsg[ItemID]{Item: item, Prefer: true}
    out <- wave.VoteMsg[ItemID]{Item: item, Prefer: true}
    out <- wave.VoteMsg[ItemID]{Item: item, Prefer: true}
    close(out)
    return out, nil
}

func main() {
    cfg := config.DefaultParams()
    peers := []types.NodeID{"n1","n2","n3","n4","n5"}
    sel := prism.New(peers, prism.DefaultOptions())
    tx := txStub{}
    e := chain.New[ItemID](cfg, sel, tx)

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    id := ItemID("block#1")
    for i := 0; i < 10; i++ {
        e.Tick(ctx, id)
        if st, ok := e.State(id); ok {
            fmt.Printf("prefer=%v conf=%d decided=%v\n", st.Step.Prefer, st.Step.Conf, st.Decided)
            if st.Decided { break }
        }
        time.Sleep(25 * time.Millisecond)
    }
}
EOF

echo "✅ bootstrap done."
