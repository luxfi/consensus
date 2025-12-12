// Copyright (C) 2025, Lux Industries Inc All rights reserved.

// Package nova provides linear blockchain consensus mode.
//
// Nova implements classic linear chain consensus where blocks extend sequentially.
// It wraps the ray engine to provide blockchain-specific semantics: height tracking,
// preference management, and sequential finality.
//
// In the cosmic metaphor, a nova is a stellar explosion - a singular, decisive
// event in linear time, representing the finality of each block in sequence.
//
// Nova is designed for chains that require strict ordering and single-chain
// semantics, such as the P-Chain for platform operations.
//
// Key concepts:
//   - Linear progression: blocks extend a single chain
//   - Sequential finality: one block finalized at a time
//   - Height tracking: maintains current blockchain height
//   - Preference tracking: manages preferred block selection
//
// Usage:
//
//	cfg := nova.Config{
//	    SampleSize: 20,
//	    Alpha:      0.8,
//	    Beta:       15,
//	    RoundTO:    250 * time.Millisecond,
//	}
//	n := nova.NewNova(cfg, cut, transport, source, sink)
//	n.Start(ctx)
//
// See also: ray (underlying engine), wave (voting primitive).
package nova
