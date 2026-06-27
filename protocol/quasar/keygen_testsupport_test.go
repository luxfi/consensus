// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

// keygen_testsupport_test.go — trusted-dealer keygen helpers, TEST ONLY.
//
// These build a SignerConfig / threshold shares by sampling the full secret in
// a single process (trusted dealer). That is fine for in-process test fixtures
// but is NOT the production keying path: production chain genesis runs the
// dealerless Pedersen DKG via keyera.Bootstrap (EpochManager.InitializeEpoch in
// epoch.go). These helpers were relocated out of quasar.go into this _test.go
// file so that NO non-test code in the consensus engine can route epoch/genesis
// keying through a trusted dealer (H-1 / corona-genesis hardening). All callers
// are package-quasar tests.

import (
	"context"
	"fmt"

	"github.com/luxfi/crypto/threshold"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// GenerateDualKeys generates both BLS and Corona threshold keys for an epoch.
// TEST-ONLY trusted-dealer keygen; production uses keyera.Bootstrap.
func GenerateDualKeys(t, n int) (*SignerConfig, error) {
	ctx := context.Background()

	// Generate BLS threshold keys
	blsScheme, err := threshold.GetScheme(threshold.SchemeBLS)
	if err != nil {
		return nil, fmt.Errorf("failed to get BLS scheme: %w", err)
	}

	blsDealer, err := blsScheme.NewTrustedDealer(threshold.DealerConfig{
		Threshold:    t,
		TotalParties: n,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create BLS dealer: %w", err)
	}

	blsShares, blsGroupKey, err := blsDealer.GenerateShares(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate BLS shares: %w", err)
	}

	// Generate Corona threshold keys (native)
	coronaShares, coronaGroupKey, err := coronaThreshold.GenerateKeys(t, n, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Corona shares: %w", err)
	}

	// Convert to maps keyed by validator ID
	blsShareMap := make(map[string]threshold.KeyShare)
	coronaShareMap := make(map[string]*coronaThreshold.KeyShare)

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("v%d", i)
		blsShareMap[id] = blsShares[i]
		coronaShareMap[id] = coronaShares[i]
	}

	return &SignerConfig{
		Threshold:      t,
		TotalParties:   n,
		BLSKeyShares:   blsShareMap,
		BLSGroupKey:    blsGroupKey,
		CoronaShares:   coronaShareMap,
		CoronaGroupKey: coronaGroupKey,
	}, nil
}

// GenerateThresholdKeys generates threshold keys for a single scheme.
// TEST-ONLY trusted-dealer keygen.
func GenerateThresholdKeys(schemeID threshold.SchemeID, t, n int) ([]threshold.KeyShare, threshold.PublicKey, error) {
	scheme, err := threshold.GetScheme(schemeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get scheme: %w", err)
	}

	dealer, err := scheme.NewTrustedDealer(threshold.DealerConfig{
		Threshold:    t,
		TotalParties: n,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dealer: %w", err)
	}

	return dealer.GenerateShares(context.Background())
}

// GenerateDualThresholdKeys generates both BLS and Corona keys. TEST-ONLY.
func GenerateDualThresholdKeys(t, n int) (*SignerConfig, error) {
	return GenerateDualKeys(t, n)
}
