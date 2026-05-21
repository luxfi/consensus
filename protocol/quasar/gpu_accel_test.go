// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"testing"
)

// TestUseCoronaGPUAccelerator — the consensus-level wrapper flips the
// corona/gpu global flag and the disable helper clears it. Both
// branches survive on every supported build.
func TestUseCoronaGPUAccelerator(t *testing.T) {
	t.Cleanup(DisableCoronaGPUAccelerator)

	if CoronaGPUAcceleratorEnabled() {
		t.Fatal("baseline: CoronaGPUAcceleratorEnabled() should be false")
	}
	if err := UseCoronaGPUAccelerator(); err != nil {
		t.Fatalf("UseCoronaGPUAccelerator: %v", err)
	}
	if !CoronaGPUAcceleratorEnabled() {
		t.Fatal("flag did not flip after UseCoronaGPUAccelerator")
	}
	DisableCoronaGPUAccelerator()
	if CoronaGPUAcceleratorEnabled() {
		t.Fatal("flag did not clear after DisableCoronaGPUAccelerator")
	}
}

// TestCoronaGPUBackendNonEmpty — backend descriptor is always populated
// (lattice/gpu returns "CPU (pure Go)" on no-cgo builds, "Metal" /
// "CUDA" on -tags gpu builds with the library linked).
func TestCoronaGPUBackendNonEmpty(t *testing.T) {
	if got := CoronaGPUBackend(); got == "" {
		t.Fatal("CoronaGPUBackend() returned empty string")
	}
}
