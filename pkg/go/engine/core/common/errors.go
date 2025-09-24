package common

import "errors"

// Common errors
var (
	ErrUndefined = errors.New("undefined error")
)

// AppError represents an application error
type AppError struct {
	Code    int32
	Message string
}

// Error returns the error message
func (e *AppError) Error() string {
	return e.Message
}
