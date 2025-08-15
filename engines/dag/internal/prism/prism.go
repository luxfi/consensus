// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package prism implements the sampler (Prism/Cut/Refract pattern)
package prism

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/types"
)

type Sampler[T comparable] interface {
	Sample(ctx context.Context, k int, topic types.Topic) []types.NodeID
	Report(node types.NodeID, probe types.Probe)
	Allow(topic types.Topic) bool
}

type Options struct {
	Horizon  time.Duration
	MinPeers int
	MaxPeers int
	Stake    func(types.NodeID) float64      // optional
	Latency  func(types.NodeID) time.Duration // optional
}

type DefaultSampler struct {
	mu     sync.RWMutex
	peers  []types.NodeID
	opts   Options
	health map[types.NodeID]int // simple CUT score; negative is bad
}

func NewDefault(peers []types.NodeID, opts Options) *DefaultSampler {
	if opts.MinPeers == 0 {
		opts.MinPeers = 8
	}
	if opts.MaxPeers == 0 {
		opts.MaxPeers = 64
	}
	return &DefaultSampler{
		peers:  append([]types.NodeID(nil), peers...),
		opts:   opts,
		health: make(map[types.NodeID]int, len(peers)),
	}
}

func (s *DefaultSampler) Sample(ctx context.Context, k int, topic types.Topic) []types.NodeID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if k <= 0 {
		k = s.opts.MinPeers
	}
	if k > s.opts.MaxPeers {
		k = s.opts.MaxPeers
	}

	// Refract: bias by stake / inverse latency / health
	type score struct {
		id types.NodeID
		w  float64
	}
	var all []score
	for _, id := range s.peers {
		w := 1.0
		if s.opts.Stake != nil {
			w *= s.opts.Stake(id)
		}
		if s.opts.Latency != nil {
			lat := s.opts.Latency(id)
			if lat > 0 {
				w *= 1.0 / (1.0 + float64(lat.Milliseconds()))
			}
		}
		w *= 1.0 + float64(s.health[id])*0.05
		if w <= 0 {
			continue
		}
		all = append(all, score{id, w})
	}
	// simple greedy roulette (fast & deterministic)
	out := make([]types.NodeID, 0, k)
	for i := 0; i < k && len(all) > 0; i++ {
		best := 0
		for j := 1; j < len(all); j++ {
			if all[j].w > all[best].w {
				best = j
			}
		}
		out = append(out, all[best].id)
		// CUT: dampen repeatedly chosen peer
		all[best].w *= 0.5
	}
	return out
}

func (s *DefaultSampler) Report(node types.NodeID, probe types.Probe) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch probe {
	case types.ProbeGood:
		s.health[node]++
	case types.ProbeTimeout, types.ProbeBadSig:
		s.health[node]--
	}
}

func (s *DefaultSampler) Allow(types.Topic) bool { return true }