// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pool.go -- the background NonceCert pool.
//
// NonceCert generation is a BACKGROUND validator subprotocol (Pulsar's
// NonceMPC: validators jointly compute w1 = HighBits(w) and BoundaryClear(w)
// over hidden w = A·y shares, then quorum-sign a clearance certificate). It is
// NOT on the block hot path. The hot path only CONSUMES a pre-cleared cert
// from this pool. If the pool is empty, the Pulsar profile fails over to
// Corona; it NEVER generates a nonce inline (that would either block consensus
// on an MPC ceremony or, worse, open the secret residual w).

package pulsar

import (
	"sync"

	pulsarlib "github.com/luxfi/pulsar/ref/go/pkg/pulsar"
)

// NonceCertPool is the consumer interface the hot path sees. A production
// implementation is fed by the background NonceMPC subprotocol; this package
// ships an in-memory FIFO (MemNonceCertPool) for tests and single-process
// deployments. The interface is the seam: consensus owns the pool's lifecycle
// and replenishment; this profile only takes a cert and binds it.
type NonceCertPool interface {
	// Root is the Merkle/commitment root of the current pool contents. It is
	// bound into the session id (PulsarSession.NoncePoolRoot) so the canonical
	// nonce selection is pinned to the exact pool the signers agree on.
	Root() [32]byte
	// Size is the number of unconsumed certs currently available.
	Size() uint64
	// At returns the cert at canonical pool index i (i < Size). The index is
	// chosen by pulsarlib.CanonicalNonceIndex over (sessionID, Root, Size), so
	// no coordinator can grind w1 by choosing among many cleared certs after
	// seeing the message.
	At(i uint64) (pulsarlib.NonceCert, bool)
	// Consume marks the cert at index i as spent so it is never reused across
	// rounds (nonce reuse in a Schnorr/Fiat-Shamir-style scheme leaks the key).
	Consume(i uint64)
}

// MemNonceCertPool is an in-memory, mutex-guarded NonceCertPool. It is the
// reference consumer for tests and for a single coordinator process that has
// already run the background NonceMPC. It does NOT run the MPC itself.
type MemNonceCertPool struct {
	mu       sync.Mutex
	certs    []pulsarlib.NonceCert
	consumed []bool
	root     [32]byte
}

// NewMemNonceCertPool builds a pool from already-cleared NonceCerts and a
// precomputed commitment root. The caller (the background subprotocol) is
// responsible for the root being a sound commitment to certs.
func NewMemNonceCertPool(certs []pulsarlib.NonceCert, root [32]byte) *MemNonceCertPool {
	return &MemNonceCertPool{
		certs:    certs,
		consumed: make([]bool, len(certs)),
		root:     root,
	}
}

// Root implements NonceCertPool.
func (p *MemNonceCertPool) Root() [32]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.root
}

// Size implements NonceCertPool: the count of unconsumed certs.
func (p *MemNonceCertPool) Size() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	var n uint64
	for i := range p.certs {
		if !p.consumed[i] {
			n++
		}
	}
	return n
}

// At implements NonceCertPool over the live (unconsumed) view: index i selects
// the i-th still-available cert in pool order.
func (p *MemNonceCertPool) At(i uint64) (pulsarlib.NonceCert, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var seen uint64
	for idx := range p.certs {
		if p.consumed[idx] {
			continue
		}
		if seen == i {
			return p.certs[idx], true
		}
		seen++
	}
	return pulsarlib.NonceCert{}, false
}

// Consume implements NonceCertPool: marks the i-th available cert spent.
func (p *MemNonceCertPool) Consume(i uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var seen uint64
	for idx := range p.certs {
		if p.consumed[idx] {
			continue
		}
		if seen == i {
			p.consumed[idx] = true
			return
		}
		seen++
	}
}
