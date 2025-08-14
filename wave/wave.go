// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package wave implements the poller + FPC state machine (FPC ON by default)
package wave

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/internal/types"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/prism"
	"github.com/luxfi/consensus/ray"
)

type ItemState[ID comparable] struct {
	Step    ray.Step[ID]
	Decided bool
	Result  types.Decision
	Stage   Stage
	Last    time.Time
}

type Stage uint8

const (
	StageSnowball Stage = iota
	StageFPC
)

type Config struct {
	K       int           // sample size
	Alpha   float64       // success threshold
	Beta    uint32        // confidence target
	Gamma   int           // max inconclusive steps before FPC
	RoundTO time.Duration // timeout for one wave tick
}

type Transport[ID comparable] interface {
	// send vote requests to peers; returns Photons as they arrive
	RequestVotes(ctx context.Context, peers []types.NodeID, item ID) <-chan photon.Photon[ID]
	// local emit (what we vote)
	MakeLocalPhoton(item ID, prefer bool) photon.Photon[ID]
}

type Wave[ID comparable] interface {
	Ingest(ph photon.Photon[ID])         // external votes
	Tick(ctx context.Context, item ID)   // drive one step (snowball or FPC)
	State(item ID) (ItemState[ID], bool)
}

type waveImpl[ID comparable] struct {
	cfg   Config
	sel   prism.Sampler[ID]
	tx    Transport[ID]

	mu    sync.Mutex
	state map[ID]*ItemState[ID]
	skips map[ID]int // inconclusive streak
}

func New[ID comparable](cfg Config, sel prism.Sampler[ID], tx Transport[ID]) Wave[ID] {
	if cfg.K == 0 {
		cfg.K = 20
	}
	if cfg.Alpha == 0 {
		cfg.Alpha = 0.8
	}
	if cfg.Beta == 0 {
		cfg.Beta = 15
	}
	if cfg.Gamma == 0 {
		cfg.Gamma = 3
	}
	if cfg.RoundTO == 0 {
		cfg.RoundTO = 250 * time.Millisecond
	}

	return &waveImpl[ID]{
		cfg:   cfg,
		sel:   sel,
		tx:    tx,
		state: make(map[ID]*ItemState[ID]),
		skips: make(map[ID]int),
	}
}

func (w *waveImpl[ID]) ensure(item ID) *ItemState[ID] {
	if st, ok := w.state[item]; ok {
		return st
	}
	st := &ItemState[ID]{Stage: StageSnowball}
	w.state[item] = st
	return st
}

func (w *waveImpl[ID]) Ingest(ph photon.Photon[ID]) {
	// hook for telemetry / sig checks upstream; sampler feedback
}

func (w *waveImpl[ID]) State(item ID) (ItemState[ID], bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, ok := w.state[item]
	if !ok {
		return ItemState[ID]{}, false
	}
	cp := *st
	return cp, true
}

func (w *waveImpl[ID]) Tick(ctx context.Context, item ID) {
	w.mu.Lock()
	st := w.ensure(item)
	if st.Decided {
		w.mu.Unlock()
		return
	}
	stage := st.Stage
	prev := st.Step
	w.mu.Unlock()

	peers := w.sel.Sample(ctx, w.cfg.K, types.Topic("votes"))
	ch := w.tx.RequestVotes(ctx, peers, item)

	// include our local photon immediately (start with prefer=true for initial round)
	localPrefer := prev.Prefer
	if prev.Conf == 0 && !prev.Prefer {
		localPrefer = true // Initial preference
	}
	local := w.tx.MakeLocalPhoton(item, localPrefer)
	samples := []photon.Photon[ID]{local}

	timer := time.NewTimer(w.cfg.RoundTO)
	defer timer.Stop()

collect:
	for {
		select {
		case <-ctx.Done():
			break collect
		case <-timer.C:
			break collect
		case ph, ok := <-ch:
			if !ok {
				break collect
			}
			samples = append(samples, ph)
			// If we have enough samples, stop collecting
			if len(samples) >= w.cfg.K {
				break collect
			}
		}
	}

	next := ray.Apply[ID](samples, prev, ray.Params{Alpha: w.cfg.Alpha})
	// Debug: log samples count
	_ = len(samples) // samples count available for debugging

	// Decide or escalate
	w.mu.Lock()
	defer w.mu.Unlock()
	st = w.ensure(item)
	st.Step = next
	st.Last = time.Now()

	if next.Conf >= w.cfg.Beta {
		st.Decided = true
		if next.Prefer {
			st.Result = types.DecideAccept
		} else {
			st.Result = types.DecideReject
		}
		return
	}

	if stage == StageSnowball {
		// Track inconclusive rounds (when preference doesn't change)
		if prev.Conf > 0 && samePolarity(prev.Prefer, next.Prefer) && next.Conf <= prev.Conf {
			w.skips[item]++
		} else {
			w.skips[item] = 0
		}
		if w.skips[item] >= w.cfg.Gamma {
			st.Stage = StageFPC
		}
	} else {
		// FPC: on inconclusive, flip by coin (external) — plug later if desired
	}
}

func samePolarity(a, b bool) bool { return a == b }