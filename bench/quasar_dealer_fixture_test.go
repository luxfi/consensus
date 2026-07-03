// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quasar_dealer_fixture_test.go — bench-local trusted-dealer keygen.
//
// The PQ-mode benchmarks need a fully-keyed quasar.SignerConfig (BLS +
// Corona threshold shares) to drive real threshold signing. quasar's own
// trusted-dealer helper (GenerateDualKeys) is DELIBERATELY package-private
// and test-only (protocol/quasar/keygen_testsupport_test.go, H-1
// corona-genesis hardening) so that NO production code can route
// epoch/genesis keying through a trusted dealer — production genesis runs
// the dealerless Pedersen DKG via keyera.Bootstrap.
//
// Go's internal-test import-cycle rule prevents this external package from
// reaching that package-private helper, and re-exporting it into a normal
// .go file would REGRESS the H-1 structural guarantee (production could then
// import it). So the benchmark keeps its own trusted-dealer fixture, kept in
// a _test.go file for the identical reason: trusted-dealer keying stays
// structurally unreachable from every production binary. This is a forced,
// security-preserving copy, not incidental duplication.

package bench

import (
	"context"
	"fmt"

	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/crypto/threshold"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// benchDualKeys builds a (t,n) BLS+Corona threshold SignerConfig via a
// trusted dealer. Bench-only; mirrors quasar's test-only GenerateDualKeys.
func benchDualKeys(t, n int) (*quasar.SignerConfig, error) {
	ctx := context.Background()

	blsScheme, err := threshold.GetScheme(threshold.SchemeBLS)
	if err != nil {
		return nil, fmt.Errorf("get BLS scheme: %w", err)
	}
	blsDealer, err := blsScheme.NewTrustedDealer(threshold.DealerConfig{
		Threshold:    t,
		TotalParties: n,
	})
	if err != nil {
		return nil, fmt.Errorf("new BLS dealer: %w", err)
	}
	blsShares, blsGroupKey, err := blsDealer.GenerateShares(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate BLS shares: %w", err)
	}

	coronaShares, coronaGroupKey, err := coronaThreshold.GenerateKeys(t, n, nil)
	if err != nil {
		return nil, fmt.Errorf("generate Corona shares: %w", err)
	}

	blsShareMap := make(map[string]threshold.KeyShare, n)
	coronaShareMap := make(map[string]*coronaThreshold.KeyShare, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("v%d", i)
		blsShareMap[id] = blsShares[i]
		coronaShareMap[id] = coronaShares[i]
	}

	return &quasar.SignerConfig{
		Threshold:      t,
		TotalParties:   n,
		BLSKeyShares:   blsShareMap,
		BLSGroupKey:    blsGroupKey,
		CoronaShares:   coronaShareMap,
		CoronaGroupKey: coronaGroupKey,
	}, nil
}
