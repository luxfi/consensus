// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build race

package chain

// underRace is true when the test binary is built with the race detector. The race
// detector imposes a ~10x, highly variable slowdown on every memory access, which makes
// the in-process sim's gossip latency exceed any bounded convergence settle window — a
// condition that does NOT occur on a real network (where gossip is milliseconds and the
// settle is hundreds of ms). The vote-CONVERGENCE storm gates are therefore skipped under
// -race: their timing assumption (gossip < settle) is violated only by the detector's
// artificial slowdown, not by production. Their SAFETY property (no double-finalization)
// is timing-independent and still asserted in the non-race run, and the convergence
// goroutine's shared-state access is exercised race-free by the multinode proposer/
// liveness suites, which DO run under -race.
const underRace = true
