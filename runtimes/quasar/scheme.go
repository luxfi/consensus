// SPDX-License-Identifier: BUSL-1.1
// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/rand"
	rt "github.com/luxfi/consensus/ringtail"
)

// Type aliases for cleaner API
type (
	SecretKey = []byte
	PublicKey = []byte
	Share     = rt.Precomp
	Cert      = []byte
)

const (
	Security = 128 // 128-bit PQ security
)

// KeyGen returns (sk, pk) for validator use.
func KeyGen() (SecretKey, PublicKey, error) {
	// Generate random seed
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, nil, err
	}
	
	return rt.KeyGen(seed)
}

// Precompute generates a share that can be bound later
func Precompute(sk SecretKey) (Share, error) {
	return rt.Precompute(sk)
}

// QuickSign binds a pre-computed share to blockID.
func QuickSign(share Share, blockID [32]byte) ([]byte, error) {
	return rt.QuickSign(share, blockID[:])
}

// QuickVerify single-share (for tx-level checks, optional).
func QuickVerify(pk PublicKey, blockID [32]byte, sig []byte) bool {
	return rt.VerifyShare(pk, blockID[:], sig)
}

// Aggregate combines shares into a certificate
func Aggregate(shares []Share) (Cert, error) {
	return rt.Aggregate(shares)
}

// Verify verifies an aggregate certificate
func Verify(pk PublicKey, blockID [32]byte, cert Cert) bool {
	return rt.VerifyCert(pk, blockID[:], cert)
}