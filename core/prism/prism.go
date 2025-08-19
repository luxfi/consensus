package prism

import (
    "context"
    "time"

    "github.com/luxfi/consensus/types"
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
