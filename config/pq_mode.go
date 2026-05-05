// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"fmt"
	"os"
	"strings"
)

// PQMode selects which post-quantum signature paths the consensus engine
// runs alongside BLS. All modes always include BLS12-381 as the classical
// fast path; PQ layers stack additively on top.
//
//	PQModeBLSOnly         -- classical BLS aggregate only (48 B/cert)
//	PQModeBLSPlusMLDSA    -- BLS + per-validator ML-DSA-65 (n*3309 B in cert)
//	PQModeBLSPlusRingtail -- BLS + Ringtail threshold (O(1) PQ in cert)
//	PQModeBLSPlusGroth16  -- BLS + 192 B Groth16 ML-DSA rollup (Z-Chain)
//	PQModeTripleQuantum   -- BLS + Ringtail + ML-DSA (defense in depth)
type PQMode uint8

const (
	// PQModeBLSOnly is the classical fast path: 48 byte BLS aggregate.
	PQModeBLSOnly PQMode = iota

	// PQModeBLSPlusMLDSA pairs BLS with per-validator ML-DSA-65 sigs.
	// Cert size: 48 + N*3309 B (linear in validator count).
	PQModeBLSPlusMLDSA

	// PQModeBLSPlusRingtail pairs BLS with the Ringtail (Ring-LWE) threshold
	// signature. Cert size constant in N after DKG.
	PQModeBLSPlusRingtail

	// PQModeBLSPlusGroth16 pairs BLS with the Z-Chain Groth16 rollup of N
	// ML-DSA-65 identity sigs (~192 B). Pending Z-Chain witness wiring.
	PQModeBLSPlusGroth16

	// PQModeTripleQuantum runs BLS + Ringtail + ML-DSA in parallel.
	// All three layers must verify for finality.
	PQModeTripleQuantum
)

// String returns the canonical name for a PQMode.
func (m PQMode) String() string {
	switch m {
	case PQModeBLSOnly:
		return "bls"
	case PQModeBLSPlusMLDSA:
		return "bls-mldsa"
	case PQModeBLSPlusRingtail:
		return "bls-rt"
	case PQModeBLSPlusGroth16:
		return "bls-z"
	case PQModeTripleQuantum:
		return "triple"
	default:
		return fmt.Sprintf("pq-mode(%d)", uint8(m))
	}
}

// PolicyID maps a PQMode to the canonical wire policy ID.
//
// We avoid importing pkg/wire here (config has no upstream deps); callers
// translate via the integer values:
//
//	PQModeBLSOnly         -> 1 (PolicyQuorum)
//	PQModeBLSPlusMLDSA    -> 6 (PolicyPZ, P+Z witness set)
//	PQModeBLSPlusRingtail -> 5 (PolicyPQ, P+Q witness set)
//	PQModeBLSPlusGroth16  -> 6 (PolicyPZ -- the Z-Chain Groth16 witness)
//	PQModeTripleQuantum   -> 4 (PolicyQuantum, P+Q+Z witness set)
func (m PQMode) PolicyID() uint16 {
	switch m {
	case PQModeBLSOnly:
		return 1 // PolicyQuorum
	case PQModeBLSPlusMLDSA, PQModeBLSPlusGroth16:
		return 6 // PolicyPZ
	case PQModeBLSPlusRingtail:
		return 5 // PolicyPQ
	case PQModeTripleQuantum:
		return 4 // PolicyQuantum
	default:
		return 1
	}
}

// ParsePQMode parses a canonical mode string (case-insensitive). Aliases:
//
//	"bls"             -> PQModeBLSOnly
//	"bls-mldsa"       -> PQModeBLSPlusMLDSA
//	"bls-rt", "bls-q" -> PQModeBLSPlusRingtail
//	"bls-z", "bls-groth16", "bls-zk" -> PQModeBLSPlusGroth16
//	"triple", "quasar"-> PQModeTripleQuantum
func ParsePQMode(s string) (PQMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "bls", "bls-only", "classical":
		return PQModeBLSOnly, nil
	case "bls-mldsa", "mldsa", "bls+mldsa":
		return PQModeBLSPlusMLDSA, nil
	case "bls-rt", "bls-q", "rt", "ringtail", "bls+ringtail":
		return PQModeBLSPlusRingtail, nil
	case "bls-z", "bls-groth16", "bls-zk", "groth16":
		return PQModeBLSPlusGroth16, nil
	case "triple", "quasar", "triple-quantum", "bls-rt-mldsa":
		return PQModeTripleQuantum, nil
	default:
		return PQModeBLSOnly, fmt.Errorf("unknown PQMode %q", s)
	}
}

// PQModeFromEnv reads LUX_CONSENSUS_PQ_MODE and returns the resolved mode.
// Empty / unset returns the supplied default. Invalid values return def + error.
func PQModeFromEnv(def PQMode) (PQMode, error) {
	v := os.Getenv("LUX_CONSENSUS_PQ_MODE")
	if v == "" {
		return def, nil
	}
	mode, err := ParsePQMode(v)
	if err != nil {
		return def, err
	}
	return mode, nil
}
