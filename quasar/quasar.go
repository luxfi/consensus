// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package quasar implements dual-certificate PQ finality overlay (BLS + Ringtail)
package quasar

import (
	"context"
	"sync"

	"github.com/luxfi/consensus/types"
)

// Verifier handles dual certificate creation and verification
type Verifier interface {
	Attach(ctx context.Context, b any) error                  // fill CertBundle on propose
	Verify(ctx context.Context, bundle types.CertBundle) bool // BOTH BLS + RT must pass
}

// QuasarConfig holds configuration for the Quasar overlay
type QuasarConfig struct {
	Enable     bool `json:"enable" yaml:"enable"`
	Precompute int  `json:"precompute" yaml:"precompute"`
	Threshold  int  `json:"threshold" yaml:"threshold"`
}

// Engine implements the dual-certificate overlay
type Engine struct {
	mu         sync.RWMutex
	cfg        QuasarConfig
	blsAgg     *Aggregator
	rtAgg      *Aggregator
	validators map[types.NodeID]*ValidatorKeys
}

// ValidatorKeys holds both BLS and Ringtail keys for a validator
type ValidatorKeys struct {
	BLSPublicKey *PublicKey
	RTPublicKey  *PublicKey
}

// New creates a new Quasar engine
func New(cfg QuasarConfig) *Engine {
	return &Engine{
		cfg:        cfg,
		blsAgg:     bls.NewAggregator(),
		rtAgg:      ringtail.NewAggregator(),
		validators: make(map[types.NodeID]*ValidatorKeys),
	}
}

// Attach fills the CertBundle on a block/vertex proposal
func (e *Engine) Attach(ctx context.Context, b any) error {
	if !e.cfg.Enable {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Get block/vertex data
	data, err := e.extractData(b)
	if err != nil {
		return err
	}

	// Create BLS aggregate signature
	blsSig := e.blsAgg.CreateAggregate(data, e.getValidatorBLSKeys())

	// Create Ringtail aggregate signature
	rtSig := e.rtAgg.CreateAggregate(data, e.getValidatorRTKeys())

	// Attach certificate bundle
	return e.attachBundle(b, types.CertBundle{
		BLSAgg: blsSig,
		RTCert: rtSig,
	})
}

// Verify checks that BOTH BLS and Ringtail certificates are valid
func (e *Engine) Verify(ctx context.Context, bundle types.CertBundle) bool {
	if !e.cfg.Enable {
		return true // Pass through if disabled
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// TODO: Extract data from context or bundle metadata
	data := []byte("placeholder-data")
	
	// Verify BLS aggregate
	if !e.blsAgg.VerifyAggregate(data, bundle.BLSAgg, e.getValidatorBLSKeys()) {
		return false
	}

	// Verify Ringtail aggregate
	if !e.rtAgg.VerifyAggregate(data, bundle.RTCert, e.getValidatorRTKeys()) {
		return false
	}

	return true
}

// AddValidator adds a validator with their keys
func (e *Engine) AddValidator(id types.NodeID, blsKey *PublicKey, rtKey *PublicKey) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.validators[id] = &ValidatorKeys{
		BLSPublicKey: blsKey,
		RTPublicKey:  rtKey,
	}
}

// RemoveValidator removes a validator
func (e *Engine) RemoveValidator(id types.NodeID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	delete(e.validators, id)
}

// extractData extracts the data to sign from a block or vertex
func (e *Engine) extractData(b any) ([]byte, error) {
	switch v := b.(type) {
	case interface{ Bytes() []byte }:
		return v.Bytes(), nil
	default:
		// Handle other types as needed
		return nil, nil
	}
}

// attachBundle attaches a certificate bundle to a block or vertex
func (e *Engine) attachBundle(b any, bundle types.CertBundle) error {
	switch v := b.(type) {
	case interface{ SetCertBundle(types.CertBundle) }:
		v.SetCertBundle(bundle)
		return nil
	default:
		// Handle other types as needed
		return nil
	}
}

// getValidatorBLSKeys returns all validator BLS public keys
func (e *Engine) getValidatorBLSKeys() []*PublicKey {
	keys := make([]*PublicKey, 0, len(e.validators))
	for _, v := range e.validators {
		if v.BLSPublicKey != nil {
			keys = append(keys, v.BLSPublicKey)
		}
	}
	return keys
}

// getValidatorRTKeys returns all validator Ringtail public keys
func (e *Engine) getValidatorRTKeys() []*PublicKey {
	keys := make([]*PublicKey, 0, len(e.validators))
	for _, v := range e.validators {
		if v.RTPublicKey != nil {
			keys = append(keys, v.RTPublicKey)
		}
	}
	return keys
}

// GetThreshold returns the configured threshold
func (e *Engine) GetThreshold() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if e.cfg.Threshold > 0 {
		return e.cfg.Threshold
	}
	// Default to 2/3 of validators
	return (len(e.validators)*2 + 2) / 3
}