// Copyright (C) 2025, Lux Industries Inc All rights reserved.

// Package nebula provides DAG consensus mode.
//
// Nebula implements directed acyclic graph (DAG) consensus where vertices can
// have multiple parents and progress in parallel. It wraps the field engine
// to provide DAG-specific semantics: frontier tracking, vertex proposal, and
// parallel finality.
//
// In the cosmic metaphor, a nebula is a stellar nursery - a cloud of possibilities
// collapsing into ordered structure, representing parallel block creation that
// eventually achieves consensus ordering.
//
// Nebula is designed for high-throughput chains that benefit from parallel
// block production, such as the X-Chain for asset transfers.
//
// Key concepts:
//   - DAG structure: vertices can have multiple parents
//   - Parallel finality: multiple vertices can be finalized concurrently
//   - Frontier tracking: maintains the DAG tips (unfinalized vertices)
//   - Causal ordering: ensures consistent total ordering of finalized vertices
//
// Usage:
//
//	cfg := nebula.Config{
//	    PollSize:   20,
//	    Alpha:      0.8,
//	    Beta:       15,
//	    RoundTO:    250 * time.Millisecond,
//	}
//	n := nebula.NewNebula(cfg, cut, transport, store, proposer, committer)
//	n.Start(ctx)
//
// See also: field (underlying engine), wave (voting primitive), horizon (finality).
package nebula
