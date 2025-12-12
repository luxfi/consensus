// Copyright (C) 2025, Lux Industries Inc All rights reserved.

// Package chain provides basic blockchain primitives and interfaces.
//
// This package defines the fundamental Block interface and common chain
// operations used across linear consensus implementations. It serves as
// the foundation for chain-based consensus modes like Nova.
//
// Key types:
//   - Block: interface for blockchain blocks
//   - ChainState: state tracking for linear chains
//   - BlockID: unique block identifier type
//
// The chain package is intentionally minimal, providing only the essential
// primitives needed by other consensus packages. Higher-level functionality
// is provided by packages like nova (linear consensus) and ray (linear driver).
//
// See also: nova (linear consensus mode), ray (linear consensus driver).
package chain
