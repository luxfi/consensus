// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Async quantum bundle production - runs Ringtail signing without blocking BLS.
//
// Architecture (parallel execution):
//   BLS Layer:     [B1]--[B2]--[B3]--[B4]--[B5]--[B6]--[B7]--[B8]--[B9]--...
//                   |     500ms finality per block     |
//                   |_____________________________________|
//                                    |
//   Quantum Layer:              [QB1: Merkle(B1-B6)]--------[QB2: Merkle(B7-B12)]
//                                    |  3-second interval, async Ringtail signing
//
// NTT Ringtail benchmarks (IEEE S&P 2025):
//   - 0.6s online signing phase (2-round protocol)
//   - 2.5s total including offline prep across 5 continents
//   - Our 3-second interval provides comfortable margin

package quasar

import (
	"sync"
	"time"
)

// AsyncBundleSigner wraps BundleSigner with async signing capabilities.
// BLS block production continues uninterrupted during Ringtail signing.
type AsyncBundleSigner struct {
	signer *BundleSigner

	// Async signing state
	signingInProgress bool
	signedBundles     chan *QuantumBundle

	mu sync.Mutex
}

// NewAsyncBundleSigner creates an async bundle signer.
func NewAsyncBundleSigner(em *EpochManager) *AsyncBundleSigner {
	return &AsyncBundleSigner{
		signer:        NewBundleSigner(em),
		signedBundles: make(chan *QuantumBundle, 10),
	}
}

// AddBLSBlock adds a finalized BLS block hash to the pending bundle.
func (abs *AsyncBundleSigner) AddBLSBlock(height uint64, hash [32]byte) {
	abs.signer.AddBLSBlock(height, hash)
}

// PendingCount returns the number of pending BLS blocks.
func (abs *AsyncBundleSigner) PendingCount() int {
	return abs.signer.PendingCount()
}

// CreateBundle bundles pending BLS blocks.
func (abs *AsyncBundleSigner) CreateBundle() *QuantumBundle {
	return abs.signer.CreateBundle()
}

// SignBundle signs a bundle synchronously.
func (abs *AsyncBundleSigner) SignBundle(
	bundle *QuantumBundle,
	sessionID int,
	prfKey []byte,
	validators []string,
) error {
	return abs.signer.SignBundle(bundle, sessionID, prfKey, validators)
}

// SignBundleAsync signs a bundle asynchronously.
// BLS block production continues uninterrupted during signing.
func (abs *AsyncBundleSigner) SignBundleAsync(
	bundle *QuantumBundle,
	sessionID int,
	prfKey []byte,
	validators []string,
) {
	go func() {
		abs.mu.Lock()
		abs.signingInProgress = true
		abs.mu.Unlock()

		defer func() {
			abs.mu.Lock()
			abs.signingInProgress = false
			abs.mu.Unlock()
		}()

		err := abs.signer.SignBundle(bundle, sessionID, prfKey, validators)
		if err != nil {
			return
		}

		abs.signedBundles <- bundle
	}()
}

// SignedBundles returns the channel of signed bundles.
func (abs *AsyncBundleSigner) SignedBundles() <-chan *QuantumBundle {
	return abs.signedBundles
}

// IsSigningInProgress returns true if async signing is in progress.
func (abs *AsyncBundleSigner) IsSigningInProgress() bool {
	abs.mu.Lock()
	defer abs.mu.Unlock()
	return abs.signingInProgress
}

// VerifyBundle verifies a bundle's signature.
func (abs *AsyncBundleSigner) VerifyBundle(bundle *QuantumBundle) bool {
	return abs.signer.VerifyBundle(bundle)
}

// LastBundle returns the most recent bundle.
func (abs *AsyncBundleSigner) LastBundle() *QuantumBundle {
	return abs.signer.LastBundle()
}

// ============================================================================
// BundleRunner - Automated quantum bundle production
// ============================================================================

// BundleRunner runs the quantum bundle production loop.
// It creates bundles every 3 seconds and signs them asynchronously.
type BundleRunner struct {
	signer     *AsyncBundleSigner
	validators []string
	prfKey     []byte
	sessionID  int

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewBundleRunner creates a new bundle runner.
func NewBundleRunner(signer *AsyncBundleSigner, validators []string, prfKey []byte) *BundleRunner {
	return &BundleRunner{
		signer:     signer,
		validators: validators,
		prfKey:     prfKey,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// Start begins the bundle production loop.
// Bundles are created every 3 seconds, signed asynchronously.
func (br *BundleRunner) Start() {
	go br.run()
}

// Stop stops the bundle production loop.
func (br *BundleRunner) Stop() {
	close(br.stopCh)
	<-br.doneCh
}

func (br *BundleRunner) run() {
	defer close(br.doneCh)

	ticker := time.NewTicker(QuantumCheckpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-br.stopCh:
			return
		case <-ticker.C:
			// Create bundle from pending BLS blocks
			bundle := br.signer.CreateBundle()
			if bundle == nil {
				continue // No pending blocks
			}

			// Sign asynchronously (doesn't block BLS production)
			br.sessionID++
			br.signer.SignBundleAsync(bundle, br.sessionID, br.prfKey, br.validators)
		}
	}
}
