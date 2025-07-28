// SPDX-License-Identifier: BUSL-1.1
// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/sha256"
	"sync"
	"time"
)

// Aggregator collects Ringtail shares → certificate once quorum reached.
type Aggregator struct {
	mu       sync.Mutex
	quorum   int
	shares   map[[32]byte][]Share // blockID ⇒ shares
	outbox   chan Cert            // sealed certs
	timeout  time.Duration
}

func NewAggregator(quorum int, d time.Duration) *Aggregator {
	return &Aggregator{
		quorum:  quorum,
		shares:  make(map[[32]byte][]Share),
		outbox:  make(chan Cert, 32),
		timeout: d,
	}
}

func (a *Aggregator) Add(blockID [32]byte, s Share) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.shares[blockID] = append(a.shares[blockID], s)
	if len(a.shares[blockID]) >= a.quorum {
		cert, _ := Aggregate(a.shares[blockID])
		a.outbox <- cert
		delete(a.shares, blockID) // free mem
	}
}

// Certs returns read-only channel of sealed certificates.
func (a *Aggregator) Certs() <-chan Cert { return a.outbox }

// Hash returns the SHA-256 of the serialized cert (for header field).
func Hash(c Cert) [32]byte { return sha256.Sum256(c) }

// PendingCount returns the number of blocks waiting for certificates.
func (a *Aggregator) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.shares)
}

// Clear removes shares for a specific block (e.g., if block is rejected).
func (a *Aggregator) Clear(blockID [32]byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.shares, blockID)
}

// GarbageCollect removes old pending shares after timeout.
func (a *Aggregator) GarbageCollect() {
	ticker := time.NewTicker(a.timeout)
	defer ticker.Stop()

	for range ticker.C {
		a.mu.Lock()
		// In production, track timestamps per block
		// For now, just check size bounds
		if len(a.shares) > 100 {
			// Clear oldest entries
			for k := range a.shares {
				delete(a.shares, k)
				if len(a.shares) <= 50 {
					break
				}
			}
		}
		a.mu.Unlock()
	}
}