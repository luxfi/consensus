package wave

import (
    "context"
    "time"

    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/photon"
    "github.com/luxfi/consensus/types"
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
    cfg     config.Parameters
    emitter photon.Emitter[types.NodeID]
    tx      Transport[ID]
    state   map[ID]State[ID]
}

func New[ID comparable](cfg config.Parameters, emitter photon.Emitter[types.NodeID], tx Transport[ID]) *Impl[ID] {
    return &Impl[ID]{cfg: cfg, emitter: emitter, tx: tx, state: make(map[ID]State[ID])}
}

func (w *Impl[ID]) ensure(id ID) State[ID] {
    if st, ok := w.state[id]; ok { return st }
    st := State[ID]{}
    w.state[id] = st
    return st
}

func (w *Impl[ID]) Tick(ctx context.Context, id ID) {
    st := w.ensure(id)
    // Emit K photons to select committee
    peers, _ := w.emitter.Emit(ctx, w.cfg.K, uint64(time.Now().UnixNano()))
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
