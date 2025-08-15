// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package pq provides the post-quantum dual-certificate overlay engine.
// It can wrap either chain or DAG engines with quantum-resistant certificates.
package pq

import (
	"fmt"

	"github.com/luxfi/consensus/protocol/quasar"
)

// Engine represents a consensus engine that can be wrapped with dual certificates.
type Engine[T comparable] interface {
	Initialize(genesis T) error
	Add(item T) error
	Preferred() T
	Finalized() []T
	IsFinalized(item T) bool
}

// DualCertEngine wraps a consensus engine with dual-certificate overlay.
type DualCertEngine[T comparable] struct {
	base   Engine[T]
	quasar *quasar.Quasar[T]
}

// Config configures the dual-certificate engine.
type Config[T comparable] struct {
	// Base engine to wrap
	BaseEngine Engine[T]
	// Certificate threshold
	CertThreshold int
	// Skip threshold for fast path
	SkipThreshold int
	// Quantum-resistant signature scheme
	SignatureScheme string
}

// New creates a new dual-certificate engine wrapping a base engine.
func New[T comparable](cfg Config[T]) (*DualCertEngine[T], error) {
	if cfg.BaseEngine == nil {
		return nil, fmt.Errorf("base engine required")
	}
	
	if cfg.CertThreshold <= 0 {
		return nil, fmt.Errorf("invalid certificate threshold: %d", cfg.CertThreshold)
	}

	// Create quasar overlay
	quasarConfig := quasar.Config[T]{
		CertThreshold:   cfg.CertThreshold,
		SkipThreshold:   cfg.SkipThreshold,
		SignatureScheme: cfg.SignatureScheme,
	}

	q, err := quasar.New(quasarConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create quasar overlay: %w", err)
	}

	return &DualCertEngine[T]{
		base:   cfg.BaseEngine,
		quasar: q,
	}, nil
}

// Initialize initializes both the base engine and quasar overlay.
func (e *DualCertEngine[T]) Initialize(genesis T) error {
	// Initialize base engine
	if err := e.base.Initialize(genesis); err != nil {
		return fmt.Errorf("failed to initialize base engine: %w", err)
	}

	// Initialize quasar with genesis
	if err := e.quasar.Initialize(genesis); err != nil {
		return fmt.Errorf("failed to initialize quasar: %w", err)
	}

	return nil
}

// Add adds an item with dual-certificate tracking.
func (e *DualCertEngine[T]) Add(item T) error {
	// Add to base engine first
	if err := e.base.Add(item); err != nil {
		return fmt.Errorf("failed to add to base engine: %w", err)
	}

	// Track in quasar for certificate generation
	if err := e.quasar.Track(item); err != nil {
		return fmt.Errorf("failed to track in quasar: %w", err)
	}

	// Check if we can generate certificates
	e.tryGenerateCertificates()

	return nil
}

// Preferred returns the preferred item from the base engine.
func (e *DualCertEngine[T]) Preferred() T {
	return e.base.Preferred()
}

// Finalized returns items that are both base-finalized and have certificates.
func (e *DualCertEngine[T]) Finalized() []T {
	baseFinalized := e.base.Finalized()
	
	// Filter for items that also have certificates
	var certified []T
	for _, item := range baseFinalized {
		if e.quasar.HasCertificate(item) {
			certified = append(certified, item)
		}
	}
	
	return certified
}

// IsFinalized checks if an item is finalized with certificates.
func (e *DualCertEngine[T]) IsFinalized(item T) bool {
	// Must be finalized in base engine
	if !e.base.IsFinalized(item) {
		return false
	}
	
	// Must have certificate
	return e.quasar.HasCertificate(item)
}

// HasCertificate checks if an item has a certificate.
func (e *DualCertEngine[T]) HasCertificate(item T) bool {
	return e.quasar.HasCertificate(item)
}

// HasSkipCertificate checks if an item has a skip certificate.
func (e *DualCertEngine[T]) HasSkipCertificate(item T) bool {
	return e.quasar.HasSkipCertificate(item)
}

// GetCertificate returns the certificate for an item if it exists.
func (e *DualCertEngine[T]) GetCertificate(item T) (*quasar.Certificate[T], bool) {
	return e.quasar.GetCertificate(item)
}

// tryGenerateCertificates attempts to generate certificates for eligible items.
func (e *DualCertEngine[T]) tryGenerateCertificates() {
	// Get base-finalized items
	finalized := e.base.Finalized()
	
	for _, item := range finalized {
		// Skip if already has certificate
		if e.quasar.HasCertificate(item) {
			continue
		}
		
		// Try to generate certificate
		if cert, ok := e.quasar.GenerateCertificate(item); ok {
			// Certificate generated successfully
			_ = cert // Use certificate as needed
		}
	}
}

// Metrics returns combined metrics from base engine and quasar.
func (e *DualCertEngine[T]) Metrics() map[string]interface{} {
	metrics := map[string]interface{}{
		"type":           "dual-cert",
		"certThreshold":  e.quasar.CertThreshold(),
		"skipThreshold":  e.quasar.SkipThreshold(),
		"certificates":   e.quasar.CertificateCount(),
		"skipCerts":      e.quasar.SkipCertificateCount(),
	}
	
	// Add base engine metrics if available
	if baseMetrics, ok := e.base.(interface{ Metrics() map[string]interface{} }); ok {
		baseMap := baseMetrics.Metrics()
		for k, v := range baseMap {
			metrics["base_"+k] = v
		}
	}
	
	return metrics
}

// HealthCheck checks health of both base engine and quasar.
func (e *DualCertEngine[T]) HealthCheck() error {
	// Check base engine health if supported
	if healthChecker, ok := e.base.(interface{ HealthCheck() error }); ok {
		if err := healthChecker.HealthCheck(); err != nil {
			return fmt.Errorf("base engine unhealthy: %w", err)
		}
	}
	
	// Check quasar health
	if err := e.quasar.HealthCheck(); err != nil {
		return fmt.Errorf("quasar unhealthy: %w", err)
	}
	
	return nil
}