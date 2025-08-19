package main

import (
    "context"
    "fmt"
    "time"

    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/engines/chain"
    "github.com/luxfi/consensus/photon"
    "github.com/luxfi/consensus/types"
    "github.com/luxfi/consensus/core/wave"
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
    emitter := photon.NewUniformEmitter(peers, photon.DefaultEmitterOptions())
    tx := txStub{}
    e := chain.New[ItemID](cfg, emitter, tx)

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
