// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

// gpu_accel.go — opt corona's R-LWE threshold signing into the
// lattice GPU NTT dispatch path.
//
// The corona/gpu package owns ALL build-tag plumbing for the GPU NTT
// dispatcher. quasar speaks to it through a single function call. On a
// pure-Go build the call is harmless: corona/gpu.MaybeRegister becomes
// a no-op and the existing CPU path runs.
//
// Wired here (rather than in NewSigner) because:
//   1. UseAccelerator() is idempotent and global — once is enough.
//   2. Subsequent coronaThreshold.NewParams calls (which build the
//      rings) consult the global flag via corona/gpu.MaybeRegister and
//      bind their SubRings into the lattice GPU registry.
//   3. Tests that exercise the CPU path do so by calling
//      DisableCoronaGPUAccelerator() in a t.Cleanup hook.

import (
	cgpu "github.com/luxfi/corona/gpu"
)

// UseCoronaGPUAccelerator opts every subsequent coronaThreshold
// signer into the lattice GPU NTT dispatch path. Idempotent. Safe to
// call from package init or from boot configuration before any signer
// is constructed.
//
// Returns the corona/gpu UseAccelerator error verbatim — currently
// always nil; reserved for future surface that might fail (e.g. an
// explicit Metal device probe).
//
// Defaults to a conservative SubRing dispatch threshold that excludes
// corona's production N=256 ring (see corona/gpu.UseAccelerator for
// rationale). Operators can lower the threshold via
// corona/gpu.SetThreshold once a batched GPU path is plumbed through.
func UseCoronaGPUAccelerator() error {
	return cgpu.UseAccelerator()
}

// DisableCoronaGPUAccelerator clears the opt-in flag and resets the
// SubRing dispatch threshold to 0. Subsequent NewParams() calls in
// corona will leave their rings on the CPU NTT path.
func DisableCoronaGPUAccelerator() {
	cgpu.DisableAccelerator()
}

// CoronaGPUAcceleratorEnabled reports whether the opt-in flag is set.
func CoronaGPUAcceleratorEnabled() bool {
	return cgpu.Enabled()
}

// CoronaGPUBackend returns the active GPU backend name ("Metal",
// "CUDA", or a CPU descriptor) for diagnostic logging.
func CoronaGPUBackend() string {
	return cgpu.Backend()
}
