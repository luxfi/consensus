// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// lock_invariant.go — the UNLOCK-BEFORE-CALL-OUT invariant made enforceable.
//
// THE RULE (see the Transitive.mu doc): never call ReceiveVote, VM.Accept, a network send, poll
// delivery, or any callback while holding Transitive.mu. Transitive.mu is a sync.RWMutex, which is
// NOT reentrant — a call-out that reenters the engine and takes mu.RLock while the caller holds
// mu.Lock self-deadlocks that goroutine forever (the live single-validator freeze: the selfVoter
// called engine.ReceiveVote from RequestVotes under mu). Network sends under the engine lock are
// banned for the same class of reason. This file gives the mutex a test-time instrument so a
// runtime test can PROVE no call-out runs under the write lock, turning the rule from a comment
// into a checked invariant.
package chain

import (
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
)

// lockInstrumentEnabled turns on write-holder tracking on engineMutex. It is OFF in production:
// every Lock/Unlock then pays only a single relaxed atomic load (no goroutine-id work). Tests set
// it to assert the unlock-before-call-out invariant (TestBlue_NoCallOutUnderEngineLock).
var lockInstrumentEnabled atomic.Bool

// engineMutex is the type of Transitive.mu: a sync.RWMutex that, WHEN INSTRUMENTED, records the
// goroutine holding the WRITE lock so a call-out can be checked for "am I running under the engine
// lock?" Read locks are not tracked — the demonstrated + primary deadlock is a call-out taking
// mu.RLock while the caller holds mu.Lock, so the write-holder is the safety-relevant state.
type engineMutex struct {
	sync.RWMutex
	writer atomic.Int64 // goID of the current write-lock holder (0 = none); set only while instrumented
}

// Lock overrides sync.RWMutex.Lock to record the write-holder goroutine when instrumented.
func (m *engineMutex) Lock() {
	m.RWMutex.Lock()
	if lockInstrumentEnabled.Load() {
		m.writer.Store(goID())
	}
}

// Unlock overrides sync.RWMutex.Unlock to clear the write-holder when instrumented.
func (m *engineMutex) Unlock() {
	if lockInstrumentEnabled.Load() {
		m.writer.Store(0)
	}
	m.RWMutex.Unlock()
}

// heldForWriteByCurrentGoroutine reports whether THIS goroutine currently holds the write lock —
// the condition under which a call-out must never be invoked. Always false when not instrumented.
func (m *engineMutex) heldForWriteByCurrentGoroutine() bool {
	return lockInstrumentEnabled.Load() && m.writer.Load() == goID()
}

// goID returns the current goroutine's numeric id by parsing the runtime stack header
// ("goroutine <id> [running]:"). This is a TEST/DEBUG instrument only — it is never called on the
// production hot path (Lock/Unlock gate the call on lockInstrumentEnabled). The tiny fixed buffer
// is always large enough for the header line.
func goID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := buf[:n]
	const prefix = "goroutine "
	i := len(prefix)
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	id, _ := strconv.ParseInt(string(s[i:j]), 10, 64)
	return id
}
