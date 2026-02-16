// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

/*
Package consensus provides the Quasar family of consensus protocols for the
Lux blockchain network.

# Overview

The consensus system supports two modes -- linear chain (Nova, for P-Chain and
C-Chain) and DAG (Nebula, for X-Chain) -- with optional post-quantum finality
via BLS + Ringtail + ML-DSA threshold signing.

# Sub-Protocols

The Quasar family comprises:

  - photon:  K-of-N committee selection via Fisher-Yates with luminance weighting
  - wave:    Per-round threshold voting with FPC (Fast Probabilistic Consensus)
  - focus:   Confidence accumulation (beta consecutive successes = finality)
  - nova:    Linear chain consensus mode (wraps ray)
  - nebula:  DAG consensus mode (wraps field)
  - prism:   DAG geometry: frontiers, cuts, uniform peer sampling
  - horizon: DAG order-theory: reachability, LCA, transitive closure
  - flare:   DAG certificate/skip detection (2f+1 quorum)
  - ray:     Linear chain finality driver
  - field:   DAG finality driver with safe-prefix commit
  - quasar:  BLS + Ringtail + ML-DSA parallel threshold signing

# Usage

	import "github.com/luxfi/consensus"

	chain := consensus.NewChain(consensus.DefaultConfig())
	if err := chain.Start(ctx); err != nil {
	    log.Fatal(err)
	}
	defer chain.Stop()

# Signing Modes

Each cryptographic layer is independently toggleable:

  - BLS-only: fastest classical consensus (BLS12-381 threshold)
  - BLS + ML-DSA: dual with PQ identity proof (FIPS 204)
  - BLS + Ringtail: dual with PQ threshold proof (Ring-LWE)
  - BLS + Ringtail + ML-DSA: full Quasar (all three in parallel)

# Testing

	go test ./...
	go test -bench=. ./bench/
*/
package consensus
