// Package validatorsmock re-exports github.com/luxfi/validators/validatorsmock for backward compatibility.
package validatorsmock

import (
	"github.com/luxfi/validators/validatorsmock"
	"go.uber.org/mock/gomock"
)

// State is an alias for validatorsmock.State
type State = validatorsmock.State

// StateMockRecorder is an alias for validatorsmock.StateMockRecorder
type StateMockRecorder = validatorsmock.StateMockRecorder

// NewState re-exports validatorsmock.NewState
func NewState(ctrl *gomock.Controller) *State {
	return validatorsmock.NewState(ctrl)
}
