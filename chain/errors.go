package chain

import "errors"

var (
	// ErrSkipped is returned when an operation is skipped
	ErrSkipped = errors.New("operation skipped")
)