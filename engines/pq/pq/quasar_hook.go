// SPDX-License-Identifier: BUSL-1.1
// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"sync/atomic"
	"time"

	"github.com/luxfi/consensus/quasar"
)

// QuasarHook provides post-quantum security overlay for any consensus engine.
type QuasarHook struct {
	pool   *quasar.Pool
	agg    *quasar.Aggregator
	pk     quasar.PublicKey
	quanta uint64 // atomic: last PQ-secured height
}

// QuasarCallbacks defines the interface for integrating Quasar with consensus engines.
type QuasarCallbacks interface {
	// OnProposal is called when proposing a new block
	OnProposal(func(blockID [32]byte) []byte)
	
	// OnShare is called when receiving a Ringtail share
	OnShare(func(blockID [32]byte, share quasar.Share))
	
	// InjectPQCert inserts a completed certificate
	InjectPQCert(cert quasar.Cert, hash [32]byte)
	
	// SetValidator adds header validation rule
	SetValidator(func(blockID [32]byte, quasarSig []byte) bool)
	
	// Height returns current consensus height
	Height() uint64
	
	// MinRoundInterval returns the minimum round duration
	MinRoundInterval() time.Duration
}

// AttachQuasar integrates post-quantum security into any consensus engine.
func AttachQuasar(e QuasarCallbacks, sk quasar.SecretKey, pk quasar.PublicKey, quorum int) *QuasarHook {
	h := &QuasarHook{
		pool: quasar.NewPool(sk, 64), // warm cache of 64 shares
		agg:  quasar.NewAggregator(quorum, e.MinRoundInterval()),
		pk:   pk,
	}
	
	// 1️⃣ Sign as soon as we see a proposal
	e.OnProposal(func(blockID [32]byte) []byte {
		share := h.pool.Get()
		sig, _ := quasar.QuickSign(share, blockID)
		return sig
	})
	
	// 2️⃣ Feed incoming shares
	e.OnShare(func(blockID [32]byte, share quasar.Share) {
		h.agg.Add(blockID, share)
	})
	
	// 3️⃣ Insert cert once aggregated
	go func() {
		for cert := range h.agg.Certs() {
			e.InjectPQCert(cert, quasar.Hash(cert))
			atomic.StoreUint64(&h.quanta, e.Height())
		}
	}()
	
	// 4️⃣ Header validity rule
	e.SetValidator(func(blockID [32]byte, quasarSig []byte) bool {
		return quasar.QuickVerify(h.pk, blockID, quasarSig)
	})
	
	// Start garbage collection
	go h.agg.GarbageCollect()
	
	return h
}

// GetQuantumHeight returns the last height with PQ security.
func (h *QuasarHook) GetQuantumHeight() uint64 {
	return atomic.LoadUint64(&h.quanta)
}

// Stats returns Quasar statistics.
func (h *QuasarHook) Stats() map[string]interface{} {
	return map[string]interface{}{
		"precomputed_shares": h.pool.Available(),
		"pending_certs":      h.agg.PendingCount(),
		"quantum_height":     h.GetQuantumHeight(),
	}
}