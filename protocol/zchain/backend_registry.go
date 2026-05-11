// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/config"
)

// backend_registry.go — the process-global registry mapping VerifierID
// → BackendVerifier. Distinct from VerifierManifestRegistry (which
// holds metadata pins); this registry holds the actual code that runs
// when VerifyZProofUnderProfile reaches check 15.
//
// One backend per VerifierID. Binding is monotonic: RegisterBackendVerifier
// refuses a duplicate, refuses VerifierNone, and runs once per
// VerifierID for the process lifetime. The pattern matches how
// stdlib's image and database/sql packages let third-party code bind
// concrete drivers at init() time without consensus depending on them
// at compile time.
//
// VerifyZProofUnderProfile REQUIRES a binding for every VerifierID it
// dispatches; the absence of a binding is a hard refusal at check 15.
// There is no dev/production build-tag split — one canonical build.
// Test fixtures that need a fake backend register a BackendVerifierFunc
// from a *_test.go file (excluded from production binaries by Go's
// standard rule), so the registration cannot leak into a release.

// Typed binding errors.
var (
	ErrBackendVerifierNil       = errors.New("zchain: nil BackendVerifier")
	ErrBackendVerifierInvalidID = errors.New("zchain: VerifierNone may not be bound")
	ErrBackendVerifierDuplicate = errors.New("zchain: backend verifier already bound for this VerifierID")
)

var (
	backendMu       sync.RWMutex
	backendRegistry = make(map[config.VerifierID]BackendVerifier)
)

// RegisterBackendVerifier binds backend to vid. Called from a backend
// implementation's init() function (e.g. github.com/luxfi/sp1/zverifier).
// Refuses duplicates, refuses VerifierNone, refuses nil.
func RegisterBackendVerifier(vid config.VerifierID, backend BackendVerifier) error {
	if backend == nil {
		return ErrBackendVerifierNil
	}
	if vid == config.VerifierNone {
		return ErrBackendVerifierInvalidID
	}
	backendMu.Lock()
	defer backendMu.Unlock()
	if _, exists := backendRegistry[vid]; exists {
		return fmt.Errorf("%w: %s", ErrBackendVerifierDuplicate, vid.String())
	}
	backendRegistry[vid] = backend
	return nil
}

// MustRegisterBackendVerifier wraps RegisterBackendVerifier for use in
// init() functions. Panics on error. Bindings happen at process boot
// (and ONLY at process boot); a panic at boot is the correct loud
// failure mode for a malformed binding.
func MustRegisterBackendVerifier(vid config.VerifierID, backend BackendVerifier) {
	if err := RegisterBackendVerifier(vid, backend); err != nil {
		// This is the one place in the package where a panic is
		// acceptable: boot-time misconfiguration. The verifier
		// itself never panics; binding errors do, because there is
		// no way to operate a chain whose verifier binding is
		// inconsistent.
		panic(fmt.Sprintf("zchain: MustRegisterBackendVerifier(%s): %v", vid.String(), err))
	}
}

// lookupBackendVerifier returns the BackendVerifier bound to vid, or
// nil if none is bound. Used internally by VerifyZProofUnderProfile.
func lookupBackendVerifier(vid config.VerifierID) BackendVerifier {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return backendRegistry[vid]
}

// resetBackendVerifiersForTest clears every binding. Test-only;
// production code MUST NOT touch the binding map after boot. Callers
// use this between test cases that bind a fake backend so the next
// test starts from a clean slate.
func resetBackendVerifiersForTest() {
	backendMu.Lock()
	defer backendMu.Unlock()
	backendRegistry = make(map[config.VerifierID]BackendVerifier)
}
