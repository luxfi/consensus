// Package validatorstest re-exports github.com/luxfi/validators/validatorstest for backward compatibility.
package validatorstest

import (
	"github.com/luxfi/validators/validatorstest"
)

// State is an alias for validatorstest.State
type State = validatorstest.State

// TestState is an alias for validatorstest.TestState
type TestState = validatorstest.TestState

// NewTestState re-exports validatorstest.NewTestState
func NewTestState() *TestState {
	return validatorstest.NewTestState()
}
