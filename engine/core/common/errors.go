package common

import (
	"errors"

	"github.com/luxfi/warp"
)

// Common errors
var (
	ErrUndefined = errors.New("undefined error")
)

// Error is an alias for warp.Error
type Error = warp.Error
