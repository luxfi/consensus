// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bft

import (
	"context"
	"errors"
	"testing"
)

// TestEngine_NilEpoch_Returns_Nil_From_NewEngine asserts the NewEngine
// constructor refuses a nil epoch — the wrapper assumes a non-nil
// inner Epoch on every call path.
func TestEngine_NilEpoch_Returns_Nil_From_NewEngine(t *testing.T) {
	if got := NewEngine(nil); got != nil {
		t.Fatalf("NewEngine(nil) = %v, want nil", got)
	}
}

// TestEngine_NilWrapper_IsBootstrapped_False asserts a nil *Engine is
// safe to call IsBootstrapped on and returns false. The consensus
// toolkit's Engine interface is invoked through interface values
// where nil is a representable state, so the methods must be
// nil-safe.
func TestEngine_NilWrapper_IsBootstrapped_False(t *testing.T) {
	var e *Engine
	if e.IsBootstrapped() {
		t.Fatal("nil *Engine.IsBootstrapped() = true, want false")
	}
}

// TestEngine_NilEpoch_Start_Returns_ErrNilEpoch asserts the
// fail-closed contract: a wrapper constructed with a literal struct
// (no Epoch) refuses Start with ErrNilEpoch rather than silently
// no-op'ing.
func TestEngine_NilEpoch_Start_Returns_ErrNilEpoch(t *testing.T) {
	e := &Engine{} // bypass NewEngine to get a nil-epoch wrapper
	err := e.Start(context.Background(), 0)
	if !errors.Is(err, ErrNilEpoch) {
		t.Fatalf("Start with nil epoch: got %v, want ErrNilEpoch", err)
	}
	if e.IsBootstrapped() {
		t.Fatal("IsBootstrapped() = true after failed Start, want false")
	}
}

// TestEngine_NilEpoch_HealthCheck_Returns_ErrNilEpoch mirrors the
// Start contract: HealthCheck reports the nil-epoch condition rather
// than returning a fake "healthy" status.
func TestEngine_NilEpoch_HealthCheck_Returns_ErrNilEpoch(t *testing.T) {
	e := &Engine{}
	_, err := e.HealthCheck(context.Background())
	if !errors.Is(err, ErrNilEpoch) {
		t.Fatalf("HealthCheck with nil epoch: got %v, want ErrNilEpoch", err)
	}
}

// TestEngine_NilEpoch_Stop_NoOp asserts Stop on a nil-epoch wrapper
// is a safe no-op (returns nil) rather than panicking or returning
// ErrNilEpoch. Stop is on the cleanup path and should never fail.
func TestEngine_NilEpoch_Stop_NoOp(t *testing.T) {
	e := &Engine{}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatalf("Stop with nil epoch: got %v, want nil", err)
	}
}

// TestEngine_NilWrapper_Epoch_Returns_Nil asserts the Epoch() escape
// hatch is nil-safe.
func TestEngine_NilWrapper_Epoch_Returns_Nil(t *testing.T) {
	var e *Engine
	if got := e.Epoch(); got != nil {
		t.Fatalf("nil *Engine.Epoch() = %v, want nil", got)
	}
}
