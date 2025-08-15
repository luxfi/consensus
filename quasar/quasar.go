// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package quasar implements dual-certificate PQ finality overlay (BLS + Corona)
package quasar

import (
	"context"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/types"
	// "github.com/luxfi/crypto/bls"
	// "github.com/luxfi/crypto/corona"
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
	blsAgg     *bls.Aggregator
	rtAgg      *corona.Aggregator
	validators map[types.NodeID]*ValidatorKeys
}

// ValidatorKeys holds both BLS and Corona keys for a validator
type ValidatorKeys struct {
	BLSPublicKey *bls.PublicKey
	RTPublicKey  *corona.PublicKey
}

// New creates a new Quasar engine
func New(cfg QuasarConfig) *Engine {
	return &Engine{
		cfg:        cfg,
		blsAgg:     bls.NewAggregator(),
		rtAgg:      corona.NewAggregator(),
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
	blsSig, err := e.blsAgg.CreateAggregate(data, e.getValidatorBLSKeys())
	if err != nil {
		return err
	}

	// Create Corona aggregate signature
	rtSig, err := e.rtAgg.CreateAggregate(data, e.getValidatorRTKeys())
	if err != nil {
		return err
	}

	// Attach certificate bundle
	return e.attachBundle(b, types.CertBundle{
		BLSAgg: blsSig,
		RTCert: rtSig,
	})
}

// Verify checks that BOTH BLS and Corona certificates are valid
func (e *Engine) Verify(ctx context.Context, bundle types.CertBundle) bool {
	if !e.cfg.Enable {
		return true // Pass through if disabled
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// Verify BLS aggregate
	if !e.blsAgg.VerifyAggregate(bundle.BLSAgg, e.getValidatorBLSKeys()) {
		return false
	}

	// Verify Corona aggregate
	if !e.rtAgg.VerifyAggregate(bundle.RTCert, e.getValidatorRTKeys()) {
		return false
	}

	return true
}

// AddValidator adds a validator with their keys
func (e *Engine) AddValidator(id types.NodeID, blsKey *bls.PublicKey, rtKey *corona.PublicKey) {
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
func (e *Engine) getValidatorBLSKeys() []*bls.PublicKey {
	keys := make([]*bls.PublicKey, 0, len(e.validators))
	for _, v := range e.validators {
		if v.BLSPublicKey != nil {
			keys = append(keys, v.BLSPublicKey)
		}
	}
	return keys
}

// getValidatorRTKeys returns all validator Corona public keys
func (e *Engine) getValidatorRTKeys() []*corona.PublicKey {
	keys := make([]*corona.PublicKey, 0, len(e.validators))
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