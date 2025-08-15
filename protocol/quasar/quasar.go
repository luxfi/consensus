// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"sync"
)

// QuasarConfig configures the Quasar dual-certificate protocol
type QuasarConfig[T comparable] struct {
	CertThreshold   int
	SkipThreshold   int
	SignatureScheme string
}

// Certificate represents a certificate for an item
type Certificate[T comparable] struct {
	Item      T
	Proof     []T
	Threshold int
}

// Quasar implements dual-certificate overlay
type Quasar[T comparable] struct {
	mu            sync.RWMutex
	certThreshold int
	skipThreshold int
	certificates  map[T]*Certificate[T]
	skipCerts     map[T]*Certificate[T]
	tracked       map[T]bool
}

// NewQuasar creates a new Quasar overlay
func NewQuasar[T comparable](cfg QuasarConfig[T]) (*Quasar[T], error) {
	return &Quasar[T]{
		certThreshold: cfg.CertThreshold,
		skipThreshold: cfg.SkipThreshold,
		certificates:  make(map[T]*Certificate[T]),
		skipCerts:     make(map[T]*Certificate[T]),
		tracked:       make(map[T]bool),
	}, nil
}

// Initialize initializes with genesis
func (q *Quasar[T]) Initialize(genesis T) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	q.tracked[genesis] = true
	// Genesis automatically has certificate
	q.certificates[genesis] = &Certificate[T]{
		Item:      genesis,
		Threshold: 0,
	}
	return nil
}

// Track tracks an item for certificate generation
func (q *Quasar[T]) Track(item T) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	q.tracked[item] = true
	return nil
}

// HasCertificate checks if an item has a certificate
func (q *Quasar[T]) HasCertificate(item T) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	_, exists := q.certificates[item]
	return exists
}

// HasSkipCertificate checks if an item has a skip certificate
func (q *Quasar[T]) HasSkipCertificate(item T) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	_, exists := q.skipCerts[item]
	return exists
}

// GetCertificate returns the certificate for an item
func (q *Quasar[T]) GetCertificate(item T) (*Certificate[T], bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	cert, exists := q.certificates[item]
	return cert, exists
}

// GenerateCertificate attempts to generate a certificate
func (q *Quasar[T]) GenerateCertificate(item T) (*Certificate[T], bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if !q.tracked[item] {
		return nil, false
	}
	
	// Simplified certificate generation
	cert := &Certificate[T]{
		Item:      item,
		Threshold: q.certThreshold,
	}
	
	q.certificates[item] = cert
	return cert, true
}

// CertThreshold returns the certificate threshold
func (q *Quasar[T]) CertThreshold() int {
	return q.certThreshold
}

// SkipThreshold returns the skip certificate threshold
func (q *Quasar[T]) SkipThreshold() int {
	return q.skipThreshold
}

// CertificateCount returns the number of certificates
func (q *Quasar[T]) CertificateCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.certificates)
}

// SkipCertificateCount returns the number of skip certificates
func (q *Quasar[T]) SkipCertificateCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.skipCerts)
}

// HealthCheck checks if quasar is healthy
func (q *Quasar[T]) HealthCheck() error {
	return nil
}